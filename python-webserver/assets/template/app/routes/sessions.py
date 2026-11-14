# whole-file feature:sessions
"""A person's own sessions: which devices are signed in, and signing them out
(app/models/sessions.py). An API key may do it too; it has no device of its
own, so nothing is current and revoke-others ends them all. Every id is checked against the account asking: someone else's session and one
that does not exist get the same 404. Ending the session in use is what signing
out is for.
"""
from __future__ import annotations

from typing import Annotated, ClassVar

from msgspec import UNSET, Meta, UnsetType
from starlette.requests import Request

from app.apidocs import ADMIN, PATH, Reply, route
from app.apidocs import Request as RequestType
from app.core.db import Store
from app.core.logs import KIND_ADMIN, KIND_AUTH
from app.core.security import actor_from
from app.models import sessions, to_iso, users
from app.models import ip_names  # feature:logs
from app.routes.admin import DateTime, OkReply, UserIDPath
from app.routes.helpers import PAGE_DEFAULT, Limit, Offset, Page, Paged, Refused, act_on_refusals, cut_page, get_store
from app.routes.logs import audit


class Session(Reply, kw_only=True):
    """A signed-in device, as its account (or an admin) sees it."""

    api_name: ClassVar[str] = "Session"
    id: str
    current: bool
    created_at: DateTime
    expires_at: DateTime
    last_seen_at: DateTime | UnsetType = UNSET
    ip: str | UnsetType = UNSET
    user_agent: str | UnsetType = UNSET
    label: Annotated[str, Meta(description="The name you gave this device.")] | UnsetType = UNSET
    ip_name: Annotated[str, Meta(description="What the owner calls this address on the Logs screen. The owner's own list only.")] | UnsetType = UNSET  # feature:logs


class SessionIDPath(RequestType):
    """The device a route under .../sessions/{session_id} acts on."""

    session_id: Annotated[str, PATH, Meta(description="The device's id, from GET /api/account/sessions or GET /api/admin/users/{user_id}/sessions (list_my_sessions or admin_list_user_sessions over MCP).")]


class SessionsReply(Paged):
    sessions: list[Session]


def session_row(session: sessions.Session, current_id: str) -> Session:
    """Field by field on purpose: secret_hash and csrf_token must not reach a
    response because something later adds a loop over the record."""
    row = Session(
        id=session.id,
        current=session.id == current_id,
        created_at=to_iso(session.created_at),
        expires_at=to_iso(session.expires_at),
    )
    if session.last_seen_at is not None:
        row.last_seen_at = to_iso(session.last_seen_at)
    if session.ip:
        row.ip = session.ip
    if session.user_agent:
        row.user_agent = session.user_agent
    if session.label:
        row.label = session.label
    return row


def current_session_id(request: Request) -> str:
    """The session this request came with, or "" through an API key, which has
    none: nothing in a list is then "current", and nothing is refused as "this
    device"."""
    session = actor_from(request).session
    return session.id if session is not None else ""


@route(summary="Your signed-in devices", tag="Account", feature="sessions", errors=(400, 401, 403))
def list_my_sessions(request: Request, body: Page) -> SessionsReply:
    """Where the caller is signed in, over HTTP and as the MCP
    list_my_sessions tool."""
    store, actor = get_store(request), actor_from(request)
    rows = [session_row(session, current_session_id(request)) for session in sessions.list_sessions(store, actor.user.id)]
    # >>> feature:logs
    if actor.user.id == users.get_owner_id(store):
        _name_rows(store, rows)
    # <<< feature:logs
    page = cut_page(rows, body)
    return SessionsReply(sessions=page.pop("rows"), **page)


# >>> feature:logs
def _name_rows(store: Store, rows: list[Session]) -> None:
    """Adds what the owner calls each address. Names are the owner's own notes
    about people, so they go to the owner and nobody else."""
    namer = ip_names.load_namer(store)
    for row in rows:
        name = namer(row.ip or "")
        if name:
            row.ip_name = name


# <<< feature:logs
def _target_of(store: Store, self_id: int, user_id: int) -> bool:
    """Whether the owner is asking about an account's sessions, raising
    Refused when the account may not be reached. Nobody but the owner reaches
    the owner's, the same rule as revoking the owner's access."""
    if users.get_user(store, user_id) is None:
        raise Refused(404, "no such user")
    owner_id = users.get_owner_id(store)
    by_owner = self_id == owner_id
    if user_id == owner_id and not by_owner:
        raise Refused(403, "the owner account's sessions are the owner's own")
    return by_owner


class ListUserSessions(UserIDPath):
    limit: Limit = PAGE_DEFAULT
    offset: Offset = 0


