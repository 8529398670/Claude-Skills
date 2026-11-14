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

from dataclasses import replace
from typing import Annotated, Any, ClassVar, Literal

import msgspec
from msgspec import Meta
from starlette.requests import Request

from app.apidocs import ADMIN, PATH, QUERY, Reply, route
from app.apidocs import Request as RequestType
from app.core.db import Store
from app.core.logs import KIND_ADMIN
from app.core.security import actor_from
from app.models import credentials, from_iso, login_tokens, to_iso, users, utc_now
from app.models import passwords  # feature:passwords
from app.models import two_factor  # feature:two_factor
from app.models import webhooks  # feature:webhooks
from app.routes.helpers import DateTime, Dir, Page, Paged, Refused, act_on_refusals, get_config, get_store, sorted_page
from app.routes.logs import audit

# The two things every route that mints a login link may ask for, both
# optional (see login_tokens.resolve_link).
LINK_LIMITS = "max_uses must be 0 (unlimited) or 1-1000, expires_in_minutes 0 (never) or 1-525600"
MaxUses = Annotated[int, Meta(ge=0, le=1000, description="Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1.", extra={"error": LINK_LIMITS})]
ExpiresInMinutes = Annotated[int, Meta(ge=0, le=525600, description="0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise.", extra={"error": LINK_LIMITS})]
DisplayName = Annotated[str, Meta(min_length=1, max_length=80, extra={"error": "display name must be 1-80 characters"})]
Role = Annotated[Literal["admin", "user"], Meta(extra={"error": "role must be admin or user"})]


class UserIDPath(RequestType):
    """The account a route under /api/admin/users/{user_id} acts on."""

    user_id: Annotated[int, PATH, Meta(ge=1, description="The account's id, from GET /api/admin/users (admin_list_users over MCP).", extra={"error": "invalid user id"})]


class LoginLink(Reply):
    """A login link as the UI shows it."""

    api_name: ClassVar[str] = "LoginLink"
    api_doc: ClassVar[str] = "A login link that can still be used. One per account at most."
    max_uses: Annotated[int, Meta(description="Sign-ins it allows. 0 when unlimited.")]
    unlimited: bool
    uses: Annotated[int, Meta(description="Sign-ins so far.")]
    created_at: DateTime
    expires_at: Annotated[DateTime | None, Meta(description="null when it never expires.")]
    last_used_at: DateTime | None


class Revoked(Reply):
    """What revoking an account's access, or deleting it, took away."""

    api_name: ClassVar[str] = "Revoked"
    sessions: int
    login_links: int
    api_keys: int  # feature:api_keys
    passwords: int  # feature:passwords


class UserRow(Reply):
    """An account as the admin screen lists it. The owner is marked on its
    row, so the UI can tell which delete, disable, and revoke buttons the
    server would refuse."""

    api_name: ClassVar[str] = "User"
    id: int
    display_name: str
    username: Annotated[str, Meta(description="Empty until a password is set.")]  # feature:passwords
    role: Literal["admin", "user"]
    created_at: DateTime
    disabled: bool
    owner: bool
    has_password: bool  # feature:passwords
    password_blocked: bool  # feature:passwords
    two_factor: bool  # feature:two_factor
    login_link: Annotated[LoginLink | None, Meta(description="The account's live login link, or null.")]


class ListUsers(Page):
    q: Annotated[str, QUERY, Meta(description="Keep accounts whose name or username contains this, in any case.")] = ""
    sort: Annotated[Literal["display_name", "role", "disabled", "created_at"], QUERY, Meta(description="Field to sort the whole list by. Rows without it go last.")] = ""  # type: ignore[assignment]
    dir: Dir = ""  # type: ignore[assignment]


class ListUsersReply(Paged):
    users: list[UserRow]


@route(summary="Every account", tag="Admin", role=ADMIN, errors=(400, 401, 403))
def list_users(request: Request, body: ListUsers) -> ListUsersReply:
    """Every account, a page at a time. ?q= keeps the accounts whose name or
    username contains it, in any case. Here and not in the browser, which only
    ever holds one page of them."""
    page = sorted_page(user_rows(get_store(request), body.q), body.sort, body.dir, body)
    return ListUsersReply(users=page.pop("rows"), **page)


