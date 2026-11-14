// whole-file feature:passwords
package routes

import (
	errors "errors"

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	db "{{MODULE_PATH}}/server/db"
	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// Password sign-in (models/password.go). None of this exists while
// auth.passwords is off -- RequirePasswords answers 404 -- except
// /api/login/options, which is how the signed-out page finds out.
//
// Passwords never reach these routes: password_digest is what the browser
// derived from one (static/js/password.js).

// How many wrong passwords one address gets, across every name. Every attempt
// costs a bcrypt, which is what this protects.
const passwordAddressMaxFailures = 30

type passwordLoginRequest struct {
	Username       string `json:"username"`
	PasswordDigest string `json:"password_digest"`
}

type setPasswordRequest struct {
	Username       string `json:"username"`
	PasswordDigest string `json:"password_digest"`
	// The page always sends it; without two-factor it is ignored.
	Code           string `json:"code,omitempty"`
}

type setPasswordReply struct {
	Ok          bool `json:"ok"`
	HasPassword bool `json:"has_password"`
}

type passwordAccessRequest struct {
	userIDPath
	Allowed bool `json:"allowed" error:"allowed must be true or false"`
}

type passwordAccessReply struct {
	Ok              bool `json:"ok"`
	PasswordBlocked bool `json:"password_blocked"`
}

type loginOptionsReply struct {
	Passwords bool   `json:"passwords"`
	TwoFactor bool   `json:"two_factor"` // feature:two_factor
	Salt      string `json:"salt,omitempty"`
}

// RequirePasswords makes every password route answer 404 while auth.passwords
// is off. Per request, for the same reason as RequireMCP.
func ( handlers *Handlers ) RequirePasswords( c fiber.Ctx ) ( err error ) {
	if handlers.cfg().PasswordsEnabled == false {
		err = notFound( c , "not found" )
		return
	}
	err = c.Next()
	return
}

// GetLoginOptions is what the signed-out page needs to know. Public, and the
// same for everyone: the salt is not a secret, and there is deliberately
// nothing here that depends on a username.
func ( handlers *Handlers ) GetLoginOptions() *apidoc.Endpoint[ apidoc.None , loginOptionsReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "What sign-in offers" , Tag: "Sign-in" , Access: apidoc.Public , Feature: "passwords" },
		func( c fiber.Ctx , in *apidoc.None ) ( reply *loginOptionsReply , err error ) {
			reply = &loginOptionsReply{
				Passwords: handlers.cfg().PasswordsEnabled,
				TwoFactor: handlers.cfg().TwoFactorEnabled, // feature:two_factor
			}
			if handlers.cfg().PasswordsEnabled {
				if reply.Salt , err = models.InstallSalt( handlers.Store ); err != nil { return }
			}
			return
		} )
}

// PasswordLogin signs in with a password. Unknown name, wrong password, a
// blocked or disabled account: all the same answer, after the same bcrypt. No
// CSRF token, for the same reason as /api/login.
func ( handlers *Handlers ) PasswordLogin() *apidoc.Endpoint[ passwordLoginRequest , signInReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Sign in with a password" , Tag: "Sign-in" , Access: apidoc.Public , Feature: "passwords" , Errors: []int{ 400 , 403 , 404 } },
		func( c fiber.Ctx , in *passwordLoginRequest ) ( reply *signInReply , err error ) {
			name := models.NormalizeUsername( in.Username )
			if len( name ) > 64 { name = name[ :64 ] }
			address := c.IP()
			// Reserved before the password is checked, so guesses sent in parallel
			// cannot all get past the lockout (see models.AttemptGuard).
			if handlers.PasswordNames.Reserve( name ) == false {
				err = refuse( fiber.StatusTooManyRequests , "login_locked" )
				return
			}
			if handlers.PasswordAddresses.Reserve( address ) == false {
				handlers.PasswordNames.Settle( name , models.AttemptVoid )
				err = refuse( fiber.StatusTooManyRequests , "login_locked" )
				return
			}
			name_outcome , address_outcome := models.AttemptVoid , models.AttemptVoid
			defer func() {
				handlers.PasswordNames.Settle( name , name_outcome )
				handlers.PasswordAddresses.Settle( address , address_outcome )
			}()

			var signed *models.SignIn
			user := models.CheckPassword( handlers.Store , name , in.PasswordDigest )
			if user != nil {
				if signed , err = models.StartPasswordSignIn( handlers.Store , user , handlers.signInRules() ); err != nil { return }
			}
			if signed == nil {
				name_outcome , address_outcome = models.AttemptFailed , models.AttemptFailed
				handlers.auditAs( c , logs.Anonymous() , logs.KindAuth , logs.LevelWarn , "password refused" , nil )
				err = refuse( fiber.StatusBadRequest , "invalid_login" )
				return
			}
			name_outcome = models.AttemptPassed
			message := "signed in with a password"
			if signed.Challenge != "" { message = "password accepted, waiting for a code" } // feature:two_factor
			handlers.auditAs( c , handlers.logUser( user.ID , security.MethodSession ) , logs.KindAuth , logs.LevelInfo , message , nil )
			challenge , err := handlers.signIn( c , signed )
			if err != nil { return }
			reply = newSignInReply( challenge )
			return
		} )
}