@route(
    summary="An account's signed-in devices",
    description="The owner's are the owner's alone, and only the owner sees addresses.",
    tag="Admin", role=ADMIN, feature="sessions", errors=(400, 401, 403, 404),
)
def list_user_sessions(request: Request, body: ListUserSessions) -> SessionsReply:
    """Where an account is signed in, for ending one device rather than
    everything with revoke-access, over HTTP and as the MCP
    admin_list_user_sessions tool. Addresses are for the owner only, as in the
    activity log, which keeps them from other admins too."""
    store = get_store(request)
    by_owner = _target_of(store, actor_from(request).user.id, body.user_id)
    rows = [session_row(session, current_session_id(request)) for session in sessions.list_sessions(store, body.user_id)]
    if not by_owner:
        for row in rows:
            row.ip = UNSET
    if by_owner:  # feature:logs
        _name_rows(store, rows)  # feature:logs
    page = cut_page(rows, body)
    return SessionsReply(sessions=page.pop("rows"), **page)


class RevokeUserSession(UserIDPath):
    session_id: Annotated[str, PATH, Meta(description="The device's id, from GET /api/account/sessions or GET /api/admin/users/{user_id}/sessions (list_my_sessions or admin_list_user_sessions over MCP).")]


@route(summary="Sign out one of an account's devices", tag="Admin", role=ADMIN, feature="sessions", errors=(400, 401, 403, 404))
def revoke_user_session(request: Request, body: RevokeUserSession) -> OkReply:
    """Sign one of an account's devices out, and only that: over HTTP or the
    MCP admin_revoke_user_session tool."""
    store = get_store(request)
    _target_of(store, actor_from(request).user.id, body.user_id)
    if body.session_id and body.session_id == current_session_id(request):
        raise Refused(400, "use sign out for this device")
    with act_on_refusals("end a session of"):
        found = sessions.revoke_session(store, body.user_id, body.session_id, actor_id=actor_from(request).user.id)
    if not found:
        raise Refused(404, "not found")
    audit(request, KIND_ADMIN, "session revoked", {"user_id": body.user_id})
    return OkReply(ok=True)


class LabelOne(SessionIDPath):
    label: Annotated[str, Meta(max_length=sessions.LABEL_MAX_CHARS, description="At most 80 characters.", extra={"error": "a device name can be at most 80 characters"})]
    expect: Annotated[str | None, Meta(description="The name you think it has. Answers 409 if it has another.")] = None


class LabelOneReply(Reply):
    ok: bool
    label: str


@route(
    summary="Name one of your devices", description="This device included. Empty clears the name.",
    tag="Account", feature="sessions", errors=(400, 401, 403, 404, 409),
)
def label_one(request: Request, body: LabelOne) -> LabelOneReply:
    """Name one of your own devices, this one included, over HTTP or the MCP
    label_my_session tool. An empty label clears the name. Admins see a label
    but cannot set one: it is what the person calls their own device."""
    if not sessions.valid_session_label(body.label):
        raise Refused(400, "a device name can be at most 80 characters")
    try:
        found = sessions.label_session(get_store(request), actor_from(request).user.id, body.session_id, body.label, body.expect)
    except users.RenameConflictError as stale:
        raise Refused(409, str(stale), label=stale.current) from None
    if not found:
        raise Refused(404, "not found")
    # Not the label: it is the person's own words about where they are.
    audit(request, KIND_AUTH, "session_labelled", {})
    return LabelOneReply(ok=True, label=body.label.strip())


@route(summary="Sign out one device", tag="Account", feature="sessions", errors=(400, 401, 403, 404))
def revoke_one(request: Request, body: SessionIDPath) -> OkReply:
    """Sign out one other device, over HTTP or the MCP revoke_my_session
    tool. This device is refused: ending it here would leave the browser
    holding a dead cookie, and signing out is what the sign-out button is for.
    Through an API key there is no "this device", so any of them may go."""
    if body.session_id and body.session_id == current_session_id(request):
        raise Refused(400, "use sign out for this device")
    if not sessions.revoke_session(get_store(request), actor_from(request).user.id, body.session_id):
        raise Refused(404, "not found")
    audit(request, KIND_AUTH, "session_revoked")
    return OkReply(ok=True)


class RevokeOthersReply(Reply):
    ok: bool
    removed: int


@route(summary="Sign out every other device", tag="Account", feature="sessions", errors=(400, 401, 403))
def revoke_others(request: Request) -> RevokeOthersReply:
    """The button for a lost phone or a shared computer: everything but this
    device, over HTTP or the MCP revoke_my_other_sessions tool. Through an API
    key, which is no device, that is every session the account has."""
    removed = sessions.revoke_other_sessions(get_store(request), actor_from(request).user.id, current_session_id(request))
    audit(request, KIND_AUTH, "sessions_revoked", {"removed": removed})
    return RevokeOthersReply(ok=True, removed=removed)
