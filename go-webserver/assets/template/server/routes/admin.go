package routes

import (
	errors "errors"
	strconv "strconv"
	strings "strings"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	db "{{MODULE_PATH}}/server/db"
	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

type createUserRequest struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	CSRFToken   string `json:"csrf_token"`
	models.LoginLinkRequest
}

// loginLinkBody is the body of every route that mints a login link for an
// existing account: how many times it may be used, and for how long. Both
// optional (see models.LoginLinkRequest).
type loginLinkBody struct {
	CSRFToken string `json:"csrf_token"`
	models.LoginLinkRequest
}

type csrfOnlyRequest struct {
	CSRFToken string `json:"csrf_token"`
}

type renameUserRequest struct {
	DisplayName string  `json:"display_name"`
	Expect      *string `json:"expect"`
	CSRFToken   string  `json:"csrf_token"`
}

type setRoleRequest struct {
	Role      string `json:"role"`
	CSRFToken string `json:"csrf_token"`
}

type setDisabledRequest struct {
	Disabled  bool   `json:"disabled"`
	CSRFToken string `json:"csrf_token"`
}

// ListUsers marks the owner on its row, so the UI can tell which delete,
// disable, and revoke buttons the server would refuse -- see models/owner.go.
//
// ?q= keeps the accounts whose name or username contains it, in any case. Here
// and not in the browser, which only ever holds one page of them.
func ( handlers *Handlers ) ListUsers( c fiber.Ctx ) ( err error ) {
	rows , list_err := handlers.userRowsMatching( c.Query( "q" ) )
	if list_err != nil {
		err = serverError( c , list_err )
		return
	}
	err = sendSorted( c , "users" , rows , "display_name" , "role" , "disabled" , "created_at" )
	return
}

// >>> feature:mcp
// userRows is shared with the MCP admin_list_users tool.
func ( handlers *Handlers ) userRows() ( rows []fiber.Map , err error ) {
	rows , err = handlers.userRowsMatching( "" )
	return
}
// <<< feature:mcp

func ( handlers *Handlers ) userRowsMatching( query string ) ( rows []fiber.Map , err error ) {
	users , err := models.ListUsers( handlers.Store )
	if err != nil { return }
	owner_id , err := models.OwnerID( handlers.Store )
	if err != nil { return }
	links , err := models.LiveLoginLinks( handlers.Store )
	if err != nil { return }
	query = strings.ToLower( strings.TrimSpace( query ) )
	rows = []fiber.Map{}
	for _ , user := range users {
		matched := strings.Contains( strings.ToLower( user.DisplayName ) , query )
		matched = matched || strings.Contains( strings.ToLower( user.Username ) , query ) // feature:passwords
		if matched == false { continue }
		var login_link *models.LoginLinkInfo
		if found , has := links[ user.ID ]; has { login_link = &found }
		rows = append( rows , fiber.Map{
			"id":           user.ID,
			"display_name": user.DisplayName,
			"username":     user.Username, // feature:passwords
			"role":         user.Role,
			"created_at":   user.CreatedAt,
			"disabled":     user.Disabled(),
			"owner":        user.ID == owner_id,
			"has_password":     user.HasPassword(), // feature:passwords
			"password_blocked": user.PasswordBlocked, // feature:passwords
			"two_factor":       user.TwoFactorOn(), // feature:two_factor
			"login_link":       login_link,
		} )
	}
	return
}

// loginLinkOptions fills a request's gaps from config and checks it, and notes
// the API key asking, if one is, so revoking that key voids the link.
func ( handlers *Handlers ) loginLinkOptions( c fiber.Ctx , request models.LoginLinkRequest ) ( options models.LoginLinkOptions , refused *refusal ) {
	options , resolve_err := models.ResolveLoginLink( request , handlers.cfg().LoginTokenTTL , handlers.cfg().MultiUseLoginTokenTTL )
	if resolve_err != nil {
		refused = refuse( fiber.StatusBadRequest , resolve_err.Error() )
		return
	}
	options.MintedByKey = security.ActorFrom( c ).KeyID()
	return
}

