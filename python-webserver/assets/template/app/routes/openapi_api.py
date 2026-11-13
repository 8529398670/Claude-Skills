# whole-file feature:openapi
"""The API docs for openapi.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import TypedDict

from app.apidocs import doc
from app.routes import openapi


class GetOpenapiReply(TypedDict):
    pass


doc(
    openapi.get_openapi,
    summary="This document",
    tag="Monitoring",
    feature="openapi",
    mcp_http="The API reference itself; GET /api/mcp.json is the MCP one.",
    reply=GetOpenapiReply,
    errors=(401, 404),
)
