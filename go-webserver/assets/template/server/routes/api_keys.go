package routes

import (
	errors "errors"

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// API keys are how anything that is not a browser calls this API: a script, a
// cron job, a CI pipeline, another service. A key carries a role from the same
// set users do, so there is one permission model rather than two -- see
// server/models/api_key.go and references/architecture.md.
//
// Every signed-in user can mint their own keys. An admin can additionally see
// and revoke everyone's, which is what makes "who has standing access to this
// app" an answerable question.
//
// Minting and revoking require a browser session (the group in routes.go adds
// Guard.RequireSession), so a leaked key cannot quietly mint its replacement.

type createAPIKeyRequest struct {
	Name string `json:"name"`
	Role string `json:"role"`
	// ExpiresInDays is a pointer so that three cases stay distinguishable:
	// absent means "use the configured default", 0 means "never expires", and
	// a number means that many days. With a plain int, "never" and "unset"
	// would be the same request.
	ExpiresInDays *int   `json:"expires_in_days"`
	CSRFToken     string `json:"csrf_token"`
}

// RequireAPIKeys hides the key-management endpoints when the feature is
// switched off, so an app with no programmatic callers can close the surface
// entirely. The Authorization header is ignored in that state too -- that part
// is enforced in server/security, not here, because it has to hold for every
// route rather than only these.
func ( handlers *Handlers ) RequireAPIKeys( c fiber.Ctx ) ( err error ) {
	if handlers.Config.APIKeysEnabled == false {
		err = notFound( c , "api keys are disabled on this server" )
		return
	}
	err = c.Next()
	return
}

// apiKeyRow is the only shape a key is ever returned in. The secret hash is
// not in it, and nor is the credential -- that exists in one response, from
// CreateAPIKey, and is never recoverable afterwards.
func apiKeyRow( key *models.APIKey , display_name string ) ( row fiber.Map ) {
	row = fiber.Map{
		"id":         key.ID,
		"user_id":    key.UserID,
		"name":       key.Name,
		"role":       key.Role,
		"created_at": key.CreatedAt,
		"revoked":    key.Revoked(),
		"expired":    key.Expired(),
		"live":       key.Live(),
	}
	if key.ExpiresAt != nil {
		row[ "expires_at" ] = key.ExpiresAt
	}
	if key.LastUsedAt != nil {
		row[ "last_used_at" ] = key.LastUsedAt
	}
	if key.RevokedAt != nil {
		row[ "revoked_at" ] = key.RevokedAt
	}
	if display_name != "" {
		row[ "display_name" ] = display_name
	}
	return
}

// ListMyAPIKeys answers "what has standing access as me". Revoked and expired
// keys are included, marked as such, because a key that used to exist is part
// of that answer.
func ( handlers *Handlers ) ListMyAPIKeys( c fiber.Ctx ) ( err error ) {
	user := security.UserFrom( c )
	keys , list_err := models.ListAPIKeysForUser( handlers.Store , user.ID )
	if list_err != nil {
		err = serverError( c )
		return
	}
	rows := []fiber.Map{}
	for _ , key := range keys {
		rows = append( rows , apiKeyRow( key , "" ) )
	}
	err = c.JSON( rows )
	return
}

// CreateMyAPIKey mints a key for the signed-in user and returns the credential
// once. The default role is "user" even for an admin: an admin-scoped key can
// create accounts and mint login links, so it should be asked for rather than
// handed out by omission.
//
// The ceiling that a key cannot outrank its owner lives in
// models.IssueAPIKey, not here, so the control socket and the CLI get it too.
func ( handlers *Handlers ) CreateMyAPIKey( c fiber.Ctx ) ( err error ) {
	var body createAPIKeyRequest
	if c.Bind().Body( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	if models.ValidAPIKeyName( body.Name ) == false {
		err = badRequest( c , "name must be 1-80 characters" )
		return
	}

	role := body.Role
	if role == "" {
		role = models.RoleUser
	}
	if models.ValidRole( role ) == false {
		err = badRequest( c , "role must be admin or user" )
		return
	}

	ttl , ok := models.APIKeyTTLFromDays( body.ExpiresInDays , handlers.Config.APIKeyTTL )
	if ok == false {
		err = badRequest( c , "expires_in_days must be 0 (never) or up to 3650" )
		return
	}

	user := security.UserFrom( c )
	credential , key , issue_err := models.IssueAPIKey( handlers.Store , user.ID , body.Name , role , ttl )
	if issue_err != nil {
		switch {
		case errors.Is( issue_err , models.ErrAPIKeyRole ):
			err = forbidden( c , "you cannot create an API key with more permission than your own account" )
		case errors.Is( issue_err , models.ErrAPIKeyLimit ):
			err = badRequest( c , "you already have the maximum number of live API keys -- revoke one first" )
		case errors.Is( issue_err , models.ErrAPIKeyDisabled ):
			// Not reachable through a live session today, since a disabled
			// account cannot hold one. Mapped anyway so that a future path
			// into this handler fails as a refusal rather than a 500.
			err = forbidden( c , "this account is disabled" )
		default:
			err = serverError( c )
		}
		return
	}

	// The credential appears here and nowhere else, ever.
	row := apiKeyRow( key , "" )
	row[ "credential" ] = credential
	err = c.Status( fiber.StatusCreated ).JSON( row )
	return
}

// RevokeMyAPIKey takes effect immediately -- the next request carrying that
// key fails. Someone else's key is reported as "no such api key" rather than
// "forbidden", so the endpoint does not confirm which key ids exist.
func ( handlers *Handlers ) RevokeMyAPIKey( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().Body( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}

	user := security.UserFrom( c )
	key , get_err := models.GetAPIKey( handlers.Store , c.Params( "key_id" ) )
	if get_err != nil || key.UserID != user.ID {
		err = notFound( c , "no such api key" )
		return
	}
	if models.RevokeAPIKey( handlers.Store , key.ID ) != nil {
		err = serverError( c )
		return
	}
	err = c.JSON( fiber.Map{ "ok": true } )
	return
}

// ListAllAPIKeys is the admin view: every key in the app, with whose it is.
// This is the "who and what has standing access" question, and it is the
// reason admins can see keys they did not create.
func ( handlers *Handlers ) ListAllAPIKeys( c fiber.Ctx ) ( err error ) {
	keys , list_err := models.ListAPIKeys( handlers.Store )
	if list_err != nil {
		err = serverError( c )
		return
	}
	users , users_err := models.ListUsers( handlers.Store )
	if users_err != nil {
		err = serverError( c )
		return
	}

	// One pass to build the lookup rather than a GetUser per key: a bucket
	// read per row is the shape that quietly becomes slow.
	names := map[ uint64 ]string{}
	for _ , user := range users {
		names[ user.ID ] = user.DisplayName
	}

	rows := []fiber.Map{}
	for _ , key := range keys {
		rows = append( rows , apiKeyRow( key , names[ key.UserID ] ) )
	}
	err = c.JSON( rows )
	return
}

// RevokeAnyAPIKey is how an admin kills a credential belonging to someone
// else -- a laptop that went missing, a CI system being decommissioned, a
// contractor who has finished.
func ( handlers *Handlers ) RevokeAnyAPIKey( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().Body( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}

	key , get_err := models.GetAPIKey( handlers.Store , c.Params( "key_id" ) )
	if get_err != nil {
		err = notFound( c , "no such api key" )
		return
	}
	if models.RevokeAPIKey( handlers.Store , key.ID ) != nil {
		err = serverError( c )
		return
	}
	err = c.JSON( fiber.Map{ "ok": true } )
	return
}
