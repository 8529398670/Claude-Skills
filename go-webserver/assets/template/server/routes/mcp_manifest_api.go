// whole-file feature:mcp
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
	mcp "{{MODULE_PATH}}/server/mcp"
)

// The API docs for mcp_manifest.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).GetMCPManifest , apidoc.Op{
	Summary: "The MCP tools: every tool /mcp has, its arguments, and the route it stands for",
	Tag: "Monitoring",
	Feature: "mcp",
	MCPHTTP: "The MCP reference itself; tools/list gives the same list.",
	Reply: getMCPManifestReplyDoc{},
	Errors: []int{ 401 , 404 },
} )

type getMCPManifestReplyDoc struct{}

var _ = apidoc.Doc( (*mcp.Server).Handle , apidoc.Op{
	Summary: "Model Context Protocol, at the key's role",
	Tag: "Monitoring",
	Access: apidoc.KeyOnly,
	Feature: "mcp",
	MCPHTTP: "The MCP endpoint itself.",
	Body: handleBodyDoc{},
	Reply: handleReplyDoc{},
	Errors: []int{ 400 , 401 , 404 },
} )

type handleBodyDoc struct{}

func ( handleBodyDoc ) APIDoc() ( text string ) { text = "A JSON-RPC 2.0 request."; return }

type handleReplyDoc struct{}

var _ = apidoc.Doc( (*mcp.Server).MethodNotAllowed , apidoc.Op{
	Method: "GET",
	Summary: "Refused: this server has no stream",
	Tag: "Monitoring",
	Access: apidoc.Public,
	Feature: "mcp",
	Status: 405,
	Reply: methodNotAllowedReplyDocX{},
	Errors: []int{ 404 },
} )

type methodNotAllowedReplyDocX struct {
	Ok bool `json:"ok"`
}

type methodNotAllowedReplyDocXDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*mcp.Server).MethodNotAllowed , apidoc.Op{
	Method: "DELETE",
	Summary: "Refused: this server keeps no sessions",
	Tag: "Monitoring",
	Access: apidoc.Public,
	Feature: "mcp",
	Status: 405,
	Reply: methodNotAllowedReplyDocXDoc{},
	Errors: []int{ 404 },
} )
