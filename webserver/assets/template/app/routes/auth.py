"""The entire login flow. There is no username/password form anywhere --
visiting a valid one-time link IS the login. Everything else (who you are,
what you can do) is decided by the session cookie this sets.
"""
from __future__ import annotations

from starlette.requests import Request
from starlette.responses import RedirectResponse

from app.core.security import set_session_cookie
from app.models import login_tokens, sessions


async def redeem_login_link(request: Request) -> RedirectResponse:
    token = request.path_params["token"]
    user_id = login_tokens.redeem_login_token(token)
    if user_id is None:
        # Expired, already used, or never existed -- don't leak which.
        return RedirectResponse("/?login=invalid", status_code=303)

    raw_cookie_value, _session = sessions.create_session(user_id)
    response = RedirectResponse("/", status_code=303)
    set_session_cookie(response, raw_cookie_value)
    return response
