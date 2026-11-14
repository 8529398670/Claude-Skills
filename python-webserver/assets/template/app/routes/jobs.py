# whole-file feature:jobs
"""The owner's Jobs screen: what runs in the background, how it last went, and
starting one now (app/core/jobs.py). A run is started and answered at once;
the page reads the outcome from the list."""
from __future__ import annotations

from typing import Annotated, ClassVar

import msgspec
from msgspec import UNSET, Meta, UnsetType
from starlette.requests import Request

from app.apidocs import OWNER, PATH, Reply, route
from app.apidocs import Request as RequestType
from app.core import jobs
from app.core.logs import KIND_ADMIN
from app.routes.admin import DateTime, OkReply
from app.routes.helpers import Refused
from app.routes.logs import audit


class Job(Reply, kw_only=True):
    """A background job as the Jobs screen lists it (jobs.Registry.rows)."""

    api_name: ClassVar[str] = "Job"
    name: str
    interval_seconds: int
    running: bool
    interrupted: bool
    runs: int
    next_due: DateTime
    last_started: DateTime | UnsetType = UNSET
    last_finished: DateTime | UnsetType = UNSET
    last_ok: bool | None | UnsetType = UNSET
    last_note: str | UnsetType = UNSET
    last_error: str | UnsetType = UNSET


class JobsReply(Reply):
    jobs: list[Job]


def get_jobs(request: Request) -> jobs.Registry:
    return request.app.state.jobs


@route("GET /api/owner/jobs", summary="Background jobs", tag="Owner", role=OWNER, feature="jobs")
def list_jobs(request: Request) -> JobsReply:
    """Reads SQLite, so a plain function: it runs in a worker thread."""
    return JobsReply(jobs=msgspec.convert(get_jobs(request).rows(), list[Job]))


class JobNamePath(RequestType):
    name: Annotated[str, PATH, Meta(description="The job's name, from GET /api/owner/jobs.")]


@route(
    "POST /api/owner/jobs/{name}/run", summary="Run a job now",
    tag="Owner", role=OWNER, feature="jobs", status=202, errors=(409,),
)
async def run_job(request: Request, body: JobNamePath) -> OkReply:
    """A coroutine: the run is a task on the event loop."""
    try:
        get_jobs(request).run_now(body.name)
    except jobs.UnknownJobError:
        raise Refused(404, "not found") from None
    except jobs.JobRunningError:
        raise Refused(409, "job already running") from None
    audit(request, KIND_ADMIN, "job_run", {"name": body.name})
    return OkReply(ok=True)
