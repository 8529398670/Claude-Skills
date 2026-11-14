# whole-file feature:files
"""Uploaded files. How they are stored is in app/models/files.py; here is who
may do what. Everyone has their own, and someone else's answers 404, like a key
that is not yours. Admins have their own routes for everyone's, and one opening
a file that is not theirs is written to the log.

An upload is three steps, so that every request stays under the global body
limit: POST /api/files, one POST per piece, then POST .../finish.

A piece's body is the raw bytes, so its csrf token cannot ride in the body like
everywhere else, and comes in the X-CSRF-Token header instead.
"""
from __future__ import annotations

import shutil
from typing import Annotated, ClassVar, Literal
from urllib.parse import quote

import msgspec
from msgspec import UNSET, Meta, UnsetType
from starlette.requests import ClientDisconnect, Request
from starlette.responses import Response, StreamingResponse

from app.apidocs import ADMIN, PATH, QUERY, SESSION_ONLY, Reply, route
from app.apidocs import Request as RequestType
from app.core.config import FILE_CHUNK_BYTES
from app.core.logs import KIND_ADMIN
from app.core.security import Guard, actor_from
from app.models import RestoreError, to_iso
from app.models import files as models
from app.models import users
from app.models import webhooks  # feature:webhooks
from app.routes.admin import OkReply
from app.routes.helpers import DateTime, Dir, Page, Paged, Refused, get_config, get_store, match_rows, not_found, restore_refusal, run, sorted_page
from app.routes.logs import audit

LOST = "upload_lost"
NO_SUCH_FILE = "no such file"

_locks = models.UploadLocks()


class RequireFiles(Guard):
    """Makes /api/files answer 404 while files.enabled is off. Per request,
    because it can be switched while the server runs."""

    def refuse(self, request: Request) -> Response | None:
        return None if get_config(request).files.enabled else not_found("not found")


class File(Reply):
    """A file as every route answers it. The sealed key is not in it."""

    api_name: ClassVar[str] = "File"
    id: str
    name: str
    type: str
    size: int
    state: Literal["uploading", "ready"]
    chunks: int
    received: int
    created_at: DateTime


class FileUpload(File):
    """A file with the size of its pieces: what starting an upload, and asking
    where to resume one, answer."""

    api_name: ClassVar[str] = ""
    chunk_bytes: Annotated[int, Meta(description="Every piece but the last is exactly this long.")]


class OwnedFile(File):
    """A file on the admin list, with whose it is."""

    api_name: ClassVar[str] = ""
    user_id: int
    user_name: str


def _row(file: models.File) -> File:
    """Picks the fields by hand, so the sealed key cannot reach a response by
    way of a change to the model."""
    return File(
        id=file.id,
        name=file.name,
        type=file.type,
        size=file.size,
        state=file.state,  # type: ignore[arg-type]
        chunks=file.chunks,
        received=file.received,
        created_at=to_iso(file.created_at),
    )


def _upload_row(file: models.File) -> FileUpload:
    return FileUpload(**msgspec.structs.asdict(_row(file)), chunk_bytes=FILE_CHUNK_BYTES)


def content_disposition(name: str) -> str:
    """Always attachment: an uploaded page must never run as this site. The
    plain filename is the fallback for old clients, and filename* carries the
    real name (RFC 6266)."""
    plain = "".join(character if character.isascii() and (character.isalnum() or character in "._-") else "_" for character in name)
    return f"attachment; filename=\"{plain}\"; filename*=UTF-8''{quote(name, safe='-._~')}"


class FileIDPath(RequestType):
    """The file a route under .../files/{file_id} acts on."""

    file_id: Annotated[str, PATH, Meta(description="The file's id, from GET /api/files (files_list over MCP).")]


class AdminFileIDPath(RequestType):
    """Anyone's file, for the admin routes under /api/admin/files."""

    file_id: Annotated[str, PATH, Meta(description="The file's id, from GET /api/admin/files (admin_list_files over MCP).")]


