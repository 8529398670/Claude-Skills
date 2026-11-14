// whole-file feature:mcp
package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// getMCPManifest hands out config/mcp.json: every tool /mcp has, its
// arguments, the route it stands for, and the routes that stay HTTP only.
// scripts/api_docs.sh writes it from the registered tools, and
// api_docs_test.go fails while it is out of date. Signed-in callers only,
// like openapi.json.
//
// @route("GET /api/mcp.json", summary="The MCP tools: every tool /mcp has, its arguments, and the route it stands for",
//     tag="Monitoring", feature="mcp", reply_type="application/json",
//     mcp_http="The MCP reference itself; tools/list gives the same list.",
// )
func ( handlers *Handlers ) getMCPManifest( c fiber.Ctx , in *apidoc.None ) ( err error ) {
	if len( handlers.MCPManifest ) == 0 {
		err = notFound( c , "not found" )
		return
	}
	c.Set( fiber.HeaderContentType , fiber.MIMEApplicationJSON )
	err = c.Send( handlers.MCPManifest )
	return
}

// mcpMessage is the body POST /mcp takes, which server/mcp reads itself.
type mcpMessage struct{}

func ( mcpMessage ) APIDoc() ( text string ) { text = "A JSON-RPC 2.0 request."; return }

// serveMCP is the /mcp endpoint (see server/mcp): one JSON-RPC message per
// POST, answered at the key's role.
//
// @route("POST /mcp", summary="Model Context Protocol, at the key's role",
//     tag="Monitoring", access=KEY_ONLY, feature="mcp", errors=(400,), raw_body=mcpMessage,
//     reply_type="application/json",
//     mcp_http="The MCP endpoint itself.",
// )
func ( handlers *Handlers ) serveMCP( c fiber.Ctx , in *apidoc.None ) ( err error ) {
	err = handlers.MCP.Handle( c )
	return
}

// mcpStream and MCPSession answer GET and DELETE on /mcp with 405 and
// nothing else: this server keeps no stream and no sessions.
//
// @route("GET /mcp", summary="Refused: this server has no stream",
//     tag="Monitoring", access=PUBLIC, feature="mcp", status=405,
// )
func ( handlers *Handlers ) mcpStream( c fiber.Ctx , in *apidoc.None ) ( err error ) {
	err = handlers.MCP.MethodNotAllowed( c )
	return
}

// @route("DELETE /mcp", summary="Refused: this server keeps no sessions",
//     tag="Monitoring", access=PUBLIC, feature="mcp", status=405,
// )
func ( handlers *Handlers ) mcpSession( c fiber.Ctx , in *apidoc.None ) ( err error ) {
	err = handlers.MCP.MethodNotAllowed( c )
	return
}
