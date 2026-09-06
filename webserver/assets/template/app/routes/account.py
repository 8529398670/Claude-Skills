"""Self-service endpoints available to any logged-in user (admin or not):
who am I, change my display name, log out. This is the minimum bit of
server-side logic needed -- the actual account settings UI is a static
page in static/ that calls these as a JSON API.
"""
from __future__ import annotations

from starlette.requests import Request
from starlette.responses import JSONResponse

from app.core.config import settings
from app.core.security import check_csrf, clear_session_cookie, current_user
from app.models import sessions, users


async def me(request: Request) -> JSONResponse:
    user = current_user(request)
    if user is None:
        return JSONResponse({"authenticated": False}, status_code=401)
    session = request.state.session
    return JSONResponse(
        {
            "authenticated": True,
            "id": user.id,
            "display_name": user.display_name,
            "role": user.role,
            "csrf_token": session.csrf_token,
        }
    )


async def rename(request: Request) -> JSONResponse:
    user = current_user(request)
    if user is None:
        return JSONResponse({"error": "not authenticated"}, status_code=401)

    body = await request.json()
    if not check_csrf(request, body.get("csrf_token", "")):
        return JSONResponse({"error": "invalid csrf token"}, status_code=403)

    display_name = (body.get("display_name") or "").strip()
    if not (1 <= len(display_name) <= 80):
        return JSONResponse({"error": "display_name must be 1-80 characters"}, status_code=400)

    users.rename_user(user.id, display_name)
    return JSONResponse({"ok": True, "display_name": display_name})


async def logout(request: Request) -> JSONResponse:
    raw = request.cookies.get(settings.cookie_name)
    if raw:
        sessions.destroy_session(raw)
    response = JSONResponse({"ok": True})
    clear_session_cookie(response)
    return response