def _mine(request: Request, file_id: str) -> models.File | None:
    """The signed-in account's own, not deleted. Anything else is None, which
    does not say whether the id exists. Reads SQLite: from a worker thread,
    or through run()."""
    file = models.get_file(get_store(request), file_id)
    if file is None or file.deleted or file.user_id != actor_from(request).user.id:
        return None
    return file


class MyStorage(Reply):
    used_bytes: int
    quota_bytes: int


@route(summary="How much of your quota is used", tag="Files", feature="files", errors=(401, 404))
def my_storage(request: Request) -> MyStorage:
    """How much of the quota an account has used, so it finds out before an
    upload is refused rather than from the refusal. Over HTTP and as the MCP
    my_storage tool."""
    used = models.storage_used(get_store(request), actor_from(request).user.id)
    return MyStorage(used_bytes=used, quota_bytes=get_config(request).files.quota_bytes_per_user)


# What the system says about the disk, which it may not.
DiskBytes = Annotated[int, Meta(description="Left out where the system will not say.")]


class AllStorage(Reply, kw_only=True):
    used_bytes: int
    disk_free_bytes: DiskBytes | UnsetType = UNSET
    disk_total_bytes: DiskBytes | UnsetType = UNSET


@route(summary="Every account's files together, and room on the disk", tag="Files", role=ADMIN, feature="files", errors=(401, 403, 404))
def all_storage(request: Request) -> AllStorage:
    """Every account's files together, and what room the disk has left, which
    no quota looks at. The disk figures are left out where the system will not
    say. Over HTTP and as the MCP admin_storage tool."""
    body = AllStorage(used_bytes=models.storage_used(get_store(request)))
    try:
        disk = shutil.disk_usage(get_config(request).files.dir)
    except OSError:
        return body
    body.disk_free_bytes = disk.free
    body.disk_total_bytes = disk.total
    return body


class ListMyFiles(Page):
    q: Annotated[str, QUERY, Meta(description="Keep files whose name contains this, in any case.")] = ""
    sort: Annotated[Literal["name", "size", "created_at"], QUERY, Meta(description="Field to sort the whole list by. Rows without it go last.")] = ""  # type: ignore[assignment]
    dir: Dir = ""  # type: ignore[assignment]


class FilesReply(Paged):
    files: list[File]


@route(summary="Your files, newest first", tag="Files", feature="files", errors=(400, 401, 404))
def list_my_files(request: Request, body: ListMyFiles) -> FilesReply:
    """The caller's files, over HTTP and as the MCP files_list tool."""
    rows = [_row(file) for file in models.list_files(get_store(request), actor_from(request).user.id)]
    page = sorted_page(match_rows(rows, body.q, "name"), body.sort, body.dir, body)
    return FilesReply(files=page.pop("rows"), **page)


class ListAllFiles(Page):
    q: Annotated[str, QUERY, Meta(description="Keep files whose name or owner contains this, in any case.")] = ""
    sort: Annotated[Literal["name", "user_name", "size", "created_at"], QUERY, Meta(description="Field to sort the whole list by. Rows without it go last.")] = ""  # type: ignore[assignment]
    dir: Dir = ""  # type: ignore[assignment]


class OwnedFilesReply(Paged):
    files: list[OwnedFile]


@route(summary="Every account's files", tag="Files", role=ADMIN, feature="files", errors=(400, 401, 403, 404))
def list_all_files(request: Request, body: ListAllFiles) -> OwnedFilesReply:
    """Adds who each file belongs to. The names, sizes and owners are what an
    admin needs to keep the disk in order. Over HTTP and as the MCP
    admin_list_files tool."""
    store = get_store(request)
    names = {user.id: user.display_name for user in users.list_users(store)}
    rows = [
        OwnedFile(**msgspec.structs.asdict(_row(file)), user_id=file.user_id, user_name=names.get(file.user_id, ""))
        for file in models.list_files(store)
    ]
    page = sorted_page(match_rows(rows, body.q, "name", "user_name"), body.sort, body.dir, body)
    return OwnedFilesReply(files=page.pop("rows"), **page)


