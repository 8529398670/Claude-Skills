// whole-file feature:sessions
package routes

import (
	errors "errors"
	strconv "strconv"
	strings "strings"

	fiber "github.com/gofiber/fiber/v3"

	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

type labelSessionRequest struct {
	Label     string  `json:"label"`
	Expect    *string `json:"expect"`
	CSRFToken string  `json:"csrf_token"`
}

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
	if session.Label != "" {
		row[ "label" ] = session.Label
	}
	return
}

// currentSessionID is the session this request came with, or "" through an
// API key, which has none: nothing in a list is then "current", and nothing
// is refused as "this device".
func currentSessionID( c fiber.Ctx ) ( result string ) {
	if actor := security.ActorFrom( c ); actor.Session != nil { result = actor.Session.ID }
	return
}

// ListMySessions answers "where am I signed in". Only ever the caller's own:
// an admin looks at someone else's through ListUserSessions.
func ( handlers *Handlers ) ListMySessions( c fiber.Ctx ) ( err error ) {
	rows , list_err := handlers.mySessionRows( c )
	if list_err != nil {
		err = serverError( c , list_err )
		return
	}
	err = sendPage( c , "sessions" , rows )
	return
}

// mySessionRows is ListMySessions' list, for it and the MCP list_my_sessions
// tool.
func ( handlers *Handlers ) mySessionRows( c fiber.Ctx ) ( rows []fiber.Map , err error ) {
	actor := security.ActorFrom( c )
	sessions , err := models.ListSessionsForUser( handlers.Store , actor.User.ID )
	if err != nil { return }
	rows = []fiber.Map{}
	for _ , session := range sessions {
		rows = append( rows , sessionRow( session , currentSessionID( c ) ) )
	}
	// >>> feature:logs
	owner_id , err := models.OwnerID( handlers.Store )
	if err != nil { return }
	if actor.User.ID == owner_id {
		if err = handlers.nameSessionRows( rows ); err != nil { return }
	}
	// <<< feature:logs
	return
}

// >>> feature:logs
// nameSessionRows adds what the owner calls each address. Names are the owner's
// own notes about people, so they go to the owner and nobody else.
func ( handlers *Handlers ) nameSessionRows( rows []fiber.Map ) ( err error ) {
	namer , err := models.LoadIPNamer( handlers.Store )
	if err != nil { return }
	for _ , row := range rows {
		ip , _ := row[ "ip" ].( string )
		if name := namer.Name( ip ); name != "" { row[ "ip_name" ] = name }
	}
	return
}
// <<< feature:logs

// sessionsTarget reads whose sessions an admin route is asking about. Nobody but
// the owner reaches the owner's, the same rule as revoking the owner's access.
func ( handlers *Handlers ) sessionsTarget( c fiber.Ctx , user_id uint64 ) ( by_owner bool , refused *refusal ) {
	if _ , get_err := models.GetUser( handlers.Store , user_id ); get_err != nil {
		refused = refuse( fiber.StatusNotFound , "no such user" )
		return
	}
	owner_id , owner_err := models.OwnerID( handlers.Store )
	if owner_err != nil {
		refused = internalFailure( owner_err )
		return
	}
	by_owner = security.UserFrom( c ).ID == owner_id
	if user_id == owner_id && by_owner == false {
		refused = refuse( fiber.StatusForbidden , "the owner account's sessions are the owner's own" )
	}
	return
}

// ListUserSessions shows an admin where an account is signed in, for ending one
// device rather than everything with revoke-access. Addresses are for the owner
// only, as in the activity log, which keeps them from other admins too.
func ( handlers *Handlers ) ListUserSessions( c fiber.Ctx ) ( err error ) {
	user_id , parse_err := strconv.ParseUint( c.Params( "user_id" ) , 10 , 64 )
	if parse_err != nil {
		err = badRequest( c , "invalid user id" )
		return
	}
	rows , refused := handlers.userSessionRows( c , user_id )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = sendPage( c , "sessions" , rows )
	return
}

// userSessionRows is ListUserSessions' list, for it and the MCP
// admin_list_user_sessions tool.
func ( handlers *Handlers ) userSessionRows( c fiber.Ctx , user_id uint64 ) ( rows []fiber.Map , refused *refusal ) {
	by_owner , refused := handlers.sessionsTarget( c , user_id )
	if refused != nil { return }
	sessions , list_err := models.ListSessionsForUser( handlers.Store , user_id )
	if list_err != nil {
		refused = internalFailure( list_err )
		return
	}
	rows = []fiber.Map{}
	for _ , session := range sessions {
		row := sessionRow( session , currentSessionID( c ) )
		if by_owner == false { delete( row , "ip" ) }
		rows = append( rows , row )
	}
	// >>> feature:logs
	if by_owner {
		if name_err := handlers.nameSessionRows( rows ); name_err != nil {
			refused = internalFailure( name_err )
			return
		}
	}
	// <<< feature:logs
	return
}

