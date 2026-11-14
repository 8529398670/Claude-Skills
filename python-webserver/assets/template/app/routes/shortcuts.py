# whole-file feature:shortcuts
"""The caller's own keyboard shortcuts, from the Shortcuts screen
(static/js/shortcuts.js). See app/models/shortcuts.py."""
from __future__ import annotations

from typing import Annotated

from msgspec import Meta
from starlette.requests import Request

from app.apidocs import Reply, route
from app.apidocs import Request as RequestType
from app.core.logs import KIND_AUTH
from app.core.security import actor_from
from app.models import shortcuts
from app.routes.helpers import Refused, get_store
from app.routes.logs import audit


class SetMyShortcuts(RequestType):
    shortcuts: Annotated[dict[str, str], Meta(description="Action name to binding, such as \"search\": \"Mod+K\". Mod is Cmd on a Mac and Ctrl elsewhere. Only changed shortcuts are listed; an empty binding switches that key off. Replaces the whole set; {} puts every key back to its default.")]


class SetMyShortcutsReply(Reply):
    ok: bool
    shortcuts: dict[str, str]


@route(
    "POST /api/account/shortcuts", summary="Change your keyboard shortcuts",
    tag="Account", feature="shortcuts",
    description='Replace your changed keyboard shortcuts: action name to binding, such as "search": "Mod+K" '
    '(Mod is Cmd on a Mac, Ctrl elsewhere). Send the whole set; an empty binding switches a key off, and {} '
    'puts every key back to its default.',
    mcp_note="whoami shows the current set.",  # feature:mcp
    mcp="set_shortcuts",  # feature:mcp
)
def set_my_shortcuts(request: Request, body: SetMyShortcuts) -> SetMyShortcutsReply:
    """Replaces the caller's changed shortcuts, over HTTP or the MCP
    set_shortcuts tool. The whole set is sent each time, so an undo is the
    same call with the set from before; {} puts every key back to its
    default."""
    try:
        wanted = shortcuts.check(body.shortcuts)
    except shortcuts.ShortcutsError as invalid:
        raise Refused(400, str(invalid)) from None
    if not shortcuts.set_shortcuts(get_store(request), actor_from(request).user.id, wanted):
        raise Refused.internal("shortcuts: the signed-in account has no record")
    audit(request, KIND_AUTH, "changed own shortcuts", {"count": len(wanted)})
    return SetMyShortcutsReply(ok=True, shortcuts=wanted)
