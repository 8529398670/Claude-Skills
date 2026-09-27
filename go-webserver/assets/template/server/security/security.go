// Package security is the only place that decides who a request belongs to
// and whether it is allowed to change anything. Route handlers read the
// answer; they never re-derive it. One implementation means one place to get
// right, and one place to look when auditing.
//
// There are two ways to prove who you are, and they meet here rather than
// anywhere downstream: a session cookie (a browser) or an API key in an
// Authorization header (a script, a cron job, another service). Everything
// past this package sees one Actor and one effective role, so a route does not
// know or care which door the request came through.
package security

import (
	strings "strings"

	fiber "github.com/gofiber/fiber/v3"

	config "{{MODULE_PATH}}/server/config"
	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
	models "{{MODULE_PATH}}/server/models"
)

// Typed, unexported keys so nothing else in the process can overwrite what we
// stashed on the request by accident.
type contextKey string

const keyActor contextKey = "current_actor"

// Auth method names, as reported by /api/me. Worth exposing so a client can
// tell a browser session from a key without guessing.
const (
	MethodSession = "session"
	MethodAPIKey  = "api_key"
)

// Actor is who a request belongs to, plus how they proved it.
//
// Role is the *effective* role, not the account's: a user-scoped API key held
// by an admin authenticates as a user. Every permission check reads this
// field, so the narrowing cannot be forgotten at a call site.
type Actor struct {
	User    *models.User
	Session *models.Session
	APIKey  *models.APIKey
	Role    string
}

func ( actor *Actor ) IsAdmin() ( result bool ) {
	result = actor.Role == models.RoleAdmin
	return
}

// ViaAPIKey distinguishes the two doors. It decides the CSRF question below,
// and it is what an endpoint reads if it genuinely must be browser-only.
func ( actor *Actor ) ViaAPIKey() ( result bool ) {
	result = actor.APIKey != nil
	return
}

func ( actor *Actor ) Method() ( result string ) {
	result = MethodSession
	if actor.ViaAPIKey() {
		result = MethodAPIKey
	}
	return
}

type Guard struct {
	Store  *db.Store
	Config *config.Config
}

func New( store *db.Store , cfg *config.Config ) ( guard *Guard ) {
	guard = &Guard{ Store: store , Config: cfg }
	return
}

// SetSessionCookie stores the session credential encrypted with the server
// key. The server already checks the credential against the database, so the
// encryption is not what makes the session unforgeable -- it means the cookie
// is opaque to anyone reading it out of a browser profile or a proxy log, and
// that a tampered cookie fails to decrypt instead of reaching the lookup.
func ( guard *Guard ) SetSessionCookie( c fiber.Ctx , credential string ) ( err error ) {
	sealed , err := encryption.ChaChaEncryptString( guard.Config.SecretKey , credential )
	if err != nil { return }
	c.Cookie( &fiber.Cookie{
		Name:     guard.Config.CookieName,
		Value:    sealed,
		Path:     "/",
		MaxAge:   int( guard.Config.SessionTTL.Seconds() ),
		Secure:   guard.Config.SecureCookies,
		HTTPOnly: true,
		SameSite: "Strict",
	} )
	return
}

func ( guard *Guard ) ClearSessionCookie( c fiber.Ctx ) {
	// Overwrite before clearing: some browsers hold on to a cookie whose
	// attributes do not match exactly, and an empty value is harmless either way.
	c.Cookie( &fiber.Cookie{
		Name:     guard.Config.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   guard.Config.SecureCookies,
		HTTPOnly: true,
		SameSite: "Strict",
	} )
	c.ClearCookie( guard.Config.CookieName )
}

// DecryptCookie turns the stored cookie back into a session credential.
// Exposed because logging out has to destroy the server-side record, not just
// drop the cookie -- otherwise a copy of the cookie taken earlier still works.
func ( guard *Guard ) DecryptCookie( raw string ) ( credential string , err error ) {
	credential , err = encryption.ChaChaDecryptBase64String( guard.Config.SecretKey , raw )
	return
}

// LoadUser runs on every request. It resolves the caller at most once and
// stashes the result, so a handler and a middleware asking "who is this" do
// not cost two database reads. It never rejects anything -- that is
// RequireLogin's job -- because plenty of routes are fine for anonymous
// visitors.
//
// The cookie is tried first and an API key only if there is no live session.
// Either order is safe, but this one keeps the CSRF rule below unambiguous: a
// request that arrives with ambient browser credentials is always held to the
// token check, and cannot opt out of it by also carrying a key.
func ( guard *Guard ) LoadUser( c fiber.Ctx ) ( err error ) {
	actor := guard.resolveSession( c )
	if actor == nil {
		actor = guard.resolveAPIKey( c )
	}
	if actor != nil {
		c.Locals( keyActor , actor )
	}
	err = c.Next()
	return
}

func ( guard *Guard ) resolveSession( c fiber.Ctx ) ( actor *Actor ) {
	raw := c.Cookies( guard.Config.CookieName )
	if raw == "" { return }

	credential , decrypt_err := encryption.ChaChaDecryptBase64String( guard.Config.SecretKey , raw )
	if decrypt_err != nil { return }

	session , user := models.LoadSession( guard.Store , credential )
	if user == nil { return }

	actor = &Actor{ User: user , Session: session , Role: user.Role }
	return
}

