# whole-file feature:api_keys
"""The API docs for api_keys.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, Any, NotRequired, TypedDict

from app.apidocs import ADMIN, Field, SESSION_ONLY, doc, schema
from app.routes import api_keys
from app.routes.schemas_api import APIKeySchema
from app.routes.schemas_api import NewAPIKeySchema


class ListAllKeysParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]
    q: Annotated[str, Field(query="q", doc="Keep keys whose name or owner contains this, in any case.")]
    live: Annotated[str, Field(query="live", enum=("true",), doc="Leave out revoked and expired keys.")]
    sort: Annotated[str, Field(query="sort", enum=("display_name", "name", "role", "last_used_at", "expires_at", "created_at"), doc="Field to sort the whole list by. Rows without it go last.")]
    dir: Annotated[str, Field(query="dir", enum=("asc", "desc"), doc="Default asc.")]


class ListAllKeysReply(TypedDict):
    keys: list[APIKeySchema]
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    total: int


doc(
    api_keys.list_all_keys,
    summary="Every key in the app",
    tag="API keys",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="api_keys",
    params=ListAllKeysParams,
    reply=ListAllKeysReply,
    errors=(400, 401, 403, 404),
)


class CreateKeyForUserBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expires_in_days: NotRequired[Annotated[int | None, Field(doc="Absent or null: the configured default. 0: never expires.", minimum=0, maximum=3650)]]
    name: Annotated[str, Field(min_length=1, max_length=80)]
    role: NotRequired[Annotated[str, Field(enum=("admin", "user"), default="user")]]
    user_id: int


doc(
    api_keys.create_key_for_user,
    summary="Create a key that acts as someone else",
    description="Any admin may do this for an ordinary user, and only the owner for another admin. Nobody may do it for the owner. The key records who made it and shows that to the person it acts as.",
    tag="API keys",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="api_keys",
    body=CreateKeyForUserBody,
    status=201,
    reply=NewAPIKeySchema,
    errors=(400, 401, 403, 404),
)


class DeleteAnyKeyBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class DeleteAnyKeyReply(TypedDict):
    ok: bool


doc(
    api_keys.delete_any_key,
    summary="Delete anyone's key",
    tag="API keys",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="api_keys",
    body=DeleteAnyKeyBody,
    reply=DeleteAnyKeyReply,
    errors=(400, 401, 403, 404),
)


class SetAnyKeyExpiryBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expires_in_days: Annotated[int, Field(doc="Counted from now. 0 means it never expires.", minimum=0, maximum=3650)]


@schema(doc="The key, as in the list.")
class SetAnyKeyExpiryReply(TypedDict):
    pass


doc(
    api_keys.set_any_key_expiry,
    summary="Change when anyone's key expires",
    tag="API keys",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="api_keys",
    body=SetAnyKeyExpiryBody,
    reply=SetAnyKeyExpiryReply,
    errors=(400, 401, 403, 404),
)


class RenameAnyKeyBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expect: NotRequired[Annotated[str, Field(doc="The name you think it has. Answers 409 if it has another.")]]
    name: Annotated[str, Field(doc="1-80 characters.")]


class RenameAnyKeyReply(TypedDict):
    name: str
    ok: bool


doc(
    api_keys.rename_any_key,
    summary="Rename anyone's key",
    tag="API keys",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="api_keys",
    body=RenameAnyKeyBody,
    reply=RenameAnyKeyReply,
    errors=(400, 401, 403, 404, 409),
)


class RestoreAnyKeyBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


doc(
    api_keys.restore_any_key,
    summary="Undo deleting anyone's key",
    tag="API keys",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="api_keys",
    body=RestoreAnyKeyBody,
    reply=APIKeySchema,
    errors=(400, 401, 403, 404, 409),
)


class RevokeAnyKeyBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class RevokeAnyKeyReply(TypedDict):
    ok: bool


doc(
    api_keys.revoke_any_key,
    summary="Revoke anyone's key",
    tag="API keys",
    access=SESSION_ONLY,
    role=ADMIN,
    feature="api_keys",
    body=RevokeAnyKeyBody,
    reply=RevokeAnyKeyReply,
    errors=(400, 401, 403, 404),
)


class ListMyKeysParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]
    q: Annotated[str, Field(query="q", doc="Keep keys whose name contains this, in any case.")]
    live: Annotated[str, Field(query="live", enum=("true",), doc="Leave out revoked and expired keys.")]
    sort: Annotated[str, Field(query="sort", enum=("name", "role", "last_used_at", "expires_at", "created_at"), doc="Field to sort the whole list by. Rows without it go last.")]
    dir: Annotated[str, Field(query="dir", enum=("asc", "desc"), doc="Default asc.")]


class ListMyKeysReply(TypedDict):
    keys: list[APIKeySchema]
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    total: int


doc(
    api_keys.list_my_keys,
    summary="Your API keys",
    tag="API keys",
    access=SESSION_ONLY,
    feature="api_keys",
    params=ListMyKeysParams,
    reply=ListMyKeysReply,
    errors=(400, 401, 403, 404),
)


class CreateMyKeyBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expires_in_days: NotRequired[Annotated[int | None, Field(doc="Absent or null: the configured default. 0: never expires.", minimum=0, maximum=3650)]]
    name: Annotated[str, Field(min_length=1, max_length=80)]
    role: NotRequired[Annotated[str, Field(enum=("admin", "user"), default="user")]]


doc(
    api_keys.create_my_key,
    summary="Create an API key",
    tag="API keys",
    access=SESSION_ONLY,
    feature="api_keys",
    body=CreateMyKeyBody,
    status=201,
    reply=NewAPIKeySchema,
    errors=(400, 401, 403, 404),
)


class DeleteMyKeyBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class DeleteMyKeyReply(TypedDict):
    ok: bool


doc(
    api_keys.delete_my_key,
    summary="Delete one of your keys",
    tag="API keys",
    access=SESSION_ONLY,
    feature="api_keys",
    body=DeleteMyKeyBody,
    reply=DeleteMyKeyReply,
    errors=(400, 401, 403, 404),
)


class SetMyKeyExpiryBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expires_in_days: Annotated[int, Field(doc="Counted from now. 0 means it never expires.", minimum=0, maximum=3650)]


@schema(doc="The key, as in the list.")
class SetMyKeyExpiryReply(TypedDict):
    pass


doc(
    api_keys.set_my_key_expiry,
    summary="Change when one of your keys expires",
    tag="API keys",
    access=SESSION_ONLY,
    feature="api_keys",
    body=SetMyKeyExpiryBody,
    reply=SetMyKeyExpiryReply,
    errors=(400, 401, 403, 404),
)


class RenameMyKeyBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expect: NotRequired[Annotated[str, Field(doc="The name you think it has. Answers 409 if it has another.")]]
    name: Annotated[str, Field(doc="1-80 characters.")]


class RenameMyKeyReply(TypedDict):
    name: str
    ok: bool


doc(
    api_keys.rename_my_key,
    summary="Rename one of your keys",
    tag="API keys",
    access=SESSION_ONLY,
    feature="api_keys",
    body=RenameMyKeyBody,
    reply=RenameMyKeyReply,
    errors=(400, 401, 403, 404, 409),
)


class RestoreMyKeyBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


doc(
    api_keys.restore_my_key,
    summary="Undo deleting one of your keys",
    tag="API keys",
    access=SESSION_ONLY,
    feature="api_keys",
    body=RestoreMyKeyBody,
    reply=APIKeySchema,
    errors=(400, 401, 403, 404, 409),
)


class RevokeMyKeyBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class RevokeMyKeyReply(TypedDict):
    ok: bool


doc(
    api_keys.revoke_my_key,
    summary="Revoke one of your keys",
    tag="API keys",
    access=SESSION_ONLY,
    feature="api_keys",
    body=RevokeMyKeyBody,
    reply=RevokeMyKeyReply,
    errors=(400, 401, 403, 404),
)
