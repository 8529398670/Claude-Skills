package routes

import (
	errors "errors"

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
)

// Joining through an invite link (see models/invite_link.go; managing links
// is invite_links.go). Everything here is public -- holding the link is the
// permission -- so every POST is held to the same rule as /api/login: a JSON
// body from one of this site's own pages (sameOriginJSON). The credential
// travels in the body, never a query string, and the page that carries it in
// its path is served with no-referrer.
//
// A bad, expired, switched-off, or orphaned link always answers 404
// invite_link_invalid, whichever of those it was.

type joinRequest struct {
	Credential  string `json:"credential"`
	DisplayName string `json:"display_name"`
	UserID      uint64 `json:"user_id"`
	Pin         string `json:"pin"`
}

// ShowJoinPage answers a visit to an invite link with the ordinary app page,
// which sees the /join/ path and asks for a name. Nothing happens on the GET,
// so a chat app's link preview cannot use up a place.
func ( handlers *Handlers ) ShowJoinPage( c fiber.Ctx ) ( err error ) {
	c.Set( fiber.HeaderReferrerPolicy , "no-referrer" )
	err = handlers.Static.ServeIndex( c )
	return
}

// joinLink checks the request and decodes the body, then loads the link.
// link is nil whenever a response has already been written.
func ( handlers *Handlers ) joinLink( c fiber.Ctx , body *joinRequest ) ( link *models.InviteLink , err error ) {
	if sameOriginJSON( c ) == false {
		err = forbidden( c , "invite links must be used from this site's own page" )
		return
	}
	if c.Bind().JSON( body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	link , load_err := models.LoadInviteLink( handlers.Store , body.Credential )
	if load_err != nil {
		link = nil
		err = notFound( c , "invite_link_invalid" )
	}
	return
}

func ( handlers *Handlers ) JoinInfo( c fiber.Ctx ) ( err error ) {
	var body joinRequest
	link , err := handlers.joinLink( c , &body )
	if link == nil { return }
	err = c.JSON( fiber.Map{ "label": link.Label , "remaining": link.Remaining() } )
	return
}

// JoinMatch is the "did you mean...?": the accounts from this link whose
// names are close to what was typed. The page always shows this step before
// creating anyone, even with no matches, because a typo would otherwise use
// up a place.
func ( handlers *Handlers ) JoinMatch( c fiber.Ctx ) ( err error ) {
	var body joinRequest
	link , err := handlers.joinLink( c , &body )
	if link == nil { return }
	if models.ValidDisplayName( body.DisplayName ) == false {
		err = badRequest( c , "display name must be 1-80 characters" )
		return
	}
	matches , match_err := models.InviteMatches( handlers.Store , link , body.DisplayName )
	if match_err != nil {
		err = serverError( c )
		return
	}
	rows := []fiber.Map{}
	for _ , user := range matches {
		rows = append( rows , fiber.Map{ "id": user.ID , "display_name": user.DisplayName , "has_pin": user.HasPin() } )
	}
	err = c.JSON( fiber.Map{ "matches": rows , "can_create": link.Remaining() > 0 } )
	return
}

// JoinClaim signs in as an account this link created -- the same person on
// another device -- asking for the PIN if that person set one.
func ( handlers *Handlers ) JoinClaim( c fiber.Ctx ) ( err error ) {
	var body joinRequest
	link , err := handlers.joinLink( c , &body )
	if link == nil { return }
	user , account_err := models.InviteAccount( handlers.Store , link , body.UserID )
	if account_err != nil {
		err = notFound( c , "no_such_account" )
		return
	}
	if user.HasPin() {
		if handlers.Pins.Locked( user.ID ) {
			err = c.Status( fiber.StatusTooManyRequests ).JSON( fiber.Map{ "error": "pin_locked" } )
			return
		}
		ok := user.CheckPin( body.Pin )
		handlers.Pins.Record( user.ID , ok )
		if ok == false {
			code := "pin_wrong"
			if body.Pin == "" { code = "pin_needed" }
			err = forbidden( c , code )
			return
		}
	}

	session_credential , _ , claim_err := models.ClaimInviteAccount( handlers.Store , body.Credential , user.ID , handlers.Config.SessionTTL )
	switch {
	case errors.Is( claim_err , models.ErrInviteLinkInvalid ):
		err = notFound( c , "invite_link_invalid" )
		return
	case errors.Is( claim_err , models.ErrNotInviteAccount ):
		err = notFound( c , "no_such_account" )
		return
	case claim_err != nil:
		err = serverError( c )
		return
	}
	if handlers.startSession( c , session_credential ) != nil {
		err = serverError( c )
		return
	}
	err = c.JSON( fiber.Map{ "ok": true , "display_name": user.DisplayName } )
	return
}

// JoinCreate makes a new account through the link, if it has places left,
// and signs it in.
func ( handlers *Handlers ) JoinCreate( c fiber.Ctx ) ( err error ) {
	var body joinRequest
	link , err := handlers.joinLink( c , &body )
	if link == nil { return }
	if models.ValidDisplayName( body.DisplayName ) == false {
		err = badRequest( c , "display name must be 1-80 characters" )
		return
	}

	session_credential , user , join_err := models.JoinInviteLink( handlers.Store , body.Credential , body.DisplayName , handlers.Config.SessionTTL )
	switch {
	case errors.Is( join_err , models.ErrInviteLinkInvalid ):
		err = notFound( c , "invite_link_invalid" )
		return
	case errors.Is( join_err , models.ErrInviteLinkFull ):
		err = c.Status( fiber.StatusConflict ).JSON( fiber.Map{ "error": "invite_link_full" } )
		return
	case join_err != nil:
		err = serverError( c )
		return
	}
	if handlers.startSession( c , session_credential ) != nil {
		err = serverError( c )
		return
	}
	err = c.Status( fiber.StatusCreated ).JSON( fiber.Map{ "ok": true , "display_name": user.DisplayName } )
	return
}

// startSession switches this browser to a session that was just stored: the
// one it had is ended server-side first, exactly like RedeemLoginLink.
func ( handlers *Handlers ) startSession( c fiber.Ctx , session_credential string ) ( err error ) {
	handlers.destroyCookieSession( c )
	err = handlers.Guard.SetSessionCookie( c , session_credential )
	return
}
