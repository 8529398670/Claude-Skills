"""Self-service endpoints for any signed-in caller: who am I, rename myself,
a login link for another device, log out. The account screen itself is static/js/app.js, calling these."""
from __future__ import annotations

from typing import Annotated, ClassVar, Literal

from msgspec import UNSET, Meta, UnsetType
from starlette.requests import Request

from app.apidocs import SESSION_ONLY, Reply, route
from app.apidocs import Request as RequestType
from app.apidocs.route import reply_response
from app.core.config import Config
from app.core.config import repo_file  # feature:openapi|mcp
from app.core.encryption import BadKeyError, CipherTextError
from app.core.logs import KIND_AUTH
from app.core.security import actor_from, clear_session_cookie, decrypt_cookie
from app.models import login_tokens, sessions, users
from app.models import passwords  # feature:passwords
from app.models import pins  # feature:invite_links
from app.models import themes  # feature:appearance
from app.models import two_factor  # feature:two_factor
from app.routes.admin import CancelLoginLinkReply, DisplayName, ExpiresInMinutes, LoginLink, LoginLinkReply, MaxUses, OkReply, RenameUserReply, link_options, link_reply
from app.routes.helpers import Refused, get_config, get_store, run
from app.routes.logs import audit
from app.routes.themes import Theme, theme_list  # feature:appearance


class Me(Reply, kw_only=True):
    """Who is asking, and what the page needs to know before it draws."""

    api_name: ClassVar[str] = "Me"
    authenticated: bool
    id: int
    display_name: str
    role: Annotated[str, Meta(description="The effective role: never higher than an API key's own.")]
    account_role: str
    is_owner: bool
    login_link: Annotated[LoginLink | None, Meta(description="Your own live login link, or null.")]
    maintenance: Literal["none", "read_only", "full"]  # feature:maintenance
    version: Annotated[str, Meta(description="The running build. dev when none was stamped.")]
    announcement: Annotated[str, Meta(description="server.announcement. Empty when there is none.")]
    repo: Annotated[str, Meta(description="app.repo without a trailing / or .git: where the source lives. Empty when it is not set.")]
    description: Annotated[str, Meta(description="app.description. Empty when there is none.")]
    show_about: Annotated[bool, Meta(description="app.show_about: whether the menu has About.")]  # feature:about
    openapi_url: Annotated[str, Meta(description="Where to read this API's reference: config/openapi.json in app.repo, or /api/openapi.json when there is no repo.")]  # feature:openapi
    mcp_url: Annotated[str, Meta(description="Where to read the MCP reference: config/mcp.json in app.repo, or /api/mcp.json when there is no repo. Only with server.mcp on.")]  # feature:mcp
    shortcuts: Annotated[dict[str, str], Meta(description="The keyboard shortcuts you changed, by action. Mod is Cmd on a Mac and Ctrl elsewhere.")]  # feature:shortcuts
    themes: Annotated[list[Theme] | None, Meta(description="Your themes (see the Theme schema): your own list, else the app's defaults, else null for the built-in ones.")]  # feature:appearance
    themes_custom: Annotated[bool, Meta(description="Whether you have your own list rather than following the defaults.")]  # feature:appearance
    theme_id: Annotated[str, Meta(description="The theme you use. Empty: the first.")]  # feature:appearance
    has_pin: bool  # feature:invite_links
    username: str  # feature:passwords
    has_password: bool  # feature:passwords
    password_blocked: bool  # feature:passwords
    two_factor: bool  # feature:two_factor
    passwords_enabled: bool  # feature:passwords
    two_factor_enabled: bool  # feature:two_factor
    auth: str
    api_keys_enabled: bool  # feature:api_keys
    mcp_enabled: Annotated[bool, Meta(description="server.mcp: whether POST /mcp answers.")]  # feature:mcp
    files_enabled: Annotated[bool, Meta(description="files.enabled: whether the Files screen and routes are on.")]  # feature:files
    invite_links_enabled: Annotated[bool, Meta(description="Whether invite links are on.")]  # feature:invite_links
    webhooks_enabled: Annotated[bool, Meta(description="webhooks.enabled: whether webhooks are sent.")]  # feature:webhooks
    logs_enabled: bool  # feature:logs
    logs_ui_clicks: bool  # feature:logs
    csrf_token: Annotated[str, Meta(description="Browser sessions only.")] | UnsetType = UNSET
    api_key_id: Annotated[str, Meta(description="API keys only.")] | UnsetType = UNSET  # feature:api_keys
    api_key_name: Annotated[str, Meta(description="API keys only.")] | UnsetType = UNSET  # feature:api_keys


