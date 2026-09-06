"""A non-admin-gated view of who else uses this app. Almost any real
feature built on top of this template ends up needing to reference other
users -- an owner picker, an assignee dropdown, an @mention list -- and
that must not require admin rights just to render. `admin.list_users`
stays admin-only because it exposes role/disabled status; this endpoint
exposes only id + display_name, which is safe for any logged-in user to
see.
"""
from __future__ import annotations

from starlette.requests import Request
from starlette.responses import JSONResponse

from app.core.security import current_user
from app.models import users


async def list_team_members(request: Request) -> JSONResponse:
    if current_user(request) is None:
        return JSONResponse({"error": "not authenticated"}, status_code=401)
    return JSONResponse(
        [
            {"id": u.id, "display_name": u.display_name}
            for u in users.list_users()
            if not u.disabled
        ]
    )
