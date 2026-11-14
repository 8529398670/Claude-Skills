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
from starlette.requests import Request  # feature:mcp|metrics|passwords|two_factor|invite_links|webhooks
from starlette.responses import Response  # feature:mcp|metrics|passwords|two_factor|invite_links|webhooks
from starlette.routing import BaseRoute, Route

from app.apidocs.route import mounted
from app.core.config import Config
from app.core.security import RequireAdmin, RequireLogin, RequireSession
from app.core.security import Guard  # feature:mcp|metrics|passwords|two_factor|invite_links|webhooks
from app.core.security import RequireOwner  # feature:logs|settings|jobs|webhooks|passwords
from app.core.security import RequireAPIKey  # feature:mcp|metrics
from app.core.static_files import StaticServer
from app.routes import (
    account,
    admin,
    api_keys,  # feature:api_keys
    auth,
    files,  # feature:files
    invite_links,  # feature:invite_links
    jobs,  # feature:jobs
    join,  # feature:invite_links
    logs,  # feature:logs
    mcp_manifest,  # feature:mcp
    meta,
    metrics,  # feature:metrics
    openapi,  # feature:openapi
    passwords,  # feature:passwords
    sessions,  # feature:sessions
    settings,  # feature:settings
    shortcuts,  # feature:shortcuts
    team,
    themes,  # feature:appearance
    two_factor,  # feature:two_factor
    webhooks,  # feature:webhooks
)
from app.routes.api_keys import RequireAPIKeys  # feature:api_keys
from app.routes.files import RequireFiles  # feature:files
from app.routes.helpers import get_config, not_found  # feature:mcp|metrics|passwords|two_factor|invite_links|webhooks


# >>> feature:mcp
class RequireMCP(Guard):
    """Makes /mcp answer 404 while server.mcp or auth.api_keys is off --
    exactly what an unregistered route would say. Checked per request rather
    than by registering /mcp or not, because both can be switched while the
    server runs."""

    def refuse(self, request: Request) -> Response | None:
        cfg = get_config(request)
        if not cfg.mcp_enabled or not cfg.api_keys_enabled:
            return not_found("not found")
        return None


# <<< feature:mcp
# >>> feature:metrics
class RequireMetrics(Guard):
    """Makes /metrics answer 404 while metrics.enabled or auth.api_keys is
    off, per request, for the same reason as RequireMCP."""

    def refuse(self, request: Request) -> Response | None:
        cfg = get_config(request)
        if not cfg.metrics_enabled or not cfg.api_keys_enabled:
            return not_found("not found")
        return None


# <<< feature:metrics
# >>> feature:passwords
class RequirePasswords(Guard):
    """Makes every password route answer 404 while auth.passwords is off.
    Per request, for the same reason as RequireMCP."""

    def refuse(self, request: Request) -> Response | None:
        return None if get_config(request).passwords_enabled else not_found("not found")


# <<< feature:passwords
# >>> feature:two_factor
class RequireTwoFactor(Guard):
    """The same for setting up 2FA while auth.two_factor is off. Finishing a
    sign-in is not behind this: one already under way may as well end."""

    def refuse(self, request: Request) -> Response | None:
        return None if get_config(request).two_factor_enabled else not_found("not found")


# <<< feature:two_factor
# >>> feature:invite_links
class RequireInviteLinks(Guard):
    """Makes every invite route answer 404 while auth.invite_links is off. Per
    request, for the same reason as RequireMCP. The /join page itself is
    StaticServer.serve_join_page, so a person gets the styled 404 instead."""

    def refuse(self, request: Request) -> Response | None:
        return None if get_config(request).invite_links_enabled else not_found("not found")


# <<< feature:invite_links
# >>> feature:webhooks
class RequireWebhooks(Guard):
    """The same for the Webhooks screen's routes while webhooks.enabled is off."""

    def refuse(self, request: Request) -> Response | None:
        return None if get_config(request).webhooks.enabled else not_found("not found")


# <<< feature:webhooks
def group(*guards: type) -> Callable[..., Route]:
    """A route builder whose routes all run behind the given guards, in
    order. Usage: signed_in = group(RequireLogin); signed_in("/api/me", me)."""
    middleware = [Middleware(guard) for guard in guards]

    def route(path: str, endpoint: Callable, methods: Sequence[str] = ("GET",)) -> Route:
        mounted(endpoint, methods[0], path)
        return Route(path, endpoint, methods=list(methods), middleware=middleware)

    return route