@route(summary="Who am I", tag="Account", errors=(401,))
def me(request: Request) -> Me:
    """The frontend's first call on every page load: it answers "who am I,
    what may I do, and what CSRF token do I echo back". Handing the token out
    here means no page needs a hidden form field or a template variable. The
    MCP whoami tool answers the same, so the two cannot describe the same
    caller differently.

    "role" is the *effective* role, not the account's: a request made with a
    user-scoped API key belonging to an admin reports "user", because that is
    what it may actually do. account_role reports the account itself, for the
    rare caller that needs to tell the two apart. Reporting the account's role
    as "role" would have a client believe it has permissions the server will
    refuse to act on."""
    store, cfg, actor = get_store(request), get_config(request), actor_from(request)
    # is_owner tells the UI which account-management buttons to offer; the
    # server checks ownership again on every request that depends on it.
    owner_id = users.get_owner_id(store)
    my_themes, themes_custom = themes.mine(store, actor.user.themes)  # feature:appearance
    link = login_tokens.live_login_link(store, actor.user.id)
    reply = Me(
        authenticated=True,
        id=actor.user.id,
        display_name=actor.user.display_name,
        role=actor.role,
        account_role=actor.user.role,
        is_owner=actor.user.id == owner_id,
        login_link=LoginLink(**link) if link is not None else None,
        maintenance=cfg.maintenance,  # feature:maintenance
        version=request.app.state.version,
        announcement=cfg.announcement,
        repo=cfg.repo,
        description=cfg.description,
        show_about=cfg.show_about,  # feature:about
        openapi_url=open_api_url(cfg),  # feature:openapi
        mcp_url=mcp_url(cfg),  # feature:mcp
        shortcuts=actor.user.shortcuts,  # feature:shortcuts
        themes=theme_list(my_themes),  # feature:appearance
        themes_custom=themes_custom,  # feature:appearance
        theme_id=actor.user.theme_id,  # feature:appearance
        has_pin=pins.has_pin(actor.user),  # feature:invite_links
        username=actor.user.username,  # feature:passwords
        has_password=passwords.has_password(actor.user),  # feature:passwords
        password_blocked=actor.user.password_blocked,  # feature:passwords
        two_factor=two_factor.is_on(actor.user),  # feature:two_factor
        passwords_enabled=cfg.passwords_enabled,  # feature:passwords
        two_factor_enabled=cfg.two_factor_enabled,  # feature:two_factor
        auth=actor.method,
        api_keys_enabled=cfg.api_keys_enabled,  # feature:api_keys
        mcp_enabled=cfg.mcp_enabled,  # feature:mcp
        files_enabled=cfg.files.enabled,  # feature:files
        invite_links_enabled=cfg.invite_links_enabled,  # feature:invite_links
        webhooks_enabled=cfg.webhooks.enabled,  # feature:webhooks
        # >>> feature:logs
        # Whether track.js sends anything, and whether it sends clicks.
        logs_enabled=cfg.logs.enabled,
        logs_ui_clicks=cfg.logs.enabled and cfg.logs.ui_clicks,
        # <<< feature:logs
    )
    # Only a session has a CSRF token. A key-authenticated request has no
    # ambient credential to protect, so there is nothing to hand back -- see
    # check_csrf.
    if actor.session is not None:
        reply.csrf_token = actor.session.csrf_token
    # >>> feature:api_keys
    if actor.api_key is not None:
        reply.api_key_id = actor.api_key.id
        reply.api_key_name = actor.api_key.name
    # <<< feature:api_keys
    return reply


# >>> feature:openapi
def open_api_url(cfg: Config) -> str:
    """Where the API reference is read: config/openapi.json in app.repo when
    that is set, so it opens as a page there, else this server's own copy,
    which needs the same sign-in."""
    return repo_file(cfg, "config/openapi.json") or "/api/openapi.json"


# <<< feature:openapi
# >>> feature:mcp
def mcp_url(cfg: Config) -> str:
    """Where the MCP reference is read: config/mcp.json in app.repo when that
    is set, else this server's own copy, which needs the same sign-in."""
    return repo_file(cfg, "config/mcp.json") or "/api/mcp.json"


# <<< feature:mcp
class Rename(RequestType):
    display_name: DisplayName
    expect: str | None = None