def user_rows(store: Store, query: str = "") -> list[UserRow]:
    owner_id = users.get_owner_id(store)
    links = login_tokens.live_login_links(store)
    wanted = query.strip().lower()
    return [
        UserRow(
            id=u.id,
            display_name=u.display_name,
            username=u.username or "",  # feature:passwords
            role=u.role,
            created_at=to_iso(u.created_at),
            disabled=u.disabled,
            owner=u.id == owner_id,
            has_password=passwords.has_password(u),  # feature:passwords
            password_blocked=u.password_blocked,  # feature:passwords
            two_factor=two_factor.is_on(u),  # feature:two_factor
            login_link=msgspec.convert(links[u.id], LoginLink) if u.id in links else None,
        )
        for u in users.list_users(store)
        if wanted in u.display_name.lower() or wanted in (u.username or "").lower()  # feature:passwords
        if wanted in u.display_name.lower()  # feature:!passwords
    ]


def link_options(request: Request, body: Any) -> login_tokens.LinkOptions:
    """What the request asked a new login link to allow, with the configured
    defaults filling the gaps (see login_tokens.resolve_link), and the API key
    asking, if one is, so revoking that key voids the link. body is a dict or
    a request type with max_uses and expires_in_minutes."""
    cfg = get_config(request)
    asked = body if isinstance(body, dict) else {"max_uses": body.max_uses, "expires_in_minutes": body.expires_in_minutes}
    try:
        options = login_tokens.resolve_link(
            asked.get("max_uses"), asked.get("expires_in_minutes"),
            cfg.login_token_ttl_seconds, cfg.multi_use_login_token_ttl_seconds,
        )
    except login_tokens.BadLoginLinkError as refusal:
        raise Refused(400, str(refusal)) from None
    return replace(options, minted_by_key=actor_from(request).key_id)


class LoginLinkReply(Reply):
    """What every route that mints a link answers. The link is a path, not a
    full URL (see create_user); the MCP tools add login_url from the address
    the agent reached /mcp at."""

    login_path: Annotated[str, Meta(description="Path of the login link. A credential: treat it like a password.")]
    expires_in_seconds: Annotated[int, Meta(description="0 when it never expires.")]
    expires_at: Annotated[DateTime | None, Meta(description="null when it never expires.")]
    max_uses: Annotated[int, Meta(description="0 when unlimited.")]
    unlimited: bool


def link_reply(credential: str, info: dict) -> dict[str, Any]:
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


class CreateUser(RequestType):
    display_name: DisplayName
    role: Role
    max_uses: MaxUses | None = None
    expires_in_minutes: ExpiresInMinutes | None = None


class CreateUserReply(LoginLinkReply):
    user_id: int


@route(summary="Create an account and its login link", tag="Admin", role=ADMIN, status=201, errors=(400, 401, 403))
def create_user(request: Request, body: CreateUser) -> CreateUserReply:
    """The signup process: an admin names someone, and gets back a login link
    to hand them -- once by default, or reusable (see
    login_tokens.resolve_link). The link is returned as a path, not a full URL -- the
    browser knows the origin it is talking to, whereas the server behind a
    reverse proxy often does not, and a login link with the wrong host is
    worse than useless."""
    if not users.valid_display_name(body.display_name):
        raise Refused(400, "display name must be 1-80 characters")
    options = link_options(request, body)
    store = get_store(request)
    user = users.create_user(store, body.display_name, body.role)
    credential, info = login_tokens.issue_login_link(store, user.id, options)
    audit(request, KIND_ADMIN, "user created", {"user_id": user.id, "name": user.display_name, "role": user.role})
    request.app.state.webhooks.emit(webhooks.EVENT_USER_CREATED, {"user_id": user.id, "display_name": user.display_name, "role": user.role})  # feature:webhooks
    return CreateUserReply(user_id=user.id, **link_reply(credential, info))


class ReissueLogin(UserIDPath):
    max_uses: MaxUses | None = None
    expires_in_minutes: ExpiresInMinutes | None = None


@route(summary="A new login link for an account", tag="Admin", role=ADMIN, errors=(400, 401, 403, 404))
def reissue_login(request: Request, body: ReissueLogin) -> LoginLinkReply:
    """Covers the only real failure mode of this auth model: someone lost
    their cookie or their device, and there is no password reset to fall back
    on. An admin mints a fresh link instead. Any link the account could still
    sign in with stops working (see login_tokens.issue_login_link).

    It is not a reset: sessions the account already has stay signed in. When
    the worry is that someone else holds the account's credentials, use
    revoke_access first, then reissue."""
    store = get_store(request)
    if users.get_user(store, body.user_id) is None:
        raise Refused(404, "no such user")
    options = replace(link_options(request, body), actor_id=actor_from(request).user.id)
    with act_on_refusals("reissue a login link for"):
        credential, info = login_tokens.issue_login_link(store, body.user_id, options)
    audit(request, KIND_ADMIN, "login link reissued", {"user_id": body.user_id, "max_uses": info["max_uses"], "expires_at": info["expires_at"]})
    return LoginLinkReply(**link_reply(credential, info))