def build_routes(static_server: StaticServer, cfg: Config) -> list[BaseRoute]:
    """cfg is the configuration the server started with: it decides only
    what is set up once (the activity log's routes). Everything a request
    reads, it reads from the running configuration."""
    public = group()
    # >>> feature:invite_links
    # Joining through an invite link: holding the link is the permission.
    public_invites = group(RequireInviteLinks, auth.SameOriginJoin)
    # <<< feature:invite_links
    # Signing in: only as JSON from one of this site's own pages.
    sign_in = group(auth.SameOriginLogin)
    signed_in = group(RequireLogin)
    pin_sessions = group(RequireInviteLinks, RequireLogin, RequireSession)  # feature:invite_links
    # >>> feature:api_keys
    # API keys: any signed-in user manages their own, from a browser session.
    my_keys = group(RequireAPIKeys, RequireLogin, RequireSession)
    # <<< feature:api_keys
    # >>> feature:files
    # Uploaded files: everyone their own, API keys included. See files.py.
    my_files = group(RequireFiles, RequireLogin)
    # Every account's files. An admin key may list them, but reading or
    # deleting someone else's needs a session: a leaked key must not be a way
    # to carry off the contents of every account.
    admin_files = group(RequireFiles, RequireAdmin)
    admin_file_sessions = group(RequireFiles, RequireAdmin, RequireSession)
    # <<< feature:files
    # Admin only: deciding who gets in.
    admins = group(RequireAdmin)
    # An admin-scoped key may do all of it, creating accounts and handing out
    # login links included. Every link records the key that made it, and
    # revoking or deleting that key voids the links it made that can still be
    # used (models.void_links_minted_by_key), so revoking a leaked key still
    # ends what it could hand out. Who may delete or demote whom (the owner
    # rules) is enforced in the models.
    admin_invites = group(RequireInviteLinks, RequireAdmin)  # feature:invite_links
    # >>> feature:api_keys
    # Admin view of every key in the app -- "who has standing access" -- and
    # the ability to revoke or delete someone else's.
    admin_keys = group(RequireAPIKeys, RequireAdmin, RequireSession)
    # <<< feature:api_keys
    # >>> feature:logs|settings|jobs
    # The activity log and the Settings screen: the owner only, from a
    # browser session -- not other admins, not any API key, not the signed
    # out; all of them get 404 (see RequireOwner).
    owner = group(RequireOwner)
    # <<< feature:logs|settings|jobs
    owner_webhooks = group(RequireWebhooks, RequireOwner)  # feature:webhooks
    # >>> feature:passwords|two_factor
    # Passwords and 2FA change how an account signs in, so setting them needs
    # a browser session, like the PIN: a key that could set a password could
    # turn itself into a session.
    password_login = group(RequirePasswords, auth.SameOriginLogin)  # feature:passwords
    my_password = group(RequirePasswords, RequireLogin, RequireSession)  # feature:passwords
    owner_passwords = group(RequirePasswords, RequireOwner)  # feature:passwords
    my_two_factor = group(RequireTwoFactor, RequireLogin, RequireSession)  # feature:two_factor
    # <<< feature:passwords|two_factor

    routes: list[BaseRoute] = [
        # Public: the login link's page and the call it makes, the UI's text
        # (needed to render the signed-out page at all), and the healthcheck.
        # Opening a login link only shows a page; the link is spent by the
        # POST that page sends when the person confirms. See auth.py.
        public("/login/{credential:path}", static_server.serve_login_page),
        sign_in("/api/login", auth.redeem_login_link, ["POST"]),
        public("/api/login/options", passwords.login_options),  # feature:passwords
        sign_in("/api/login/two-factor", two_factor.finish_sign_in, ["POST"]),  # feature:two_factor
        password_login("/api/login/password", passwords.password_login, ["POST"]),  # feature:passwords
        public("/api/language", meta.get_language),
        public("/api/health", meta.get_health),
        public("/api/ready", meta.get_ready),
        # >>> feature:invite_links
        # Public: invite links (see join.py). Holding the link is the
        # permission; like a login link, the GET only serves the page.
        public("/join/{credential:path}", static_server.serve_join_page),
        public_invites("/api/join/info", join.join_info, ["POST"]),
        public_invites("/api/join/match", join.join_match, ["POST"]),
        public_invites("/api/join/claim", join.join_claim, ["POST"]),
        public_invites("/api/join/create", join.join_create, ["POST"]),
        # <<< feature:invite_links
        # Any signed-in user.
        signed_in("/api/me", account.me),
        signed_in("/api/account/rename", account.rename, ["POST"]),
        signed_in("/api/account/shortcuts", shortcuts.set_my_shortcuts, ["POST"]),  # feature:shortcuts
        signed_in("/api/account/themes", themes.set_my_themes, ["POST"]),  # feature:appearance
        signed_in("/api/account/theme", themes.set_my_theme, ["POST"]),  # feature:appearance
        # >>> feature:invite_links
        # A session only: a PIN lets its holder sign in through an invite
        # link, so a key that could set one could turn itself into a session.
        pin_sessions("/api/account/pin", account.set_pin, ["POST"]),
        # <<< feature:invite_links
        # A key may make its owner a login link, but only a key carrying the
        # account's full role (see account.create_my_login_link), and revoking
        # the key voids the link if it is still unused.
        signed_in("/api/account/login-link", account.create_my_login_link, ["POST"]),
        signed_in("/api/account/login-link/cancel", account.cancel_my_login_link, ["POST"]),
        # >>> feature:sessions
        # The account's own sessions. A key may list and end them too; through
        # a key there is no current device, so revoke-others ends them all.
        signed_in("/api/account/sessions", sessions.list_my_sessions),
        # <<< feature:sessions
        my_files("/api/account/storage", files.my_storage),  # feature:files
        signed_in("/api/account/sessions/revoke-others", sessions.revoke_others, ["POST"]),  # feature:sessions
        signed_in("/api/account/sessions/{session_id}/label", sessions.label_one, ["POST"]),  # feature:sessions
        signed_in("/api/account/sessions/{session_id}/revoke", sessions.revoke_one, ["POST"]),  # feature:sessions
        my_password("/api/account/password", passwords.set_password, ["POST"]),  # feature:passwords
        my_two_factor("/api/account/two-factor/start", two_factor.start_setup, ["POST"]),  # feature:two_factor
        my_two_factor("/api/account/two-factor/confirm", two_factor.confirm_setup, ["POST"]),  # feature:two_factor
        my_two_factor("/api/account/two-factor/disable", two_factor.disable, ["POST"]),  # feature:two_factor
        signed_in("/api/logout", account.logout, ["POST"]),
        signed_in("/api/team", team.list_team),
        signed_in("/api/openapi.json", openapi.get_openapi),  # feature:openapi
        group(RequireLogin, RequireMCP)("/api/mcp.json", mcp_manifest.get_mcp_manifest),  # feature:mcp
        my_keys("/api/keys", api_keys.list_my_keys),  # feature:api_keys
        my_keys("/api/keys", api_keys.create_my_key, ["POST"]),  # feature:api_keys
        my_keys("/api/keys/{key_id}/rename", api_keys.rename_my_key, ["POST"]),  # feature:api_keys
        my_keys("/api/keys/{key_id}/expiry", api_keys.set_my_key_expiry, ["POST"]),  # feature:api_keys
        my_keys("/api/keys/{key_id}/revoke", api_keys.revoke_my_key, ["POST"]),  # feature:api_keys
        my_keys("/api/keys/{key_id}/delete", api_keys.delete_my_key, ["POST"]),  # feature:api_keys
        my_keys("/api/keys/{key_id}/restore", api_keys.restore_my_key, ["POST"]),  # feature:api_keys
        my_files("/api/files", files.list_my_files),  # feature:files
        my_files("/api/files", files.create_file, ["POST"]),  # feature:files
        my_files("/api/files/{file_id}", files.get_my_file),  # feature:files
        my_files("/api/files/{file_id}/chunks/{index}", files.put_chunk, ["POST"]),  # feature:files
        my_files("/api/files/{file_id}/finish", files.finish_file, ["POST"]),  # feature:files
        my_files("/api/files/{file_id}/download", files.download_my_file),  # feature:files
        my_files("/api/files/{file_id}/rename", files.rename_my_file, ["POST"]),  # feature:files
        my_files("/api/files/{file_id}/delete", files.delete_my_file, ["POST"]),  # feature:files
        my_files("/api/files/{file_id}/restore", files.restore_my_file, ["POST"]),  # feature:files
        admin_files("/api/admin/files", files.list_all_files),  # feature:files
        admin_file_sessions("/api/admin/files/{file_id}/download", files.download_any_file),  # feature:files
        admin_file_sessions("/api/admin/files/{file_id}/rename", files.rename_any_file, ["POST"]),  # feature:files
        admin_file_sessions("/api/admin/files/{file_id}/delete", files.delete_any_file, ["POST"]),  # feature:files
        admin_file_sessions("/api/admin/files/{file_id}/restore", files.restore_any_file, ["POST"]),  # feature:files
        admins("/api/admin/users", admin.list_users),
        admins("/api/admin/users", admin.create_user, ["POST"]),
        admins("/api/admin/users/{user_id}/reissue-login", admin.reissue_login, ["POST"]),
        admins("/api/admin/users/{user_id}/cancel-login-link", admin.cancel_login_link, ["POST"]),
        admins("/api/admin/users/{user_id}/disabled", admin.set_user_disabled, ["POST"]),
        admins("/api/admin/users/{user_id}/revoke-access", admin.revoke_access, ["POST"]),
        admins("/api/admin/users/{user_id}/rename", admin.rename_user, ["POST"]),
        admins("/api/admin/users/{user_id}/role", admin.set_user_role, ["POST"]),
        admins("/api/admin/users/{user_id}/delete", admin.delete_user, ["POST"]),
        admins("/api/admin/users/{user_id}/reset-two-factor", admin.reset_two_factor, ["POST"]),  # feature:two_factor
        # >>> feature:sessions
        # An account's devices, like your own list above.
        admins("/api/admin/users/{user_id}/sessions", sessions.list_user_sessions),
        admins("/api/admin/users/{user_id}/sessions/{session_id}/revoke", sessions.revoke_user_session, ["POST"]),
        # <<< feature:sessions
        admin_files("/api/admin/storage", files.all_storage),  # feature:files
        # The themes everyone starts from. Not a credential, so a key may. # feature:appearance
        admins("/api/admin/theme-defaults", themes.get_theme_defaults),  # feature:appearance
        admins("/api/admin/theme-defaults", themes.set_theme_defaults, ["POST"]),  # feature:appearance
        # >>> feature:invite_links
        # Invite links: one link for a group, each person who opens it joins
        # as a new user (see invite_links.py). An admin key may manage them;
        # like a login link, one a key made is switched off when that key is
        # revoked.
        admin_invites("/api/admin/invite-links", invite_links.list_invite_links),
        admin_invites("/api/admin/invite-links", invite_links.create_invite_link, ["POST"]),
        admin_invites("/api/admin/invite-links/{link_id}/rename", invite_links.rename_invite_link, ["POST"]),
        admin_invites("/api/admin/invite-links/{link_id}/limits", invite_links.set_invite_link_limits, ["POST"]),
        admin_invites("/api/admin/invite-links/{link_id}/switch-on", invite_links.switch_on_invite_link, ["POST"]),
        admin_invites("/api/admin/invite-links/{link_id}/revoke", invite_links.revoke_invite_link, ["POST"]),
        admin_invites("/api/admin/invite-links/{link_id}/delete", invite_links.delete_invite_link, ["POST"]),
        admin_invites("/api/admin/invite-links/{link_id}/restore", invite_links.restore_invite_link, ["POST"]),
        # <<< feature:invite_links
        admin_keys("/api/admin/keys", api_keys.list_all_keys),  # feature:api_keys
        admin_keys("/api/admin/keys", api_keys.create_key_for_user, ["POST"]),  # feature:api_keys
        admin_keys("/api/admin/keys/{key_id}/rename", api_keys.rename_any_key, ["POST"]),  # feature:api_keys
        admin_keys("/api/admin/keys/{key_id}/expiry", api_keys.set_any_key_expiry, ["POST"]),  # feature:api_keys
        admin_keys("/api/admin/keys/{key_id}/revoke", api_keys.revoke_any_key, ["POST"]),  # feature:api_keys
        admin_keys("/api/admin/keys/{key_id}/delete", api_keys.delete_any_key, ["POST"]),  # feature:api_keys
        admin_keys("/api/admin/keys/{key_id}/restore", api_keys.restore_any_key, ["POST"]),  # feature:api_keys
        # >>> feature:settings
        # config.yaml, as the owner's Settings screen. See settings.py.
        owner("/api/owner/settings", settings.get_settings),
        owner("/api/owner/settings", settings.save_settings, ["POST"]),
        # <<< feature:settings
        # >>> feature:jobs
        # Background jobs. See app/core/jobs.py.
        owner("/api/owner/jobs", jobs.list_jobs),
        owner("/api/owner/jobs/{name}/run", jobs.run_job, ["POST"]),
        # <<< feature:jobs
        # >>> feature:webhooks
        # Webhooks. See app/core/webhooks.py.
        owner_webhooks("/api/owner/webhooks", webhooks.list_webhooks),
        owner_webhooks("/api/owner/webhooks", webhooks.create_webhook, ["POST"]),
        owner_webhooks("/api/owner/webhooks/events", webhooks.list_events),
        owner_webhooks("/api/owner/webhooks/{webhook_id}/enabled", webhooks.set_enabled, ["POST"]),
        owner_webhooks("/api/owner/webhooks/{webhook_id}/rename", webhooks.rename_webhook, ["POST"]),
        owner_webhooks("/api/owner/webhooks/{webhook_id}/target", webhooks.set_webhook_target, ["POST"]),
        owner_webhooks("/api/owner/webhooks/{webhook_id}/rotate-secret", webhooks.rotate_webhook_secret, ["POST"]),
        owner_webhooks("/api/owner/webhooks/{webhook_id}/delete", webhooks.delete_webhook, ["POST"]),
        owner_webhooks("/api/owner/webhooks/{webhook_id}/restore", webhooks.restore_webhook, ["POST"]),
        owner_webhooks("/api/owner/webhooks/{webhook_id}/test", webhooks.send_test, ["POST"]),
        owner_webhooks("/api/owner/webhooks/{webhook_id}/deliveries", webhooks.list_deliveries),
        owner_webhooks("/api/owner/webhooks/{webhook_id}/deliveries/{delivery_id}/resend", webhooks.resend_delivery, ["POST"]),
        # <<< feature:webhooks
        # >>> feature:passwords
        # The per-account password switch. See passwords.py.
        owner_passwords("/api/owner/users/{user_id}/password-access", passwords.set_password_access, ["POST"]),
        # <<< feature:passwords
    ]

    # >>> feature:logs
    # The activity log (app/core/logs.py). Switched off in config.yaml, none
    # of it exists and track.js sends nothing.
    if cfg.logs.enabled:
        routes += [
            # What someone did on the page, sent by track.js. Theirs to send,
            # the owner's to read.
            signed_in("/api/logs/events", logs.record_ui_events, ["POST"]),
            owner("/api/owner/logs/days", logs.list_log_days),
            owner("/api/owner/logs/days/{date}", logs.get_log_day),
            owner("/api/owner/logs/days/{date}/download", logs.download_log_day),
            owner("/api/owner/logs/days/{date}/delete", logs.delete_log_day, ["POST"]),
            owner("/api/owner/logs/prune", logs.prune_logs, ["POST"]),
            owner("/api/owner/logs/ip-names", logs.list_ip_names),
            owner("/api/owner/logs/ip-names", logs.set_ip_name, ["POST"]),
            owner("/api/owner/logs/ip-names/delete", logs.delete_ip_name, ["POST"]),
            owner("/api/owner/logs/stream", logs.stream_log_changes),
        ]
    # <<< feature:logs

    # >>> feature:mcp
    # MCP: agents calling the app with an API key (see app/mcp and
    # mcp_tools.py). Keys only -- RequireAPIKey turns a browser session away --
    # and every tool runs at the key's effective role, so this is the API above
    # in another shape, not a wider one.
    #
    # Always registered: server.mcp and auth.api_keys can be switched while
    # the server runs, so RequireMCP checks them per request and answers 404
    # when either is off -- without API keys nothing could authenticate to it.
    routes += [
        group(RequireMCP, RequireAPIKey)("/mcp", mcp_manifest.mcp_endpoint, ["POST"]),
        group(RequireMCP)("/mcp", mcp_manifest.mcp_no_stream),
        group(RequireMCP)("/mcp", mcp_manifest.mcp_no_sessions, ["DELETE"]),
    ]
    # <<< feature:mcp

    # >>> feature:metrics
    # Monitoring: an admin-level key, nothing else. RequireAPIKey before
    # RequireAdmin, so a signed-in browser is refused however high its role:
    # this is for a scraper, and a page has no reason to read it.
    routes.append(group(RequireMetrics, RequireAPIKey, RequireAdmin)("/metrics", metrics.get_metrics))
    # <<< feature:metrics

    return routes + [
        # Any other /api/ path is a JSON 404, not the index.html fallback.
        Route("/api/{rest:path}", meta.unknown_api),
        # Catch-all static serving. Must stay last -- it matches every
        # remaining path, so anything added after this line never runs.
        Route("/{path:path}", static_server.endpoint),
    ]
