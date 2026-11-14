// whole-file feature:invite_links
package routes

import (
	errors "errors"
	strings "strings"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
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

// linkIDPath is the link a route under /api/admin/invite-links/{link_id} acts on.
type linkIDPath struct {
	LinkID string `json:"link_id" path:"link_id" jsonschema_description:"The link's id, from GET /api/admin/invite-links (admin_list_invite_links over MCP)."`
}

// inviteLinkRow is a link as the admin screen lists it. The credential is
// never in it: that is shown once, when the link is made.
type inviteLinkRow struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	MaxUses       int       `json:"max_uses"`
	Used          int       `json:"used"`
	Remaining     int       `json:"remaining"`
	Live          bool      `json:"live"`
	ExpiresAt     time.Time `json:"expires_at"`
	Revoked       bool      `json:"revoked"`
	CreatedBy     uint64    `json:"created_by"`
	CreatedByName string    `json:"created_by_name,omitempty"`
}

func ( inviteLinkRow ) APIName() ( name string ) { name = "InviteLink"; return }

func ( handlers *Handlers ) inviteLinkRow( link *models.InviteLink ) ( row inviteLinkRow ) {
	row = inviteLinkRow{
		ID:        link.ID,
		Label:     link.Label,
		MaxUses:   link.MaxUses,
		Used:      len( link.Accounts ),
		Remaining: link.Remaining(),
		Live:      link.Live(),
		ExpiresAt: link.ExpiresAt,
		Revoked:   link.RevokedAt != nil,
		CreatedBy: link.CreatedBy,
	}
	// A link outlives its maker's admin rights only on paper: it stops
	// working the moment they lose them (see models.LoadInviteLink).
	if creator , get_err := models.GetUser( handlers.Store , link.CreatedBy ); get_err == nil {
		row.CreatedByName = creator.DisplayName
		if creator.Disabled() || creator.IsAdmin() == false { row.Live = false }
	} else {
		row.Live = false
	}
	return
}

type listInviteLinksRequest struct {
	Page
	Q    string `json:"q,omitempty" query:"q" jsonschema_description:"Keep links whose label or maker contains this, in any case."`
	Sort string `json:"sort,omitempty" query:"sort" jsonschema:"enum=label,enum=used,enum=expires_at" jsonschema_description:"Field to sort the whole list by. Rows without it go last."`
	Direction
}

type listInviteLinksReply struct {
	InviteLinks []inviteLinkRow `json:"invite_links"`
	Paged
}

// ListInviteLinks is every link, a page at a time.
func ( handlers *Handlers ) ListInviteLinks() *apidoc.Endpoint[ listInviteLinksRequest , listInviteLinksReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Every invite link" , Tag: "Invite links" , Role: apidoc.Admin , Feature: "invite_links" , Errors: []int{ 400 , 401 , 403 } },
		func( c fiber.Ctx , in *listInviteLinksRequest ) ( reply *listInviteLinksReply , err error ) {
			links , err := models.ListInviteLinks( handlers.Store )
			if err != nil { return }
			rows := []inviteLinkRow{}
			for _ , link := range links {
				rows = append( rows , handlers.inviteLinkRow( link ) )
			}
			rows = matchRows( rows , in.Q , "label" , "created_by_name" )
			reply = &listInviteLinksReply{}
			reply.InviteLinks , reply.Paged = sortedPage( rows , in.Sort , in.Direction , in.Page )
			return
		} )
}

const inviteLinkLimits = "label must be 1-80 characters, uses 1-1000, days 1-365"

type inviteLinkRequest struct {
	Label   string `json:"label" jsonschema:"minLength=1,maxLength=80" error:"label must be 1-80 characters, uses 1-1000, days 1-365"`
	MaxUses int    `json:"max_uses" jsonschema:"minimum=1,maximum=1000" error:"label must be 1-80 characters, uses 1-1000, days 1-365"`
	Days    int    `json:"days" jsonschema:"minimum=1,maximum=365" error:"label must be 1-80 characters, uses 1-1000, days 1-365"`
}

type createdInviteLinkReply struct {
	inviteLinkRow
	JoinPath string `json:"join_path" jsonschema_description:"The link to hand out, shown in this response only. A credential: treat it like a password."`
}

