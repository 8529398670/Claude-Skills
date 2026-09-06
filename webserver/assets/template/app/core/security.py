"""Everything route handlers need to check "who is this" and "is this
request allowed to change state" -- kept in one place so every route
enforces auth and CSRF the same way instead of each reinventing it.
"""
from __future__ import annotations

from typing import Optional

from starlette.requests import Request
from starlette.responses import RedirectResponse, Response

from app.core.config import settings
from app.models import sessions as sessions_model
from app.models import users as users_model
from app.models.users import User


def set_session_cookie(response: Response, raw_cookie_value: str) -> None:
    response.set_cookie(
        settings.cookie_name,
        raw_cookie_value,
        max_age=settings.session_ttl_seconds,
        httponly=True,
        secure=settings.secure_cookies,
        samesite="strict",
        path="/",
    )


def clear_session_cookie(response: Response) -> None:
    response.delete_cookie(settings.cookie_name, path="/")


def current_user(request: Request) -> Optional[User]:
    """Cached per-request so route handlers and middleware can both call
    this without hitting the database twice."""
    if hasattr(request.state, "user"):
        return request.state.user
    raw = request.cookies.get(settings.cookie_name)
    session = sessions_model.load_session(raw) if raw else None
    user = users_model.get_user(session.user_id) if session else None
    request.state.user = user
    request.state.session = session
    return user


def require_login(request: Request) -> Optional[RedirectResponse]:
    """Call at the top of a protected route. Returns a redirect to hand
    straight back to Starlette if the caller isn't logged in, else None."""
    if current_user(request) is None:
        return RedirectResponse("/", status_code=303)
    return None


def require_role(request: Request, role: str) -> Optional[Response]:
    user = current_user(request)
    if user is None:
        return RedirectResponse("/", status_code=303)
    if user.role != role:
        return Response("Forbidden", status_code=403)
    return None


def check_csrf(request: Request, submitted_token: str) -> bool:
    session = getattr(request.state, "session", None)
    if session is None:
        current_user(request)  # populate request.state.session
        session = getattr(request.state, "session", None)
    return bool(session) and bool(submitted_token) and submitted_token == session.csrf_token
