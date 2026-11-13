"""Admin-only: deciding who gets in, and removing who should not. This --
plus the first-run bootstrap and `python -m app manage` -- is the entire
signup system.

One admin is the owner (see app/models/users.py). Any admin may delete an
ordinary user; only the owner may delete another admin; and nobody here may
delete, disable, or revoke the owner.

Note this is the *only* thing admin rights gate in the base template. See
references/architecture.md on defaulting new features to "any signed-in
user".
"""
from __future__ import annotations

from dataclasses import dataclass, replace
from typing import Any

from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.core.db import Store
from app.core.logs import KIND_ADMIN
from app.core.security import actor_from, check_csrf
from app.models import credentials, from_iso, login_tokens, to_iso, users, utc_now
from app.models import passwords  # feature:passwords
from app.models import two_factor  # feature:two_factor
from app.models import webhooks  # feature:webhooks
from app.routes.helpers import Refused, act_on_refusals, bad_request, error, forbidden, get_config, get_store, parse_id, read_json, run, send_page
from app.routes.helpers import emit  # feature:webhooks
from app.routes.logs import audit


async def list_users(request: Request) -> Response:
    """Marks the owner on its row, so the UI can tell which delete, disable,
    and revoke buttons the server would refuse.

    ?q= keeps the accounts whose name or username contains it, in any case.
    Here and not in the browser, which only ever holds one page of them."""
    return send_page(request, "users", await run(user_rows, get_store(request), request.query_params.get("q", "")), ("display_name", "role", "disabled", "created_at"))


def user_rows(store: Store, query: str = "") -> list[dict[str, Any]]:
    """Shared with the MCP admin_list_users tool."""
    owner_id = users.get_owner_id(store)
    links = login_tokens.live_login_links(store)
    wanted = query.strip().lower()
    return [
        {
            "id": u.id,
            "display_name": u.display_name,
            # >>> feature:passwords
            "username": u.username,
            # <<< feature:passwords
            "role": u.role,
            "created_at": to_iso(u.created_at),
            "disabled": u.disabled,
            "owner": u.id == owner_id,
            # >>> feature:passwords
            "has_password": passwords.has_password(u),
            "password_blocked": u.password_blocked,
            # <<< feature:passwords
            "two_factor": two_factor.is_on(u),  # feature:two_factor
            "login_link": links.get(u.id),
        }
        for u in users.list_users(store)
        if wanted in u.display_name.lower() or wanted in (u.username or "").lower()  # feature:passwords
        if wanted in u.display_name.lower()  # feature:!passwords
    ]


def link_options(request: Request, body: dict) -> login_tokens.LinkOptions:
    """What the body asked a new login link to allow, with the configured
    defaults filling the gaps (see login_tokens.resolve_link), and the API key
    asking, if one is, so revoking that key voids the link. Raises Refused
    when it is out of range."""
    cfg = get_config(request)
    try:
        options = login_tokens.resolve_link(
            body.get("max_uses"), body.get("expires_in_minutes"),
            cfg.login_token_ttl_seconds, cfg.multi_use_login_token_ttl_seconds,
        )
    except login_tokens.BadLoginLinkError as refusal:
        raise Refused(400, str(refusal)) from None
    return replace(options, minted_by_key=actor_from(request).key_id)


def link_response(credential: str, info: dict) -> dict[str, Any]:
    """What every route that mints a link answers. The link is a path, not a
    full URL (see create_user); the MCP tools add login_url from the address
    the agent reached /mcp at."""
    expires_in = 0
    if info["expires_at"] is not None:
        expires_in = max(0, round((from_iso(info["expires_at"]) - utc_now()).total_seconds()))
    return {
        "login_path": f"/login/{credential}",
        "expires_in_seconds": expires_in,
        "expires_at": info["expires_at"],
        "max_uses": info["max_uses"],
        "unlimited": info["unlimited"],
    }