// CreateInviteLink makes a link and answers with it, the only time its path
// is shown.
func ( handlers *Handlers ) CreateInviteLink() *apidoc.Endpoint[ inviteLinkRequest , createdInviteLinkReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Create an invite link" , Tag: "Invite links" , Role: apidoc.Admin , Feature: "invite_links" , Status: fiber.StatusCreated , Errors: []int{ 400 , 401 , 403 } },
		func( c fiber.Ctx , in *inviteLinkRequest ) ( reply *createdInviteLinkReply , err error ) {
			if models.ValidInviteLink( in.Label , in.MaxUses , in.Days ) == false {
				err = refuse( fiber.StatusBadRequest , inviteLinkLimits )
				return
			}
			ttl := time.Duration( in.Days ) * 24 * time.Hour
			actor := security.ActorFrom( c )
			credential , link , err := models.IssueInviteLink( handlers.Store , in.Label , in.MaxUses , ttl , actor.User.ID , actor.KeyID() )
			if err != nil { return }
			reply = &createdInviteLinkReply{ inviteLinkRow: handlers.inviteLinkRow( link ) , JoinPath: "/join/" + credential }
			handlers.audit( c , logs.KindAdmin , "invite link created" , fiber.Map{ "link_id": link.ID , "label": link.Label , "max_uses": link.MaxUses } )
			handlers.Webhooks.Emit( models.EventInviteLinkCreated , map[string]any{ "link_id": link.ID , "label": link.Label } ) // feature:webhooks
			return
		} )
}

// renameInviteRequest's Expect is the label the caller thinks it is replacing, so
// an undo cannot overwrite a rename made elsewhere.
type renameInviteRequest struct {
	linkIDPath
	Label  string  `json:"label" jsonschema:"minLength=1,maxLength=80" jsonschema_description:"1-80 characters." error:"label must be 1-80 characters"`
	Expect *string `json:"expect,omitempty" jsonschema_description:"The label you think it has. Answers 409 if it has another."`
}

type renameInviteReply struct {
	Ok    bool   `json:"ok"`
	Label string `json:"label"`
}

// RenameInviteLink changes a link's label.
func ( handlers *Handlers ) RenameInviteLink() *apidoc.Endpoint[ renameInviteRequest , renameInviteReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Rename a link" , Tag: "Invite links" , Role: apidoc.Admin , Feature: "invite_links" , Errors: []int{ 400 , 401 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *renameInviteRequest ) ( reply *renameInviteReply , err error ) {
			if models.ValidInviteLabel( in.Label ) == false {
				err = refuse( fiber.StatusBadRequest , "label must be 1-80 characters" )
				return
			}
			rename_err := models.RenameInviteLink( handlers.Store , in.LinkID , in.Label , in.Expect )
			var stale *models.RenameConflictError
			switch {
			case errors.As( rename_err , &stale ):
				err = refuse( fiber.StatusConflict , stale.Error() , fiber.Map{ "label": stale.Current } )
				return
			case errors.Is( rename_err , db.ErrNotFound ):
				err = refuse( fiber.StatusNotFound , "no such invite link" )
				return
			case rename_err != nil:
				err = rename_err
				return
			}
			label := strings.TrimSpace( in.Label )
			handlers.audit( c , logs.KindAdmin , "invite link renamed" , fiber.Map{ "link_id": in.LinkID , "label": label } )
			reply = &renameInviteReply{ Ok: true , Label: label }
			return
		} )
}

// inviteLimitsRequest's Days is a pointer because leaving it out means "keep the
// expiry", which is not the same as any number of days.
type inviteLimitsRequest struct {
	linkIDPath
	MaxUses int  `json:"max_uses" jsonschema:"minimum=1,maximum=1000" jsonschema_description:"Not fewer than have joined." error:"uses must be 1-1000 and days 1-365"`
	Days    *int `json:"days,omitempty" jsonschema:"minimum=1,maximum=365" jsonschema_description:"Counted from now. Leave out to keep the expiry." error:"uses must be 1-1000 and days 1-365"`
}

// SetInviteLinkLimits changes a link's places, and its expiry when days is given.
// Through an API key the link becomes that key's: revoking the key switches it
// off (see models.SetInviteLinkLimits).
func ( handlers *Handlers ) SetInviteLinkLimits() *apidoc.Endpoint[ inviteLimitsRequest , inviteLinkRow ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Change a link's places and expiry" , Tag: "Invite links" , Role: apidoc.Admin , Feature: "invite_links" , Errors: []int{ 400 , 401 , 403 , 404 } },
		func( c fiber.Ctx , in *inviteLimitsRequest ) ( reply *inviteLinkRow , err error ) {
			if models.ValidInviteLimits( in.MaxUses , in.Days ) == false {
				err = refuse( fiber.StatusBadRequest , "uses must be 1-1000 and days 1-365" )
				return
			}
			var ttl time.Duration
			if in.Days != nil { ttl = time.Duration( *in.Days ) * 24 * time.Hour }
			link , set_err := models.SetInviteLinkLimits( handlers.Store , in.LinkID , in.MaxUses , ttl , security.ActorFrom( c ).KeyID() )
			switch {
			case errors.Is( set_err , db.ErrNotFound ):
				err = refuse( fiber.StatusNotFound , "no such invite link" )
				return
			case errors.Is( set_err , models.ErrInvitePlaces ):
				err = refuse( fiber.StatusBadRequest , set_err.Error() )
				return
			case set_err != nil:
				err = set_err
				return
			}
			handlers.audit( c , logs.KindAdmin , "invite link limits changed" , fiber.Map{ "link_id": link.ID , "max_uses": link.MaxUses , "expires_at": link.ExpiresAt } )
			row := handlers.inviteLinkRow( link )
			reply = &row
			return
		} )
}

