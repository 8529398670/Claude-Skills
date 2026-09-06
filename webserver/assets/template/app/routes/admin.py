"""Admin-only JSON API: list users, mint a login link for a new or existing
user, enable/disable accounts. This -- plus scripts/manage.py for
bootstrapping the very first admin -- is the entire "signup" system.
"""
from __future__ import annotations

from starlette.requests import Request
from starlette.responses import JSONResponse

from app.core.security import check_csrf, current_user, require_role
from app.models import login_tokens, sessions, users


def _login_url(request: Request, token: str) -> str:
    return str(request.base_url).rstrip("/") + f"/login/{token}"


async def list_users(request: Request) -> JSONResponse:
    if (forbidden := require_role(request, "admin")) is not None:
        return forbidden
    return JSONResponse(
        [
            {"id": u.id, "display_name": u.display_name, "role": u.role, "disabled": u.disabled}
            for u in users.list_users()
        ]
    )


async def create_user(request: Request) -> JSONResponse:
    if (forbidden := require_role(request, "admin")) is not None:
        return forbidden

    body = await request.json()
    if not check_csrf(request, body.get("csrf_token", "")):
        return JSONResponse({"error": "invalid csrf token"}, status_code=403)

    display_name = (body.get("display_name") or "").strip()
    role = body.get("role", "user")
    if not (1 <= len(display_name) <= 80) or role not in ("admin", "user"):
        return JSONResponse({"error": "invalid display_name or role"}, status_code=400)

    user = users.create_user(display_name, role)
    issued = login_tokens.issue_login_token(user.id)
    return JSONResponse(
        {"user_id": user.id, "login_url": _login_url(request, issued.token)}, status_code=201
    )


async def reissue_login(request: Request) -> JSONResponse:
    """Mint a fresh one-time link for an existing user -- e.g. they lost
    their cookie/device and an admin needs to get them back in."""
    if (forbidden := require_role(request, "admin")) is not None:
        return forbidden

    body = await request.json()
    if not check_csrf(request, body.get("csrf_token", "")):
        return JSONResponse({"error": "invalid csrf token"}, status_code=403)

    user_id = int(request.path_params["user_id"])
    if users.get_user(user_id) is None:
        return JSONResponse({"error": "no such user"}, status_code=404)

    issued = login_tokens.issue_login_token(user_id)
    return JSONResponse({"login_url": _login_url(request, issued.token)})


async def set_disabled(request: Request) -> JSONResponse:
    if (forbidden := require_role(request, "admin")) is not None:
        return forbidden

    body = await request.json()
    if not check_csrf(request, body.get("csrf_token", "")):
        return JSONResponse({"error": "invalid csrf token"}, status_code=403)

    user_id = int(request.path_params["user_id"])
    target = users.get_user(user_id)
    if target is None:
        return JSONResponse({"error": "no such user"}, status_code=404)
    if target.id == current_user(request).id and bool(body.get("disabled")):
        return JSONResponse({"error": "cannot disable your own account"}, status_code=400)

    disabled = bool(body.get("disabled"))
    users.set_disabled(user_id, disabled)
    if disabled:
        sessions.destroy_all_sessions_for_user(user_id)
    return JSONResponse({"ok": True})