# The two things every upload must say. create_file checks them (and the type)
# again, cleaned, with the same words.
BAD_FILE = str(models.BadFileError())


class CreateFile(RequestType):
    name: Annotated[str, Meta(extra={"error": BAD_FILE})]
    size: Annotated[int, Meta(ge=0, extra={"error": BAD_FILE})]
    type: Annotated[str, Meta(description="Shown in the list. Downloads are always application/octet-stream.")] = ""


@route(
    summary="Start an upload",
    description="Then send each piece in order and finish. A file over files.max_file_bytes, or one that would pass files.quota_bytes_per_user, is a 409.",
    tag="Files", feature="files", status=201, reply_description="Created", errors=(400, 401, 403, 404, 409),
)
def create_file(request: Request, body: CreateFile) -> FileUpload:
    """Start an upload, over HTTP and as the MCP files_start_upload tool."""
    try:
        file = models.create_file(get_store(request), get_config(request), actor_from(request).user.id, body.name, body.type, body.size)
    except models.BadFileError as refusal:
        raise Refused(400, str(refusal)) from None
    except (models.FileTooLargeError, models.FileQuotaError) as refusal:
        raise Refused(409, str(refusal)) from None
    return _upload_row(file)


@route(summary="One of your files", description="received is where to resume an upload.", tag="Files", feature="files", errors=(401, 404))
def get_my_file(request: Request, body: FileIDPath) -> FileUpload:
    """How a client finds out where to resume: see received. Over HTTP and as
    the MCP files_get tool."""
    file = _mine(request, body.file_id)
    if file is None:
        raise Refused(404, NO_SUCH_FILE)
    return _upload_row(file)


class PutChunk(FileIDPath):
    # Checked by hand too: it must also be the next piece, and one the file has.
    index: Annotated[int, PATH, Meta(ge=0, extra={"error": "bad piece number"})]


class PutChunkReply(Reply):
    received: int


@route(
    summary="Send the next piece of an upload",
    description="The body is the raw bytes. Pieces go in order, from 0. A browser session sends its csrf token in the X-CSRF-Token header. A 409 carries received, the piece to send next. Counts against server.rate_limit_upload_max.",
    tag="Files", feature="files",
    mcp_http="File bytes: MCP messages are JSON, and clients cap their size. files_start_upload says where to send each piece, with the same key.",
    body_type="application/octet-stream", errors=(400, 401, 403, 404, 409),
)
async def put_chunk(request: Request, body: PutChunk) -> PutChunkReply:
    """Take the next piece. Out of order is a 409 carrying received, which is
    also what a retry of a piece that did arrive gets: the client goes on from
    there. The body is the raw bytes, so its csrf token comes in the
    X-CSRF-Token header, which @route checks (body_type)."""
    index = body.index
    if not _locks.hold(body.file_id):
        raise Refused(409, "another request is writing this upload")
    try:
        file = await run(_mine, request, body.file_id)
        if file is None:
            raise Refused(404, NO_SUCH_FILE)
        if file.state != models.UPLOADING or index != file.received:
            raise Refused(409, "that is not the next piece", received=file.received)
        if index >= file.chunks:
            raise Refused(400, "bad piece number")
        try:
            piece = await request.body()
        except ClientDisconnect:
            raise Refused(400, "the piece did not arrive") from None
        try:
            await run(models.write_chunk, get_store(request), get_config(request), file, index, piece)
        except models.ChunkSizeError as refusal:
            raise Refused(400, str(refusal)) from None
        except models.FileMissingError:
            raise Refused(404, LOST) from None
        return PutChunkReply(received=index + 1)
    finally:
        _locks.release(body.file_id)


