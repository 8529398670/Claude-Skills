# whole-file feature:shortcuts
"""The API docs for shortcuts.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, NotRequired, TypedDict

from app.apidocs import Field, doc
from app.routes import shortcuts


class SetMyShortcutsBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    shortcuts: Annotated[dict[str, str], Field(doc="Action name to binding, such as \"search\": \"Mod+K\". Mod is Cmd on a Mac and Ctrl elsewhere. Only changed shortcuts are listed; an empty binding switches that key off. Replaces the whole set; {} puts every key back to its default.")]


class SetMyShortcutsReply(TypedDict):
    ok: bool
    shortcuts: dict[str, str]


doc(
    shortcuts.set_my_shortcuts,
    summary="Change your keyboard shortcuts",
    tag="Account",
    feature="shortcuts",
    body=SetMyShortcutsBody,
    reply=SetMyShortcutsReply,
    errors=(400, 401, 403),
)
