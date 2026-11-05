"""A person's own sessions: which devices are signed in, and signing them out
(app/models/sessions.py). A session is required, so an API key gets 403, and
every id is checked against the account asking: someone else's session and one
that does not exist get the same 404. Ending the session in use is what signing
out is for.
"""
from __future__ import annotations

from typing import Any

from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.core.logs import KIND_AUTH
from app.core.security import actor_from, check_csrf
from app.models import sessions, to_iso
from app.routes.helpers import bad_request, forbidden, get_store, not_found, read_json, run, send_page
from app.routes.logs import audit


def session_row(session: sessions.Session, current_id: str) -> dict[str, Any]:
    """Field by field on purpose: secret_hash and csrf_token must not reach a
    response because something later adds a loop over the record."""
    row: dict[str, Any] = {
        "id": session.id,
        "current": session.id == current_id,
        "created_at": to_iso(session.created_at),
        "expires_at": to_iso(session.expires_at),
    }
    if session.last_seen_at is not None:
        row["last_seen_at"] = to_iso(session.last_seen_at)
    if session.ip:
        row["ip"] = session.ip
    if session.user_agent:
        row["user_agent"] = session.user_agent
    return row


async def list_my_sessions(request: Request) -> Response:
    actor = actor_from(request)
    found = await run(sessions.list_sessions, get_store(request), actor.user.id)
    return send_page(request, "sessions", [session_row(session, actor.session.id) for session in found])


async def revoke_one(request: Request) -> Response:
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    actor = actor_from(request)
    session_id = request.path_params["session_id"]
    if session_id == actor.session.id:
        return bad_request("use sign out for this device")
    if not await run(sessions.revoke_session, get_store(request), actor.user.id, session_id):
        return not_found("not found")
    audit(request, KIND_AUTH, "session_revoked")
    return JSONResponse({"ok": True})


async def revoke_others(request: Request) -> Response:
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    actor = actor_from(request)
    removed = await run(sessions.revoke_other_sessions, get_store(request), actor.user.id, actor.session.id)
    audit(request, KIND_AUTH, "sessions_revoked", {"removed": removed})
    return JSONResponse({"ok": True, "removed": removed})
