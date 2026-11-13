"""The API docs for auth.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import TypedDict

from app.apidocs import PUBLIC, doc
from app.routes import auth
from app.routes.schemas_api import SignInSchema


class RedeemLoginLinkBody(TypedDict):
    credential: str


doc(
    auth.redeem_login_link,
    summary="Redeem a login link",
    tag="Sign-in",
    access=PUBLIC,
    body=RedeemLoginLinkBody,
    reply=SignInSchema,
    errors=(400, 403),
)