@route(summary="Finish an upload", tag="Files", feature="files", errors=(400, 401, 403, 404, 409))
def finish_file(request: Request, body: FileIDPath) -> File:
    """Seal an upload whose pieces have all arrived, over HTTP and as the MCP
    files_finish_upload tool."""
    if not _locks.hold(body.file_id):
        raise Refused(409, "another request is writing this upload")
    try:
        file = _mine(request, body.file_id)
        if file is None:
            raise Refused(404, NO_SUCH_FILE)
        # Asked twice, the second answer is the same as the first.
        if file.state == models.READY:
            return _row(file)
        if file.received != file.chunks:
            raise Refused(409, "some of the file has not arrived", received=file.received)
        try:
            finished = models.finish_file(get_store(request), get_config(request), file)
        except models.FileMissingError:
            raise Refused(404, LOST) from None
    finally:
        _locks.release(body.file_id)
    audit(request, KIND_ADMIN, "file uploaded", {"file_id": finished.id, "size": finished.size})
    request.app.state.webhooks.emit(webhooks.EVENT_FILE_UPLOADED, {"file_id": finished.id, "user_id": finished.user_id, "name": finished.name, "size": finished.size})  # feature:webhooks
    return _row(finished)


async def _send(request: Request, file: models.File) -> Response:
    """Stream a finished file. It is only ever an attachment of an opaque type:
    nosniff is on every response, and nothing the uploader said about the file
    reaches the headers except its cleaned name.

    Starlette walks the reader in a worker thread, so the reads and the
    decryption stay off the event loop. The fixed length means a piece that fails
    to open ends the connection short rather than looking complete."""
    if file.state != models.READY:
        raise Refused(404, NO_SUCH_FILE)
    try:
        reader = await run(models.FileReader, get_config(request), file)
    except models.FileMissingError:
        print(f"[files] file {file.id} has a record but not the right data on disk", flush=True)
        raise Refused(404, NO_SUCH_FILE) from None
    headers = {
        "Content-Disposition": content_disposition(file.name),
        "Content-Length": str(file.size),
        "Cache-Control": "private, no-store",
    }
    if request.method == "HEAD":
        reader.close()
        return Response(media_type="application/octet-stream", headers=headers)
    return StreamingResponse(iter(reader), media_type="application/octet-stream", headers=headers)


@route(
    summary="Download one of your files", tag="Files", feature="files",
    reply_type="application/octet-stream", reply_description="The file, always as an attachment", errors=(401, 404),
)
async def download_my_file(request: Request, body: FileIDPath) -> Response:
    file = await run(_mine, request, body.file_id)
    if file is None:
        raise Refused(404, NO_SUCH_FILE)
    return await _send(request, file)


@route(
    summary="Download anyone's file", description="Written to the activity log when the file is someone else's.",
    tag="Files", access=SESSION_ONLY, role=ADMIN, feature="files",
    reply_type="application/octet-stream", reply_description="The file, always as an attachment", errors=(401, 403, 404),
)
async def download_any_file(request: Request, body: AdminFileIDPath) -> Response:
    file = await run(models.get_file, get_store(request), body.file_id)
    if file is None or file.deleted:
        raise Refused(404, NO_SUCH_FILE)
    if file.user_id != actor_from(request).user.id:
        audit(request, KIND_ADMIN, "file opened by admin", {"file_id": file.id, "user_id": file.user_id})
    return await _send(request, file)


class RenameFile(FileIDPath):
    name: Annotated[str, Meta(description="Cleaned like an upload's name. 1-255 bytes.", extra={"error": str(models.BadFileNameError())})]
    expect: Annotated[str | None, Meta(description="The name you think it has. Answers 409 if it has another.")] = None


class RenameAnyFile(AdminFileIDPath):
    name: Annotated[str, Meta(description="Cleaned like an upload's name. 1-255 bytes.", extra={"error": str(models.BadFileNameError())})]
    expect: Annotated[str | None, Meta(description="The name you think it has. Answers 409 if it has another.")] = None


class RenameFileReply(Reply):
    ok: bool
    name: Annotated[str, Meta(description="As stored.")]