// sessionID is the id of the session this request came with, to leave it
// alone when the account's others end.
func ( handlers *Handlers ) sessionID( c fiber.Ctx ) ( session_id string ) {
	credential , err := handlers.Guard.DecryptCookie( c.Cookies( handlers.cfg().CookieName ) )
	if err != nil { return }
	session_id = models.SessionID( credential )
	return
}

// SetPassword sets the account's username and password, or with an empty
// password_digest removes the password. Session only: a key that could set one
// could turn itself into a session.
func ( handlers *Handlers ) SetPassword() *apidoc.Endpoint[ setPasswordRequest , setPasswordReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Set or clear your password" , Tag: "Account" , Access: apidoc.SessionOnly , Feature: "passwords" , Errors: []int{ 400 , 401 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *setPasswordRequest ) ( reply *setPasswordReply , err error ) {
			user_id := security.UserFrom( c ).ID
			// >>> feature:two_factor
			cfg := handlers.cfg()
			if handlers.Codes.Reserve( user_id ) == false {
				err = refuse( fiber.StatusTooManyRequests , "two_factor_locked" )
				return
			}
			// Reserved before the code is checked (see models.AttemptGuard). A right
			// code here forgives nothing, as before: only a wrong one counts.
			code_outcome := models.AttemptVoid
			defer func() { handlers.Codes.Settle( user_id , code_outcome ) }()
			// <<< feature:two_factor
			set_err := models.SetPassword( handlers.Store , user_id , in.Username , in.PasswordDigest , handlers.sessionID( c ) , in.Code , cfg.TwoFactorEnabled , cfg.SecretKey ) // feature:two_factor
			set_err := models.SetPassword( handlers.Store , user_id , in.Username , in.PasswordDigest , handlers.sessionID( c ) ) // feature:!two_factor
			switch {
			case errors.Is( set_err , models.ErrBadUsername ):
				err = refuse( fiber.StatusBadRequest , "bad_username" )
				return
			case errors.Is( set_err , models.ErrBadDigest ):
				err = refuse( fiber.StatusBadRequest , "bad_digest" )
				return
			case errors.Is( set_err , models.ErrUsernameTaken ):
				err = refuse( fiber.StatusConflict , "username_taken" )
				return
			case errors.Is( set_err , models.ErrPasswordBlocked ):
				err = refuse( fiber.StatusForbidden , "password_blocked" )
				return
			// >>> feature:two_factor
			case errors.Is( set_err , models.ErrWrongCode ):
				code_outcome = models.AttemptFailed
				err = refuse( fiber.StatusForbidden , "two_factor_wrong" )
				return
			// <<< feature:two_factor
			case set_err != nil:
				err = set_err
				return
			}
			message := "password changed"
			if in.PasswordDigest == "" { message = "password removed" }
			handlers.audit( c , logs.KindAuth , message , nil )
			reply = &setPasswordReply{ Ok: true , HasPassword: in.PasswordDigest != "" }
			return
		} )
}

// SetPasswordAccess is the owner's per-account switch (RequireOwner).
// {allowed: false} stops this account's password working and stops it setting
// one.
func ( handlers *Handlers ) SetPasswordAccess() *apidoc.Endpoint[ passwordAccessRequest , passwordAccessReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Allow or block an account's password" , Tag: "Owner" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "passwords" , Errors: []int{ 400 , 403 , 404 } },
		func( c fiber.Ctx , in *passwordAccessRequest ) ( reply *passwordAccessReply , err error ) {
			block_err := models.SetPasswordBlocked( handlers.Store , in.UserID , in.Allowed == false )
			if errors.Is( block_err , db.ErrNotFound ) {
				err = refuse( fiber.StatusNotFound , "no such user" )
				return
			}
			if block_err != nil {
				err = block_err
				return
			}
			handlers.audit( c , logs.KindAdmin , "password access set" , fiber.Map{ "user_id": in.UserID , "allowed": in.Allowed } )
			reply = &passwordAccessReply{ Ok: true , PasswordBlocked: in.Allowed == false }
			return
		} )
}