async def create_user(request: Request) -> Response:
    """The signup process: an admin names someone, and gets back a login link
    to hand them -- once by default, or reusable (see
    login_tokens.resolve_link). The link is returned as a path, not a full URL -- the
    browser knows the origin it is talking to, whereas the server behind a
    reverse proxy often does not, and a login link with the wrong host is
    worse than useless."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    try:
        return JSONResponse(await run(create_user_checked, request, body), status_code=201)
    except Refused as refused:
        return refused.response(request)


def create_user_checked(request: Request, body: dict) -> dict[str, Any]:
    """create_user after the request is read, for it and the MCP
    admin_create_user tool. Synchronous: it runs in a worker thread."""
    display_name, role = body.get("display_name"), body.get("role")
    if not users.valid_display_name(display_name):
        raise Refused(400, "display name must be 1-80 characters")
    if not users.valid_role(role):
        raise Refused(400, "role must be admin or user")
    options = link_options(request, body)
    store = get_store(request)
    user = users.create_user(store, display_name, role)
    credential, info = login_tokens.issue_login_link(store, user.id, options)
    audit(request, KIND_ADMIN, "user created", {"user_id": user.id, "name": user.display_name, "role": user.role})
    request.app.state.webhooks.emit(webhooks.EVENT_USER_CREATED, {"user_id": user.id, "display_name": user.display_name, "role": user.role})  # feature:webhooks
    return {"user_id": user.id, **link_response(credential, info)}


async def reissue_login(request: Request) -> Response:
    """Covers the only real failure mode of this auth model: someone lost
    their cookie or their device, and there is no password reset to fall back
    on. An admin mints a fresh link instead. Any link the account could still
    sign in with stops working (see login_tokens.issue_login_link).

    It is not a reset: sessions the account already has stay signed in. When
    the worry is that someone else holds the account's credentials, use
    revoke_access first, then reissue."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    try:
        return JSONResponse(await run(reissue_login_checked, request, user_id, body))
    except Refused as refused:
        return refused.response(request)


def reissue_login_checked(request: Request, user_id: int, body: dict) -> dict[str, Any]:
    """reissue_login after the request is read, for it and the MCP
    admin_reissue_login tool."""
    store = get_store(request)
    if users.get_user(store, user_id) is None:
        raise Refused(404, "no such user")
    options = replace(link_options(request, body), actor_id=actor_from(request).user.id)
    with act_on_refusals("reissue a login link for"):
        credential, info = login_tokens.issue_login_link(store, user_id, options)
    audit(request, KIND_ADMIN, "login link reissued", {"user_id": user_id, "max_uses": info["max_uses"], "expires_at": info["expires_at"]})
    return link_response(credential, info)


async def cancel_login_link(request: Request) -> Response:
    """Stop an account's live login link working, without making a new one:
    a reusable link that went somewhere it should not have. Sessions it
    already made stay signed in; revoke_access ends those too.

    An API key may do this, like revoke_access: it only takes a way in away."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    try:
        return JSONResponse(await run(cancel_login_link_checked, request, user_id))
    except Refused as refused:
        return refused.response(request)


def cancel_login_link_checked(request: Request, user_id: int) -> dict[str, Any]:
    """cancel_login_link after the request is read, for it and the MCP
    admin_cancel_login_link tool."""
    store = get_store(request)
    if users.get_user(store, user_id) is None:
        raise Refused(404, "no such user")
    with act_on_refusals("cancel the login link of"):
        removed = login_tokens.cancel_login_link(store, user_id, actor_from(request).user.id)
    audit(request, KIND_ADMIN, "login link cancelled", {"user_id": user_id, "removed": removed})
    return {"ok": True, "removed": removed}


async def set_user_disabled(request: Request) -> Response:
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    disabled = body.get("disabled")
    if not isinstance(disabled, bool):
        return bad_request("disabled must be true or false")

    refused = await run(set_disabled_checked, get_store(request), actor_from(request).user.id, user_id, disabled)
    if refused is not None:
        return refused.response()
    audit(request, KIND_ADMIN, "user disabled set", {"user_id": user_id, "disabled": disabled})
    await emit(request, webhooks.EVENT_USER_DISABLED, {"user_id": user_id, "disabled": disabled})  # feature:webhooks
    return JSONResponse({"ok": True})


async def revoke_access(request: Request) -> Response:
    """The response to "this account may be compromised": every session,
    live login link, and live API key it holds stops working at once. The
    account itself stays enabled; reissue a login link to let its owner back
    in. See credentials.revoke_all_credentials for why this is separate from
    both disabling and reissuing."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    revoked = await run(revoke_access_checked, get_store(request), actor_from(request).user.id, user_id)
    if isinstance(revoked, RuleRefusal):
        return revoked.response()
    audit(request, KIND_ADMIN, "access revoked", {"user_id": user_id})
    return JSONResponse({"ok": True, "revoked": revoked.to_dict()})


