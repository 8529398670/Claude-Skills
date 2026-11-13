# whole-file feature:invite_links
"""The API docs for invite_links.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, NotRequired, TypedDict

from app.apidocs import ADMIN, Field, doc, schema
from app.routes import invite_links
from app.routes.schemas_api import InviteLinkSchema
from app.routes.schemas_api import JoinPathSchema


class ListInviteLinksParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]
    q: Annotated[str, Field(query="q", doc="Keep links whose label or maker contains this, in any case.")]
    sort: Annotated[str, Field(query="sort", enum=("label", "used", "expires_at"), doc="Field to sort the whole list by. Rows without it go last.")]
    dir: Annotated[str, Field(query="dir", enum=("asc", "desc"), doc="Default asc.")]


class ListInviteLinksReply(TypedDict):
    invite_links: list[InviteLinkSchema]
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    total: int


doc(
    invite_links.list_invite_links,
    summary="Every invite link",
    tag="Invite links",
    role=ADMIN,
    feature="invite_links",
    params=ListInviteLinksParams,
    reply=ListInviteLinksReply,
    errors=(400, 401, 403),
)


class CreateInviteLinkBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    days: int
    label: str
    max_uses: int


class CreateInviteLinkReply(InviteLinkSchema, JoinPathSchema):
    pass


doc(
    invite_links.create_invite_link,
    summary="Create an invite link",
    tag="Invite links",
    role=ADMIN,
    feature="invite_links",
    body=CreateInviteLinkBody,
    status=201,
    reply=CreateInviteLinkReply,
    errors=(400, 401, 403),
)


class DeleteInviteLinkBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class DeleteInviteLinkReply(TypedDict):
    ok: bool


doc(
    invite_links.delete_invite_link,
    summary="Delete an invite link",
    tag="Invite links",
    role=ADMIN,
    feature="invite_links",
    body=DeleteInviteLinkBody,
    reply=DeleteInviteLinkReply,
    errors=(400, 401, 403, 404),
)


class SetInviteLinkLimitsBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    days: NotRequired[Annotated[int, Field(doc="Counted from now. Leave out to keep the expiry.", minimum=1, maximum=365)]]
    max_uses: Annotated[int, Field(doc="Not fewer than have joined.", minimum=1, maximum=1000)]


@schema(doc="The link, as in the list.")
class SetInviteLinkLimitsReply(TypedDict):
    pass


doc(
    invite_links.set_invite_link_limits,
    summary="Change a link's places and expiry",
    tag="Invite links",
    role=ADMIN,
    feature="invite_links",
    body=SetInviteLinkLimitsBody,
    reply=SetInviteLinkLimitsReply,
    errors=(400, 401, 403, 404),
)


class RenameInviteLinkBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expect: NotRequired[Annotated[str, Field(doc="The label you think it has. Answers 409 if it has another.")]]
    label: Annotated[str, Field(doc="1-80 characters.")]


class RenameInviteLinkReply(TypedDict):
    label: str
    ok: bool


doc(
    invite_links.rename_invite_link,
    summary="Rename a link",
    tag="Invite links",
    role=ADMIN,
    feature="invite_links",
    body=RenameInviteLinkBody,
    reply=RenameInviteLinkReply,
    errors=(400, 401, 403, 404, 409),
)


class RestoreInviteLinkBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


doc(
    invite_links.restore_invite_link,
    summary="Undo deleting an invite link",
    tag="Invite links",
    role=ADMIN,
    feature="invite_links",
    body=RestoreInviteLinkBody,
    reply=InviteLinkSchema,
    errors=(400, 401, 403, 404, 409),
)


class RevokeInviteLinkBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class RevokeInviteLinkReply(TypedDict):
    ok: bool


doc(
    invite_links.revoke_invite_link,
    summary="Switch an invite link off",
    tag="Invite links",
    role=ADMIN,
    feature="invite_links",
    body=RevokeInviteLinkBody,
    reply=RevokeInviteLinkReply,
    errors=(400, 401, 403, 404),
)


class SwitchOnInviteLinkBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


@schema(doc="The link, as in the list.")
class SwitchOnInviteLinkReply(TypedDict):
    pass


doc(
    invite_links.switch_on_invite_link,
    summary="Switch a link back on",
    tag="Invite links",
    role=ADMIN,
    feature="invite_links",
    body=SwitchOnInviteLinkBody,
    reply=SwitchOnInviteLinkReply,
    errors=(400, 401, 403, 404),
)