def _rename(request: Request, body: RenameFile, own_only: bool) -> RenameFileReply:
    """The two rename routes. own_only is the difference: an admin may rename
    anyone's, as an admin may delete anyone's."""
    file = _mine(request, body.file_id) if own_only else models.get_file(get_store(request), body.file_id)
    if file is None or file.deleted:
        raise Refused(404, NO_SUCH_FILE)
    try:
        cleaned = models.rename_file(get_store(request), file.id, body.name, body.expect)
    except models.BadFileNameError as refusal:
        raise Refused(400, str(refusal)) from None
    except users.RenameConflictError as refusal:
        raise Refused(409, str(refusal), name=refusal.current) from None
    if cleaned is None:
        raise Refused(404, NO_SUCH_FILE)
    audit(request, KIND_ADMIN, "file renamed", {"file_id": file.id, "user_id": file.user_id})
    return RenameFileReply(ok=True, name=cleaned)


@route(summary="Rename one of your files", tag="Files", feature="files", errors=(400, 401, 403, 404, 409))
def rename_my_file(request: Request, body: RenameFile) -> RenameFileReply:
    """Over HTTP and as the MCP files_rename tool."""
    return _rename(request, body, True)


@route(summary="Rename any file", tag="Files", access=SESSION_ONLY, role=ADMIN, feature="files", errors=(400, 401, 403, 404, 409))
def rename_any_file(request: Request, body: RenameAnyFile) -> RenameFileReply:
    """An admin may rename anyone's, as an admin may delete anyone's."""
    return _rename(request, body, False)


def _delete(request: Request, file_id: str, own_only: bool) -> OkReply:
    """The two delete routes, after each has decided whose files the caller
    may reach. The file goes to the bin, where restore finds it."""
    file = _mine(request, file_id) if own_only else models.get_file(get_store(request), file_id)
    if file is None or file.deleted:
        raise Refused(404, NO_SUCH_FILE)
    if not models.delete_file(get_store(request), file.id, actor_from(request).user.id):
        raise Refused(404, NO_SUCH_FILE)
    audit(request, KIND_ADMIN, "file deleted", {"file_id": file.id, "user_id": file.user_id})
    request.app.state.webhooks.emit(webhooks.EVENT_FILE_DELETED, {"file_id": file.id, "user_id": file.user_id, "name": file.name})  # feature:webhooks
    return OkReply(ok=True)


@route(summary="Delete one of your files", description="It can be restored for ten minutes.", tag="Files", feature="files", errors=(400, 401, 403, 404))
def delete_my_file(request: Request, body: FileIDPath) -> OkReply:
    """Over HTTP and as the MCP files_delete tool."""
    return _delete(request, body.file_id, True)


@route(summary="Delete anyone's file", tag="Files", access=SESSION_ONLY, role=ADMIN, feature="files", errors=(400, 401, 403, 404))
def delete_any_file(request: Request, body: AdminFileIDPath) -> OkReply:
    return _delete(request, body.file_id, False)


def _restore(request: Request, file_id: str, as_admin: bool) -> File:
    """Answers an undo, for the two restore routes. Someone else's file looks
    like no file at all, unless the caller is an admin on the admin route."""
    store = get_store(request)
    user = actor_from(request).user
    if not as_admin:
        existing = models.get_file(store, file_id)
        if existing is None or existing.user_id != user.id:
            raise Refused(404, "it is not there to restore")
    try:
        file = models.restore_file(store, get_config(request), file_id, user.id, as_admin)
    except models.FileQuotaError as quota:
        raise Refused(409, str(quota)) from None
    except RestoreError as refused:
        raise restore_refusal(refused) from None
    audit(request, KIND_ADMIN, "file restored", {"file_id": file_id})
    return _row(file)


@route(summary="Put back a file you deleted", tag="Files", feature="files", errors=(400, 401, 403, 404, 409))
def restore_my_file(request: Request, body: FileIDPath) -> File:
    """Over HTTP and as the MCP files_restore tool."""
    return _restore(request, body.file_id, False)


@route(summary="Put back a deleted file", tag="Files", access=SESSION_ONLY, role=ADMIN, feature="files", errors=(400, 401, 403, 404, 409))
def restore_any_file(request: Request, body: AdminFileIDPath) -> File:
    return _restore(request, body.file_id, True)