// RevokeUserSession signs one of an account's devices out, and only that.
func ( handlers *Handlers ) RevokeUserSession( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	user_id , parse_err := strconv.ParseUint( c.Params( "user_id" ) , 10 , 64 )
	if parse_err != nil {
		err = badRequest( c , "invalid user id" )
		return
	}
	// Clone: the log entry outlives this request, and fiber reuses the buffer.
	payload , refused := handlers.revokeUserSession( c , user_id , strings.Clone( c.Params( "session_id" ) ) )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// revokeUserSession is RevokeUserSession after the request is read, for it
// and the MCP admin_revoke_user_session tool.
func ( handlers *Handlers ) revokeUserSession( c fiber.Ctx , user_id uint64 , session_id string ) ( payload fiber.Map , refused *refusal ) {
	if _ , refused = handlers.sessionsTarget( c , user_id ); refused != nil { return }
	if session_id != "" && session_id == currentSessionID( c ) {
		refused = refuse( fiber.StatusBadRequest , "use sign out for this device" )
		return
	}
	revoke_err := models.RevokeSession( handlers.Store , user_id , session_id )
	switch {
	case errors.Is( revoke_err , models.ErrSessionNotFound ):
		refused = refuse( fiber.StatusNotFound , "not found" )
	case revoke_err != nil:
		refused = internalFailure( revoke_err )
	default:
		handlers.audit( c , logs.KindAdmin , "session revoked" , fiber.Map{ "user_id": user_id } )
		payload = fiber.Map{ "ok": true }
	}
	return
}

// LabelMySession names one of your own devices, this one included. An empty
// label clears the name. Admins see a label but cannot set one: it is what the
// person calls their own device.
func ( handlers *Handlers ) LabelMySession( c fiber.Ctx ) ( err error ) {
	var body labelSessionRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.labelMySession( c , c.Params( "session_id" ) , body.Label , body.Expect )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// labelMySession is LabelMySession after the request is read, for it and the
// MCP label_my_session tool.
func ( handlers *Handlers ) labelMySession( c fiber.Ctx , session_id string , label string , expect *string ) ( payload fiber.Map , refused *refusal ) {
	if models.ValidSessionLabel( label ) == false {
		refused = refuse( fiber.StatusBadRequest , "a device name can be at most 80 characters" )
		return
	}
	label_err := models.LabelSession( handlers.Store , security.UserFrom( c ).ID , session_id , label , expect )
	var stale *models.RenameConflictError
	switch {
	case errors.As( label_err , &stale ):
		refused = refuse( fiber.StatusConflict , stale.Error() , fiber.Map{ "label": stale.Current } )
	case errors.Is( label_err , models.ErrSessionNotFound ):
		refused = refuse( fiber.StatusNotFound , "not found" )
	case label_err != nil:
		refused = internalFailure( label_err )
	default:
		// Not the label: it is the person's own words about where they are.
		handlers.audit( c , logs.KindAuth , "session_labelled" , nil )
		payload = fiber.Map{ "ok": true , "label": strings.TrimSpace( label ) }
	}
	return
}

// RevokeMySession signs out one other device. This device is refused: ending
// it here would leave the browser holding a dead cookie, and signing out is
// what the sign-out button is for. Through an API key there is no "this
// device", so any of them may go.
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
	// Clone: the log entry outlives this request, and fiber reuses the buffer.
	payload , refused := handlers.revokeMySession( c , strings.Clone( c.Params( "session_id" ) ) )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// revokeMySession is RevokeMySession after the request is read, for it and
// the MCP revoke_my_session tool.
func ( handlers *Handlers ) revokeMySession( c fiber.Ctx , session_id string ) ( payload fiber.Map , refused *refusal ) {
	if session_id != "" && session_id == currentSessionID( c ) {
		refused = refuse( fiber.StatusBadRequest , "use sign out for this device" )
		return
	}
	revoke_err := models.RevokeSession( handlers.Store , security.UserFrom( c ).ID , session_id )
	switch {
	case errors.Is( revoke_err , models.ErrSessionNotFound ):
		refused = refuse( fiber.StatusNotFound , "not found" )
	case revoke_err != nil:
		refused = internalFailure( revoke_err )
	default:
		handlers.audit( c , logs.KindAuth , "session_revoked" , nil )
		payload = fiber.Map{ "ok": true }
	}
	return
}

// RevokeMyOtherSessions is the button for a lost phone or a shared computer:
// everything but this device. Through an API key, which is no device, that is
// every session the account has.
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
	payload , refused := handlers.revokeMyOtherSessions( c )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// revokeMyOtherSessions is RevokeMyOtherSessions after the request is read,
// for it and the MCP revoke_my_other_sessions tool.
func ( handlers *Handlers ) revokeMyOtherSessions( c fiber.Ctx ) ( payload fiber.Map , refused *refusal ) {
	removed , revoke_err := models.RevokeOtherSessions( handlers.Store , security.UserFrom( c ).ID , currentSessionID( c ) )
	if revoke_err != nil {
		refused = internalFailure( revoke_err )
		return
	}
	handlers.audit( c , logs.KindAuth , "sessions_revoked" , fiber.Map{ "removed": removed } )
	payload = fiber.Map{ "ok": true , "removed": removed }
	return
}
