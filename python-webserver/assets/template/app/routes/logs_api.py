"""The API docs for logs.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, Any, NotRequired, TypedDict

from app.apidocs import Field, OWNER, SESSION_ONLY, doc
from app.routes import logs
from app.routes.schemas_api import IPNamesSchema  # feature:logs
from app.routes.schemas_api import LogDaySchema  # feature:logs
from app.routes.schemas_api import LogDaysSchema  # feature:logs
from app.routes.schemas_api import PruneResultSchema  # feature:logs


# >>> feature:logs
class RecordUIEventsBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    events: list[dict[str, Any]]
    tab: str


class RecordUIEventsReply(TypedDict):
    accepted: int


doc(
    logs.record_ui_events,
    summary="Record what happened on the page",
    tag="Account",
    feature="logs",
    mcp_http="The browser's own activity reports (track.js), not an action.",
    body=RecordUIEventsBody,
    reply=RecordUIEventsReply,
    errors=(400, 401, 403, 404),
)
# <<< feature:logs


# >>> feature:logs
class ListLogDaysParams(TypedDict):
    q: Annotated[str, Field(query="q", doc="The Logs screen's search.")]


doc(
    logs.list_log_days,
    summary="The days on record",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="logs",
    params=ListLogDaysParams,
    reply=LogDaysSchema,
    errors=(404,),
)
# <<< feature:logs


# >>> feature:logs
class GetLogDayParams(TypedDict):
    q: Annotated[str, Field(query="q", doc="The Logs screen's search.")]
    before: Annotated[int, Field(query="before")]
    limit: Annotated[int, Field(query="limit")]


doc(
    logs.get_log_day,
    summary="One day's entries",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="logs",
    params=GetLogDayParams,
    reply=LogDaySchema,
    errors=(400, 404),
)
# <<< feature:logs


# >>> feature:logs
class DeleteLogDayBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class DeleteLogDayReply(TypedDict):
    deleted: bool


doc(
    logs.delete_log_day,
    summary="Delete one day",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="logs",
    body=DeleteLogDayBody,
    reply=DeleteLogDayReply,
    errors=(400, 403, 404, 409),
)
# <<< feature:logs


# >>> feature:logs
doc(
    logs.download_log_day,
    summary="One day's entries as a file",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="logs",
    reply_type="application/x-ndjson",
    errors=(400, 404),
)
# <<< feature:logs


# >>> feature:logs
doc(
    logs.list_ip_names,
    summary="What the owner calls addresses",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="logs",
    reply=IPNamesSchema,
    errors=(403, 404),
)
# <<< feature:logs


# >>> feature:logs
class SetIPNameBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    name: Annotated[str, Field(max_length=80)]
    target: Annotated[str, Field(doc="An address, or a range such as 10.0.0.0/24.", example="10.0.0.0/24")]


class SetIPNameReply(TypedDict):
    name: NotRequired[str]
    target: NotRequired[str]


doc(
    logs.set_ip_name,
    summary="Name an address or a range, or rename one",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="logs",
    body=SetIPNameBody,
    reply=SetIPNameReply,
    errors=(400, 403, 404),
)
# <<< feature:logs


# >>> feature:logs
class DeleteIPNameBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    target: Annotated[str, Field(doc="An address, or a range such as 10.0.0.0/24.", example="10.0.0.0/24")]


class DeleteIPNameReply(TypedDict):
    deleted: NotRequired[bool]


doc(
    logs.delete_ip_name,
    summary="Take a name off an address or a range",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="logs",
    body=DeleteIPNameBody,
    reply=DeleteIPNameReply,
    errors=(400, 403, 404),
)
# <<< feature:logs


# >>> feature:logs
class PruneLogsBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


doc(
    logs.prune_logs,
    summary="Apply the retention rules now",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="logs",
    body=PruneLogsBody,
    reply=PruneResultSchema,
    errors=(400, 403, 404),
)
# <<< feature:logs


# >>> feature:logs
class StreamLogChangesParams(TypedDict):
    q: Annotated[str, Field(query="q", doc="The Logs screen's search.")]


doc(
    logs.stream_log_changes,
    summary="New log entries as they happen",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="logs",
    params=StreamLogChangesParams,
    reply_type="text/event-stream",
    errors=(404,),
)
# <<< feature:logs
