// whole-file feature:two_factor
package routes

import (
	fmt "fmt"
	strings "strings"
	testing "testing"

	fiber "github.com/gofiber/fiber/v3"

	encryption "{{MODULE_PATH}}/server/encryption"
	models "{{MODULE_PATH}}/server/models"
)

// Two-factor sign-in, through the real routes: every way of signing in being
// asked for a code, and the code working only once.

func TestRFC6238Vector( t *testing.T ) {
	// Appendix B of the RFC, seed "12345678901234567890", t = 59. The RFC
	// shows eight digits, 94287082; six is the same value's tail.
	code , err := encryption.TOTPCode( "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" , 59/30 )
	if err != nil || code != "287082" { t.Errorf( "%q %v" , code , err ) }
}

func TestTwoFactorSecretIsEncryptedOnItsOwn( t *testing.T ) {
	fixture := newAuthFixture( t )
	secret , _ := fixture.signIn( t , fixture.alice.ID ).enrol( t )
	stored , _ := models.GetUser( fixture.store , fixture.alice.ID )
	if stored.TOTPSecret == secret { t.Fatalf( "stored in the clear" ) }
	opened , err := encryption.ChaChaDecryptBase64String( fixture.cfg.SecretKey , stored.TOTPSecret )
	if err != nil || opened != secret { t.Errorf( "%q %v" , opened , err ) }
}

func TestLoginLinkAsksForTheCode( t *testing.T ) {
	fixture := newAuthFixture( t )
	secret , _ := fixture.signIn( t , fixture.alice.ID ).enrol( t )
	fresh := fixture.browser()
	_ , reply := fresh.post( t , "/api/login" , fiber.Map{ "credential": fixture.link( t , fixture.alice.ID ) } )
	if reply[ "two_factor" ] != true { t.Fatalf( "%v" , reply ) }
	if fresh.signedIn( t ) { t.Errorf( "signed in before the code" ) }
	if _ , wrong := fresh.post( t , "/api/login/two-factor" , fiber.Map{ "challenge": reply[ "challenge" ] , "code": "000000" } ); wrong[ "error" ] != "two_factor_wrong" {
		t.Errorf( "%v" , wrong )
	}
	if status , _ := fresh.post( t , "/api/login/two-factor" , fiber.Map{ "challenge": reply[ "challenge" ] , "code": codeFor( t , secret , 1 ) } ); status != 200 {
		t.Errorf( "right code: %d" , status )
	}
	if fresh.signedIn( t ) == false { t.Errorf( "not signed in" ) }
}

// >>> feature:passwords
func TestPasswordAsksForTheCode( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	alice.setPassword( t , "alice" , "correct horse" , "" )
	secret , _ := alice.enrol( t )
	fresh := fixture.browser()
	_ , reply := fresh.login( t , "alice" , "correct horse" )
	if reply[ "two_factor" ] != true { t.Fatalf( "%v" , reply ) }
	if fresh.signedIn( t ) { t.Errorf( "signed in before the code" ) }
	fresh.post( t , "/api/login/two-factor" , fiber.Map{ "challenge": reply[ "challenge" ] , "code": codeFor( t , secret , 1 ) } )
	if fresh.signedIn( t ) == false { t.Errorf( "not signed in" ) }
}
// <<< feature:passwords

func TestCodesAndChallengesWorkOnce( t *testing.T ) {
	fixture := newAuthFixture( t )
	secret , recovery := fixture.signIn( t , fixture.alice.ID ).enrol( t )
	if len( recovery ) != models.RecoveryCodeCount { t.Fatalf( "%d recovery codes" , len( recovery ) ) }

	code := codeFor( t , secret , 1 )
	if status , _ := fixture.finish( t , fixture.challenge( t , fixture.alice.ID ) , code ); status != 200 {
		t.Errorf( "first use: %d" , status )
	}
	if status , _ := fixture.finish( t , fixture.challenge( t , fixture.alice.ID ) , code ); status != 403 {
		t.Errorf( "replay: %d" , status )
	}

	challenge := fixture.challenge( t , fixture.alice.ID )
	if status , _ := fixture.finish( t , challenge , strings.ToLower( recovery[ 0 ] ) ); status != 200 {
		t.Errorf( "recovery code: %d" , status )
	}
	if _ , reply := fixture.finish( t , challenge , recovery[ 1 ] ); reply[ "error" ] != "challenge_invalid" {
		t.Errorf( "spent challenge: %v" , reply )
	}
	if status , _ := fixture.finish( t , fixture.challenge( t , fixture.alice.ID ) , recovery[ 0 ] ); status != 403 {
		t.Errorf( "spent recovery code: %d" , status )
	}
}

