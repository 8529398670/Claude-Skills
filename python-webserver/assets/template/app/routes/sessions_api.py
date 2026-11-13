# whole-file feature:sessions
"""The API docs for sessions.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, NotRequired, TypedDict

from app.apidocs import ADMIN, Field, doc
from app.routes import sessions
from app.routes.schemas_api import SessionSchema


class ListMySessionsParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]


class ListMySessionsReply(TypedDict):
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    sessions: list[SessionSchema]
    total: int


doc(
    sessions.list_my_sessions,
    summary="Your signed-in devices",
    tag="Account",
    feature="sessions",
    params=ListMySessionsParams,
    reply=ListMySessionsReply,
    errors=(400, 401, 403),
)


class RevokeOthersBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class RevokeOthersReply(TypedDict):
    ok: bool
    removed: int


doc(
    sessions.revoke_others,
    summary="Sign out every other device",
    tag="Account",
    feature="sessions",
    body=RevokeOthersBody,
    reply=RevokeOthersReply,
    errors=(400, 401, 403),
)


class LabelOneBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expect: NotRequired[Annotated[str, Field(doc="The name you think it has. Answers 409 if it has another.")]]
    label: Annotated[str, Field(doc="At most 80 characters.")]


class LabelOneReply(TypedDict):
    label: str
    ok: bool


doc(
    sessions.label_one,
    summary="Name one of your devices",
    description="This device included. Empty clears the name.",
    tag="Account",
    feature="sessions",
    body=LabelOneBody,
    reply=LabelOneReply,
    errors=(400, 401, 403, 404, 409),
)


class RevokeOneBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class RevokeOneReply(TypedDict):
    ok: bool


doc(
    sessions.revoke_one,
    summary="Sign out one device",
    tag="Account",
    feature="sessions",
    body=RevokeOneBody,
    reply=RevokeOneReply,
    errors=(400, 401, 403, 404),
)


class ListUserSessionsParams(TypedDict):
    user_id: Annotated[int, Field(path="user_id")]
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]


class ListUserSessionsReply(TypedDict):
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    sessions: list[SessionSchema]
    total: int


doc(
    sessions.list_user_sessions,
    summary="An account's signed-in devices",
    description="The owner's are the owner's alone, and only the owner sees addresses.",
    tag="Admin",
    role=ADMIN,
    feature="sessions",
    params=ListUserSessionsParams,
    reply=ListUserSessionsReply,
    errors=(400, 401, 403, 404),
)


class RevokeUserSessionParams(TypedDict):
    user_id: Annotated[int, Field(path="user_id")]


class RevokeUserSessionBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class RevokeUserSessionReply(TypedDict):
    ok: bool


doc(
    sessions.revoke_user_session,
    summary="Sign out one of an account's devices",
    tag="Admin",
    role=ADMIN,
    feature="sessions",
    params=RevokeUserSessionParams,
    body=RevokeUserSessionBody,
    reply=RevokeUserSessionReply,
    errors=(400, 401, 403, 404),
)
