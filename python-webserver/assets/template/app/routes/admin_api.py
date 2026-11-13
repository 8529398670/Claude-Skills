"""The API docs for admin.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from datetime import datetime
from typing import Annotated, NotRequired, TypedDict

from app.apidocs import ADMIN, Field, doc
from app.routes import admin
from app.routes.schemas_api import RevokedSchema
from app.routes.schemas_api import UserSchema


class ListUsersParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]
    q: Annotated[str, Field(query="q", doc="Keep accounts whose name or username contains this, in any case.")]
    sort: Annotated[str, Field(query="sort", enum=("display_name", "role", "disabled", "created_at"), doc="Field to sort the whole list by. Rows without it go last.")]
    dir: Annotated[str, Field(query="dir", enum=("asc", "desc"), doc="Default asc.")]


class ListUsersReply(TypedDict):
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    total: int
    users: list[UserSchema]


doc(
    admin.list_users,
    summary="Every account",
    tag="Admin",
    role=ADMIN,
    params=ListUsersParams,
    reply=ListUsersReply,
    errors=(400, 401, 403),
)


class CreateUserBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    display_name: str
    expires_in_minutes: NotRequired[Annotated[int, Field(doc="0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise.", minimum=0, maximum=525600)]]
    max_uses: NotRequired[Annotated[int, Field(doc="Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1.", minimum=0, maximum=1000)]]
    role: NotRequired[Annotated[str, Field(enum=("admin", "user"))]]


class CreateUserReply(TypedDict):
    expires_at: Annotated[datetime | None, Field(doc="null when it never expires.")]
    expires_in_seconds: Annotated[int, Field(doc="0 when it never expires.")]
    login_path: Annotated[str, Field(doc="Path of the login link. A credential: treat it like a password.")]
    max_uses: Annotated[int, Field(doc="0 when unlimited.")]
    unlimited: bool
    user_id: int


doc(
    admin.create_user,
    summary="Create an account and its login link",
    tag="Admin",
    role=ADMIN,
    body=CreateUserBody,
    status=201,
    reply=CreateUserReply,
    errors=(400, 401, 403),
)


class CancelLoginLinkBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class CancelLoginLinkReply(TypedDict):
    ok: bool
    removed: Annotated[int, Field(doc="Links removed: 0 or 1.")]


doc(
    admin.cancel_login_link,
    summary="Stop an account's login link working",
    tag="Admin",
    role=ADMIN,
    body=CancelLoginLinkBody,
    reply=CancelLoginLinkReply,
    errors=(400, 401, 403, 404),
)


class DeleteUserBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class DeleteUserReply(TypedDict):
    ok: bool
    removed: RevokedSchema


doc(
    admin.delete_user,
    summary="Delete an account",
    tag="Admin",
    role=ADMIN,
    body=DeleteUserBody,
    reply=DeleteUserReply,
    errors=(400, 401, 403, 404),
)


class SetUserDisabledBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    disabled: bool


class SetUserDisabledReply(TypedDict):
    ok: bool


doc(
    admin.set_user_disabled,
    summary="Disable or enable an account",
    tag="Admin",
    role=ADMIN,
    body=SetUserDisabledBody,
    reply=SetUserDisabledReply,
    errors=(400, 401, 403, 404),
)


class ReissueLoginBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expires_in_minutes: NotRequired[Annotated[int, Field(doc="0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise.", minimum=0, maximum=525600)]]
    max_uses: NotRequired[Annotated[int, Field(doc="Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1.", minimum=0, maximum=1000)]]


class ReissueLoginReply(TypedDict):
    expires_at: Annotated[datetime | None, Field(doc="null when it never expires.")]
    expires_in_seconds: Annotated[int, Field(doc="0 when it never expires.")]
    login_path: Annotated[str, Field(doc="Path of the login link. A credential: treat it like a password.")]
    max_uses: Annotated[int, Field(doc="0 when unlimited.")]
    unlimited: bool


doc(
    admin.reissue_login,
    summary="A new login link for an account",
    tag="Admin",
    role=ADMIN,
    body=ReissueLoginBody,
    reply=ReissueLoginReply,
    errors=(400, 401, 403, 404),
)


class RenameUserBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    display_name: Annotated[str, Field(doc="1-80 characters.")]
    expect: NotRequired[Annotated[str, Field(doc="The name you think they have. Answers 409 if it is another.")]]


class RenameUserReply(TypedDict):
    display_name: str
    ok: bool


doc(
    admin.rename_user,
    summary="Rename someone else",
    description="Only the owner can rename an admin.",
    tag="Admin",
    role=ADMIN,
    body=RenameUserBody,
    reply=RenameUserReply,
    errors=(400, 401, 403, 404, 409),
)


# >>> feature:two_factor
class ResetTwoFactorBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class ResetTwoFactorReply(TypedDict):
    ok: bool


doc(
    admin.reset_two_factor,
    summary="Turn off an account's two-factor",
    tag="Admin",
    role=ADMIN,
    feature="two_factor",
    body=ResetTwoFactorBody,
    reply=ResetTwoFactorReply,
    errors=(400, 401, 403, 404),
)
# <<< feature:two_factor


class RevokeAccessBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class RevokeAccessReply(TypedDict):
    ok: bool
    revoked: RevokedSchema


doc(
    admin.revoke_access,
    summary="End every credential an account holds",
    tag="Admin",
    role=ADMIN,
    body=RevokeAccessBody,
    reply=RevokeAccessReply,
    errors=(400, 401, 403, 404),
)


class SetUserRoleBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    role: Annotated[str, Field(enum=("admin", "user"))]


class SetUserRoleReply(TypedDict):
    ok: bool
    role: str


doc(
    admin.set_user_role,
    summary="Change an account's role",
    description="Only the owner can take an admin's rights away. Becoming an admin ends the account's sessions and live login link.",
    tag="Admin",
    role=ADMIN,
    body=SetUserRoleBody,
    reply=SetUserRoleReply,
    errors=(400, 401, 403, 404),
)
