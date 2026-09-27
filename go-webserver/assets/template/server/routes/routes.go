// Package routes contains one file per group of endpoints, plus this file,
// which is the only place that knows the full URL map.
//
// The API here stays deliberately small: it covers logging in, managing who
// can log in, issuing the API keys that let non-browser callers in, and
// reading the UI's text. Everything else an app does belongs in a new file in
// this package (server side) and in static/js/ (browser side) -- see
// references/architecture.md for where the line sits and why.
package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	config "{{MODULE_PATH}}/server/config"
	db "{{MODULE_PATH}}/server/db"
	language "{{MODULE_PATH}}/server/language"
	security "{{MODULE_PATH}}/server/security"
	static "{{MODULE_PATH}}/server/static"
)

// Handlers carries the dependencies every route group needs. Passing this in
// rather than reaching for package-level globals is what makes the handlers
// testable and keeps the wiring visible in main.go.
type Handlers struct {
	Store    *db.Store
	Config   *config.Config
	Guard    *security.Guard
	Language *language.Language
	Static   *static.Server
}

func New( store *db.Store , cfg *config.Config , guard *security.Guard , lang *language.Language , static_server *static.Server ) ( handlers *Handlers ) {
	handlers = &Handlers{
		Store:    store,
		Config:   cfg,
		Guard:    guard,
		Language: lang,
		Static:   static_server,
	}
	return
}

// Register wires every route. Read it top to bottom to see the whole surface
// of this server.
func ( handlers *Handlers ) Register( app *fiber.App ) {
	// Resolve the session once, before anything else looks at the request.
	app.Use( handlers.Guard.LoadUser )

	// Public: the login link itself, and the UI's text (needed to render the
	// signed-out page at all).
	app.Get( "/login/*" , handlers.RedeemLoginLink )
	app.Get( "/api/language" , handlers.GetLanguage )
	app.Get( "/api/health" , handlers.GetHealth )

	// Any signed-in user.
	account := app.Group( "/api" , handlers.Guard.RequireLogin )
	account.Get( "/me" , handlers.GetMe )
	account.Post( "/account/rename" , handlers.RenameAccount )
	account.Post( "/logout" , handlers.Logout )
	account.Get( "/team" , handlers.ListTeam )

	// API keys: any signed-in user manages their own.
	//
	// Note the group prefix. A group's middleware is mounted on a path
	// *prefix*, not on the routes written under it, so it applies to every
	// route registered after it whose path starts with that prefix. Mounting
	// these two on "/api" would put RequireSession in front of /api/admin/users
	// as well, and an admin-scoped API key would then be refused there. That is
	// also why the prefix is exactly "/api/keys": a future "/api/keysets" would
	// pick up this middleware by sharing the string, so keep new routes off
	// these prefixes unless they want these rules.
	keys := app.Group( "/api/keys" , handlers.RequireAPIKeys , handlers.Guard.RequireLogin , handlers.Guard.RequireSession )
	keys.Get( "" , handlers.ListMyAPIKeys )
	keys.Post( "" , handlers.CreateMyAPIKey )
	keys.Post( "/:key_id/revoke" , handlers.RevokeMyAPIKey )

	// Admin only: deciding who gets in. Note this is the *only* thing admin
	// rights gate in the base template -- see architecture.md on defaulting
	// new features to "any signed-in user".
	admin := app.Group( "/api/admin" , handlers.Guard.RequireAdmin )
	admin.Get( "/users" , handlers.ListUsers )
	admin.Post( "/users" , handlers.CreateUser )
	admin.Post( "/users/:user_id/reissue-login" , handlers.ReissueLogin )
	admin.Post( "/users/:user_id/disabled" , handlers.SetUserDisabled )

	// Admin view of every key in the app -- "who has standing access" -- and
	// the ability to revoke someone else's. Its own prefix for the same reason
	// as above: mounted on "/api/admin" it would gate the account routes too.
	admin_keys := app.Group( "/api/admin/keys" , handlers.RequireAPIKeys , handlers.Guard.RequireAdmin , handlers.Guard.RequireSession )
	admin_keys.Get( "" , handlers.ListAllAPIKeys )
	admin_keys.Post( "/:key_id/revoke" , handlers.RevokeAnyAPIKey )

	// Catch-all static serving. Must be registered last -- it matches every
	// remaining path, so anything added after this line never runs.
	app.Get( "/*" , handlers.Static.Handler )
}

// badRequest / notFound keep error bodies uniform, so the frontend has one
// shape to handle and no handler accidentally leaks internal detail.
func badRequest( c fiber.Ctx , message string ) ( err error ) {
	err = c.Status( fiber.StatusBadRequest ).JSON( fiber.Map{ "error": message } )
	return
}

func notFound( c fiber.Ctx , message string ) ( err error ) {
	err = c.Status( fiber.StatusNotFound ).JSON( fiber.Map{ "error": message } )
	return
}

func forbidden( c fiber.Ctx , message string ) ( err error ) {
	err = c.Status( fiber.StatusForbidden ).JSON( fiber.Map{ "error": message } )
	return
}

func serverError( c fiber.Ctx ) ( err error ) {
	err = c.Status( fiber.StatusInternalServerError ).JSON( fiber.Map{ "error": "internal error" } )
	return
}