// resolveAPIKey reads "Authorization: Bearer <id>.<secret>".
//
// One header and one scheme, deliberately: a second accepted spelling (an
// X-API-Key, a query parameter) is a second code path to audit, and a
// credential in a query string ends up in access logs and browser history.
//
// APIKeysEnabled is checked here rather than only on the routes that mint
// keys, so turning the feature off actually closes the door: the header stops
// being believed at all, and existing keys stop authenticating.
func ( guard *Guard ) resolveAPIKey( c fiber.Ctx ) ( actor *Actor ) {
	if guard.Config.APIKeysEnabled == false { return }

	header := strings.TrimSpace( c.Get( fiber.HeaderAuthorization ) )
	if header == "" { return }
	scheme , credential , found := strings.Cut( header , " " )
	if found == false || strings.EqualFold( scheme , "bearer" ) == false { return }
	credential = strings.TrimSpace( credential )
	if credential == "" { return }

	key , user := models.LoadAPIKey( guard.Store , credential )
	if user == nil { return }

	actor = &Actor{
		User:   user,
		APIKey: key,
		// The ceiling, applied per request. See models.NarrowRole: the stored
		// role is what was asked for, this is what is granted.
		Role: models.NarrowRole( user.Role , key.Role ),
	}
	return
}

// RequireLogin, RequireAdmin, and RequireSession are middleware rather than
// helpers called at the top of each handler, so a new protected route is
// protected by where it is registered -- forgetting the check is not something
// you can do silently.
func ( guard *Guard ) RequireLogin( c fiber.Ctx ) ( err error ) {
	if ActorFrom( c ) == nil {
		err = c.Status( fiber.StatusUnauthorized ).JSON( fiber.Map{ "error": "not authenticated" } )
		return
	}
	err = c.Next()
	return
}

func ( guard *Guard ) RequireAdmin( c fiber.Ctx ) ( err error ) {
	actor := ActorFrom( c )
	if actor == nil {
		err = c.Status( fiber.StatusUnauthorized ).JSON( fiber.Map{ "error": "not authenticated" } )
		return
	}
	if actor.IsAdmin() == false {
		err = c.Status( fiber.StatusForbidden ).JSON( fiber.Map{ "error": "forbidden" } )
		return
	}
	err = c.Next()
	return
}

// RequireSession refuses a request that authenticated with an API key, even a
// correctly scoped one. Exactly one group of routes uses it: the ones that
// mint and revoke API keys.
//
// The reasoning is containment rather than privilege. A key is a bearer
// credential that lives in a config file or a CI secret; if a leaked key could
// mint more keys, revoking it would not end the compromise, because the key it
// minted while you were not looking survives. Requiring a browser session to
// manage keys means the list of keys is always something a person created, and
// revoking a key actually revokes it.
//
// Drop this from the group if an app genuinely needs to provision keys
// programmatically -- but do it knowingly, and say so in that app's docs.
func ( guard *Guard ) RequireSession( c fiber.Ctx ) ( err error ) {
	actor := ActorFrom( c )
	if actor == nil {
		err = c.Status( fiber.StatusUnauthorized ).JSON( fiber.Map{ "error": "not authenticated" } )
		return
	}
	if actor.ViaAPIKey() {
		err = c.Status( fiber.StatusForbidden ).JSON( fiber.Map{ "error": "this endpoint requires a signed-in session, not an API key" } )
		return
	}
	err = c.Next()
	return
}

func ActorFrom( c fiber.Ctx ) ( actor *Actor ) {
	actor , _ = c.Locals( keyActor ).( *Actor )
	return
}

// UserFrom, SessionFrom, and APIKeyFrom are the narrow reads a handler
// usually wants. UserFrom is the account; ActorFrom is the account plus how
// it authenticated and what it may do, which is what a permission decision
// needs.
func UserFrom( c fiber.Ctx ) ( user *models.User ) {
	if actor := ActorFrom( c ); actor != nil {
		user = actor.User
	}
	return
}

func SessionFrom( c fiber.Ctx ) ( session *models.Session ) {
	if actor := ActorFrom( c ); actor != nil {
		session = actor.Session
	}
	return
}

func APIKeyFrom( c fiber.Ctx ) ( key *models.APIKey ) {
	if actor := ActorFrom( c ); actor != nil {
		key = actor.APIKey
	}
	return
}

// CheckCSRF guards state-changing requests. SameSite=Strict already blocks
// the common cross-site POST, but it is a browser-side promise: older
// browsers, odd cross-scheme cases, and non-browser clients do not all honour
// it. A per-session token echoed back in the JSON body costs nothing and does
// not depend on the browser behaving.
//
// A request authenticated by an API key is exempt, and that is not a hole.
// CSRF exists because a cookie is *ambient* -- the browser attaches it to a
// request some other site caused. Nothing attaches an Authorization header on
// its own; a forged cross-site request from an attacker's page carries no key,
// so there is nothing to forge with. Requiring a token here would instead mean
// every script had to fetch one first, for no gain.
func ( guard *Guard ) CheckCSRF( c fiber.Ctx , submitted string ) ( ok bool ) {
	actor := ActorFrom( c )
	if actor == nil { return }
	if actor.ViaAPIKey() {
		ok = true
		return
	}
	if actor.Session == nil || submitted == "" { return }
	ok = encryption.ConstantTimeEqual( actor.Session.CSRFToken , submitted )
	return
}