class CancelLoginLinkReply(Reply):
    ok: bool
    removed: Annotated[int, Meta(description="Links removed: 0 or 1.")]


@route(summary="Stop an account's login link working", tag="Admin", role=ADMIN, errors=(400, 401, 403, 404))
def cancel_login_link(request: Request, body: UserIDPath) -> CancelLoginLinkReply:
    """Stop an account's live login link working, without making a new one:
    a reusable link that went somewhere it should not have. Sessions it
    already made stay signed in; revoke_access ends those too.

    An API key may do this, like revoke_access: it only takes a way in away."""
    store = get_store(request)
    if users.get_user(store, body.user_id) is None:
        raise Refused(404, "no such user")
    with act_on_refusals("cancel the login link of"):
        removed = login_tokens.cancel_login_link(store, body.user_id, actor_from(request).user.id)
    audit(request, KIND_ADMIN, "login link cancelled", {"user_id": body.user_id, "removed": removed})
    return CancelLoginLinkReply(ok=True, removed=removed)


class OkReply(Reply):
    """The answer of a route that has nothing to say but yes."""

    ok: bool


class SetUserDisabled(UserIDPath):
    disabled: Annotated[bool, Meta(extra={"error": "disabled must be true or false"})]


@route(summary="Disable or enable an account", tag="Admin", role=ADMIN, errors=(400, 401, 403, 404))
def set_user_disabled(request: Request, body: SetUserDisabled) -> OkReply:
    """Switch an account off, or back on, over HTTP and as the MCP
    admin_set_user_disabled tool."""
    store, self_id = get_store(request), actor_from(request).user.id
    if users.get_user(store, body.user_id) is None:
        raise Refused(404, "no such user")
    # Locking yourself out is unrecoverable without shell access, so it is
    # worth one guard rail even though an admin could still disable a peer.
    if body.disabled and body.user_id == self_id:
        raise Refused(400, "you cannot disable your own account")
    # Disabling also deletes the account's sessions and live login links,
    # in the same transaction -- see credentials.set_user_disabled. The owner
    # cannot be disabled, and only the owner switches another admin off or
    # on; those checks are in the model so every route gets them.
    try:
        found = credentials.set_user_disabled(store, body.user_id, body.disabled, actor_id=self_id)
    except credentials.OwnerProtectedError:
        raise Refused(403, "the owner account cannot be disabled") from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can disable or enable an admin account") from None
    if not found:
        raise Refused(404, "no such user")
    audit(request, KIND_ADMIN, "user disabled set", {"user_id": body.user_id, "disabled": body.disabled})
    request.app.state.webhooks.emit(webhooks.EVENT_USER_DISABLED, {"user_id": body.user_id, "disabled": body.disabled})  # feature:webhooks
    return OkReply(ok=True)


class RevokeAccessReply(Reply):
    ok: bool
    revoked: Revoked


@route(summary="End every credential an account holds", tag="Admin", role=ADMIN, errors=(400, 401, 403, 404))
def revoke_access(request: Request, body: UserIDPath) -> RevokeAccessReply:
    """The response to "this account may be compromised": every session,
    live login link, and live API key it holds stops working at once. The
    account itself stays enabled; reissue a login link to let its owner back
    in. See credentials.revoke_all_credentials for why this is separate from
    both disabling and reissuing. Over HTTP and as the MCP
    admin_revoke_access tool."""
    self_id = actor_from(request).user.id
    # Same guard rail as disabling: revoking your own access signs you out
    # with no link to get back in.
    if body.user_id == self_id:
        raise Refused(400, "you cannot revoke your own access")
    # The owner is refused here (allow_owner False), and only the owner
    # revokes another admin. Only an operator with shell access may cut the
    # owner off -- see revoke_all_credentials.
    try:
        revoked = credentials.revoke_all_credentials(get_store(request), body.user_id, actor_id=self_id)
    except credentials.OwnerProtectedError:
        raise Refused(403, "the owner account's access cannot be revoked") from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can revoke the access of an admin account") from None
    if revoked is None:
        raise Refused(404, "no such user")
    audit(request, KIND_ADMIN, "access revoked", {"user_id": body.user_id})
    return RevokeAccessReply(ok=True, revoked=msgspec.convert(revoked.to_dict(), Revoked))


class RenameUser(UserIDPath):
    display_name: Annotated[str, Meta(min_length=1, max_length=80, description="1-80 characters.", extra={"error": "display name must be 1-80 characters"})]
    expect: Annotated[str | None, Meta(description="The name you think they have. Answers 409 if it is another.")] = None


