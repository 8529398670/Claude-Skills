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
from app.models import users, webhooks
from app.routes.helpers import bad_request, conflict, emit, forbidden, get_config, get_store, not_found, read_json, restore_refused, run, send_page
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


async def my_storage(request: Request) -> Response:
    """How much of the quota an account has used, so it finds out before an
    upload is refused rather than from the refusal."""
    used = await run(models.storage_used, get_store(request), actor_from(request).user.id)
    return JSONResponse({"used_bytes": used, "quota_bytes": get_config(request).files.quota_bytes_per_user})


async def all_storage(request: Request) -> Response:
    """Every account's files together, and what room the disk has left, which
    no quota looks at. The disk figures are left out where the system will not
    say."""
    body: dict[str, Any] = {"used_bytes": await run(models.storage_used, get_store(request))}
    try:
        disk = await run(shutil.disk_usage, get_config(request).files.dir)
    except OSError:
        return JSONResponse(body)
    body["disk_free_bytes"] = disk.free
    body["disk_total_bytes"] = disk.total
    return JSONResponse(body)


async def list_my_files(request: Request) -> Response:
    files = await run(models.list_files, get_store(request), actor_from(request).user.id)
    return send_page(request, "files", [_row(file) for file in files])


async def list_all_files(request: Request) -> Response:
    """Adds who each file belongs to. The names, sizes and owners are what an
    admin needs to keep the disk in order."""
    store = get_store(request)
    files = await run(models.list_files, store)
    names = {user.id: user.display_name for user in await run(users.list_users, store)}
    return send_page(request, "files", [{**_row(file), "user_id": file.user_id, "user_name": names.get(file.user_id, "")} for file in files])


async def create_file(request: Request) -> Response:
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    name, size, declared = body.get("name"), body.get("size"), body.get("type", "")
    if not isinstance(name, str) or not isinstance(declared, str) or not isinstance(size, int) or isinstance(size, bool):
        return bad_request(str(models.BadFileError()))
    try:
        file = await run(models.create_file, get_store(request), get_config(request), actor_from(request).user.id, name, declared, size)
    except models.BadFileError as refusal:
        return bad_request(str(refusal))
    except (models.FileTooLargeError, models.FileQuotaError) as refusal:
        return conflict(str(refusal))
    return JSONResponse({**_row(file), "chunk_bytes": FILE_CHUNK_BYTES}, status_code=201)


async def get_my_file(request: Request) -> Response:
    """How a client finds out where to resume: see received."""
    file = await _my_file(request)
    if file is None:
        return not_found(NO_SUCH_FILE)
    return JSONResponse({**_row(file), "chunk_bytes": FILE_CHUNK_BYTES})


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
    file_id = request.path_params["file_id"]
    if not _locks.hold(file_id):
        return conflict("another request is writing this upload")
    try:
        file = await _my_file(request)
        if file is None:
            return not_found(NO_SUCH_FILE)
        # Asked twice, the second answer is the same as the first.
        if file.state == models.READY:
            return JSONResponse(_row(file))
        if file.received != file.chunks:
            return conflict("some of the file has not arrived", received=file.received)
        try:
            finished = await run(models.finish_file, get_store(request), get_config(request), file)
        except models.FileMissingError:
            return not_found(LOST)
    finally:
        _locks.release(file_id)
    audit(request, KIND_ADMIN, "file uploaded", {"file_id": finished.id, "size": finished.size})
    await emit(request, webhooks.EVENT_FILE_UPLOADED, {"file_id": finished.id, "user_id": finished.user_id, "name": finished.name, "size": finished.size})
    return JSONResponse(_row(finished))


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


async def _remove(request: Request, file: models.File) -> Response:
    """The two delete routes after each has decided the caller may."""
    if not await run(models.delete_file, get_store(request), file.id, actor_from(request).user.id):
        return not_found(NO_SUCH_FILE)
    audit(request, KIND_ADMIN, "file deleted", {"file_id": file.id, "user_id": file.user_id})
    await emit(request, webhooks.EVENT_FILE_DELETED, {"file_id": file.id, "user_id": file.user_id, "name": file.name})
    return JSONResponse({"ok": True})


async def delete_my_file(request: Request) -> Response:
    refusal = await _checked_body(request)
    if refusal is not None:
        return refusal
    file = await _my_file(request)
    if file is None:
        return not_found(NO_SUCH_FILE)
    return await _remove(request, file)


async def delete_any_file(request: Request) -> Response:
    refusal = await _checked_body(request)
    if refusal is not None:
        return refusal
    file = await run(models.get_file, get_store(request), request.path_params["file_id"])
    if file is None or file.deleted:
        return not_found(NO_SUCH_FILE)
    return await _remove(request, file)


async def _restore(request: Request, as_admin: bool) -> Response:
    """Answers an undo. Someone else's file looks like no file at all, unless
    the caller is an admin on the admin route."""
    refusal = await _checked_body(request)
    if refusal is not None:
        return refusal
    file_id = request.path_params["file_id"]
    store = get_store(request)
    user = actor_from(request).user
    if not as_admin:
        existing = await run(models.get_file, store, file_id)
        if existing is None or existing.user_id != user.id:
            return not_found("it is not there to restore")
    try:
        file = await run(models.restore_file, store, get_config(request), file_id, user.id, as_admin)
    except models.FileQuotaError as quota:
        return conflict(str(quota))
    except RestoreError as refused:
        return restore_refused(refused)
    audit(request, KIND_ADMIN, "file restored", {"file_id": file_id})
    return JSONResponse(_row(file))


async def restore_my_file(request: Request) -> Response:
    return await _restore(request, False)


async def restore_any_file(request: Request) -> Response:
    return await _restore(request, True)
