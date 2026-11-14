// whole-file feature:mcp
package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// GetMCPManifest hands out config/mcp.json: every tool /mcp has, its
// arguments, the route it stands for, and the routes that stay HTTP only.
// scripts/api_docs.sh writes it from the registered tools, and
// api_docs_test.go fails while it is out of date. Signed-in callers only,
// like openapi.json.
func ( handlers *Handlers ) GetMCPManifest() *apidoc.Endpoint[ apidoc.None , apidoc.None ] {
	return apidoc.Stream( apidoc.Op{ Summary: "The MCP tools: every tool /mcp has, its arguments, and the route it stands for" , Tag: "Monitoring" , Feature: "mcp" , MCPHTTP: "The MCP reference itself; tools/list gives the same list." , ReplyType: fiber.MIMEApplicationJSON , Errors: []int{ 401 , 404 } },
		func( c fiber.Ctx , in *apidoc.None ) ( err error ) {
			if len( handlers.MCPManifest ) == 0 {
				err = notFound( c , "not found" )
				return
			}
			c.Set( fiber.HeaderContentType , fiber.MIMEApplicationJSON )
			err = c.Send( handlers.MCPManifest )
			return
		} )
}

// mcpMessage is the body POST /mcp takes, which server/mcp reads itself.
type mcpMessage struct{}

func ( mcpMessage ) APIDoc() ( text string ) { text = "A JSON-RPC 2.0 request."; return }

// ServeMCP is the /mcp endpoint (see server/mcp): one JSON-RPC message per
// POST, answered at the key's role.
func ( handlers *Handlers ) ServeMCP() *apidoc.Endpoint[ apidoc.None , apidoc.None ] {
	return apidoc.Stream( apidoc.Op{ Summary: "Model Context Protocol, at the key's role" , Tag: "Monitoring" , Access: apidoc.KeyOnly , Feature: "mcp" , MCPHTTP: "The MCP endpoint itself." , RawBody: mcpMessage{} , ReplyType: fiber.MIMEApplicationJSON , Errors: []int{ 400 , 401 , 404 } },
		func( c fiber.Ctx , in *apidoc.None ) ( err error ) {
			err = handlers.MCP.Handle( c )
			return
		} )
}

// MCPStream and MCPSession answer GET and DELETE on /mcp with 405 and
// nothing else: this server keeps no stream and no sessions.
func ( handlers *Handlers ) MCPStream() *apidoc.Endpoint[ apidoc.None , apidoc.None ] {
	return apidoc.Stream( apidoc.Op{ Summary: "Refused: this server has no stream" , Tag: "Monitoring" , Access: apidoc.Public , Feature: "mcp" , Status: 405 , Errors: []int{ 404 } },
		func( c fiber.Ctx , in *apidoc.None ) ( err error ) {
			err = handlers.MCP.MethodNotAllowed( c )
			return
		} )
}

func ( handlers *Handlers ) MCPSession() *apidoc.Endpoint[ apidoc.None , apidoc.None ] {
	return apidoc.Stream( apidoc.Op{ Summary: "Refused: this server keeps no sessions" , Tag: "Monitoring" , Access: apidoc.Public , Feature: "mcp" , Status: 405 , Errors: []int{ 404 } },
		func( c fiber.Ctx , in *apidoc.None ) ( err error ) {
			err = handlers.MCP.MethodNotAllowed( c )
			return
		} )
}
