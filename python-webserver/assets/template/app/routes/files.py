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
from typing import Any
from urllib.parse import quote

from starlette.requests import ClientDisconnect, Request
from starlette.responses import JSONResponse, Response, StreamingResponse

from app.core.config import FILE_CHUNK_BYTES
from app.core.logs import KIND_ADMIN
from app.core.security import Guard, actor_from, check_csrf
from app.models import RestoreError, to_iso
from app.models import files as models
from app.models import users
from app.models import webhooks  # feature:webhooks
from app.routes.helpers import Refused, bad_request, conflict, forbidden, get_config, get_store, match_rows, not_found, read_json, restore_refusal, run, send_page
from app.routes.logs import audit

CSRF_HEADER = "x-csrf-token"
LOST = "upload_lost"
NO_SUCH_FILE = "no such file"

_locks = models.UploadLocks()


class RequireFiles(Guard):
    """Makes /api/files answer 404 while files.enabled is off. Per request,
    because it can be switched while the server runs."""

    def refuse(self, request: Request) -> Response | None:
        return None if get_config(request).files.enabled else not_found("not found")


def _row(file: models.File) -> dict[str, Any]:
    """Picks the fields by hand, so the sealed key cannot reach a response by
    way of a change to the model."""
    return {
        "id": file.id,
        "name": file.name,
        "type": file.type,
        "size": file.size,
        "state": file.state,
        "chunks": file.chunks,
        "received": file.received,
        "created_at": to_iso(file.created_at),
    }


def content_disposition(name: str) -> str:
    """Always attachment: an uploaded page must never run as this site. The
    plain filename is the fallback for old clients, and filename* carries the
    real name (RFC 6266)."""
    plain = "".join(character if character.isascii() and (character.isalnum() or character in "._-") else "_" for character in name)
    return f"attachment; filename=\"{plain}\"; filename*=UTF-8''{quote(name, safe='-._~')}"


async def _my_file(request: Request) -> models.File | None:
    """The signed-in account's own, not deleted. Anything else is None, which
    does not say whether the id exists."""
    file = await run(models.get_file, get_store(request), request.path_params["file_id"])
    if file is None or file.deleted or file.user_id != actor_from(request).user.id:
        return None
    return file


async def _checked_body(request: Request) -> Response | None:
    """A refusal when the JSON body or its csrf token will not do."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    return None if check_csrf(request, body.get("csrf_token")) else forbidden("invalid csrf token")


def _mine(request: Request, file_id: str) -> models.File | None:
    """_my_file for a helper already in a worker thread."""
    file = models.get_file(get_store(request), file_id)
    if file is None or file.deleted or file.user_id != actor_from(request).user.id:
        return None
    return file


async def _answer(helper: Any, request: Request, *args: Any, status: int = 200) -> Response:
    """A shared helper's answer as a response, or its refusal."""
    try:
        return JSONResponse(await run(helper, request, *args), status_code=status)
    except Refused as refused:
        return refused.response(request)


async def my_storage(request: Request) -> Response:
    """How much of the quota an account has used, so it finds out before an
    upload is refused rather than from the refusal."""
    return await _answer(my_storage_row, request)


def my_storage_row(request: Request) -> dict[str, Any]:
    used = models.storage_used(get_store(request), actor_from(request).user.id)
    return {"used_bytes": used, "quota_bytes": get_config(request).files.quota_bytes_per_user}


async def all_storage(request: Request) -> Response:
    """Every account's files together, and what room the disk has left, which
    no quota looks at. The disk figures are left out where the system will not
    say."""
    return await _answer(all_storage_row, request)


def all_storage_row(request: Request) -> dict[str, Any]:
    body: dict[str, Any] = {"used_bytes": models.storage_used(get_store(request))}
    try:
        disk = shutil.disk_usage(get_config(request).files.dir)
    except OSError:
        return body
    body["disk_free_bytes"] = disk.free
    body["disk_total_bytes"] = disk.total
    return body


async def list_my_files(request: Request) -> Response:
    rows = match_rows(await run(my_file_rows, request), request.query_params.get("q", ""), "name")
    return send_page(request, "files", rows, ("name", "size", "created_at"))


def my_file_rows(request: Request) -> list[dict[str, Any]]:
    """The caller's files, for list_my_files and the MCP files_list tool."""
    return [_row(file) for file in models.list_files(get_store(request), actor_from(request).user.id)]


