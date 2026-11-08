# whole-file feature:shortcuts
"""The caller's own keyboard shortcuts, from the Shortcuts screen
(static/js/shortcuts.js). See app/models/shortcuts.py."""
from __future__ import annotations

from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.core.logs import KIND_AUTH
from app.core.security import actor_from, check_csrf
from app.models import shortcuts
from app.routes.helpers import bad_request, forbidden, get_store, read_json, run, server_error
from app.routes.logs import audit


async def set_my_shortcuts(request: Request) -> Response:
    """Replaces the caller's changed shortcuts. The whole set is sent each
    time, so an undo is the same call with the set from before; {} puts every
    key back to its default."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        wanted = shortcuts.check(body.get("shortcuts") if body.get("shortcuts") is not None else {})
    except shortcuts.ShortcutsError as invalid:
        return bad_request(str(invalid))
    if not await run(shortcuts.set_shortcuts, get_store(request), actor_from(request).user.id, wanted):
        return server_error(request, "shortcuts: the signed-in account has no record")
    audit(request, KIND_AUTH, "changed own shortcuts", {"count": len(wanted)})
    return JSONResponse({"ok": True, "shortcuts": wanted})
