// whole-file feature:openapi
package routes

import (
	fiber "github.com/gofiber/fiber/v3"
)

// GetOpenAPI hands out config/openapi.json, which scripts/api_docs.sh writes
// from the routes and the docs each handler declares (the *_api.go files);
// api_docs_test.go fails while it is out of date. Signed-in callers only: it
// maps the admin surface, and nobody who cannot sign in has a use for it.
func ( handlers *Handlers ) GetOpenAPI( c fiber.Ctx ) ( err error ) {
	if len( handlers.OpenAPI ) == 0 {
		err = notFound( c , "not found" )
		return
	}
	c.Set( fiber.HeaderContentType , fiber.MIMEApplicationJSON )
	err = c.Send( handlers.OpenAPI )
	return
}
