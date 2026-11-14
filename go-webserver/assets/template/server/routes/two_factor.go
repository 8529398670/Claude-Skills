// whole-file feature:two_factor
package routes

import (
	errors "errors"

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// Two-factor sign-in, from a person's point of view: finishing a sign-in with
// a code, and turning it on and off on their account (models/two_factor.go,
// models/sign_in.go).
//
// Setting it up needs a browser session, like every route that touches how an
// account signs in: a key that could change this could lock its owner out, or
// remove what protects them.

type finishSignInRequest struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}

type codeRequest struct {
	Code string `json:"code"`
}

type startTwoFactorReply struct {
	Secret string `json:"secret"`
}

type recoveryCodesReply struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

// RequireTwoFactor makes setting up 2FA answer 404 while auth.two_factor is
// off. Finishing a sign-in is not behind this: one already under way may as
// well end.
func ( handlers *Handlers ) RequireTwoFactor( c fiber.Ctx ) ( err error ) {
	if handlers.cfg().TwoFactorEnabled == false {
		err = notFound( c , "not found" )
		return
	}
	err = c.Next()
	return
}

// FinishSignIn is the second half of a sign-in. No CSRF token, for the same
// reason as /api/login: there is no session yet to hold one, and the JSON body
// rule (sameOrigin, in routes.go) guards it the same way.
func ( handlers *Handlers ) FinishSignIn() *apidoc.Endpoint[ finishSignInRequest , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Finish a sign-in with a code" , Tag: "Sign-in" , Access: apidoc.Public , Feature: "two_factor" , Errors: []int{ 400 , 403 } },
		func( c fiber.Ctx , in *finishSignInRequest ) ( reply *okReply , err error ) {
			user_id , ok := models.ChallengeUserID( handlers.Store , in.Challenge )
			if ok == false {
				err = refuse( fiber.StatusBadRequest , "challenge_invalid" )
				return
			}
			// Reserved before the code is checked, so guesses sent in parallel
			// cannot all get past the lockout (see models.AttemptGuard).
			if handlers.Codes.Reserve( user_id ) == false {
				err = refuse( fiber.StatusTooManyRequests , "two_factor_locked" )
				return
			}
			outcome := models.AttemptVoid
			defer func() { handlers.Codes.Settle( user_id , outcome ) }()

			cfg := handlers.cfg()
			signed , method , finish_err := models.FinishChallenge( handlers.Store , in.Challenge , in.Code , cfg.SessionTTL , cfg.SecretKey )
			switch {
			case errors.Is( finish_err , models.ErrChallengeInvalid ):
				err = refuse( fiber.StatusBadRequest , "challenge_invalid" )
				return
			case errors.Is( finish_err , models.ErrWrongCode ):
				outcome = models.AttemptFailed
				handlers.auditAs( c , handlers.logUser( user_id , "anon" ) , logs.KindAuth , logs.LevelWarn , "wrong two-factor code" , nil )
				err = refuse( fiber.StatusForbidden , "two_factor_wrong" )
				return
			case finish_err != nil:
				err = finish_err
				return
			}
			outcome = models.AttemptPassed
			message := "signed in with a two-factor code"
			if method == models.CodeMethodRecovery { message = "signed in with a recovery code" }
			handlers.auditAs( c , handlers.logUser( user_id , security.MethodSession ) , logs.KindAuth , logs.LevelInfo , message , nil )
			if _ , err = handlers.signIn( c , signed ); err != nil { return }
			reply = &okReply{ Ok: true }
			return
		} )
}

// StartTwoFactor makes a new secret for the person's authenticator app. The
// page builds the QR code from it, so nothing about it is stored as anything
// but cipher text and it is shown only here.
func ( handlers *Handlers ) StartTwoFactor() *apidoc.Endpoint[ apidoc.None , startTwoFactorReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Begin two-factor setup" , Tag: "Account" , Access: apidoc.SessionOnly , Feature: "two_factor" , Errors: []int{ 400 , 401 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *apidoc.None ) ( reply *startTwoFactorReply , err error ) {
			secret , start_err := models.StartTwoFactorSetup( handlers.Store , security.UserFrom( c ).ID , handlers.cfg().SecretKey )
			if errors.Is( start_err , models.ErrTwoFactorOn ) {
				err = refuse( fiber.StatusConflict , "two_factor_on" )
				return
			}
			if start_err != nil {
				err = start_err
				return
			}
			reply = &startTwoFactorReply{ Secret: secret }
			return
		} )
}

// ConfirmTwoFactor takes the code that proves the app has the secret, and
// answers with the recovery codes, which are never shown again.
func ( handlers *Handlers ) ConfirmTwoFactor() *apidoc.Endpoint[ codeRequest , recoveryCodesReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Turn two-factor on" , Tag: "Account" , Access: apidoc.SessionOnly , Feature: "two_factor" , Errors: []int{ 400 , 401 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *codeRequest ) ( reply *recoveryCodesReply , err error ) {
			user_id := security.UserFrom( c ).ID
			if handlers.Codes.Reserve( user_id ) == false {
				err = refuse( fiber.StatusTooManyRequests , "two_factor_locked" )
				return
			}
			outcome := models.AttemptVoid
			defer func() { handlers.Codes.Settle( user_id , outcome ) }()
			codes , confirm_err := models.ConfirmTwoFactorSetup( handlers.Store , user_id , in.Code , handlers.cfg().SecretKey )
			switch {
			case errors.Is( confirm_err , models.ErrTwoFactorOn ):
				err = refuse( fiber.StatusConflict , "two_factor_on" )
				return
			case errors.Is( confirm_err , models.ErrWrongCode ):
				outcome = models.AttemptFailed
				err = refuse( fiber.StatusForbidden , "two_factor_wrong" )
				return
			case confirm_err != nil:
				err = confirm_err
				return
			}
			outcome = models.AttemptPassed
			handlers.audit( c , logs.KindAuth , "two-factor sign-in turned on" , nil )
			reply = &recoveryCodesReply{ RecoveryCodes: codes }
			return
		} )
}

// DisableTwoFactor switches it off again, given a code: a session alone is
// not enough to remove what protects the account.
func ( handlers *Handlers ) DisableTwoFactor() *apidoc.Endpoint[ codeRequest , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Turn two-factor off" , Tag: "Account" , Access: apidoc.SessionOnly , Feature: "two_factor" , Errors: []int{ 400 , 401 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *codeRequest ) ( reply *okReply , err error ) {
			user_id := security.UserFrom( c ).ID
			if handlers.Codes.Reserve( user_id ) == false {
				err = refuse( fiber.StatusTooManyRequests , "two_factor_locked" )
				return
			}
			outcome := models.AttemptVoid
			defer func() { handlers.Codes.Settle( user_id , outcome ) }()
			disable_err := models.DisableTwoFactor( handlers.Store , user_id , in.Code , handlers.cfg().SecretKey )
			switch {
			case errors.Is( disable_err , models.ErrTwoFactorOff ):
				err = refuse( fiber.StatusConflict , "two_factor_off" )
				return
			case errors.Is( disable_err , models.ErrWrongCode ):
				outcome = models.AttemptFailed
				err = refuse( fiber.StatusForbidden , "two_factor_wrong" )
				return
			case disable_err != nil:
				err = disable_err
				return
			}
			outcome = models.AttemptPassed
			handlers.audit( c , logs.KindAuth , "two-factor sign-in turned off" , nil )
			reply = &okReply{ Ok: true }
			return
		} )
}
