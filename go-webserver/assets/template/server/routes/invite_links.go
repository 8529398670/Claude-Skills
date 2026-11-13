// whole-file feature:invite_links
package routes

import (
	errors "errors"
	strings "strings"
	time "time"


	fiber "github.com/gofiber/fiber/v3"

	db "{{MODULE_PATH}}/server/db"
	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// Managing invite links (see models/invite_link.go; joining through one is
// join.go). Admin only, registered under /api/admin. The link's credential is
// returned once, on creation, as a path -- the browser adds its own origin,
// like a login link.

// inviteLinksOn is auth.invite_links, read per request since it can change
// while the server runs.
func ( handlers *Handlers ) inviteLinksOn() ( result bool ) {
	result = handlers.cfg().InviteLinksEnabled
	return
}

// RequireInviteLinks makes every invite route answer 404 while
// auth.invite_links is off. The /join page does its own check, so a person
// gets the styled page rather than this JSON. See ShowJoinPage.
func ( handlers *Handlers ) RequireInviteLinks( c fiber.Ctx ) ( err error ) {
	if handlers.inviteLinksOn() == false {
		err = notFound( c , "not found" )
		return
	}
	err = c.Next()
	return
}

type inviteLinkRequest struct {
	Label     string `json:"label"`
	MaxUses   int    `json:"max_uses"`
	Days      int    `json:"days"`
	CSRFToken string `json:"csrf_token"`
}

// renameInviteRequest's Expect is the label the caller thinks it is replacing, so
// an undo cannot overwrite a rename made elsewhere.
type renameInviteRequest struct {
	Label     string  `json:"label"`
	Expect    *string `json:"expect"`
	CSRFToken string  `json:"csrf_token"`
}

// inviteLimitsRequest's Days is a pointer because leaving it out means "keep the
// expiry", which is not the same as any number of days.
type inviteLimitsRequest struct {
	MaxUses   int    `json:"max_uses"`
	Days      *int   `json:"days"`
	CSRFToken string `json:"csrf_token"`
}

func ( handlers *Handlers ) inviteLinkRow( link *models.InviteLink ) ( row fiber.Map ) {
	row = fiber.Map{
		"id":         link.ID,
		"label":      link.Label,
		"max_uses":   link.MaxUses,
		"used":       len( link.Accounts ),
		"remaining":  link.Remaining(),
		"live":       link.Live(),
		"expires_at": link.ExpiresAt,
		"revoked":    link.RevokedAt != nil,
		"created_by": link.CreatedBy,
	}
	// A link outlives its maker's admin rights only on paper: it stops
	// working the moment they lose them (see models.LoadInviteLink).
	if creator , get_err := models.GetUser( handlers.Store , link.CreatedBy ); get_err == nil {
		row[ "created_by_name" ] = creator.DisplayName
		if creator.Disabled() || creator.IsAdmin() == false { row[ "live" ] = false }
	} else {
		row[ "live" ] = false
	}
	return
}

func ( handlers *Handlers ) ListInviteLinks( c fiber.Ctx ) ( err error ) {
	rows , list_err := handlers.inviteLinkRows()
	if list_err != nil {
		err = serverError( c , list_err )
		return
	}
	rows = matchRows( rows , c.Query( "q" ) , "label" , "created_by_name" )
	err = sendSorted( c , "invite_links" , rows , "label" , "used" , "expires_at" )
	return
}

// inviteLinkRows is shared with the MCP admin_list_invite_links tool.
func ( handlers *Handlers ) inviteLinkRows() ( rows []fiber.Map , err error ) {
	links , err := models.ListInviteLinks( handlers.Store )
	if err != nil { return }
	rows = []fiber.Map{}
	for _ , link := range links {
		rows = append( rows , handlers.inviteLinkRow( link ) )
	}
	return
}

func ( handlers *Handlers ) CreateInviteLink( c fiber.Ctx ) ( err error ) {
	var body inviteLinkRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.createInviteLink( c , body.Label , body.MaxUses , body.Days )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.Status( fiber.StatusCreated ).JSON( payload )
	return
}

// createInviteLink is CreateInviteLink after the request is read, for it and
// the MCP admin_create_invite_link tool.
func ( handlers *Handlers ) createInviteLink( c fiber.Ctx , label string , max_uses int , days int ) ( payload fiber.Map , refused *refusal ) {
	if models.ValidInviteLink( label , max_uses , days ) == false {
		refused = refuse( fiber.StatusBadRequest , "label must be 1-80 characters, uses 1-1000, days 1-365" )
		return
	}
	ttl := time.Duration( days ) * 24 * time.Hour
	actor := security.ActorFrom( c )
	credential , link , issue_err := models.IssueInviteLink( handlers.Store , label , max_uses , ttl , actor.User.ID , actor.KeyID() )
	if issue_err != nil {
		refused = internalFailure( issue_err )
		return
	}
	payload = handlers.inviteLinkRow( link )
	payload[ "join_path" ] = "/join/" + credential
	handlers.audit( c , logs.KindAdmin , "invite link created" , fiber.Map{ "link_id": link.ID , "label": link.Label , "max_uses": link.MaxUses } )
	handlers.Webhooks.Emit( models.EventInviteLinkCreated , map[string]any{ "link_id": link.ID , "label": link.Label } ) // feature:webhooks
	return
}

// RevokeInviteLink switches a link off. The accounts it made keep working.
// RenameInviteLink changes a link's label.
func ( handlers *Handlers ) RenameInviteLink( c fiber.Ctx ) ( err error ) {
	var body renameInviteRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.renameInviteLink( c , strings.Clone( c.Params( "link_id" ) ) , body.Label , body.Expect )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// renameInviteLink is RenameInviteLink after the request is read, for it and
// the MCP admin_rename_invite_link tool.
func ( handlers *Handlers ) renameInviteLink( c fiber.Ctx , link_id string , label string , expect *string ) ( payload fiber.Map , refused *refusal ) {
	if models.ValidInviteLabel( label ) == false {
		refused = refuse( fiber.StatusBadRequest , "label must be 1-80 characters" )
		return
	}
	rename_err := models.RenameInviteLink( handlers.Store , link_id , label , expect )
	var stale *models.RenameConflictError
	switch {
	case errors.As( rename_err , &stale ):
		refused = refuse( fiber.StatusConflict , stale.Error() , fiber.Map{ "label": stale.Current } )
		return
	case errors.Is( rename_err , db.ErrNotFound ):
		refused = refuse( fiber.StatusNotFound , "no such invite link" )
		return
	case rename_err != nil:
		refused = internalFailure( rename_err )
		return
	}
	label = strings.TrimSpace( label )
	handlers.audit( c , logs.KindAdmin , "invite link renamed" , fiber.Map{ "link_id": link_id , "label": label } )
	payload = fiber.Map{ "ok": true , "label": label }
	return
}

// SetInviteLinkLimits changes a link's places, and its expiry when days is given.
// Through an API key the link becomes that key's: revoking the key switches it
// off (see models.SetInviteLinkLimits).
func ( handlers *Handlers ) SetInviteLinkLimits( c fiber.Ctx ) ( err error ) {
	var body inviteLimitsRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.setInviteLinkLimits( c , c.Params( "link_id" ) , body.MaxUses , body.Days )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// setInviteLinkLimits is SetInviteLinkLimits after the request is read, for it
// and the MCP admin_set_invite_link_limits tool.
func ( handlers *Handlers ) setInviteLinkLimits( c fiber.Ctx , link_id string , max_uses int , days *int ) ( payload fiber.Map , refused *refusal ) {
	if models.ValidInviteLimits( max_uses , days ) == false {
		refused = refuse( fiber.StatusBadRequest , "uses must be 1-1000 and days 1-365" )
		return
	}
	var ttl time.Duration
	if days != nil { ttl = time.Duration( *days ) * 24 * time.Hour }
	link , set_err := models.SetInviteLinkLimits( handlers.Store , link_id , max_uses , ttl , security.ActorFrom( c ).KeyID() )
	switch {
	case errors.Is( set_err , db.ErrNotFound ):
		refused = refuse( fiber.StatusNotFound , "no such invite link" )
		return
	case errors.Is( set_err , models.ErrInvitePlaces ):
		refused = refuse( fiber.StatusBadRequest , set_err.Error() )
		return
	case set_err != nil:
		refused = internalFailure( set_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "invite link limits changed" , fiber.Map{ "link_id": link.ID , "max_uses": link.MaxUses , "expires_at": link.ExpiresAt } )
	payload = handlers.inviteLinkRow( link )
	return
}

// SwitchOnInviteLink undoes RevokeInviteLink. A key that switches a link on
// takes it over, so revoking that key closes it again.
func ( handlers *Handlers ) SwitchOnInviteLink( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.switchOnInviteLink( c , c.Params( "link_id" ) )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// switchOnInviteLink is SwitchOnInviteLink after the request is read, for it
// and the MCP admin_switch_on_invite_link tool.
func ( handlers *Handlers ) switchOnInviteLink( c fiber.Ctx , link_id string ) ( payload fiber.Map , refused *refusal ) {
	link , switch_err := models.SwitchOnInviteLink( handlers.Store , link_id , security.ActorFrom( c ).KeyID() )
	if errors.Is( switch_err , db.ErrNotFound ) {
		refused = refuse( fiber.StatusNotFound , "no such invite link" )
		return
	}
	if switch_err != nil {
		refused = internalFailure( switch_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "invite link switched back on" , fiber.Map{ "link_id": link.ID } )
	payload = handlers.inviteLinkRow( link )
	return
}

func ( handlers *Handlers ) RevokeInviteLink( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	if models.RevokeInviteLink( handlers.Store , c.Params( "link_id" ) ) != nil {
		err = notFound( c , "no such invite link" )
		return
	}
	handlers.audit( c , logs.KindAdmin , "invite link revoked" , fiber.Map{ "link_id": c.Params( "link_id" ) } )
	handlers.Webhooks.Emit( models.EventInviteLinkRevoked , map[string]any{ "link_id": c.Params( "link_id" ) } ) // feature:webhooks
	err = c.JSON( fiber.Map{ "ok": true } )
	return
}

// DeleteInviteLink drops a link from the list. The accounts it made stay;
// they just can no longer sign in on a new device through it.
// RestoreInviteLink undoes it for a few minutes.
func ( handlers *Handlers ) DeleteInviteLink( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.deleteInviteLink( c , strings.Clone( c.Params( "link_id" ) ) )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// deleteInviteLink is DeleteInviteLink after the request is read, for it and
// the MCP admin_delete_invite_link tool.
func ( handlers *Handlers ) deleteInviteLink( c fiber.Ctx , link_id string ) ( payload fiber.Map , refused *refusal ) {
	if models.DeleteInviteLink( handlers.Store , link_id , security.UserFrom( c ).ID ) != nil {
		refused = refuse( fiber.StatusNotFound , "no such invite link" )
		return
	}
	handlers.audit( c , logs.KindAdmin , "invite link deleted" , fiber.Map{ "link_id": link_id } )
	payload = fiber.Map{ "ok": true }
	return
}

// RestoreInviteLink undoes DeleteInviteLink, within models.RestoreWindow. The
// link works again exactly as before, uses and expiry included.
func ( handlers *Handlers ) RestoreInviteLink( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.restoreInviteLink( c , strings.Clone( c.Params( "link_id" ) ) )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// restoreInviteLink is RestoreInviteLink after the request is read, for it and
// the MCP admin_restore_invite_link tool.
func ( handlers *Handlers ) restoreInviteLink( c fiber.Ctx , link_id string ) ( payload fiber.Map , refused *refusal ) {
	actor := security.ActorFrom( c )
	link , restore_err := models.RestoreInviteLink( handlers.Store , link_id , actor.User.ID , actor.User.IsAdmin() , actor.KeyID() )
	if restore_err != nil {
		refused = restoreRefusal( restore_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "invite link restored" , fiber.Map{ "link_id": link_id } )
	payload = handlers.inviteLinkRow( link )
	return
}