// SwitchOnInviteLink undoes RevokeInviteLink. A key that switches a link on
// takes it over, so revoking that key closes it again.
func ( handlers *Handlers ) SwitchOnInviteLink() *apidoc.Endpoint[ linkIDPath , inviteLinkRow ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Switch a link back on" , Tag: "Invite links" , Role: apidoc.Admin , Feature: "invite_links" , Errors: []int{ 400 , 401 , 403 , 404 } },
		func( c fiber.Ctx , in *linkIDPath ) ( reply *inviteLinkRow , err error ) {
			link , switch_err := models.SwitchOnInviteLink( handlers.Store , in.LinkID , security.ActorFrom( c ).KeyID() )
			if errors.Is( switch_err , db.ErrNotFound ) {
				err = refuse( fiber.StatusNotFound , "no such invite link" )
				return
			}
			if switch_err != nil {
				err = switch_err
				return
			}
			handlers.audit( c , logs.KindAdmin , "invite link switched back on" , fiber.Map{ "link_id": link.ID } )
			row := handlers.inviteLinkRow( link )
			reply = &row
			return
		} )
}

// RevokeInviteLink switches a link off. The accounts it made keep working.
func ( handlers *Handlers ) RevokeInviteLink() *apidoc.Endpoint[ linkIDPath , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Switch an invite link off" , Tag: "Invite links" , Role: apidoc.Admin , Feature: "invite_links" , Errors: []int{ 400 , 401 , 403 , 404 } },
		func( c fiber.Ctx , in *linkIDPath ) ( reply *okReply , err error ) {
			if models.RevokeInviteLink( handlers.Store , in.LinkID ) != nil {
				err = refuse( fiber.StatusNotFound , "no such invite link" )
				return
			}
			handlers.audit( c , logs.KindAdmin , "invite link revoked" , fiber.Map{ "link_id": in.LinkID } )
			handlers.Webhooks.Emit( models.EventInviteLinkRevoked , map[string]any{ "link_id": in.LinkID } ) // feature:webhooks
			reply = &okReply{ Ok: true }
			return
		} )
}

// DeleteInviteLink drops a link from the list. The accounts it made stay;
// they just can no longer sign in on a new device through it.
// RestoreInviteLink undoes it for a few minutes.
func ( handlers *Handlers ) DeleteInviteLink() *apidoc.Endpoint[ linkIDPath , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Delete an invite link" , Tag: "Invite links" , Role: apidoc.Admin , Feature: "invite_links" , Errors: []int{ 400 , 401 , 403 , 404 } },
		func( c fiber.Ctx , in *linkIDPath ) ( reply *okReply , err error ) {
			if models.DeleteInviteLink( handlers.Store , in.LinkID , security.UserFrom( c ).ID ) != nil {
				err = refuse( fiber.StatusNotFound , "no such invite link" )
				return
			}
			handlers.audit( c , logs.KindAdmin , "invite link deleted" , fiber.Map{ "link_id": in.LinkID } )
			reply = &okReply{ Ok: true }
			return
		} )
}

// RestoreInviteLink undoes DeleteInviteLink, within models.RestoreWindow. The
// link works again exactly as before, uses and expiry included.
func ( handlers *Handlers ) RestoreInviteLink() *apidoc.Endpoint[ linkIDPath , inviteLinkRow ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Undo deleting an invite link" , Tag: "Invite links" , Role: apidoc.Admin , Feature: "invite_links" , Errors: []int{ 400 , 401 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *linkIDPath ) ( reply *inviteLinkRow , err error ) {
			actor := security.ActorFrom( c )
			link , restore_err := models.RestoreInviteLink( handlers.Store , in.LinkID , actor.User.ID , actor.User.IsAdmin() , actor.KeyID() )
			if restore_err != nil {
				err = restoreRefusal( restore_err )
				return
			}
			handlers.audit( c , logs.KindAdmin , "invite link restored" , fiber.Map{ "link_id": in.LinkID } )
			row := handlers.inviteLinkRow( link )
			reply = &row
			return
		} )
}
