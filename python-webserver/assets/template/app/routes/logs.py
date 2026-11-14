# >>> feature:logs
"""The activity log's endpoints (see app/core/logs.py). Reading it is the
owner's alone, under /api/owner/logs (RequireOwner); writing to it from the
page -- what track.js sends -- is any signed-in user's, at /api/logs/events,
since what they do is what gets logged. audit() and audit_as() are how the
other route modules write auth and admin entries."""
# <<< feature:logs
# >>> feature:!logs
"""What the other route modules call to write to the activity log, in a project
without one: nothing is written."""
# <<< feature:!logs
from __future__ import annotations

from typing import Any

from starlette.requests import Request

# >>> feature:logs
import json
import threading
import time
from collections import deque
from typing import Annotated, ClassVar

from msgspec import Meta
from starlette.responses import Response, StreamingResponse

from app.apidocs import OWNER, PATH, QUERY, SESSION_ONLY, Reply, route
from app.apidocs import Request as RequestType
from app.core import logs, request_id, sse
from app.core.config import trusted_proxy_networks
from app.core.middleware import client_ip
from app.core.security import actor_from, decrypt_cookie, log_actor, still_owner
from app.models import ip_names, users
from app.routes.helpers import PAGE_DEFAULT, DateTime, Limit, Refused, error, get_config, get_store, run

# Page events a browser may send per call, and per account per minute.
EVENTS_PER_CALL = 50
EVENTS_PER_MINUTE = 600
DETAIL_MAX_BYTES = 2048

# The page actions track.js sends. Anything else is refused, so the log
# cannot be filled with made-up kinds.
UI_ACTIONS = {"session_start", "nav", "click", "submit", "undo", "redo", "error", "fetch_fail"}


def get_logs(request: Request) -> logs.Logger:
    return request.app.state.logs


def _settings(logger: logs.Logger) -> dict[str, Any]:
    current = logger.settings
    return {
        "enabled": current.enabled,
        "retention_days": current.retention_days,
        "max_total_mb": current.max_total_bytes // 1024 // 1024,
        "max_file_mb": current.max_file_bytes // 1024 // 1024,
        "compress_after_days": current.compress_after_days,
        "static_assets": current.static_assets,
        "ui_clicks": current.ui_clicks,
        "encrypted": logger.encrypt,
    }


def _query(request: Request, text: str = "") -> logs.Query:
    """A search that knows what the owner calls each address, which reading a
    day, searching and downloading all need. Reads SQLite: from a worker
    thread, or through run()."""
    return logs.Query(text, ip_names.load_namer(get_store(request)))


# The Logs screen's search, which the routes that read the log take.
Search = Annotated[str, QUERY, Meta(description="The Logs screen's search.")]


class LogSearch(RequestType):
    q: Search = ""


class LogDays(Reply):
    """The days on record, as the Logs screen lists them."""

    api_name: ClassVar[str] = "LogDays"
    days: list[dict[str, Any]]
    total_bytes: int
    dropped: int
    settings: dict[str, Any]


@route(summary="The days on record", tag="Owner", access=SESSION_ONLY, role=OWNER, feature="logs", errors=(404,))
def list_log_days(request: Request, body: LogSearch) -> LogDays:
    logger = get_logs(request)
    days, total = logger.days(_query(request, body.q))
    return LogDays(days=days, total_bytes=total, dropped=logger.dropped, settings=_settings(logger))


class DatePath(RequestType):
    """The day a route under .../days/{date} acts on. valid_day checks it:
    it must be a real date, which no limit can say."""

    date: Annotated[str, PATH, Meta(description="The day's date, from GET /api/owner/logs/days.")]


class GetLogDay(DatePath):
    q: Search = ""
    before: Annotated[int, QUERY, Meta(ge=0, extra={"error": "before must be a line number"})] | None = None
    limit: Limit = PAGE_DEFAULT


class LogDay(Reply):
    api_name: ClassVar[str] = "LogDay"
    date: str
    entries: list[dict[str, Any]]
    next: int | None


