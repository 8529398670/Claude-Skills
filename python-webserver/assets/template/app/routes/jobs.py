"""The owner's Jobs screen: what runs in the background, how it last went, and
starting one now (app/core/jobs.py). A run is started and answered at once;
the page reads the outcome from the list."""
from __future__ import annotations

from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.core import jobs
from app.core.logs import KIND_ADMIN
from app.core.security import check_csrf
from app.routes.helpers import bad_request, conflict, forbidden, not_found, read_json, run
from app.routes.logs import audit


def get_jobs(request: Request) -> jobs.Registry:
    return request.app.state.jobs


async def list_jobs(request: Request) -> Response:
    return JSONResponse({"jobs": await run(get_jobs(request).rows)})


async def run_job(request: Request) -> Response:
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    name = request.path_params["name"]
    try:
        get_jobs(request).run_now(name)
    except jobs.UnknownJobError:
        return not_found("not found")
    except jobs.JobRunningError:
        return conflict("job already running")
    audit(request, KIND_ADMIN, "job_run", {"name": name})
    return JSONResponse({"ok": True}, status_code=202)