async def rename_user(request: Request) -> Response:
    """An admin renaming someone else. Who may rename whom lives in
    credentials.rename_user_as."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    try:
        return JSONResponse(await run(rename_user_checked, request, user_id, body.get("display_name"), body.get("expect")))
    except Refused as refused:
        return refused.response(request)


def rename_user_checked(request: Request, user_id: int, name: Any, expect: Any) -> dict[str, Any]:
    """rename_user after the request is read, for it and the MCP
    admin_rename_user tool."""
    if not users.valid_display_name(name):
        raise Refused(400, "display name must be 1-80 characters")
    if expect is not None and not isinstance(expect, str):
        raise Refused(400, "expect must be a string")
    try:
        found = credentials.rename_user_as(get_store(request), user_id, actor_from(request).user.id, name, expect)
    except users.RenameConflictError as stale:
        raise Refused(409, str(stale), display_name=stale.current) from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can rename an admin") from None
    if not found:
        raise Refused(404, "no such user")
    audit(request, KIND_ADMIN, "user renamed", {"user_id": user_id, "name": name.strip()})
    return {"ok": True, "display_name": name.strip()}


async def set_user_role(request: Request) -> Response:
    """Make an account an admin or an ordinary user. Who may do what lives in
    credentials.set_user_role."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    try:
        return JSONResponse(await run(set_user_role_checked, request, user_id, body.get("role")))
    except Refused as refused:
        return refused.response(request)


def set_user_role_checked(request: Request, user_id: int, role: Any) -> dict[str, Any]:
    """set_user_role after the request is read, for it and the MCP
    admin_set_user_role tool."""
    if not users.valid_role(role):
        raise Refused(400, "role must be admin or user")
    try:
        changed = credentials.set_user_role(get_store(request), user_id, actor_from(request).user.id, role)
    except credentials.SelfRoleError as refusal:
        raise Refused(400, str(refusal)) from None
    except credentials.OwnerProtectedError:
        raise Refused(403, "the owner is always an admin") from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can take away an admin's rights") from None
    if changed is None:
        raise Refused(404, "no such user")
    if changed:
        audit(request, KIND_ADMIN, "user role changed", {"user_id": user_id, "role": role})
    return {"ok": True, "role": role}


async def delete_user(request: Request) -> Response:
    """Remove an account for good, together with every session, login link,
    and API key it held. Disabling is the reversible version; this is for an
    account that should not exist at all.

    Who may delete whom lives in credentials.delete_user, checked inside its
    transaction. An admin-scoped API key may do it too, and the MCP tool is
    marked destructive so a client asks first."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    try:
        return JSONResponse(await run(delete_user_checked, request, user_id))
    except Refused as refused:
        return refused.response(request)


def delete_user_checked(request: Request, user_id: int) -> dict[str, Any]:
    """delete_user after the request is read, for it and the MCP
    admin_delete_user tool."""
    try:
        removed = credentials.delete_user(get_store(request), user_id, actor_from(request).user.id)
    except credentials.SelfDeleteError:
        raise Refused(400, "you cannot delete your own account") from None
    except credentials.OwnerProtectedError:
        raise Refused(403, "the owner account cannot be deleted") from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can delete an admin account") from None
    if removed is None:
        raise Refused(404, "no such user")
    audit(request, KIND_ADMIN, "user deleted", {"user_id": user_id})
    request.app.state.webhooks.emit(webhooks.EVENT_USER_DELETED, {"user_id": user_id})  # feature:webhooks
    return {"ok": True, "removed": removed.to_dict()}


# >>> feature:two_factor
async def reset_two_factor(request: Request) -> Response:
    """For someone who lost both the phone and the recovery codes. Who may
    reset whom lives in credentials.reset_two_factor."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    try:
        return JSONResponse(await run(reset_two_factor_checked, request, user_id))
    except Refused as refused:
        return refused.response(request)


