# whole-file feature:passwords
"""The API docs for passwords.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, NotRequired, TypedDict

from app.apidocs import Field, OWNER, PUBLIC, SESSION_ONLY, doc
from app.routes import passwords
from app.routes.schemas_api import SignInSchema


class SetPasswordBody(TypedDict):
    code: NotRequired[str]
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    password_digest: str
    username: str


class SetPasswordReply(TypedDict):
    has_password: bool
    ok: bool


doc(
    passwords.set_password,
    summary="Set or clear your password",
    tag="Account",
    access=SESSION_ONLY,
    feature="passwords",
    body=SetPasswordBody,
    reply=SetPasswordReply,
    errors=(400, 401, 403, 404, 409),
)


class LoginOptionsReply(TypedDict):
    passwords: bool
    salt: NotRequired[str]
    two_factor: bool  # feature:two_factor


doc(
    passwords.login_options,
    summary="What sign-in offers",
    tag="Sign-in",
    access=PUBLIC,
    feature="passwords",
    reply=LoginOptionsReply,
)


class PasswordLoginBody(TypedDict):
    password_digest: str
    username: str


doc(
    passwords.password_login,
    summary="Sign in with a password",
    tag="Sign-in",
    access=PUBLIC,
    feature="passwords",
    body=PasswordLoginBody,
    reply=SignInSchema,
    errors=(400, 403, 404),
)


class SetPasswordAccessBody(TypedDict):
    allowed: bool
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class SetPasswordAccessReply(TypedDict):
    ok: bool
    password_blocked: bool


doc(
    passwords.set_password_access,
    summary="Allow or block an account's password",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="passwords",
    body=SetPasswordAccessBody,
    reply=SetPasswordAccessReply,
    errors=(400, 403, 404),
)
