"""The URL map -- the only place that knows every route this server answers.
Read build_routes top to bottom to see the whole surface.

The API stays deliberately small: it covers logging in, managing who can log
in, issuing the API keys that let non-browser callers in, and reading the UI's
text. Everything else an app does belongs in a new module in this package
(server side) and in static/js/ (browser side) -- see
references/architecture.md for where the line sits and why.

Protection comes from where a route is registered. group() attaches guards to
each route it builds, so a protected route cannot be written without its
guard, and the list below is a complete statement of who may call what.
Guards attach per route (Starlette's Route(middleware=...)), not per path
prefix, so a group never leaks onto routes outside it.
"""
from __future__ import annotations

from typing import Callable, Sequence

from starlette.middleware import Middleware
from starlette.routing import BaseRoute, Route

from app.core.security import RequireAdmin, RequireLogin, RequireSession
from app.core.static_files import StaticServer
from app.routes import account, admin, api_keys, auth, meta, team
from app.routes.api_keys import RequireAPIKeys


def group(*guards: type) -> Callable[..., Route]:
    """A route builder whose routes all run behind the given guards, in
    order. Usage: signed_in = group(RequireLogin); signed_in("/api/me", me)."""
    middleware = [Middleware(guard) for guard in guards]

    def route(path: str, endpoint: Callable, methods: Sequence[str] = ("GET",)) -> Route:
        return Route(path, endpoint, methods=list(methods), middleware=middleware)

    return route


def build_routes(static_server: StaticServer) -> list[BaseRoute]:
    public = group()
    signed_in = group(RequireLogin)
    # API keys: any signed-in user manages their own, from a browser session.
    my_keys = group(RequireAPIKeys, RequireLogin, RequireSession)
    # Admin only: deciding who gets in.
    admins = group(RequireAdmin)
    # The two admin routes that hand out a login link also require a browser
    # session. A login link becomes a session, and a session can mint API
    # keys -- so if an admin-scoped key could create accounts or reissue
    # links, a leaked key could turn itself into a fresh session and a fresh
    # key, and revoking the leaked one would not end the compromise. The rule
    # is: an API key may use what an account can do, but it never creates
    # credentials. See RequireSession.
    admin_sessions = group(RequireAdmin, RequireSession)
    # Admin view of every key in the app -- "who has standing access" -- and
    # the ability to revoke someone else's.
    admin_keys = group(RequireAPIKeys, RequireAdmin, RequireSession)

    return [
        # Public: the login link's page and the call it makes, the UI's text
        # (needed to render the signed-out page at all), and the healthcheck.
        # Opening a login link only shows a page; the link is spent by the
        # POST that page sends when the person confirms. See auth.py.
        public("/login/{credential:path}", static_server.serve_login_page),
        public("/api/login", auth.redeem_login_link, ["POST"]),
        public("/api/language", meta.get_language),
        public("/api/health", meta.get_health),
        # Any signed-in user.
        signed_in("/api/me", account.me),
        signed_in("/api/account/rename", account.rename, ["POST"]),
        signed_in("/api/logout", account.logout, ["POST"]),
        signed_in("/api/team", team.list_team),
        my_keys("/api/keys", api_keys.list_my_keys),
        my_keys("/api/keys", api_keys.create_my_key, ["POST"]),
        my_keys("/api/keys/{key_id}/revoke", api_keys.revoke_my_key, ["POST"]),
        admins("/api/admin/users", admin.list_users),
        admin_sessions("/api/admin/users", admin.create_user, ["POST"]),
        admin_sessions("/api/admin/users/{user_id}/reissue-login", admin.reissue_login, ["POST"]),
        admins("/api/admin/users/{user_id}/disabled", admin.set_user_disabled, ["POST"]),
        admins("/api/admin/users/{user_id}/revoke-access", admin.revoke_access, ["POST"]),
        admin_keys("/api/admin/keys", api_keys.list_all_keys),
        admin_keys("/api/admin/keys/{key_id}/revoke", api_keys.revoke_any_key, ["POST"]),
        # Any other /api/ path is a JSON 404, not the index.html fallback.
        Route("/api/{rest:path}", meta.unknown_api),
        # Catch-all static serving. Must stay last -- it matches every
        # remaining path, so anything added after this line never runs.
        Route("/{path:path}", static_server.endpoint),
    ]