@route(summary="Change your display name", tag="Account", errors=(400, 401, 403, 409))
def rename(request: Request, body: Rename) -> RenameUserReply:
    """Rename yourself, over HTTP or the MCP rename_account tool. expect is
    the name the caller expects to replace: the frontend always sends it, so
    its undo cannot overwrite a rename made elsewhere."""
    if not users.valid_display_name(body.display_name):
        raise Refused(400, "display name must be 1-80 characters")
    try:
        renamed = users.rename_user(get_store(request), actor_from(request).user.id, body.display_name, body.expect)
    except users.RenameConflictError as refusal:
        raise Refused(409, str(refusal), display_name=refusal.current) from None
    if not renamed:
        raise Refused.internal("rename: the signed-in account has no record")
    name = body.display_name.strip()
    audit(request, KIND_AUTH, "renamed own account", {"name": name})
    return RenameUserReply(ok=True, display_name=name)


# >>> feature:invite_links
class SetPIN(RequestType):
    pin: Annotated[str, Meta(extra={"error": "bad_pin"})]


class SetPINReply(Reply):
    ok: bool
    has_pin: bool


@route(summary="Set or clear your PIN", tag="Account", access=SESSION_ONLY, feature="invite_links", errors=(400, 401, 403))
def set_pin(request: Request, body: SetPIN) -> SetPINReply:
    """Set, or with an empty pin clear, the PIN asked for when this account
    signs back in through an invite link (see app/models/pins.py)."""
    try:
        if not pins.set_user_pin(get_store(request), actor_from(request).user.id, body.pin):
            raise Refused.internal("set pin: the signed-in account has no record")
    except pins.BadPinError:
        raise Refused(400, "bad_pin") from None
    audit(request, KIND_AUTH, "pin changed", {"has_pin": body.pin != ""})
    return SetPINReply(ok=True, has_pin=body.pin != "")


# <<< feature:invite_links
class MyLoginLink(RequestType):
    max_uses: MaxUses | None = None
    expires_in_minutes: ExpiresInMinutes | None = None


@route(summary="A login link for yourself", tag="Account", errors=(400, 401, 403))
def create_my_login_link(request: Request, body: MyLoginLink) -> LoginLinkReply:
    """Mint a login link for the caller's own account: the way to sign in on
    another device without asking an admin. Once by default, or reusable,
    like admin.reissue_login -- it is reissue_login pointed at yourself, so it
    has the same effect: any link the account could still use stops working,
    and existing sessions stay signed in.

    An API key may ask too, but only one that carries the account's full
    role: the link becomes a session, and a session acts with the account's
    role, so a key scoped down to "user" on an admin's account would otherwise
    climb. Revoking the key voids the link while it can still be used."""
    actor = actor_from(request)
    if actor.via_api_key and actor.role != actor.user.role:
        raise Refused(403, "this key acts with less than your account's role, so it cannot make a login link")
    options = link_options(request, body)
    credential, info = login_tokens.issue_login_link(get_store(request), actor.user.id, options)
    audit(request, KIND_AUTH, "own login link created", {"max_uses": info["max_uses"], "expires_at": info["expires_at"]})
    return LoginLinkReply(**link_reply(credential, info))


@route(summary="Stop your own login link working", tag="Account", errors=(400, 401, 403))
def cancel_my_login_link(request: Request) -> CancelLoginLinkReply:
    """Stop the caller's own live login link working."""
    removed = login_tokens.cancel_login_link(get_store(request), actor_from(request).user.id)
    audit(request, KIND_AUTH, "own login link cancelled", {"removed": removed})
    return CancelLoginLinkReply(ok=True, removed=removed)


@route(summary="Sign out", tag="Account", mcp_http="Ends a browser session; a key has none to end.", errors=(400, 401, 403))
async def logout(request: Request) -> OkReply:
    """Deletes the session server-side as well as clearing the cookie, so a
    copy of the cookie captured earlier is dead too.

    It is held to the CSRF token like every other state change. SameSite=
    Strict already stops another site from sending the cookie here, but not a
    page on a sibling subdomain, and being signed out by someone else's page
    is the first step of tricking a person into signing in to the wrong
    account.

    Called with an API key it does nothing and says so by succeeding: there is
    no session to destroy. A key is ended by revoking it, not by logging out."""
    audit(request, KIND_AUTH, "signed out")
    await destroy_cookie_session(request)
    response = reply_response(OkReply(ok=True))
    clear_session_cookie(response, get_config(request))
    return response  # type: ignore[return-value]


async def destroy_cookie_session(request: Request) -> None:
    """Delete the session named by this request's cookie, if any. Deleting
    the record rather than only the cookie is what matters: a copy of the
    cookie taken earlier is then dead too."""
    cfg = get_config(request)
    raw = request.cookies.get(cfg.cookie_name, "")
    if not raw:
        return
    try:
        credential = decrypt_cookie(cfg, raw)
    except (CipherTextError, BadKeyError):
        return
    await run(sessions.destroy_session, get_store(request), credential)