// loginLinkResponse is what every route that mints a link answers. The link
// is returned as a path, not a full URL (see CreateUser); the MCP tools add
// login_url from the address the agent reached /mcp at.
func loginLinkResponse( credential string , info models.LoginLinkInfo ) ( payload fiber.Map ) {
	expires_in := 0
	if info.ExpiresAt != nil { expires_in = int( time.Until( *info.ExpiresAt ).Seconds() + 0.5 ) }
	payload = fiber.Map{
		"login_path":         "/login/" + credential,
		"expires_in_seconds": expires_in,
		"expires_at":         info.ExpiresAt,
		"max_uses":           info.MaxUses,
		"unlimited":          info.Unlimited,
	}
	return
}

// CreateUser is the signup process: an admin names someone, and gets back a
// login link to hand them -- once by default, or reusable (see
// models.LoginLinkRequest). The link is returned as a path, not a full URL
// -- the browser knows the origin it is talking to, whereas the server behind
// a reverse proxy often does not, and a login link with the wrong host is
// worse than useless.
func ( handlers *Handlers ) CreateUser( c fiber.Ctx ) ( err error ) {
	var body createUserRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.createUser( c , body.DisplayName , body.Role , body.LoginLinkRequest )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.Status( fiber.StatusCreated ).JSON( payload )
	return
}

// createUser is CreateUser after the request is read, for it and the MCP
// admin_create_user tool.
func ( handlers *Handlers ) createUser( c fiber.Ctx , display_name string , role string , request models.LoginLinkRequest ) ( payload fiber.Map , refused *refusal ) {
	if models.ValidDisplayName( display_name ) == false {
		refused = refuse( fiber.StatusBadRequest , "display name must be 1-80 characters" )
		return
	}
	if models.ValidRole( role ) == false {
		refused = refuse( fiber.StatusBadRequest , "role must be admin or user" )
		return
	}
	options , refused := handlers.loginLinkOptions( c , request )
	if refused != nil { return }

	user , create_err := models.CreateUser( handlers.Store , display_name , role )
	if create_err != nil {
		refused = internalFailure( create_err )
		return
	}
	credential , info , token_err := models.IssueLoginLink( handlers.Store , user.ID , options )
	if token_err != nil {
		refused = internalFailure( token_err )
		return
	}

	handlers.audit( c , logs.KindAdmin , "user created" , fiber.Map{ "user_id": user.ID , "name": user.DisplayName , "role": user.Role } )
	handlers.Webhooks.Emit( models.EventUserCreated , map[string]any{ "user_id": user.ID , "display_name": user.DisplayName , "role": user.Role } ) // feature:webhooks
	payload = loginLinkResponse( credential , info )
	payload[ "user_id" ] = user.ID
	return
}

