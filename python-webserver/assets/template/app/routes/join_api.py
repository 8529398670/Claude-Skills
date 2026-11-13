# whole-file feature:invite_links
"""The API docs for join.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import NotRequired, TypedDict

from app.apidocs import PUBLIC, doc
from app.routes import join
from app.routes.schemas_api import JoinedSchema
from app.routes.schemas_api import MemberSchema


class JoinClaimBody(TypedDict):
    credential: str
    display_name: NotRequired[str]
    pin: NotRequired[str]
    user_id: NotRequired[int]


doc(
    join.join_claim,
    summary="Sign in as an existing account",
    tag="Invite links",
    access=PUBLIC,
    feature="invite_links",
    body=JoinClaimBody,
    reply=JoinedSchema,
    errors=(400, 403, 404),
)


class JoinCreateBody(TypedDict):
    credential: str
    display_name: NotRequired[str]
    pin: NotRequired[str]
    user_id: NotRequired[int]


doc(
    join.join_create,
    summary="Join as a new account",
    tag="Invite links",
    access=PUBLIC,
    feature="invite_links",
    body=JoinCreateBody,
    status=201,
    reply=JoinedSchema,
    errors=(400, 404, 409),
)


class JoinInfoBody(TypedDict):
    credential: str
    display_name: NotRequired[str]
    pin: NotRequired[str]
    user_id: NotRequired[int]


class JoinInfoReply(TypedDict):
    label: str
    remaining: int


doc(
    join.join_info,
    summary="What an invite link is",
    tag="Invite links",
    access=PUBLIC,
    feature="invite_links",
    body=JoinInfoBody,
    reply=JoinInfoReply,
    errors=(404,),
)


class JoinMatchBody(TypedDict):
    credential: str
    display_name: NotRequired[str]
    pin: NotRequired[str]
    user_id: NotRequired[int]


class JoinMatchReply(TypedDict):
    can_create: bool
    matches: list[MemberSchema]


doc(
    join.join_match,
    summary="Accounts matching a name",
    tag="Invite links",
    access=PUBLIC,
    feature="invite_links",
    body=JoinMatchBody,
    reply=JoinMatchReply,
    errors=(400, 404),
)
