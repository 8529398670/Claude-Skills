"""Application entry point. This file only wires things together --
routing table, middleware stack, startup hook -- it holds no business
logic of its own. Run it with uvicorn (see Dockerfile / dockerRun.sh):

    uvicorn app.main:app --host 0.0.0.0 --port 8000
"""
from __future__ import annotations

from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.routing import Route

from app.core.db import init_db
from app.core.middleware import RateLimitMiddleware, SecurityHeadersMiddleware
from app.routes import account, admin, auth, pages, team

routes = [
    Route("/login/{token}", auth.redeem_login_link, methods=["GET"]),
    Route("/api/me", account.me, methods=["GET"]),
    Route("/api/account/rename", account.rename, methods=["POST"]),
    Route("/api/logout", account.logout, methods=["POST"]),
    Route("/api/team", team.list_team_members, methods=["GET"]),
    Route("/api/admin/users", admin.list_users, methods=["GET"]),
    Route("/api/admin/users", admin.create_user, methods=["POST"]),
    Route("/api/admin/users/{user_id}/reissue-login", admin.reissue_login, methods=["POST"]),
    Route("/api/admin/users/{user_id}/disabled", admin.set_disabled, methods=["POST"]),
    # Catch-all: everything else is a static asset (or falls back to
    # index.html for client-side routing). Must stay last.
    Route("/{path:path}", pages.static_page, methods=["GET"]),
]

middleware = [
    Middleware(SecurityHeadersMiddleware),
    Middleware(RateLimitMiddleware, max_requests=120, window_seconds=60),
]


def _startup() -> None:
    init_db()


app = Starlette(routes=routes, middleware=middleware, on_startup=[_startup])
