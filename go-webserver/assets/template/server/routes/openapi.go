// whole-file feature:openapi
package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// GetOpenAPI hands out config/openapi.json, which scripts/api_docs.sh writes
// from the routes as each declares itself (apidoc.Handle, next to its
// handler); api_docs_test.go fails while it is out of date. Signed-in callers
// only: it maps the admin surface, and nobody who cannot sign in has a use for
// it.
func ( handlers *Handlers ) GetOpenAPI() *apidoc.Endpoint[ apidoc.None , apidoc.None ] {
	return apidoc.Stream( apidoc.Op{ Summary: "This document" , Tag: "Monitoring" , Feature: "openapi" , MCPHTTP: "The API reference itself; GET /api/mcp.json is the MCP one." , ReplyType: fiber.MIMEApplicationJSON , Errors: []int{ 401 , 404 } },
		func( c fiber.Ctx , in *apidoc.None ) ( err error ) {
			if len( handlers.OpenAPI ) == 0 {
				err = notFound( c , "not found" )
				return
			}
			c.Set( fiber.HeaderContentType , fiber.MIMEApplicationJSON )
			err = c.Send( handlers.OpenAPI )
			return
		} )
}
