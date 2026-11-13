# whole-file feature:jobs
"""The API docs for jobs.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, NotRequired, TypedDict

from app.apidocs import Field, OWNER, SESSION_ONLY, doc
from app.routes import jobs
from app.routes.schemas_api import JobSchema


class ListJobsReply(TypedDict):
    jobs: list[JobSchema]


doc(
    jobs.list_jobs,
    summary="Background jobs",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="jobs",
    reply=ListJobsReply,
    errors=(404,),
)


class RunJobBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class RunJobReply(TypedDict):
    ok: bool


doc(
    jobs.run_job,
    summary="Run a job now",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="jobs",
    body=RunJobBody,
    status=202,
    reply=RunJobReply,
    errors=(400, 403, 404, 409),
)
