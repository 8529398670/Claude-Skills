"""The API docs for account.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from datetime import datetime
from typing import Annotated, NotRequired, TypedDict

from app.apidocs import Field, SESSION_ONLY, doc
from app.routes import account
from app.routes.schemas_api import MeSchema


class CreateMyLoginLinkBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expires_in_minutes: NotRequired[Annotated[int, Field(doc="0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise.", minimum=0, maximum=525600)]]
    max_uses: NotRequired[Annotated[int, Field(doc="Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1.", minimum=0, maximum=1000)]]


class CreateMyLoginLinkReply(TypedDict):
    expires_at: Annotated[datetime | None, Field(doc="null when it never expires.")]
    expires_in_seconds: Annotated[int, Field(doc="0 when it never expires.")]
    login_path: Annotated[str, Field(doc="Path of the login link. A credential: treat it like a password.")]
    max_uses: Annotated[int, Field(doc="0 when unlimited.")]
    unlimited: bool


doc(
    account.create_my_login_link,
    summary="A login link for yourself",
    tag="Account",
    body=CreateMyLoginLinkBody,
    reply=CreateMyLoginLinkReply,
    errors=(400, 401, 403),
)


class CancelMyLoginLinkBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class CancelMyLoginLinkReply(TypedDict):
    ok: bool
    removed: Annotated[int, Field(doc="Links removed: 0 or 1.")]


doc(
    account.cancel_my_login_link,
    summary="Stop your own login link working",
    tag="Account",
    body=CancelMyLoginLinkBody,
    reply=CancelMyLoginLinkReply,
    errors=(400, 401, 403),
)


# >>> feature:invite_links
class SetPINBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    pin: str


class SetPINReply(TypedDict):
    has_pin: bool
    ok: bool


doc(
    account.set_pin,
    summary="Set or clear your PIN",
    tag="Account",
    access=SESSION_ONLY,
    feature="invite_links",
    body=SetPINBody,
    reply=SetPINReply,
    errors=(400, 401, 403),
)
# <<< feature:invite_links


class RenameBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    display_name: str
    expect: NotRequired[str | None]


class RenameReply(TypedDict):
    display_name: str
    ok: bool


doc(
    account.rename,
    summary="Change your display name",
    tag="Account",
    body=RenameBody,
    reply=RenameReply,
    errors=(400, 401, 403, 409),
)


class LogoutBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class LogoutReply(TypedDict):
    ok: bool


doc(
    account.logout,
    summary="Sign out",
    tag="Account",
    mcp_http="Ends a browser session; a key has none to end.",
    body=LogoutBody,
    reply=LogoutReply,
    errors=(400, 401, 403),
)


doc(
    account.me,
    summary="Who am I",
    tag="Account",
    reply=MeSchema,
    errors=(401,),
)
