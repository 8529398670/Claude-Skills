package routes

import (
	errors "errors"
	strings "strings"

	fiber "github.com/gofiber/fiber/v3"

	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// sessionRow picks out what the devices list shows. It names each field,
// rather than returning the record, so the secret's hash and the CSRF token
// cannot reach a response by way of a change to the model.
func sessionRow( session *models.Session , current_id string ) ( row fiber.Map ) {
	row = fiber.Map{
		"id":         session.ID,
		"current":    session.ID == current_id,
		"created_at": session.CreatedAt,
		"expires_at": session.ExpiresAt,
	}
	if session.LastSeenAt != nil {
		row[ "last_seen_at" ] = session.LastSeenAt
	}
	if session.IP != "" {
		row[ "ip" ] = session.IP
	}
	if session.UserAgent != "" {
		row[ "user_agent" ] = session.UserAgent
	}
	return
}

// ListMySessions answers "where am I signed in". Only ever the caller's own:
// an admin's way to end someone else's sessions is revoke-access.
func ( handlers *Handlers ) ListMySessions( c fiber.Ctx ) ( err error ) {
	actor := security.ActorFrom( c )
	sessions , list_err := models.ListSessionsForUser( handlers.Store , actor.User.ID )
	if list_err != nil {
		err = serverError( c , list_err )
		return
	}
	rows := []fiber.Map{}
	for _ , session := range sessions {
		rows = append( rows , sessionRow( session , actor.Session.ID ) )
	}
	err = sendPage( c , "sessions" , rows )
	return
}

// RevokeMySession signs out one other device. This device is refused: ending
// it here would leave the browser holding a dead cookie, and signing out is
// what the sign-out button is for.
func ( handlers *Handlers ) RevokeMySession( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	actor := security.ActorFrom( c )
	// Clone: the log entry outlives this request, and fiber reuses the buffer.
	session_id := strings.Clone( c.Params( "session_id" ) )
	if session_id == actor.Session.ID {
		err = badRequest( c , "use sign out for this device" )
		return
	}
	revoke_err := models.RevokeSession( handlers.Store , actor.User.ID , session_id )
	switch {
	case errors.Is( revoke_err , models.ErrSessionNotFound ):
		err = notFound( c , "not found" )
	case revoke_err != nil:
		err = serverError( c , revoke_err )
	default:
		handlers.audit( c , logs.KindAuth , "session_revoked" , nil )
		err = c.JSON( fiber.Map{ "ok": true } )
	}
	return
}

// RevokeMyOtherSessions is the button for a lost phone or a shared computer:
// everything but this device.
func ( handlers *Handlers ) RevokeMyOtherSessions( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	actor := security.ActorFrom( c )
	removed , revoke_err := models.RevokeOtherSessions( handlers.Store , actor.User.ID , actor.Session.ID )
	if revoke_err != nil {
		err = serverError( c , revoke_err )
		return
	}
	handlers.audit( c , logs.KindAuth , "sessions_revoked" , fiber.Map{ "removed": removed } )
	err = c.JSON( fiber.Map{ "ok": true , "removed": removed } )
	return
}
