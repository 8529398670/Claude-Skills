# whole-file feature:sessions
"""A person's own sessions: which devices are signed in, and signing them out
(app/models/sessions.py). An API key may do it too; it has no device of its
own, so nothing is current and revoke-others ends them all. Every id is checked against the account asking: someone else's session and one
that does not exist get the same 404. Ending the session in use is what signing
out is for.
"""
from __future__ import annotations

from typing import Any

from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.core.db import Store
from app.core.logs import KIND_ADMIN, KIND_AUTH
from app.core.security import actor_from, check_csrf
from app.models import sessions, to_iso, users
from app.models import ip_names  # feature:logs
from app.routes.helpers import Refused, bad_request, forbidden, get_store, parse_id, read_json, run, send_page
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
    if session.label:
        row["label"] = session.label
    return row


def current_session_id(request: Request) -> str:
    """The session this request came with, or "" through an API key, which has
    none: nothing in a list is then "current", and nothing is refused as "this
    device"."""
    session = actor_from(request).session
    return session.id if session is not None else ""


async def list_my_sessions(request: Request) -> Response:
    return send_page(request, "sessions", await run(my_session_rows, request))


def my_session_rows(request: Request) -> list[dict[str, Any]]:
    """list_my_sessions' list, for it and the MCP list_my_sessions tool.
    Synchronous: it runs in a worker thread."""
    store, actor = get_store(request), actor_from(request)
    rows = [session_row(session, current_session_id(request)) for session in sessions.list_sessions(store, actor.user.id)]
    # >>> feature:logs
    if actor.user.id == users.get_owner_id(store):
        _name_rows(store, rows)
    # <<< feature:logs
    return rows


# >>> feature:logs
def _name_rows(store: Store, rows: list[dict[str, Any]]) -> None:
    """Adds what the owner calls each address. Names are the owner's own notes
    about people, so they go to the owner and nobody else."""
    namer = ip_names.load_namer(store)
    for row in rows:
        name = namer(row.get("ip", ""))
        if name:
            row["ip_name"] = name


# <<< feature:logs
def _target_of(store: Store, self_id: int, user_id: int) -> bool:
    """Whether the owner is asking about an account's sessions, raising
    Refused when the account may not be reached. Nobody but the owner reaches
    the owner's, the same rule as revoking the owner's access."""
    if users.get_user(store, user_id) is None:
        raise Refused(404, "no such user")
    owner_id = users.get_owner_id(store)
    by_owner = self_id == owner_id
    if user_id == owner_id and not by_owner:
        raise Refused(403, "the owner account's sessions are the owner's own")
    return by_owner


async def list_user_sessions(request: Request) -> Response:
    """Where an account is signed in, for ending one device rather than
    everything with revoke-access. Addresses are for the owner only, as in the
    activity log, which keeps them from other admins too."""
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    try:
        rows = await run(user_session_rows, request, user_id)
    except Refused as refused:
        return refused.response(request)
    return send_page(request, "sessions", rows)


def user_session_rows(request: Request, user_id: int) -> list[dict[str, Any]]:
    """list_user_sessions' list, for it and the MCP admin_list_user_sessions
    tool."""
    store = get_store(request)
    by_owner = _target_of(store, actor_from(request).user.id, user_id)
    rows = [session_row(session, current_session_id(request)) for session in sessions.list_sessions(store, user_id)]
    if not by_owner:
        for row in rows:
            row.pop("ip", None)
    if by_owner:  # feature:logs
        _name_rows(store, rows)  # feature:logs
    return rows


async def revoke_user_session(request: Request) -> Response:
    """Sign one of an account's devices out, and only that."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    try:
        return JSONResponse(await run(revoke_user_session_checked, request, user_id, request.path_params["session_id"]))
    except Refused as refused:
        return refused.response(request)


def revoke_user_session_checked(request: Request, user_id: int, session_id: str) -> dict[str, Any]:
    """revoke_user_session after the request is read, for it and the MCP
    admin_revoke_user_session tool."""
    store = get_store(request)
    _target_of(store, actor_from(request).user.id, user_id)
    if session_id and session_id == current_session_id(request):
        raise Refused(400, "use sign out for this device")
    if not sessions.revoke_session(store, user_id, session_id):
        raise Refused(404, "not found")
    audit(request, KIND_ADMIN, "session revoked", {"user_id": user_id})
    return {"ok": True}


async def label_one(request: Request) -> Response:
    """Name one of your own devices, this one included. An empty label clears
    the name. Admins see a label but cannot set one: it is what the person
    calls their own device."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        return JSONResponse(await run(label_one_checked, request, request.path_params["session_id"], body.get("label"), body.get("expect")))
    except Refused as refused:
        return refused.response(request)


def label_one_checked(request: Request, session_id: str, label: Any, expect: Any) -> dict[str, Any]:
    """label_one after the request is read, for it and the MCP label_my_session
    tool."""
    if not sessions.valid_session_label(label):
        raise Refused(400, "a device name can be at most 80 characters")
    if expect is not None and not isinstance(expect, str):
        raise Refused(400, "expect must be a string")
    try:
        found = sessions.label_session(get_store(request), actor_from(request).user.id, session_id, label, expect)
    except users.RenameConflictError as stale:
        raise Refused(409, str(stale), label=stale.current) from None
    if not found:
        raise Refused(404, "not found")
    # Not the label: it is the person's own words about where they are.
    audit(request, KIND_AUTH, "session_labelled", {})
    return {"ok": True, "label": label.strip()}


async def revoke_one(request: Request) -> Response:
    """Sign out one other device. This device is refused: ending it here would
    leave the browser holding a dead cookie, and signing out is what the
    sign-out button is for. Through an API key there is no "this device", so
    any of them may go."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        return JSONResponse(await run(revoke_one_checked, request, request.path_params["session_id"]))
    except Refused as refused:
        return refused.response(request)


def revoke_one_checked(request: Request, session_id: str) -> dict[str, Any]:
    """revoke_one after the request is read, for it and the MCP
    revoke_my_session tool."""
    if session_id and session_id == current_session_id(request):
        raise Refused(400, "use sign out for this device")
    if not sessions.revoke_session(get_store(request), actor_from(request).user.id, session_id):
        raise Refused(404, "not found")
    audit(request, KIND_AUTH, "session_revoked")
    return {"ok": True}


async def revoke_others(request: Request) -> Response:
    """The button for a lost phone or a shared computer: everything but this
    device. Through an API key, which is no device, that is every session the
    account has."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    return JSONResponse(await run(revoke_others_checked, request))


def revoke_others_checked(request: Request) -> dict[str, Any]:
    """revoke_others after the request is read, for it and the MCP
    revoke_my_other_sessions tool."""
    removed = sessions.revoke_other_sessions(get_store(request), actor_from(request).user.id, current_session_id(request))
    audit(request, KIND_AUTH, "sessions_revoked", {"removed": removed})
    return {"ok": True, "removed": removed}