func TestWrongCodesLock( t *testing.T ) {
	fixture := newAuthFixture( t )
	secret , _ := fixture.signIn( t , fixture.alice.ID ).enrol( t )
	challenge := fixture.challenge( t , fixture.alice.ID )
	for range models.AttemptMaxFailures {
		fixture.finish( t , challenge , "000000" )
	}
	if _ , reply := fixture.finish( t , challenge , codeFor( t , secret , 1 ) ); reply[ "error" ] != "two_factor_locked" {
		t.Errorf( "%v" , reply )
	}
}

func TestTurningTwoFactorOffNeedsACode( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	secret , _ := alice.enrol( t )
	token := alice.csrf( t )
	if status , _ := alice.post( t , "/api/account/two-factor/disable" , fiber.Map{ "code": "" , "csrf_token": token } ); status != 403 {
		t.Errorf( "no code: %d" , status )
	}
	if status , _ := alice.post( t , "/api/account/two-factor/disable" , fiber.Map{ "code": codeFor( t , secret , 1 ) , "csrf_token": token } ); status != 200 {
		t.Errorf( "right code: %d" , status )
	}
	fixture.signIn( t , fixture.alice.ID )
}

// >>> feature:passwords
func TestChangingAPasswordNeedsACode( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	secret , _ := alice.enrol( t )
	if _ , reply := alice.setPassword( t , "alice" , "correct horse" , "" ); reply[ "error" ] != "two_factor_wrong" {
		t.Errorf( "%v" , reply )
	}
	if status , _ := alice.setPassword( t , "alice" , "correct horse" , codeFor( t , secret , 1 ) ); status != 200 {
		t.Errorf( "with a code: %d" , status )
	}
}
// <<< feature:passwords

func TestTwoFactorToggleStopsAskingAndRestores( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	alice.enrol( t )
	fixture.cfg.TwoFactorEnabled = false
	fixture.signIn( t , fixture.alice.ID )
	if status , _ := alice.post( t , "/api/account/two-factor/start" , fiber.Map{ "csrf_token": alice.csrf( t ) } ); status != 404 {
		t.Errorf( "setup while off: %d" , status )
	}
	fixture.cfg.TwoFactorEnabled = true
	fixture.challenge( t , fixture.alice.ID )
}

func TestDisablingEndsASignInInProgress( t *testing.T ) {
	fixture := newAuthFixture( t )
	secret , _ := fixture.signIn( t , fixture.alice.ID ).enrol( t )
	challenge := fixture.challenge( t , fixture.alice.ID )
	models.SetUserDisabled( fixture.store , fixture.alice.ID , true )
	if _ , reply := fixture.finish( t , challenge , codeFor( t , secret , 1 ) ); reply[ "error" ] != "challenge_invalid" {
		t.Errorf( "%v" , reply )
	}
}

func TestResetTwoFactorRules( t *testing.T ) {
	fixture := newAuthFixture( t )
	second , _ := models.CreateUser( fixture.store , "Second" , models.RoleAdmin )
	admin := fixture.signIn( t , second.ID )
	owner := fixture.signIn( t , fixture.owner.ID )
	alice := fixture.signIn( t , fixture.alice.ID )
	alice.enrol( t )
	owner.enrol( t )
	admin.enrol( t )

	reset := func( client *browser , user_id uint64 ) ( status int ) {
		status , _ = client.post( t , fmt.Sprintf( "/api/admin/users/%d/reset-two-factor" , user_id ) , fiber.Map{ "csrf_token": client.csrf( t ) } )
		return
	}
	for _ , check := range []struct {
		name   string
		client *browser
		target uint64
		want   int
	}{
		{ "an admin on the owner" , admin , fixture.owner.ID , 403 },
		{ "an admin on themselves" , admin , second.ID , 400 },
		{ "the owner on themselves" , owner , fixture.owner.ID , 400 },
		{ "an admin on a user" , admin , fixture.alice.ID , 200 },
		{ "the owner on an admin" , owner , second.ID , 200 },
	} {
		if status := reset( check.client , check.target ); status != check.want {
			t.Errorf( "%s: %d, want %d" , check.name , status , check.want )
		}
	}
	// The CLI may do what the web may not.
	if err := models.ResetTwoFactor( fixture.store , fixture.owner.ID , 0 ); err != nil { t.Fatal( err ) }
	stored , _ := models.GetUser( fixture.store , fixture.owner.ID )
	if stored.TwoFactorOn() { t.Errorf( "still on" ) }
}