def reset_two_factor_checked(request: Request, user_id: int) -> dict[str, Any]:
    """reset_two_factor after the request is read, for it and the MCP
    admin_reset_two_factor tool."""
    try:
        found = credentials.reset_two_factor(get_store(request), user_id, actor_from(request).user.id)
    except credentials.SelfResetError:
        raise Refused(400, "you cannot reset your own two-factor sign-in") from None
    except credentials.OwnerProtectedError:
        raise Refused(403, "the owner's two-factor sign-in can only be reset from a shell") from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can reset an admin's two-factor sign-in") from None
    if not found:
        raise Refused(404, "no such user")
    audit(request, KIND_ADMIN, "two-factor reset", {"user_id": user_id})
    return {"ok": True}


# <<< feature:two_factor
# ---------------------------------------------------------------------------
# rules shared with the MCP tools
#
# A refusal is kept apart from how it is delivered: the HTTP handlers above
# send it as a status and an error body, the MCP tools (mcp_tools.py) as a
# tool error with the same words. Writing each rule once is what keeps the two
# doors from drifting apart. These are synchronous; call them through run().
# ---------------------------------------------------------------------------


@dataclass
class RuleRefusal:
    status: int
    message: str

    def response(self) -> JSONResponse:
        return error(self.status, self.message)


def set_disabled_checked(store: Store, self_id: int, user_id: int, disabled: bool) -> RuleRefusal | None:
    """Every rule for switching an account off or on."""
    if users.get_user(store, user_id) is None:
        return RuleRefusal(404, "no such user")
    # Locking yourself out is unrecoverable without shell access, so it is
    # worth one guard rail even though an admin could still disable a peer.
    if disabled and user_id == self_id:
        return RuleRefusal(400, "you cannot disable your own account")

    # Disabling also deletes the account's sessions and live login links,
    # in the same transaction -- see credentials.set_user_disabled. The owner
    # cannot be disabled, and only the owner switches another admin off or
    # on; those checks are in the model so every route gets them.
    try:
        found = credentials.set_user_disabled(store, user_id, disabled, actor_id=self_id)
    except credentials.OwnerProtectedError:
        return RuleRefusal(403, "the owner account cannot be disabled")
    except credentials.OwnerRequiredError:
        return RuleRefusal(403, "only the owner can disable or enable an admin account")
    if not found:
        return RuleRefusal(404, "no such user")
    return None


def revoke_access_checked(store: Store, self_id: int, user_id: int) -> credentials.Revoked | RuleRefusal:
    """Every rule for cutting an account's credentials off."""
    # Same guard rail as disabling: revoking your own access signs you out
    # with no link to get back in.
    if user_id == self_id:
        return RuleRefusal(400, "you cannot revoke your own access")

    # The owner is refused here (allow_owner False), and only the owner
    # revokes another admin. Only an operator with shell access may cut the
    # owner off -- see revoke_all_credentials.
    try:
        revoked = credentials.revoke_all_credentials(store, user_id, actor_id=self_id)
    except credentials.OwnerProtectedError:
        return RuleRefusal(403, "the owner account's access cannot be revoked")
    except credentials.OwnerRequiredError:
        return RuleRefusal(403, "only the owner can revoke the access of an admin account")
    if revoked is None:
        return RuleRefusal(404, "no such user")
    return revoked
