# whole-file feature:metrics
"""The API docs for metrics.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from app.apidocs import ADMIN, KEY_ONLY, doc
from app.routes import metrics


doc(
    metrics.get_metrics,
    summary="Prometheus metrics",
    tag="Monitoring",
    access=KEY_ONLY,
    role=ADMIN,
    feature="metrics",
    mcp_http="Scraped by Prometheus over HTTP.",
    reply_type="text/plain",
    errors=(401, 403, 404),
)
