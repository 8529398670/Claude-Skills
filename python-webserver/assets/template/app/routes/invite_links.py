# whole-file feature:invite_links
"""Managing invite links (see app/models/invite_links.py; joining through one
is app/routes/join.py). Admin only. The link's credential is returned once,
on creation, as a path -- the browser adds its own origin, like a login
link."""
from __future__ import annotations

from datetime import timedelta
from typing import Any

from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.core.db import Store
from app.core.logs import KIND_ADMIN
from app.core.security import actor_from, check_csrf
from app.models import RestoreError, invite_links, users
from app.models import webhooks  # feature:webhooks
from app.models.invite_links import InviteLink
from app.models.users import User
from app.routes.helpers import Refused, bad_request, forbidden, get_store, match_rows, not_found, read_json, restore_refusal, run, send_page
from app.routes.helpers import emit  # feature:webhooks
from app.routes.logs import audit


def _row(link: InviteLink, creator: User | None) -> dict[str, Any]:
    live = link.live
    # A link outlives its maker's admin rights only on paper: it stops
    # working the moment they lose them (see load_invite_link).
    if creator is None or creator.disabled or not creator.is_admin:
        live = False
    row: dict[str, Any] = {
        "id": link.id,
        "label": link.label,
        "max_uses": link.max_uses,
        "used": len(link.accounts),
        "remaining": link.remaining,
        "live": live,
        "expires_at": link.expires_at.isoformat(),
        "revoked": link.revoked_at is not None,
        "created_by": link.created_by,
    }
    if creator is not None:
        row["created_by_name"] = creator.display_name
    return row


def invite_link_rows(store: Store) -> list[dict[str, Any]]:
    creators = {user.id: user for user in users.list_users(store)}
    return [_row(link, creators.get(link.created_by)) for link in invite_links.list_invite_links(store)]


async def list_invite_links(request: Request) -> Response:
    rows = match_rows(await run(invite_link_rows, get_store(request)), request.query_params.get("q", ""), "label", "created_by_name")
    return send_page(request, "invite_links", rows, ("label", "used", "expires_at"))


async def create_invite_link(request: Request) -> Response:
    """One link for a group (see invite_links.py's module docs)."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        return JSONResponse(await run(create_invite_link_checked, request, body.get("label"), body.get("max_uses"), body.get("days")), status_code=201)
    except Refused as refused:
        return refused.response(request)


def create_invite_link_checked(request: Request, label: Any, max_uses: Any, days: Any) -> dict[str, Any]:
    """create_invite_link after the request is read, for it and the MCP
    admin_create_invite_link tool. Synchronous: it runs in a worker thread."""
    if not invite_links.valid_invite_link(label, max_uses, days):
        raise Refused(400, "label must be 1-80 characters, uses 1-1000, days 1-365")
    actor = actor_from(request)
    credential, link = invite_links.issue_invite_link(get_store(request), label, max_uses, timedelta(days=days), actor.user.id, actor.key_id)
    row = _row(link, actor.user)
    row["join_path"] = f"/join/{credential}"
    audit(request, KIND_ADMIN, "invite link created", {"link_id": link.id, "label": link.label, "max_uses": link.max_uses})
    request.app.state.webhooks.emit(webhooks.EVENT_INVITE_LINK_CREATED, {"link_id": link.id, "label": link.label})  # feature:webhooks
    return row


async def rename_invite_link(request: Request) -> Response:
    """Change a link's label."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        return JSONResponse(await run(rename_invite_link_checked, request, request.path_params["link_id"], body.get("label"), body.get("expect")))
    except Refused as refused:
        return refused.response(request)


def rename_invite_link_checked(request: Request, link_id: str, label: Any, expect: Any) -> dict[str, Any]:
    """rename_invite_link after the request is read, for it and the MCP
    admin_rename_invite_link tool."""
    if not invite_links.valid_invite_label(label):
        raise Refused(400, "label must be 1-80 characters")
    # Optional: the label the caller expects to replace, so an undo cannot
    # overwrite a rename made elsewhere.
    if expect is not None and not isinstance(expect, str):
        raise Refused(400, "expect must be a string")
    try:
        renamed = invite_links.rename_invite_link(get_store(request), link_id, label, expect)
    except users.RenameConflictError as refusal:
        raise Refused(409, str(refusal), label=refusal.current) from None
    if not renamed:
        raise Refused(404, "no such invite link")
    audit(request, KIND_ADMIN, "invite link renamed", {"link_id": link_id, "label": label.strip()})
    return {"ok": True, "label": label.strip()}


