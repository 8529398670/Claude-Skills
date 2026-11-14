# whole-file feature:metrics
"""GET /metrics (app/core/metrics.py): an admin-level key, nothing else.
RequireAPIKey comes before RequireAdmin, so a signed-in browser is refused
however high its role: this is for a scraper, and a page has no reason to read
it. RequireMetrics (routes/__init__.py) answers 404 while it is off."""
from __future__ import annotations

from starlette.requests import Request
from starlette.responses import Response

from app.apidocs import ADMIN, KEY_ONLY, route
from app.core.metrics import CONTENT_TYPE
from app.routes.helpers import run


@route(
    "GET /metrics", summary="Prometheus metrics",
    tag="Monitoring", access=KEY_ONLY, role=ADMIN, feature="metrics", reply_type="text/plain",
    mcp_http="Scraped by Prometheus over HTTP.",
)
async def get_metrics(request: Request) -> Response:
    state = request.app.state
    records = await run(state.jobs.records)
    return Response(state.metrics.render(state.logs.dropped, records), media_type=CONTENT_TYPE)
