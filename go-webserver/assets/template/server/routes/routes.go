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
	models "{{MODULE_PATH}}/server/models"
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
	Pins     *models.PinGuard
}

func New( store *db.Store , cfg *config.Config , guard *security.Guard , lang *language.Language , static_server *static.Server ) ( handlers *Handlers ) {
	handlers = &Handlers{
		Store:    store,
		Config:   cfg,
		Guard:    guard,
		Language: lang,
		Static:   static_server,
		Pins:     models.NewPinGuard(),
	}
	return
}

// Register wires every route. Read it top to bottom to see the whole surface
// of this server.
func ( handlers *Handlers ) Register( app *fiber.App ) {
	// Resolve the session once, before anything else looks at the request.
	app.Use( handlers.Guard.LoadUser )

	// Public: the login link's page and the call it makes, and the UI's text
	// (needed to render the signed-out page at all). Opening a login link
	// only shows a page; the link is spent by the POST that page sends when
	// the person confirms. See auth.go.
	app.Get( "/login/*" , handlers.ShowLoginPage )
	app.Post( "/api/login" , handlers.RedeemLoginLink )
	app.Get( "/api/language" , handlers.GetLanguage )
	app.Get( "/api/health" , handlers.GetHealth )

	// Public: invite links (see join.go). Holding the link is the permission,
	// so these sit before the signed-in "/api" group below -- registered
	// after it, they would pick up RequireLogin from the shared prefix.
	app.Get( "/join/*" , handlers.ShowJoinPage )
	app.Post( "/api/join/info" , handlers.JoinInfo )
	app.Post( "/api/join/match" , handlers.JoinMatch )
	app.Post( "/api/join/claim" , handlers.JoinClaim )
	app.Post( "/api/join/create" , handlers.JoinCreate )

	// Any signed-in user.
	account := app.Group( "/api" , handlers.Guard.RequireLogin )
	account.Get( "/me" , handlers.GetMe )
	account.Post( "/account/rename" , handlers.RenameAccount )
	// A session only: a PIN lets its holder sign in through an invite link,
	// so a key that could set one could turn itself into a session.
	account.Post( "/account/pin" , handlers.Guard.RequireSession , handlers.SetPin )
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
	keys.Post( "/:key_id/delete" , handlers.DeleteMyAPIKey )

	// Admin only: deciding who gets in. Note this is the *only* thing admin
	// rights gate in the base template -- see architecture.md on defaulting
	// new features to "any signed-in user".
	//
	// The two routes that hand out a login link also require a browser
	// session, attached per route rather than on a group prefix. A login link
	// becomes a session, and a session can mint API keys -- so if an
	// admin-scoped key could create accounts or reissue links, a leaked key
	// could turn itself into a fresh session and a fresh key, and revoking
	// the leaked one would not end the compromise. The rule is: an API key
	// may use what an account can do, but it never creates credentials.
	//
	// Deleting an account requires a session too, for a different reason: it
	// cannot be undone. A leaked admin key can disable accounts, which an
	// admin can reverse; it must not be able to erase them. Who may delete
	// whom (the owner rules) is enforced in models.DeleteUser.
	admin := app.Group( "/api/admin" , handlers.Guard.RequireAdmin )
	admin.Get( "/users" , handlers.ListUsers )
	admin.Post( "/users" , handlers.Guard.RequireSession , handlers.CreateUser )
	admin.Post( "/users/:user_id/reissue-login" , handlers.Guard.RequireSession , handlers.ReissueLogin )
	admin.Post( "/users/:user_id/disabled" , handlers.SetUserDisabled )
	admin.Post( "/users/:user_id/revoke-access" , handlers.RevokeAccess )
	admin.Post( "/users/:user_id/delete" , handlers.Guard.RequireSession , handlers.DeleteUser )

	// Invite links: one link for a group, each person who opens it joins as a
	// new user (see invite_links.go). Creating one needs a session for the
	// same reason as creating a user -- the link mints accounts and sessions.
	// Deleting one needs a session like deleting a user. Listing and
	// switching one off are safe for an admin key.
	admin.Get( "/invite-links" , handlers.ListInviteLinks )
	admin.Post( "/invite-links" , handlers.Guard.RequireSession , handlers.CreateInviteLink )
	admin.Post( "/invite-links/:link_id/revoke" , handlers.RevokeInviteLink )
	admin.Post( "/invite-links/:link_id/delete" , handlers.Guard.RequireSession , handlers.DeleteInviteLink )

	// Admin view of every key in the app -- "who has standing access" -- and
	// the ability to revoke or delete someone else's. Its own prefix for the
	// same reason as above: mounted on "/api/admin" it would gate the account
	// routes too.
	admin_keys := app.Group( "/api/admin/keys" , handlers.RequireAPIKeys , handlers.Guard.RequireAdmin , handlers.Guard.RequireSession )
	admin_keys.Get( "" , handlers.ListAllAPIKeys )
	admin_keys.Post( "/:key_id/revoke" , handlers.RevokeAnyAPIKey )
	admin_keys.Post( "/:key_id/delete" , handlers.DeleteAnyAPIKey )

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