// ReissueLogin covers the only real failure mode of this auth model: someone
// lost their cookie or their device, and there is no password reset to fall
// back on. An admin mints a fresh link instead. Any link the account could
// still sign in with stops working (see models.IssueLoginLink).
//
// It is not a reset: sessions the account already has stay signed in. When
// the worry is that someone else holds the account's credentials, use
// RevokeAccess first, then reissue.
func ( handlers *Handlers ) ReissueLogin( c fiber.Ctx ) ( err error ) {
	var body loginLinkBody
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
	payload , refused := handlers.reissueLogin( c , user_id , body.LoginLinkRequest )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// reissueLogin is ReissueLogin after the request is read, for it and the MCP
// admin_reissue_login tool.
func ( handlers *Handlers ) reissueLogin( c fiber.Ctx , user_id uint64 , request models.LoginLinkRequest ) ( payload fiber.Map , refused *refusal ) {
	if _ , get_err := models.GetUser( handlers.Store , user_id ); get_err != nil {
		refused = refuse( fiber.StatusNotFound , "no such user" )
		return
	}
	options , refused := handlers.loginLinkOptions( c , request )
	if refused != nil { return }

	credential , info , token_err := models.IssueLoginLink( handlers.Store , user_id , options )
	if token_err != nil {
		refused = internalFailure( token_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "login link reissued" , fiber.Map{ "user_id": user_id , "max_uses": info.MaxUses , "expires_at": info.ExpiresAt } )
	payload = loginLinkResponse( credential , info )
	return
}

// CancelLoginLink stops an account's live login link working, without
// making a new one: a reusable link that went somewhere it should not have.
// Sessions it already made stay signed in; RevokeAccess ends those too.
//
// An API key may do this, like RevokeAccess: it only takes a way in away.
func ( handlers *Handlers ) CancelLoginLink( c fiber.Ctx ) ( err error ) {
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
	payload , refused := handlers.cancelLoginLink( c , user_id )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// cancelLoginLink is CancelLoginLink after the request is read, for it and the
// MCP admin_cancel_login_link tool.
func ( handlers *Handlers ) cancelLoginLink( c fiber.Ctx , user_id uint64 ) ( payload fiber.Map , refused *refusal ) {
	if _ , get_err := models.GetUser( handlers.Store , user_id ); get_err != nil {
		refused = refuse( fiber.StatusNotFound , "no such user" )
		return
	}
	removed , cancel_err := models.CancelLoginLink( handlers.Store , user_id )
	if cancel_err != nil {
		refused = internalFailure( cancel_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "login link cancelled" , fiber.Map{ "user_id": user_id , "removed": removed } )
	payload = fiber.Map{ "ok": true , "removed": removed }
	return
}

// RenameUser is an admin renaming someone else. Who may rename whom lives in
// models.RenameUserAs.
func ( handlers *Handlers ) RenameUser( c fiber.Ctx ) ( err error ) {
	var body renameUserRequest
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
	payload , refused := handlers.renameUser( c , user_id , body.DisplayName , body.Expect )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// renameUser is RenameUser after the request is read, for it and the MCP
// admin_rename_user tool.
func ( handlers *Handlers ) renameUser( c fiber.Ctx , user_id uint64 , display_name string , expect *string ) ( payload fiber.Map , refused *refusal ) {
	if models.ValidDisplayName( display_name ) == false {
		refused = refuse( fiber.StatusBadRequest , "display name must be 1-80 characters" )
		return
	}
	rename_err := models.RenameUserAs( handlers.Store , user_id , security.UserFrom( c ).ID , display_name , expect )
	var stale *models.RenameConflictError
	switch {
	case errors.As( rename_err , &stale ):
		refused = refuse( fiber.StatusConflict , stale.Error() , fiber.Map{ "display_name": stale.Current } )
		return
	case errors.Is( rename_err , db.ErrNotFound ):
		refused = refuse( fiber.StatusNotFound , "no such user" )
		return
	case errors.Is( rename_err , models.ErrOwnerRequired ):
		refused = refuse( fiber.StatusForbidden , "only the owner can rename an admin" )
		return
	case rename_err != nil:
		refused = internalFailure( rename_err )
		return
	}
	name := strings.TrimSpace( display_name )
	handlers.audit( c , logs.KindAdmin , "user renamed" , fiber.Map{ "user_id": user_id , "name": name } )
	payload = fiber.Map{ "ok": true , "display_name": name }
	return
}

// SetUserRole makes an account an admin or an ordinary user. Who may do what
// lives in models.SetUserRole.
func ( handlers *Handlers ) SetUserRole( c fiber.Ctx ) ( err error ) {
	var body setRoleRequest
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
	payload , refused := handlers.setUserRole( c , user_id , body.Role )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// setUserRole is SetUserRole after the request is read, for it and the MCP
// admin_set_user_role tool.
func ( handlers *Handlers ) setUserRole( c fiber.Ctx , user_id uint64 , role string ) ( payload fiber.Map , refused *refusal ) {
	if models.ValidRole( role ) == false {
		refused = refuse( fiber.StatusBadRequest , "role must be admin or user" )
		return
	}
	changed , role_err := models.SetUserRole( handlers.Store , user_id , security.UserFrom( c ).ID , role )
	switch {
	case errors.Is( role_err , db.ErrNotFound ):
		refused = refuse( fiber.StatusNotFound , "no such user" )
		return
	case errors.Is( role_err , models.ErrRoleSelf ):
		refused = refuse( fiber.StatusBadRequest , role_err.Error() )
		return
	case errors.Is( role_err , models.ErrOwnerProtected ):
		refused = refuse( fiber.StatusForbidden , "the owner is always an admin" )
		return
	case errors.Is( role_err , models.ErrOwnerRequired ):
		refused = refuse( fiber.StatusForbidden , "only the owner can take away an admin's rights" )
		return
	case role_err != nil:
		refused = internalFailure( role_err )
		return
	}
	if changed {
		handlers.audit( c , logs.KindAdmin , "user role changed" , fiber.Map{ "user_id": user_id , "role": role } )
	}
	payload = fiber.Map{ "ok": true , "role": role }
	return
}

func ( handlers *Handlers ) SetUserDisabled( c fiber.Ctx ) ( err error ) {
	var body setDisabledRequest
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
	if refused := handlers.setUserDisabled( security.UserFrom( c ).ID , user_id , body.Disabled ); refused != nil {
		err = refused.send( c )
		return
	}
	handlers.audit( c , logs.KindAdmin , "user disabled set" , fiber.Map{ "user_id": user_id , "disabled": body.Disabled } )
	err = c.JSON( fiber.Map{ "ok": true } )
	return
}

// RevokeAccess is the response to "this account may be compromised": every
// session, live login link, and live API key it holds stops working at
// once. The account itself stays enabled; reissue a login link to let its
// owner back in. See models.RevokeAllCredentials for why this is separate
// from both disabling and reissuing.
func ( handlers *Handlers ) RevokeAccess( c fiber.Ctx ) ( err error ) {
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
	revoked , refused := handlers.revokeAccess( security.UserFrom( c ).ID , user_id )
	if refused != nil {
		err = refused.send( c )
		return
	}
	handlers.audit( c , logs.KindAdmin , "access revoked" , fiber.Map{ "user_id": user_id } )
	err = c.JSON( fiber.Map{ "ok": true , "revoked": revoked } )
	return
}

// >>> feature:two_factor
// ResetTwoFactor is for someone who lost both the phone and the recovery codes.
// Who may reset whom lives in models.ResetTwoFactor.
func ( handlers *Handlers ) ResetTwoFactor( c fiber.Ctx ) ( err error ) {
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
	payload , refused := handlers.resetTwoFactor( c , user_id )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// resetTwoFactor is ResetTwoFactor after the request is read, for it and the
// MCP admin_reset_two_factor tool.
func ( handlers *Handlers ) resetTwoFactor( c fiber.Ctx , user_id uint64 ) ( payload fiber.Map , refused *refusal ) {
	reset_err := models.ResetTwoFactor( handlers.Store , user_id , security.UserFrom( c ).ID )
	switch {
	case errors.Is( reset_err , db.ErrNotFound ):
		refused = refuse( fiber.StatusNotFound , "no such user" )
		return
	case errors.Is( reset_err , models.ErrResetSelf ):
		refused = refuse( fiber.StatusBadRequest , "you cannot reset your own two-factor sign-in" )
		return
	case errors.Is( reset_err , models.ErrOwnerProtected ):
		refused = refuse( fiber.StatusForbidden , "the owner's two-factor sign-in can only be reset from a shell" )
		return
	case errors.Is( reset_err , models.ErrOwnerRequired ):
		refused = refuse( fiber.StatusForbidden , "only the owner can reset an admin's two-factor sign-in" )
		return
	case reset_err != nil:
		refused = internalFailure( reset_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "two-factor reset" , fiber.Map{ "user_id": user_id } )
	payload = fiber.Map{ "ok": true }
	return
}
// <<< feature:two_factor

// DeleteUser removes an account for good, together with every session, login
// link, and API key it held. Disabling is the reversible version; this is for
// an account that should not exist at all.
//
// Any admin may delete an ordinary user. Deleting an admin takes the owner,
// and nobody deletes the owner or themselves -- those rules live in
// models.DeleteUser, checked inside its transaction. An admin-scoped API key
// may do it too, and the MCP tool is marked destructive so a client asks
// first.
func ( handlers *Handlers ) DeleteUser( c fiber.Ctx ) ( err error ) {
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
	payload , refused := handlers.deleteUser( c , user_id )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// deleteUser is DeleteUser after the request is read, for it and the MCP
// admin_delete_user tool.
func ( handlers *Handlers ) deleteUser( c fiber.Ctx , user_id uint64 ) ( payload fiber.Map , refused *refusal ) {
	removed , delete_err := models.DeleteUser( handlers.Store , user_id , security.UserFrom( c ).ID )
	switch {
	case delete_err == nil:
		handlers.audit( c , logs.KindAdmin , "user deleted" , fiber.Map{ "user_id": user_id } )
		handlers.Webhooks.Emit( models.EventUserDeleted , map[string]any{ "user_id": user_id } ) // feature:webhooks
		payload = fiber.Map{ "ok": true , "removed": removed }
	case errors.Is( delete_err , db.ErrNotFound ):
		refused = refuse( fiber.StatusNotFound , "no such user" )
	case errors.Is( delete_err , models.ErrDeleteSelf ):
		refused = refuse( fiber.StatusBadRequest , "you cannot delete your own account" )
	case errors.Is( delete_err , models.ErrOwnerProtected ):
		refused = refuse( fiber.StatusForbidden , "the owner account cannot be deleted" )
	case errors.Is( delete_err , models.ErrOwnerRequired ):
		refused = refuse( fiber.StatusForbidden , "only the owner can delete an admin account" )
	default:
		refused = internalFailure( delete_err )
	}
	return
}

// refusal is a rule saying no, kept apart from how it is delivered: the HTTP
// handlers send it as a status and an error body, the MCP tools as a tool
// error with the same words. Writing the rule once is what keeps the two
// doors from drifting apart.
type refusal struct {
	status  int
	message string
	// cause is why a 5xx happened. It goes to the log, never to the caller.
	cause error
	// extra is more for the caller beside the message, such as the current
	// value a 409 was about.
	extra fiber.Map
}

func ( refused *refusal ) send( c fiber.Ctx ) ( err error ) {
	if refused.status >= 500 {
		err = serverError( c , refused.cause )
		return
	}
	body := fiber.Map{ "error": refused.message }
	for name , value := range refused.extra {
		body[ name ] = value
	}
	err = c.Status( refused.status ).JSON( body )
	return
}

// refuse is a refusal with the caller's mistake in it. internalFailure is
// ours: a 500 whose cause goes to the log.
func refuse( status int , message string , extra ...fiber.Map ) ( refused *refusal ) {
	refused = &refusal{ status: status , message: message }
	if len( extra ) > 0 { refused.extra = extra[ 0 ] }
	return
}

func internalFailure( cause error ) ( refused *refusal ) {
	refused = &refusal{ status: fiber.StatusInternalServerError , message: "internal error" , cause: cause }
	return
}

// setUserDisabled carries every rule for switching an account off or on, for
// SetUserDisabled and the MCP admin_set_user_disabled tool.
func ( handlers *Handlers ) setUserDisabled( self_id uint64 , user_id uint64 , disabled bool ) ( refused *refusal ) {
	if _ , get_err := models.GetUser( handlers.Store , user_id ); get_err != nil {
		refused = &refusal{ status: fiber.StatusNotFound , message: "no such user" }
		return
	}

	// Locking yourself out is unrecoverable without shell access, so it is
	// worth one guard rail even though an admin could still disable a peer.
	if disabled && user_id == self_id {
		refused = &refusal{ status: fiber.StatusBadRequest , message: "you cannot disable your own account" }
		return
	}

	// Disabling also deletes the account's sessions and live login links,
	// in the same transaction -- see models.SetUserDisabled. The owner cannot
	// be disabled; that check is in the model so every route gets it.
	if disable_err := models.SetUserDisabled( handlers.Store , user_id , disabled ); disable_err != nil {
		if errors.Is( disable_err , models.ErrOwnerProtected ) {
			refused = &refusal{ status: fiber.StatusForbidden , message: "the owner account cannot be disabled" }
			return
		}
		refused = &refusal{ status: fiber.StatusInternalServerError , message: "internal error" , cause: disable_err }
		return
	}
	handlers.Webhooks.Emit( models.EventUserDisabled , map[string]any{ "user_id": user_id , "disabled": disabled } ) // feature:webhooks
	return
}

// revokeAccess carries every rule for cutting an account's credentials off,
// for RevokeAccess and the MCP admin_revoke_access tool.
func ( handlers *Handlers ) revokeAccess( self_id uint64 , user_id uint64 ) ( revoked models.Revoked , refused *refusal ) {
	if _ , get_err := models.GetUser( handlers.Store , user_id ); get_err != nil {
		refused = &refusal{ status: fiber.StatusNotFound , message: "no such user" }
		return
	}
	// Same guard rail as disabling: revoking your own access signs you out
	// with no link to get back in.
	if user_id == self_id {
		refused = &refusal{ status: fiber.StatusBadRequest , message: "you cannot revoke your own access" }
		return
	}

	// The owner is refused here (allow_owner false). Only an operator with
	// shell access may cut the owner off -- see models.RevokeAllCredentials.
	revoked , revoke_err := models.RevokeAllCredentials( handlers.Store , user_id , false )
	if revoke_err != nil {
		if errors.Is( revoke_err , models.ErrOwnerProtected ) {
			refused = &refusal{ status: fiber.StatusForbidden , message: "the owner account's access cannot be revoked" }
			return
		}
		refused = &refusal{ status: fiber.StatusInternalServerError , message: "internal error" , cause: revoke_err }
	}
	return
}
