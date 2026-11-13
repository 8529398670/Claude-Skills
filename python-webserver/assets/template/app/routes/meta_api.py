"""The API docs for meta.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import TypedDict

from app.apidocs import PUBLIC, doc
from app.routes import meta


class GetHealthReply(TypedDict):
    ok: bool


doc(
    meta.get_health,
    summary="Is the process up",
    tag="Monitoring",
    access=PUBLIC,
    reply=GetHealthReply,
)


class GetLanguageReply(TypedDict):
    pass


doc(
    meta.get_language,
    summary="The UI's text",
    tag="Account",
    access=PUBLIC,
    reply=GetLanguageReply,
)


class GetReadyReply(TypedDict):
    ok: bool


doc(
    meta.get_ready,
    summary="Can the server do real work",
    tag="Monitoring",
    access=PUBLIC,
    reply=GetReadyReply,
)