async def set_invite_link_limits(request: Request) -> Response:
    """Change a link's places, and its expiry when days is given; leaving days
    out keeps the expiry. Through an API key the link becomes that key's:
    revoking the key switches it off (see invite_links.set_invite_link_limits)."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        return JSONResponse(await run(set_invite_link_limits_checked, request, request.path_params["link_id"], body.get("max_uses"), body.get("days")))
    except Refused as refused:
        return refused.response(request)


def set_invite_link_limits_checked(request: Request, link_id: str, max_uses: Any, days: Any) -> dict[str, Any]:
    """set_invite_link_limits after the request is read, for it and the MCP
    admin_set_invite_link_limits tool."""
    if not invite_links.valid_invite_limits(max_uses, days):
        raise Refused(400, "uses must be 1-1000 and days 1-365")
    store = get_store(request)
    ttl = None if days is None else timedelta(days=days)
    try:
        link = invite_links.set_invite_link_limits(store, link_id, max_uses, ttl, actor_from(request).key_id)
    except invite_links.InvitePlacesError as refusal:
        raise Refused(400, str(refusal)) from None
    if link is None:
        raise Refused(404, "no such invite link")
    audit(request, KIND_ADMIN, "invite link limits changed", {"link_id": link.id, "max_uses": link.max_uses, "expires_at": link.expires_at.isoformat()})
    return _row(link, users.get_user(store, link.created_by))


async def switch_on_invite_link(request: Request) -> Response:
    """Undo revoke_invite_link. A key that switches a link on takes it over, so
    revoking that key closes it again."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        return JSONResponse(await run(switch_on_invite_link_checked, request, request.path_params["link_id"]))
    except Refused as refused:
        return refused.response(request)


def switch_on_invite_link_checked(request: Request, link_id: str) -> dict[str, Any]:
    """switch_on_invite_link after the request is read, for it and the MCP
    admin_switch_on_invite_link tool."""
    store = get_store(request)
    link = invite_links.switch_on_invite_link(store, link_id, actor_from(request).key_id)
    if link is None:
        raise Refused(404, "no such invite link")
    audit(request, KIND_ADMIN, "invite link switched back on", {"link_id": link.id})
    return _row(link, users.get_user(store, link.created_by))


async def revoke_invite_link(request: Request) -> Response:
    """Switch a link off. The accounts it made keep working."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    if not await run(invite_links.revoke_invite_link, get_store(request), request.path_params["link_id"]):
        return not_found("no such invite link")
    audit(request, KIND_ADMIN, "invite link revoked", {"link_id": request.path_params["link_id"]})
    await emit(request, webhooks.EVENT_INVITE_LINK_REVOKED, {"link_id": request.path_params["link_id"]})  # feature:webhooks
    return JSONResponse({"ok": True})


async def delete_invite_link(request: Request) -> Response:
    """Drop a link from the list. The accounts it made stay; they just can no
    longer sign in on a new device through it. restore_invite_link undoes it
    for a few minutes."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        return JSONResponse(await run(delete_invite_link_checked, request, request.path_params["link_id"]))
    except Refused as refused:
        return refused.response(request)


def delete_invite_link_checked(request: Request, link_id: str) -> dict[str, Any]:
    """delete_invite_link after the request is read, for it and the MCP
    admin_delete_invite_link tool."""
    if not invite_links.delete_invite_link(get_store(request), link_id, actor_from(request).user.id):
        raise Refused(404, "no such invite link")
    audit(request, KIND_ADMIN, "invite link deleted", {"link_id": link_id})
    return {"ok": True}


async def restore_invite_link(request: Request) -> Response:
    """Undo delete_invite_link, within RESTORE_WINDOW. The link works again
    exactly as before, uses and expiry included."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        return JSONResponse(await run(restore_invite_link_checked, request, request.path_params["link_id"]))
    except Refused as refused:
        return refused.response(request)


def restore_invite_link_checked(request: Request, link_id: str) -> dict[str, Any]:
    """restore_invite_link after the request is read, for it and the MCP
    admin_restore_invite_link tool."""
    store, actor = get_store(request), actor_from(request)
    try:
        link = invite_links.restore_invite_link(store, link_id, actor.user.id, actor.user.is_admin, actor.key_id)
    except RestoreError as refusal:
        raise restore_refusal(refusal) from None
    audit(request, KIND_ADMIN, "invite link restored", {"link_id": link_id})
    return _row(link, users.get_user(store, link.created_by))