class RenameUserReply(Reply):
    ok: bool
    display_name: str


@route(summary="Rename someone else", description="Only the owner can rename an admin.", tag="Admin", role=ADMIN, errors=(400, 401, 403, 404, 409))
def rename_user(request: Request, body: RenameUser) -> RenameUserReply:
    """An admin renaming someone else. Who may rename whom lives in
    credentials.rename_user_as."""
    if not users.valid_display_name(body.display_name):
        raise Refused(400, "display name must be 1-80 characters")
    try:
        found = credentials.rename_user_as(get_store(request), body.user_id, actor_from(request).user.id, body.display_name, body.expect)
    except users.RenameConflictError as stale:
        raise Refused(409, str(stale), display_name=stale.current) from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can rename an admin") from None
    if not found:
        raise Refused(404, "no such user")
    name = body.display_name.strip()
    audit(request, KIND_ADMIN, "user renamed", {"user_id": body.user_id, "name": name})
    return RenameUserReply(ok=True, display_name=name)


class SetUserRole(UserIDPath):
    role: Role


class SetUserRoleReply(Reply):
    ok: bool
    role: str


@route(
    summary="Change an account's role",
    description="Only the owner can take an admin's rights away. Becoming an admin ends the account's sessions and live login link.",
    tag="Admin", role=ADMIN, errors=(400, 401, 403, 404),
)
def set_user_role(request: Request, body: SetUserRole) -> SetUserRoleReply:
    """Make an account an admin or an ordinary user. Who may do what lives in
    credentials.set_user_role."""
    try:
        changed = credentials.set_user_role(get_store(request), body.user_id, actor_from(request).user.id, body.role)
    except credentials.SelfRoleError as refusal:
        raise Refused(400, str(refusal)) from None
    except credentials.OwnerProtectedError:
        raise Refused(403, "the owner is always an admin") from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can take away an admin's rights") from None
    if changed is None:
        raise Refused(404, "no such user")
    if changed:
        audit(request, KIND_ADMIN, "user role changed", {"user_id": body.user_id, "role": body.role})
    return SetUserRoleReply(ok=True, role=body.role)


class DeleteUserReply(Reply):
    ok: bool
    removed: Revoked


@route(summary="Delete an account", tag="Admin", role=ADMIN, errors=(400, 401, 403, 404))
def delete_user(request: Request, body: UserIDPath) -> DeleteUserReply:
    """Remove an account for good, together with every session, login link,
    and API key it held. Disabling is the reversible version; this is for an
    account that should not exist at all.

    Who may delete whom lives in credentials.delete_user, checked inside its
    transaction. An admin-scoped API key may do it too, and the MCP tool is
    marked destructive so a client asks first."""
    try:
        removed = credentials.delete_user(get_store(request), body.user_id, actor_from(request).user.id)
    except credentials.SelfDeleteError:
        raise Refused(400, "you cannot delete your own account") from None
    except credentials.OwnerProtectedError:
        raise Refused(403, "the owner account cannot be deleted") from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can delete an admin account") from None
    if removed is None:
        raise Refused(404, "no such user")
    audit(request, KIND_ADMIN, "user deleted", {"user_id": body.user_id})
    request.app.state.webhooks.emit(webhooks.EVENT_USER_DELETED, {"user_id": body.user_id})  # feature:webhooks
    return DeleteUserReply(ok=True, removed=msgspec.convert(removed.to_dict(), Revoked))


# >>> feature:two_factor
@route(summary="Turn off an account's two-factor", tag="Admin", role=ADMIN, feature="two_factor", errors=(400, 401, 403, 404))
def reset_two_factor(request: Request, body: UserIDPath) -> OkReply:
    """For someone who lost both the phone and the recovery codes. Who may
    reset whom lives in credentials.reset_two_factor."""
    try:
        found = credentials.reset_two_factor(get_store(request), body.user_id, actor_from(request).user.id)
    except credentials.SelfResetError:
        raise Refused(400, "you cannot reset your own two-factor sign-in") from None
    except credentials.OwnerProtectedError:
        raise Refused(403, "the owner's two-factor sign-in can only be reset from a shell") from None
    except credentials.OwnerRequiredError:
        raise Refused(403, "only the owner can reset an admin's two-factor sign-in") from None
    if not found:
        raise Refused(404, "no such user")
    audit(request, KIND_ADMIN, "two-factor reset", {"user_id": body.user_id})
    return OkReply(ok=True)


# <<< feature:two_factor