@route(summary="One day's entries", tag="Owner", access=SESSION_ONLY, role=OWNER, feature="logs", errors=(400, 404))
def get_log_day(request: Request, body: GetLogDay) -> LogDay:
    if not logs.valid_day(body.date):
        raise Refused(400, "bad_day")
    before = -1 if body.before is None else body.before
    records, following = get_logs(request).read_day(body.date, _query(request, body.q), before, body.limit)
    return LogDay(date=body.date, entries=records, next=following)


@route(
    summary="One day's entries as a file", tag="Owner", access=SESSION_ONLY, role=OWNER, feature="logs",
    reply_type="application/x-ndjson", errors=(400, 404),
)
async def download_log_day(request: Request, body: DatePath) -> Response:
    """A day as plain JSON lines, decrypted: the file a person can grep, or
    hand to whatever reads logs. Each line has ip_name beside the address, as
    named when this was asked for. Starlette walks the generator in a worker
    thread, so the file reads stay off the event loop."""
    day = body.date
    if not logs.valid_day(day):
        raise Refused(400, "bad_day")
    return StreamingResponse(
        get_logs(request).download(day, await run(_query, request)),
        media_type="application/x-ndjson",
        headers={"Content-Disposition": f'attachment; filename="logs-{day}.jsonl"'},
    )


class DeletedReply(Reply):
    deleted: bool


@route(summary="Delete one day", tag="Owner", access=SESSION_ONLY, role=OWNER, feature="logs", errors=(400, 403, 404, 409))
def delete_log_day(request: Request, body: DatePath) -> DeletedReply:
    try:
        get_logs(request).delete_day(body.date)
    except logs.BadDayError as refusal:
        raise Refused(400, str(refusal)) from None
    except logs.TodayError as refusal:
        raise Refused(409, str(refusal)) from None
    audit(request, logs.KIND_ADMIN, "log day deleted", {"date": body.date})
    return DeletedReply(deleted=True)


class PruneResult(Reply):
    """What Logger.prune did."""

    api_name: ClassVar[str] = "PruneResult"
    compressed: int
    deleted: int
    bytes_before: int
    bytes_after: int


@route(summary="Apply the retention rules now", tag="Owner", access=SESSION_ONLY, role=OWNER, feature="logs", errors=(400, 403, 404))
def prune_logs(request: Request) -> PruneResult:
    result = get_logs(request).prune()
    audit(request, logs.KIND_ADMIN, "logs pruned", {"compressed": result["compressed"], "deleted": result["deleted"]})
    return PruneResult(**result)


class IPName(Reply):
    target: str
    name: str
    created_at: DateTime


class IPNames(Reply):
    api_name: ClassVar[str] = "IPNames"
    names: list[IPName]


@route(summary="What the owner calls addresses", tag="Owner", access=SESSION_ONLY, role=OWNER, feature="logs", errors=(403, 404))
def list_ip_names(request: Request) -> IPNames:
    """Everything the owner has named. It is short by nature, so unlike the lists
    people's activity grows, it comes whole."""
    return IPNames(names=[IPName(**record) for record in ip_names.list_names(get_store(request))])


# An address or a range, checked by clean_target: no limit can say what an
# address is.
Target = Annotated[str, Meta(description="An address, or a range such as 10.0.0.0/24.", examples=["10.0.0.0/24"], extra={"error": "bad_ip_target"})]


class SetIPName(RequestType):
    target: Target
    name: Annotated[str, Meta(min_length=1, max_length=ip_names.NAME_MAX, extra={"error": "bad_ip_name"})]


class SetIPNameReply(Reply):
    target: str
    name: str


