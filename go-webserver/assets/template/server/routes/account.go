package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

type renameRequest struct {
	DisplayName string `json:"display_name"`
	CSRFToken   string `json:"csrf_token"`
}

// GetMe is the frontend's first call on every page load: it answers "who am
// I, what may I do, and what CSRF token do I echo back". Handing the token
// out here means no page needs a hidden form field or a template variable.
//
// "role" is the *effective* role, not the account's: a request made with a
// user-scoped API key belonging to an admin reports "user", because that is
// what it may actually do. account_role reports the account itself, for the
// rare caller that needs to tell the two apart. Reporting the account's role
// as "role" would have a client believe it has permissions the server will
// refuse to act on.
func ( handlers *Handlers ) GetMe( c fiber.Ctx ) ( err error ) {
	actor := security.ActorFrom( c )
	user := actor.User

	payload := fiber.Map{
		"authenticated":    true,
		"id":               user.ID,
		"display_name":     user.DisplayName,
		"role":             actor.Role,
		"account_role":     user.Role,
		"auth":             actor.Method(),
		"api_keys_enabled": handlers.Config.APIKeysEnabled,
	}
	// Only a session has a CSRF token. A key-authenticated request has no
	// ambient credential to protect, so there is nothing to hand back -- see
	// Guard.CheckCSRF.
	if actor.Session != nil {
		payload[ "csrf_token" ] = actor.Session.CSRFToken
	}
	if actor.APIKey != nil {
		payload[ "api_key_id" ] = actor.APIKey.ID
		payload[ "api_key_name" ] = actor.APIKey.Name
	}
	err = c.JSON( payload )
	return
}

func ( handlers *Handlers ) RenameAccount( c fiber.Ctx ) ( err error ) {
	var body renameRequest
	if c.Bind().Body( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	if models.ValidDisplayName( body.DisplayName ) == false {
		err = badRequest( c , "display name must be 1-80 characters" )
		return
	}

	user := security.UserFrom( c )
	if models.RenameUser( handlers.Store , user.ID , body.DisplayName ) != nil {
		err = serverError( c )
		return
	}
	err = c.JSON( fiber.Map{ "ok": true , "display_name": body.DisplayName } )
	return
}

// Logout deletes the session server-side as well as clearing the cookie, so
// a copy of the cookie captured earlier is dead too.
//
// Called with an API key it does nothing and says so by succeeding: there is
// no session to destroy. A key is ended by revoking it, not by logging out.
func ( handlers *Handlers ) Logout( c fiber.Ctx ) ( err error ) {
	raw := c.Cookies( handlers.Config.CookieName )
	if raw != "" {
		if credential , decrypt_err := handlers.Guard.DecryptCookie( raw ); decrypt_err == nil {
			models.DestroySession( handlers.Store , credential )
		}
	}
	handlers.Guard.ClearSessionCookie( c )
	err = c.JSON( fiber.Map{ "ok": true } )
	return
}
