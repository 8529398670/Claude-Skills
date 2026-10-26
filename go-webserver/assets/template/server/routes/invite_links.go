package routes

import (
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// Managing invite links (see models/invite_link.go; joining through one is
// join.go). Admin only, registered under /api/admin. The link's credential is
// returned once, on creation, as a path -- the browser adds its own origin,
// like a login link.

type inviteLinkRequest struct {
	Label     string `json:"label"`
	MaxUses   int    `json:"max_uses"`
	Days      int    `json:"days"`
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
		err = serverError( c )
		return
	}
	err = c.JSON( rows )
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
	if models.ValidInviteLink( body.Label , body.MaxUses , body.Days ) == false {
		err = badRequest( c , "label must be 1-80 characters, uses 1-1000, days 1-365" )
		return
	}
	ttl := time.Duration( body.Days ) * 24 * time.Hour
	credential , link , issue_err := models.IssueInviteLink( handlers.Store , body.Label , body.MaxUses , ttl , security.UserFrom( c ).ID )
	if issue_err != nil {
		err = serverError( c )
		return
	}
	row := handlers.inviteLinkRow( link )
	row[ "join_path" ] = "/join/" + credential
	err = c.Status( fiber.StatusCreated ).JSON( row )
	return
}

// RevokeInviteLink switches a link off. The accounts it made keep working.
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
	err = c.JSON( fiber.Map{ "ok": true } )
	return
}

// DeleteInviteLink drops a link from the list for good. The accounts it made
// stay; they just can no longer sign in on a new device through it.
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
	if models.DeleteInviteLink( handlers.Store , c.Params( "link_id" ) ) != nil {
		err = notFound( c , "no such invite link" )
		return
	}
	err = c.JSON( fiber.Map{ "ok": true } )
	return
}