@route(
    summary="Name an address or a range, or rename one", tag="Owner", access=SESSION_ONLY, role=OWNER, feature="logs",
    errors=(400, 403, 404),
)
def set_ip_name(request: Request, body: SetIPName) -> SetIPNameReply:
    """Names an address or a range, or renames one. The answer gives the target
    as stored, which may not be how it was typed."""
    target = ip_names.clean_target(body.target)
    if target is None:
        raise Refused(400, "bad_ip_target")
    if not ip_names.valid_name(body.name):
        raise Refused(400, "bad_ip_name")
    ip_names.set_name(get_store(request), target, body.name)
    audit(request, logs.KIND_ADMIN, "ip name set", {"target": target})
    return SetIPNameReply(target=target, name=body.name.strip())


class DeleteIPName(RequestType):
    target: Target


@route(
    summary="Take a name off an address or a range", tag="Owner", access=SESSION_ONLY, role=OWNER, feature="logs",
    errors=(400, 403, 404),
)
def delete_ip_name(request: Request, body: DeleteIPName) -> DeletedReply:
    """Takes a name away. Its target is in the body, because a range has a slash
    in it."""
    target = ip_names.clean_target(body.target)
    if target is None:
        raise Refused(400, "bad_ip_target")
    ip_names.delete_name(get_store(request), target)
    audit(request, logs.KIND_ADMIN, "ip name removed", {"target": target})
    return DeletedReply(deleted=True)


@route(
    summary="New log entries as they happen", tag="Owner", access=SESSION_ONLY, role=OWNER, feature="logs",
    reply_type="text/event-stream", errors=(404,),
)
async def stream_log_changes(request: Request, body: LogSearch) -> Response:
    """The Logs screen's live tail: "something changed" nudges, and logs.js
    runs its normal refresh. Sending the entries themselves would need a
    second filter and paging path through the same data. This route is under
    /api/owner/logs, so RequestLog skips it and the stream cannot feed itself.

    RequireOwner let this in once. The cookie is kept to ask again every tick,
    because the session can end while the stream stays open."""
    if request.method == "HEAD":
        return Response(headers=sse.HEADERS)
    cfg, store = get_config(request), get_store(request)
    credential = decrypt_cookie(cfg, request.cookies.get(cfg.cookie_name, ""))
    hub: sse.Hub = request.app.state.log_hub
    client = hub.join()
    if client is None:
        return error(503, "too many live streams")

    async def allowed() -> bool:
        return await run(still_owner, store, credential)

    return sse.EventStream(hub, client, allowed, opening=(sse.retry(sse.RETRY_MS), sse.CHANGED))