async def list_all_files(request: Request) -> Response:
    """Adds who each file belongs to. The names, sizes and owners are what an
    admin needs to keep the disk in order."""
    rows = match_rows(await run(all_file_rows, request), request.query_params.get("q", ""), "name", "user_name")
    return send_page(request, "files", rows, ("name", "user_name", "size", "created_at"))


def all_file_rows(request: Request) -> list[dict[str, Any]]:
    """Everyone's files, for list_all_files and the MCP admin_list_files tool."""
    store = get_store(request)
    names = {user.id: user.display_name for user in users.list_users(store)}
    return [{**_row(file), "user_id": file.user_id, "user_name": names.get(file.user_id, "")} for file in models.list_files(store)]


async def create_file(request: Request) -> Response:
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    return await _answer(create_file_checked, request, body.get("name"), body.get("type", ""), body.get("size"), status=201)


def create_file_checked(request: Request, name: Any, declared: Any, size: Any) -> dict[str, Any]:
    """Start an upload, for create_file and the MCP files_start_upload tool."""
    if not isinstance(name, str) or not isinstance(declared, str) or not isinstance(size, int) or isinstance(size, bool):
        raise Refused(400, str(models.BadFileError()))
    try:
        file = models.create_file(get_store(request), get_config(request), actor_from(request).user.id, name, declared, size)
    except models.BadFileError as refusal:
        raise Refused(400, str(refusal)) from None
    except (models.FileTooLargeError, models.FileQuotaError) as refusal:
        raise Refused(409, str(refusal)) from None
    return {**_row(file), "chunk_bytes": FILE_CHUNK_BYTES}


async def get_my_file(request: Request) -> Response:
    """How a client finds out where to resume: see received."""
    return await _answer(get_my_file_row, request, request.path_params["file_id"])


def get_my_file_row(request: Request, file_id: str) -> dict[str, Any]:
    file = _mine(request, file_id)
    if file is None:
        raise Refused(404, NO_SUCH_FILE)
    return {**_row(file), "chunk_bytes": FILE_CHUNK_BYTES}


async def put_chunk(request: Request) -> Response:
    """Take the next piece. Out of order is a 409 carrying received, which is
    also what a retry of a piece that did arrive gets: the client goes on from
    there."""
    if not check_csrf(request, request.headers.get(CSRF_HEADER)):
        return forbidden("invalid csrf token")
    raw_index = request.path_params["index"]
    if not raw_index.isascii() or not raw_index.isdigit():
        return bad_request("bad piece number")
    index = int(raw_index)
    file_id = request.path_params["file_id"]
    if not _locks.hold(file_id):
        return conflict("another request is writing this upload")
    try:
        file = await _my_file(request)
        if file is None:
            return not_found(NO_SUCH_FILE)
        if file.state != models.UPLOADING or index != file.received:
            return conflict("that is not the next piece", received=file.received)
        if index >= file.chunks:
            return bad_request("bad piece number")
        try:
            piece = await request.body()
        except ClientDisconnect:
            return bad_request("the piece did not arrive")
        try:
            await run(models.write_chunk, get_store(request), get_config(request), file, index, piece)
        except models.ChunkSizeError as refusal:
            return bad_request(str(refusal))
        except models.FileMissingError:
            return not_found(LOST)
        return JSONResponse({"received": index + 1})
    finally:
        _locks.release(file_id)


async def finish_file(request: Request) -> Response:
    refusal = await _checked_body(request)
    if refusal is not None:
        return refusal
    return await _answer(finish_file_checked, request, request.path_params["file_id"])


def finish_file_checked(request: Request, file_id: str) -> dict[str, Any]:
    """Seal an upload whose pieces have all arrived, for finish_file and the
    MCP files_finish_upload tool."""
    if not _locks.hold(file_id):
        raise Refused(409, "another request is writing this upload")
    try:
        file = _mine(request, file_id)
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
        _locks.release(file_id)
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
        return not_found(NO_SUCH_FILE)
    try:
        reader = await run(models.FileReader, get_config(request), file)
    except models.FileMissingError:
        print(f"[files] file {file.id} has a record but not the right data on disk", flush=True)
        return not_found(NO_SUCH_FILE)
    headers = {
        "Content-Disposition": content_disposition(file.name),
        "Content-Length": str(file.size),
        "Cache-Control": "private, no-store",
    }
    if request.method == "HEAD":
        reader.close()
        return Response(media_type="application/octet-stream", headers=headers)
    return StreamingResponse(iter(reader), media_type="application/octet-stream", headers=headers)


