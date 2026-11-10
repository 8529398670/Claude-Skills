// whole-file feature:mcp
package routes

import (
	fiber "github.com/gofiber/fiber/v3"
)

// GetMCPManifest hands out config/mcp.json: every tool /mcp has, its
// arguments, the route it stands for, and the routes that stay HTTP only.
// scripts/mcp_json.sh writes it from the registered tools, and
// mcp_manifest_test.go fails while it is out of date. Signed-in callers only,
// like openapi.json.
func ( handlers *Handlers ) GetMCPManifest( c fiber.Ctx ) ( err error ) {
	if len( handlers.MCPManifest ) == 0 {
		err = notFound( c , "not found" )
		return
	}
	c.Set( fiber.HeaderContentType , fiber.MIMEApplicationJSON )
	err = c.Send( handlers.MCPManifest )
	return
}
