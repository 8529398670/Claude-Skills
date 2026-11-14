// whole-file feature:invite_links
package routes

import (
	errors "errors"

	fiber "github.com/gofiber/fiber/v3"

	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// Joining through an invite link (see models/invite_link.go; managing links
// is invite_links.go). Everything here is public -- holding the link is the
// permission -- so every POST is held to the same rule as /api/login: a JSON
// body from one of this site's own pages (sameOrigin( joinElsewhere ), in
// routes.go). The credential
// travels in the body, never a query string, and the page that carries it in
// its path is served with no-referrer.
//
// A bad, expired, switched-off, or orphaned link always answers 404
// invite_link_invalid, whichever of those it was.

// joinRequest is what every step of joining sends; each step reads the
// fields it needs.
type joinRequest struct {
	Credential  string `json:"credential"`
	DisplayName string `json:"display_name,omitempty"`
	UserID      uint64 `json:"user_id,omitempty"`
	Pin         string `json:"pin,omitempty"`
}

// joinElsewhere is what a join route says to a request that did not come
// from this site's own page.
const joinElsewhere = "invite links must be used from this site's own page"

type joinInfoReply struct {
	Label     string `json:"label"`
	Remaining int    `json:"remaining"`
}

// joinMatch is an account the link made whose name is close to the one typed.
type joinMatch struct {
	ID          uint64 `json:"id"`
	DisplayName string `json:"display_name"`
	HasPIN      bool   `json:"has_pin"`
}

type joinMatchReply struct {
	Matches   []joinMatch `json:"matches"`
	CanCreate bool        `json:"can_create"`
}

// joinedReply is how joining ends: signed in, or a two-factor code is needed
// first, as with signInReply.
type joinedReply struct {
	Ok          bool   `json:"ok,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	TwoFactor   bool   `json:"two_factor,omitempty"`
	Challenge   string `json:"challenge,omitempty"`
}

type joined struct {
	Ok          bool   `json:"ok"`
	DisplayName string `json:"display_name"`
}

func ( joinedReply ) APIName() ( name string ) { name = "Joined"; return }

func ( joinedReply ) APIOneOf() ( shapes []any ) {
	shapes = []any{ joined{} , twoFactorNeeded{} }
	return
}

func ( joinedReply ) APIDoc() ( text string ) { text = "Either the sign-in finished, or a two-factor code is needed."; return }

// newJoinedReply is joinedReply for a sign-in signIn has ended.
func newJoinedReply( display_name string , challenge string ) ( reply *joinedReply ) {
	reply = &joinedReply{ Ok: true , DisplayName: display_name }
	if challenge != "" { reply = &joinedReply{ TwoFactor: true , Challenge: challenge } }
	return
}

// ShowJoinPage answers a visit to an invite link with the ordinary app page,
// which sees the /join/ path and asks for a name. Nothing happens on the GET,
// so a chat app's link preview cannot use up a place.
func ( handlers *Handlers ) ShowJoinPage( c fiber.Ctx ) ( err error ) {
	c.Set( fiber.HeaderReferrerPolicy , "no-referrer" )
	// A person with an old link gets the styled page, not RequireInviteLinks's
	// JSON. The referrer rule above still applies: the path holds the secret.
	if handlers.inviteLinksOn() == false {
		err = handlers.Static.NotFound( c )
		return
	}
	err = handlers.Static.ServeIndex( c )
	return
}

// joinLink is the link a request names, or the refusal every bad, expired,
// switched-off or orphaned one gets.
func ( handlers *Handlers ) joinLink( credential string ) ( link *models.InviteLink , refused *refusal ) {
	link , load_err := models.LoadInviteLink( handlers.Store , credential )
	if load_err != nil {
		link = nil
		refused = refuse( fiber.StatusNotFound , "invite_link_invalid" )
	}
	return
}

// joinInfo is what the join page shows before anyone types a name.
//
// @route("POST /api/join/info", summary="What an invite link is",
//     tag="Invite links", access=PUBLIC, feature="invite_links", use=[sameOrigin( joinElsewhere )],
// )
func ( handlers *Handlers ) joinInfo( c fiber.Ctx , in *joinRequest ) ( reply *joinInfoReply , err error ) {
	link , refused := handlers.joinLink( in.Credential )
	if refused != nil {
		err = refused
		return
	}
	reply = &joinInfoReply{ Label: link.Label , Remaining: link.Remaining() }
	return
}

// joinMatch is the "did you mean...?": the accounts from this link whose
// names are close to what was typed. The page always shows this step before
// creating anyone, even with no matches, because a typo would otherwise use
// up a place.
//
// @route("POST /api/join/match", summary="Accounts matching a name",
//     tag="Invite links", access=PUBLIC, feature="invite_links", use=[sameOrigin( joinElsewhere )],
// )
func ( handlers *Handlers ) joinMatch( c fiber.Ctx , in *joinRequest ) ( reply *joinMatchReply , err error ) {
	link , refused := handlers.joinLink( in.Credential )
	if refused != nil {
		err = refused
		return
	}
	if models.ValidDisplayName( in.DisplayName ) == false {
		err = refuse( fiber.StatusBadRequest , "display name must be 1-80 characters" )
		return
	}
	matches , err := models.InviteMatches( handlers.Store , link , in.DisplayName )
	if err != nil { return }
	reply = &joinMatchReply{ Matches: []joinMatch{} , CanCreate: link.Remaining() > 0 }
	for _ , user := range matches {
		reply.Matches = append( reply.Matches , joinMatch{ ID: user.ID , DisplayName: user.DisplayName , HasPIN: user.HasPin() } )
	}
	return
}

// joinClaim signs in as an account this link created -- the same person on
// another device -- asking for the PIN if that person set one.
//
// An account with a password and no PIN is refused: anyone holding the link
// can pick any name on it, and that must not walk around the password. The
// person signs in with it instead. An account with two-factor on still has to
// give its code after this, PIN or not.
//
// @route("POST /api/join/claim", summary="Sign in as an existing account",
//     tag="Invite links", access=PUBLIC, feature="invite_links", use=[sameOrigin( joinElsewhere )],
// )
func ( handlers *Handlers ) joinClaim( c fiber.Ctx , in *joinRequest ) ( reply *joinedReply , err error ) {
	link , refused := handlers.joinLink( in.Credential )
	if refused != nil {
		err = refused
		return
	}
	user , account_err := models.InviteAccount( handlers.Store , link , in.UserID )
	if account_err != nil {
		err = refuse( fiber.StatusNotFound , "no_such_account" )
		return
	}
	if user.HasPin() {
		if handlers.Pins.Reserve( user.ID ) == false {
			err = refuse( fiber.StatusTooManyRequests , "pin_locked" )
			return
		}
		ok := user.CheckPin( in.Pin )
		outcome := models.AttemptFailed
		if ok { outcome = models.AttemptPassed }
		handlers.Pins.Settle( user.ID , outcome )
		if ok == false {
			handlers.auditAs( c , handlers.logUser( user.ID , "anon" ) , logs.KindAuth , logs.LevelWarn , "wrong pin on an invite link" , fiber.Map{ "link_id": link.ID } )
			code := "pin_wrong"
			if in.Pin == "" { code = "pin_needed" }
			err = refuse( fiber.StatusForbidden , code )
			return
		}
	// >>> feature:passwords
	} else if user.PasswordUsable( handlers.cfg().PasswordsEnabled ) {
		err = refuse( fiber.StatusForbidden , "use_password" )
		return
	// <<< feature:passwords
	}

	signed , _ , claim_err := models.ClaimInviteAccount( handlers.Store , in.Credential , user.ID , handlers.signInRules() )
	switch {
	case errors.Is( claim_err , models.ErrInviteLinkInvalid ):
		err = refuse( fiber.StatusNotFound , "invite_link_invalid" )
		return
	case errors.Is( claim_err , models.ErrNotInviteAccount ):
		err = refuse( fiber.StatusNotFound , "no_such_account" )
		return
	case claim_err != nil:
		err = claim_err
		return
	}
	message := "signed back in through an invite link"
	if signed.Challenge != "" { message = "invite link accepted, waiting for a code" } // feature:two_factor
	handlers.auditAs( c , handlers.logUser( user.ID , security.MethodSession ) , logs.KindAuth , logs.LevelInfo , message , fiber.Map{ "link_id": link.ID } )
	challenge , err := handlers.signIn( c , signed )
	if err != nil { return }
	reply = newJoinedReply( user.DisplayName , challenge )
	return
}

// joinCreate makes a new account through the link, if it has places left,
// and signs it in.
//
// @route("POST /api/join/create", summary="Join as a new account",
//     tag="Invite links", access=PUBLIC, feature="invite_links", status=201, errors=(409,),
//     use=[sameOrigin( joinElsewhere )],
// )
func ( handlers *Handlers ) joinCreate( c fiber.Ctx , in *joinRequest ) ( reply *joinedReply , err error ) {
	link , refused := handlers.joinLink( in.Credential )
	if refused != nil {
		err = refused
		return
	}
	if models.ValidDisplayName( in.DisplayName ) == false {
		err = refuse( fiber.StatusBadRequest , "display name must be 1-80 characters" )
		return
	}

	session_credential , user , join_err := models.JoinInviteLink( handlers.Store , in.Credential , in.DisplayName , handlers.cfg().SessionTTL )
	switch {
	case errors.Is( join_err , models.ErrInviteLinkInvalid ):
		err = refuse( fiber.StatusNotFound , "invite_link_invalid" )
		return
	case errors.Is( join_err , models.ErrInviteLinkFull ):
		err = refuse( fiber.StatusConflict , "invite_link_full" )
		return
	case join_err != nil:
		err = join_err
		return
	}
	handlers.auditAs( c , handlers.logUser( user.ID , security.MethodSession ) , logs.KindAuth , logs.LevelInfo , "joined through an invite link" , fiber.Map{ "link_id": link.ID } )
	handlers.Webhooks.Emit( models.EventUserCreated , map[string]any{ "user_id": user.ID , "display_name": user.DisplayName , "role": user.Role , "invite_link_id": link.ID } ) // feature:webhooks
	signed := &models.SignIn{ UserID: user.ID , SessionCredential: session_credential }
	challenge , err := handlers.signIn( c , signed )
	if err != nil { return }
	reply = newJoinedReply( user.DisplayName , challenge )
	return
}
