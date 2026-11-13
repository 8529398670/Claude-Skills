# whole-file feature:mcp
"""The API docs for mcp_manifest.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import TypedDict

from app.apidocs import KEY_ONLY, PUBLIC, doc, schema
from app.mcp.protocol import Server
from app.routes import mcp_manifest


class GetMCPManifestReply(TypedDict):
    pass


doc(
    mcp_manifest.get_mcp_manifest,
    summary="The MCP tools: every tool /mcp has, its arguments, and the route it stands for",
    tag="Monitoring",
    feature="mcp",
    mcp_http="The MCP reference itself; tools/list gives the same list.",
    reply=GetMCPManifestReply,
    errors=(401, 404),
)


class MethodNotAllowedReply(TypedDict):
    ok: bool


class MethodNotAllowedReplyX(TypedDict):
    ok: bool


doc(
    Server.method_not_allowed,
    method="DELETE",
    summary="Refused: this server keeps no sessions",
    tag="Monitoring",
    access=PUBLIC,
    feature="mcp",
    status=405,
    reply=MethodNotAllowedReply,
    errors=(404,),
)


doc(
    Server.method_not_allowed,
    method="GET",
    summary="Refused: this server has no stream",
    tag="Monitoring",
    access=PUBLIC,
    feature="mcp",
    status=405,
    reply=MethodNotAllowedReplyX,
    errors=(404,),
)


@schema(doc="A JSON-RPC 2.0 request.")
class EndpointBody(TypedDict):
    pass


class EndpointReply(TypedDict):
    pass


doc(
    Server.endpoint,
    summary="Model Context Protocol, at the key's role",
    tag="Monitoring",
    access=KEY_ONLY,
    feature="mcp",
    mcp_http="The MCP endpoint itself.",
    body=EndpointBody,
    reply=EndpointReply,
    errors=(400, 401, 404),
)
