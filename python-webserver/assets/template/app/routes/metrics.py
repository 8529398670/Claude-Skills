"""GET /metrics (app/core/metrics.py). Who may read it is decided where it is
registered: see RequireMetrics in routes/__init__.py."""
from __future__ import annotations

from starlette.requests import Request
from starlette.responses import Response

from app.core.metrics import CONTENT_TYPE
from app.routes.helpers import run


async def get_metrics(request: Request) -> Response:
    state = request.app.state
    records = await run(state.jobs.records)
    return Response(state.metrics.render(state.logs.dropped, records), media_type=CONTENT_TYPE)
