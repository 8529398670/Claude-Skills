// whole-file feature:openapi
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for openapi.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).GetOpenAPI , apidoc.Op{
	Summary: "This document",
	Tag: "Monitoring",
	Feature: "openapi",
	MCPHTTP: "The API reference itself; GET /api/mcp.json is the MCP one.",
	Reply: getOpenAPIReplyDoc{},
	Errors: []int{ 401 , 404 },
} )

type getOpenAPIReplyDoc struct{}
