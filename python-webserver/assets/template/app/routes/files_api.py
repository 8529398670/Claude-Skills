# whole-file feature:files
"""The API docs for files.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, NotRequired, TypedDict

from app.apidocs import ADMIN, Field, SESSION_ONLY, doc
from app.routes import files
from app.routes.schemas_api import FileSchema


class MyStorageReply(TypedDict):
    quota_bytes: int
    used_bytes: int


doc(
    files.my_storage,
    summary="How much of your quota is used",
    tag="Files",
    feature="files",
    reply=MyStorageReply,
    errors=(401, 404),
)


class ListAllFilesParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]
    q: Annotated[str, Field(query="q", doc="Keep files whose name or owner contains this, in any case.")]
    sort: Annotated[str, Field(query="sort", enum=("name", "user_name", "size", "created_at"), doc="Field to sort the whole list by. Rows without it go last.")]
    dir: Annotated[str, Field(query="dir", enum=("asc", "desc"), doc="Default asc.")]


class ListAllFilesReplyFilesItem(FileSchema):
    user_id: int
    user_name: str


class ListAllFilesReply(TypedDict):
    files: list[ListAllFilesReplyFilesItem]
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    total: int


doc(
    files.list_all_files,
    summary="Every account's files",
    tag="Files",
    role=ADMIN,
    feature="files",
    params=ListAllFilesParams,
    reply=ListAllFilesReply,
    errors=(400, 401, 403, 404),
)


class DeleteAnyFileBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class DeleteAnyFileReply(TypedDict):
    ok: bool


doc(
    files.delete_any_file,
    summary="Delete anyone's file",
    tag="Files",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="files",
    body=DeleteAnyFileBody,
    reply=DeleteAnyFileReply,
    errors=(400, 401, 403, 404),
)


doc(
    files.download_any_file,
    summary="Download anyone's file",
    description="Written to the activity log when the file is someone else's.",
    tag="Files",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="files",
    reply_type="application/octet-stream",
    reply_description="The file, always as an attachment",
    errors=(401, 403, 404),
)


class RenameAnyFileBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expect: NotRequired[Annotated[str, Field(doc="The name you think it has. Answers 409 if it has another.")]]
    name: Annotated[str, Field(doc="Cleaned like an upload's name. 1-255 bytes.")]


class RenameAnyFileReply(TypedDict):
    name: Annotated[str, Field(doc="As stored.")]
    ok: bool


doc(
    files.rename_any_file,
    summary="Rename any file",
    tag="Files",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="files",
    body=RenameAnyFileBody,
    reply=RenameAnyFileReply,
    errors=(400, 401, 403, 404, 409),
)


class RestoreAnyFileBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


doc(
    files.restore_any_file,
    summary="Put back a deleted file",
    tag="Files",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="files",
    body=RestoreAnyFileBody,
    reply=FileSchema,
    errors=(400, 401, 403, 404, 409),
)


class AllStorageReply(TypedDict):
    disk_free_bytes: NotRequired[Annotated[int, Field(doc="Left out where the system will not say.")]]
    disk_total_bytes: NotRequired[Annotated[int, Field(doc="Left out where the system will not say.")]]
    used_bytes: int


doc(
    files.all_storage,
    summary="Every account's files together, and room on the disk",
    tag="Files",
    role=ADMIN,
    feature="files",
    reply=AllStorageReply,
    errors=(401, 403, 404),
)


class ListMyFilesParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]
    q: Annotated[str, Field(query="q", doc="Keep files whose name contains this, in any case.")]
    sort: Annotated[str, Field(query="sort", enum=("name", "size", "created_at"), doc="Field to sort the whole list by. Rows without it go last.")]
    dir: Annotated[str, Field(query="dir", enum=("asc", "desc"), doc="Default asc.")]


class ListMyFilesReply(TypedDict):
    files: list[FileSchema]
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    total: int


doc(
    files.list_my_files,
    summary="Your files, newest first",
    tag="Files",
    feature="files",
    params=ListMyFilesParams,
    reply=ListMyFilesReply,
    errors=(400, 401, 404),
)


class CreateFileBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    name: str
    size: Annotated[int, Field(minimum=0)]
    type: NotRequired[Annotated[str, Field(doc="Shown in the list. Downloads are always application/octet-stream.")]]


class CreateFileReply(FileSchema):
    chunk_bytes: Annotated[int, Field(doc="Every piece but the last is exactly this long.")]


doc(
    files.create_file,
    summary="Start an upload",
    description="Then send each piece in order and finish. A file over files.max_file_bytes, or one that would pass files.quota_bytes_per_user, is a 409.",
    tag="Files",
    feature="files",
    body=CreateFileBody,
    status=201,
    reply=CreateFileReply,
    reply_description="Created",
    errors=(400, 401, 403, 404, 409),
)


class GetMyFileReply(FileSchema):
    chunk_bytes: Annotated[int, Field(doc="Every piece but the last is exactly this long.")]


doc(
    files.get_my_file,
    summary="One of your files",
    description="received is where to resume an upload.",
    tag="Files",
    feature="files",
    reply=GetMyFileReply,
    errors=(401, 404),
)


class PutChunkParams(TypedDict):
    index: Annotated[int, Field(path="index", minimum=0)]
    x_csrf_token: Annotated[str, Field(header="X-CSRF-Token", doc="From GET /api/me. Browser sessions only.")]


class PutChunkReply(TypedDict):
    received: int


doc(
    files.put_chunk,
    summary="Send the next piece of an upload",
    description="The body is the raw bytes. Pieces go in order, from 0. A browser session sends its csrf token in the X-CSRF-Token header. A 409 carries received, the piece to send next. Counts against server.rate_limit_upload_max.",
    tag="Files",
    feature="files",
    mcp_http="File bytes: MCP messages are JSON, and clients cap their size. files_start_upload says where to send each piece, with the same key.",
    params=PutChunkParams,
    body_type="application/octet-stream",
    reply=PutChunkReply,
    errors=(400, 401, 403, 404, 409),
)


class DeleteMyFileBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class DeleteMyFileReply(TypedDict):
    ok: bool


doc(
    files.delete_my_file,
    summary="Delete one of your files",
    description="It can be restored for ten minutes.",
    tag="Files",
    feature="files",
    body=DeleteMyFileBody,
    reply=DeleteMyFileReply,
    errors=(400, 401, 403, 404),
)


doc(
    files.download_my_file,
    summary="Download one of your files",
    tag="Files",
    feature="files",
    reply_type="application/octet-stream",
    reply_description="The file, always as an attachment",
    errors=(401, 404),
)


class FinishFileBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


doc(
    files.finish_file,
    summary="Finish an upload",
    tag="Files",
    feature="files",
    body=FinishFileBody,
    reply=FileSchema,
    errors=(400, 401, 403, 404, 409),
)


class RenameMyFileBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expect: NotRequired[Annotated[str, Field(doc="The name you think it has. Answers 409 if it has another.")]]
    name: Annotated[str, Field(doc="Cleaned like an upload's name. 1-255 bytes.")]


class RenameMyFileReply(TypedDict):
    name: Annotated[str, Field(doc="As stored.")]
    ok: bool


doc(
    files.rename_my_file,
    summary="Rename one of your files",
    tag="Files",
    feature="files",
    body=RenameMyFileBody,
    reply=RenameMyFileReply,
    errors=(400, 401, 403, 404, 409),
)


class RestoreMyFileBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


doc(
    files.restore_my_file,
    summary="Put back a file you deleted",
    tag="Files",
    feature="files",
    body=RestoreMyFileBody,
    reply=FileSchema,
    errors=(400, 401, 403, 404, 409),
)