class _EventBudget:
    """The per-account cap on page events: a minute's worth, in a sliding
    window, so a stuck loop in a browser cannot fill the disk."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._seen: dict[int, deque[float]] = {}

    def take(self, user_id: int, wanted: int) -> int:
        with self._lock:
            now = time.monotonic()
            recent = self._seen.setdefault(user_id, deque())
            while recent and now - recent[0] >= 60:
                recent.popleft()
            allowed = max(0, min(wanted, EVENTS_PER_MINUTE - len(recent)))
            recent.extend([now] * allowed)
            if not recent:
                del self._seen[user_id]
            return allowed


_budget = _EventBudget()


def _cap_fields(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [logs.truncate(field, 60) for field in value[:30] if isinstance(field, str)]


def _cap_map(value: Any) -> dict[str, Any]:
    """A browser-sent object, kept only if it is small, with its strings
    capped. Too big, and it is dropped whole rather than cut mid-structure."""
    if not isinstance(value, dict) or not value:
        return {}
    capped = {str(name): logs.truncate(item, 200) if isinstance(item, str) else item for name, item in value.items()}
    try:
        encoded = json.dumps(capped)
    except (TypeError, ValueError):
        return {}
    return capped if len(encoded.encode("utf-8")) <= DETAIL_MAX_BYTES else {}


def _ip(request: Request) -> str:
    cfg = get_config(request)
    return client_ip(request.scope, cfg, trusted_proxy_networks(cfg))


class RecordUIEvents(RequestType):
    tab: str
    # Each is checked on its own below: one a browser got wrong is skipped,
    # not the batch refused.
    events: Annotated[list[dict[str, Any]], Meta(extra={"error": "events must be a list"})]


class RecordUIEventsReply(Reply):
    accepted: int


@route(
    summary="Record what happened on the page", tag="Account", feature="logs",
    mcp_http="The browser's own activity reports (track.js), not an action.", errors=(400, 401, 403, 404),
)
def record_ui_events(request: Request, body: RecordUIEvents) -> RecordUIEventsReply:
    """A batch from track.js. The server stamps each with its own time, the
    signed-in account, the address and the user agent, so a browser can
    describe what it did but not who did it or when it arrived."""
    events = body.events[:EVENTS_PER_CALL]
    actor = actor_from(request)
    allowed = _budget.take(actor.user.id, len(events))
    logger = get_logs(request)
    tab = logs.truncate(body.tab, 32)
    who, ip, ua = log_actor(actor), _ip(request), logs.truncate(request.headers.get("user-agent", ""), 300)
    accepted = 0
    for index, event in enumerate(events):
        if index >= allowed or not isinstance(event, dict) or event.get("action") not in UI_ACTIONS:
            continue
        action = event["action"]
        ui = {
            "action": action,
            "screen": logs.truncate(event.get("screen", ""), 120),
            "target": logs.truncate(event.get("target", ""), 120),
            "fields": _cap_fields(event.get("fields")),
            "detail": _cap_map(event.get("detail")),
            "at": logs.truncate(event.get("at", ""), 40),
        }
        client: dict[str, Any] = {"tab": tab}
        if action == "session_start":
            client.update(_cap_map(event.get("client")))
        level = logs.LEVEL_ERROR if action in ("error", "fetch_fail") else logs.LEVEL_INFO
        logger.log(
            logs.entry(
                logs.KIND_UI,
                level,
                actor=who,
                ip=ip,
                ua=ua,
                ui={name: value for name, value in ui.items() if value not in ("", [], {})},
                client=client,
            )
        )
        accepted += 1
    return RecordUIEventsReply(accepted=accepted)


# ---------------------------------------------------------------------------
# audit entries, for the other route modules
# ---------------------------------------------------------------------------


def audit(request: Request, kind: str, message: str, detail: dict[str, Any] | None = None) -> None:
    """Log an auth or admin action by whoever made this request. detail says
    what it touched: ids and names, never a credential. Never blocks."""
    audit_as(request, log_actor(actor_from(request)), kind, logs.LEVEL_INFO, message, detail)


def audit_as(
    request: Request, actor: dict[str, Any], kind: str, level: str, message: str, detail: dict[str, Any] | None = None
) -> None:
    """audit for a request whose actor is not the signed-in one: a sign-in
    (nobody yet), or a failed one."""
    logger = get_logs(request)
    if not logger.enabled:
        return
    logger.log(
        logs.entry(
            kind,
            level,
            request_id=request_id.of(request.scope),
            actor=actor,
            ip=_ip(request),
            ua=logs.truncate(request.headers.get("user-agent", ""), 300),
            msg=message,
            detail=detail,
        )
    )


def user_actor(store: Any, user_id: int, method: str) -> dict[str, Any]:
    """The actor for an account named by id, for an entry made before the
    request has an actor of its own. Reads SQLite: call it through run()."""
    actor: dict[str, Any] = {"user_id": user_id, "method": method}
    user = users.get_user(store, user_id)
    if user is not None:
        actor["name"] = user.display_name
        actor["role"] = user.role
    return actor
# <<< feature:logs
# >>> feature:!logs


def audit(request: Request, kind: str, message: str, detail: dict[str, Any] | None = None) -> None:
    pass


def audit_as(
    request: Request, actor: dict[str, Any], kind: str, level: str, message: str, detail: dict[str, Any] | None = None
) -> None:
    pass


def user_actor(store: Any, user_id: int, method: str) -> dict[str, Any]:
    return {}
# <<< feature:!logs
