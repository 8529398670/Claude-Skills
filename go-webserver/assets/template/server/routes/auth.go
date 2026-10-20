package routes

import (
	errors "errors"
	strings "strings"

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
)

type loginRequest struct {
	Credential string `json:"credential"`
}

// ShowLoginPage answers a visit to a login link with the ordinary app page,
// which sees the /login/ path and asks the person to confirm. Nothing is
// redeemed here.
//
// That is the point of the two steps. Link previews in chat apps, mail
// scanners, and browser prefetch all fetch a URL without anyone asking them
// to; if the GET were the login, they would spend the link before its owner
// ever clicked it. And someone already signed in gets told that continuing
// switches accounts, rather than being silently signed in as whoever the link
// belongs to.
//
// The route is registered as /login/* rather than /login/:credential because
// the credential contains a "." and route parameters should not have to care
// about the shape of a secret.
func ( handlers *Handlers ) ShowLoginPage( c fiber.Ctx ) ( err error ) {
	// The credential is in this page's URL; make sure no request the page
	// makes carries it onward in a Referer header.
	c.Set( fiber.HeaderReferrerPolicy , "no-referrer" )
	err = handlers.Static.ServeIndex( c )
	return
}

// RedeemLoginLink is the whole login. Redeeming a valid link is the login;
// there is no form, no password, and nothing to phish out of a user beyond
// the link itself, which dies on first use.
//
// It is the one state-changing route with no CSRF token -- there is no
// session yet to hold one -- so it is guarded differently. A forged login
// signs a victim's browser in to the *attacker's* account, after which
// anything the victim does lands where the attacker can read it. Requiring
// a JSON body is what blocks that: an HTML form cannot send one, and a
// cross-origin fetch that tries needs a CORS preflight this server never
// approves. Sec-Fetch-Site, which every current browser sends, refuses the
// rest outright.
func ( handlers *Handlers ) RedeemLoginLink( c fiber.Ctx ) ( err error ) {
	if sameOriginJSON( c ) == false {
		err = forbidden( c , "login must be submitted from this site's own login page" )
		return
	}
	var body loginRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}

	credential := strings.Trim( strings.TrimSpace( body.Credential ) , "/" )
	session_credential , _ , redeem_err := models.RedeemLoginToken( handlers.Store , credential , handlers.Config.SessionTTL )
	if redeem_err != nil {
		// Expired, already spent, never existed, wrong secret, or a disabled
		// account all get the same answer. Telling them apart would let
		// someone probe which links are real.
		if errors.Is( redeem_err , models.ErrLoginTokenInvalid ) {
			err = badRequest( c , "invalid login link" )
			return
		}
		err = serverError( c )
		return
	}

	// Switching accounts: the session this browser had is ended server-side,
	// not merely replaced in the cookie jar.
	handlers.destroyCookieSession( c )
	if cookie_err := handlers.Guard.SetSessionCookie( c , session_credential ); cookie_err != nil {
		err = serverError( c )
		return
	}
	err = c.JSON( fiber.Map{ "ok": true } )
	return
}

// sameOriginJSON is true for a request with a JSON content type that, if it
// came from a browser, came from one of this site's own pages.
func sameOriginJSON( c fiber.Ctx ) ( ok bool ) {
	media_type , _ , _ := strings.Cut( c.Get( fiber.HeaderContentType ) , ";" )
	if strings.EqualFold( strings.TrimSpace( media_type ) , fiber.MIMEApplicationJSON ) == false { return }
	site := c.Get( "Sec-Fetch-Site" )
	ok = site == "" || site == "same-origin"
	return
}

// destroyCookieSession deletes the session named by this request's cookie,
// if any. Deleting the record rather than only the cookie is what matters: a
// copy of the cookie taken earlier is then dead too.
func ( handlers *Handlers ) destroyCookieSession( c fiber.Ctx ) {
	raw := c.Cookies( handlers.Config.CookieName )
	if raw == "" { return }
	if credential , decrypt_err := handlers.Guard.DecryptCookie( raw ); decrypt_err == nil {
		models.DestroySession( handlers.Store , credential )
	}
}