async def download_my_file(request: Request) -> Response:
    file = await _my_file(request)
    if file is None:
        return not_found(NO_SUCH_FILE)
    return await _send(request, file)


async def download_any_file(request: Request) -> Response:
    file = await run(models.get_file, get_store(request), request.path_params["file_id"])
    if file is None or file.deleted:
        return not_found(NO_SUCH_FILE)
    if file.user_id != actor_from(request).user.id:
        audit(request, KIND_ADMIN, "file opened by admin", {"file_id": file.id, "user_id": file.user_id})
    return await _send(request, file)


def _remove(request: Request, file: models.File) -> dict[str, Any]:
    """The two delete routes, and the MCP files_delete tool, after each has
    decided the caller may. The file goes to the bin, where restore finds it."""
    if not models.delete_file(get_store(request), file.id, actor_from(request).user.id):
        raise Refused(404, NO_SUCH_FILE)
    audit(request, KIND_ADMIN, "file deleted", {"file_id": file.id, "user_id": file.user_id})
    request.app.state.webhooks.emit(webhooks.EVENT_FILE_DELETED, {"file_id": file.id, "user_id": file.user_id, "name": file.name})  # feature:webhooks
    return {"ok": True}


def rename_file_checked(request: Request, file_id: str, name: Any, expect: Any, own_only: bool) -> dict[str, Any]:
    """The two rename routes and the MCP files_rename tool. own_only is the
    difference: an admin may rename anyone's, as an admin may delete anyone's."""
    file = _mine(request, file_id) if own_only else models.get_file(get_store(request), file_id)
    if file is None or file.deleted:
        raise Refused(404, NO_SUCH_FILE)
    if not isinstance(name, str):
        raise Refused(400, str(models.BadFileNameError()))
    if expect is not None and not isinstance(expect, str):
        raise Refused(400, "expect must be a string")
    try:
        cleaned = models.rename_file(get_store(request), file.id, name, expect)
    except models.BadFileNameError as refusal:
        raise Refused(400, str(refusal)) from None
    except users.RenameConflictError as refusal:
        raise Refused(409, str(refusal), name=refusal.current) from None
    if cleaned is None:
        raise Refused(404, NO_SUCH_FILE)
    audit(request, KIND_ADMIN, "file renamed", {"file_id": file.id, "user_id": file.user_id})
    return {"ok": True, "name": cleaned}


async def _rename(request: Request, own_only: bool) -> Response:
    refusal = await _checked_body(request)
    if refusal is not None:
        return refusal
    body = await read_json(request) or {}
    return await _answer(rename_file_checked, request, request.path_params["file_id"], body.get("name"), body.get("expect"), own_only)


async def rename_my_file(request: Request) -> Response:
    return await _rename(request, True)


async def rename_any_file(request: Request) -> Response:
    """An admin may rename anyone's, as an admin may delete anyone's."""
    return await _rename(request, False)


def delete_file_checked(request: Request, file_id: str, own_only: bool) -> dict[str, Any]:
    """The two delete routes and the MCP files_delete tool."""
    file = _mine(request, file_id) if own_only else models.get_file(get_store(request), file_id)
    if file is None or file.deleted:
        raise Refused(404, NO_SUCH_FILE)
    return _remove(request, file)


async def _delete(request: Request, own_only: bool) -> Response:
    refusal = await _checked_body(request)
    if refusal is not None:
        return refusal
    return await _answer(delete_file_checked, request, request.path_params["file_id"], own_only)


async def delete_my_file(request: Request) -> Response:
    return await _delete(request, True)


async def delete_any_file(request: Request) -> Response:
    return await _delete(request, False)


def restore_file_checked(request: Request, file_id: str, as_admin: bool) -> dict[str, Any]:
    """Answers an undo, for the two restore routes and the MCP files_restore
    tool. Someone else's file looks like no file at all, unless the caller is
    an admin on the admin route."""
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


async def _restore(request: Request, as_admin: bool) -> Response:
    refusal = await _checked_body(request)
    if refusal is not None:
        return refusal
    return await _answer(restore_file_checked, request, request.path_params["file_id"], as_admin)


async def restore_my_file(request: Request) -> Response:
    return await _restore(request, False)


async def restore_any_file(request: Request) -> Response:
    return await _restore(request, True)
