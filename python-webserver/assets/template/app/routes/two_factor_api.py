# whole-file feature:two_factor
"""The API docs for two_factor.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, NotRequired, TypedDict

from app.apidocs import Field, PUBLIC, SESSION_ONLY, doc
from app.routes import two_factor


class ConfirmSetupBody(TypedDict):
    code: str
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class ConfirmSetupReply(TypedDict):
    recovery_codes: list[str]


doc(
    two_factor.confirm_setup,
    summary="Turn two-factor on",
    tag="Account",
    access=SESSION_ONLY,
    feature="two_factor",
    body=ConfirmSetupBody,
    reply=ConfirmSetupReply,
    errors=(400, 401, 403, 404, 409),
)


class DisableBody(TypedDict):
    code: str
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class DisableReply(TypedDict):
    ok: bool


doc(
    two_factor.disable,
    summary="Turn two-factor off",
    tag="Account",
    access=SESSION_ONLY,
    feature="two_factor",
    body=DisableBody,
    reply=DisableReply,
    errors=(400, 401, 403, 404, 409),
)


class StartSetupBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class StartSetupReply(TypedDict):
    secret: str


doc(
    two_factor.start_setup,
    summary="Begin two-factor setup",
    tag="Account",
    access=SESSION_ONLY,
    feature="two_factor",
    body=StartSetupBody,
    reply=StartSetupReply,
    errors=(400, 401, 403, 404, 409),
)


class FinishSignInBody(TypedDict):
    challenge: str
    code: str


class FinishSignInReply(TypedDict):
    ok: bool


doc(
    two_factor.finish_sign_in,
    summary="Finish a sign-in with a code",
    tag="Sign-in",
    access=PUBLIC,
    feature="two_factor",
    body=FinishSignInBody,
    reply=FinishSignInReply,
    errors=(400, 403),
)
