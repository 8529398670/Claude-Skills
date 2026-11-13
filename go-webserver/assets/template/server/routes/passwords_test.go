// whole-file feature:passwords
package routes

import (
	fmt "fmt"
	http "net/http"
	strings "strings"
	testing "testing"
	time "time" // feature:invite_links

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
)

// Password sign-in, through the real routes: the toggle, the per-account block,
// and the answers that must not say which part was wrong.

func TestPasswordSetThenSignIn( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	if status , reply := alice.setPassword( t , "Alice" , "correct horse" , "" ); status != 200 {
		t.Fatalf( "set: %d %v" , status , reply )
	}
	stored , _ := models.GetUser( fixture.store , fixture.alice.ID )
	if stored.Username != "alice" || strings.HasPrefix( stored.PasswordHash , "$2" ) == false {
		t.Errorf( "stored: %q %q" , stored.Username , stored.PasswordHash )
	}
	fresh := fixture.browser()
	if status , reply := fresh.login( t , "ALICE" , "correct horse" ); status != 200 {
		t.Fatalf( "login: %d %v" , status , reply )
	}
	if fresh.signedIn( t ) == false { t.Errorf( "not signed in" ) }
}

func TestEveryPasswordFailureAnswersTheSame( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	alice.setPassword( t , "alice" , "correct horse" , "" )
	owner := fixture.signIn( t , fixture.owner.ID )
	owner.setPassword( t , "owner" , "correct horse" , "" )
	models.SetPasswordBlocked( fixture.store , fixture.owner.ID , true )
	dee_user , _ := models.CreateUser( fixture.store , "Dee" , models.RoleUser )
	dee := fixture.signIn( t , dee_user.ID )
	dee.setPassword( t , "dee" , "correct horse" , "" )
	models.SetUserDisabled( fixture.store , dee_user.ID , true )

	for name , attempt := range map[ string ][ 2 ]string{
		"wrong":    { "alice" , "wrong" },
		"unknown":  { "nobody" , "correct horse" },
		"blocked":  { "owner" , "correct horse" },
		"disabled": { "dee" , "correct horse" },
	} {
		status , reply := fixture.browser().login( t , attempt[ 0 ] , attempt[ 1 ] )
		if status != 400 || reply[ "error" ] != "invalid_login" {
			t.Errorf( "%s: %d %v" , name , status , reply )
		}
	}
}

func TestARawPasswordIsRefused( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	_ , reply := alice.post( t , "/api/account/password" , fiber.Map{
		"username": "alice" , "password_digest": "hunter2" , "csrf_token": alice.csrf( t ) ,
	} )
	if reply[ "error" ] != "bad_digest" { t.Errorf( "%v" , reply ) }
}

func TestPasswordLockoutTreatsUnknownNamesAlike( t *testing.T ) {
	fixture := newAuthFixture( t )
	for _ , name := range []string{ "alice" , "nobody" } {
		for range models.AttemptMaxFailures {
			fixture.browser().login( t , name , "wrong" )
		}
		if _ , reply := fixture.browser().login( t , name , "wrong" ); reply[ "error" ] != "login_locked" {
			t.Errorf( "%s: %v" , name , reply )
		}
	}
}

func TestUsernamesAreUnique( t *testing.T ) {
	fixture := newAuthFixture( t )
	fixture.signIn( t , fixture.alice.ID ).setPassword( t , "same" , "correct horse" , "" )
	_ , reply := fixture.signIn( t , fixture.owner.ID ).setPassword( t , "Same" , "correct horse" , "" )
	if reply[ "error" ] != "username_taken" { t.Errorf( "%v" , reply ) }
}

func TestChangingAPasswordEndsOtherSessionsOnly( t *testing.T ) {
	fixture := newAuthFixture( t )
	first := fixture.signIn( t , fixture.alice.ID )
	second := fixture.signIn( t , fixture.alice.ID )
	first.setPassword( t , "alice" , "correct horse" , "" )
	if first.signedIn( t ) == false { t.Errorf( "this session ended" ) }
	if second.signedIn( t ) { t.Errorf( "the other session survived" ) }
}

func TestPasswordToggleClosesAndRestores( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	alice.setPassword( t , "alice" , "correct horse" , "" )

	fixture.cfg.PasswordsEnabled = false
	if status , _ := fixture.browser().login( t , "alice" , "correct horse" ); status != 404 {
		t.Errorf( "login while off: %d" , status )
	}
	if status , _ := alice.setPassword( t , "alice" , "another one" , "" ); status != 404 {
		t.Errorf( "set while off: %d" , status )
	}
	_ , options := fixture.browser().do( t , http.MethodGet , "/api/login/options" , nil )
	if options[ "passwords" ] != false || options[ "salt" ] != nil { t.Errorf( "options: %v" , options ) }

	fixture.cfg.PasswordsEnabled = true
	if status , _ := fixture.browser().login( t , "alice" , "correct horse" ); status != 200 {
		t.Errorf( "login after restoring: %d" , status )
	}
	_ , options = fixture.browser().do( t , http.MethodGet , "/api/login/options" , nil )
	if salt , _ := options[ "salt" ].( string ); salt == "" { t.Errorf( "no salt: %v" , options ) }
}

func TestOwnerBlocksOneAccount( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	owner := fixture.signIn( t , fixture.owner.ID )
	alice.setPassword( t , "alice" , "correct horse" , "" )

	path := fmt.Sprintf( "/api/owner/users/%d/password-access" , fixture.alice.ID )
	if status , _ := alice.post( t , path , fiber.Map{ "allowed": false , "csrf_token": alice.csrf( t ) } ); status != 404 {
		t.Errorf( "a user: %d" , status )
	}
	if status , _ := owner.post( t , path , fiber.Map{ "allowed": false , "csrf_token": owner.csrf( t ) } ); status != 200 {
		t.Errorf( "the owner: %d" , status )
	}
	if status , _ := fixture.browser().login( t , "alice" , "correct horse" ); status != 400 {
		t.Errorf( "blocked login: %d" , status )
	}
	if _ , reply := alice.setPassword( t , "alice" , "another one" , "" ); reply[ "error" ] != "password_blocked" {
		t.Errorf( "blocked set: %v" , reply )
	}
	if alice.signedIn( t ) == false { t.Errorf( "a block is not a sign-out" ) }

	owner.post( t , path , fiber.Map{ "allowed": true , "csrf_token": owner.csrf( t ) } )
	if status , _ := fixture.browser().login( t , "alice" , "correct horse" ); status != 200 {
		t.Errorf( "after unblocking: %d" , status )
	}
}

func TestRevokingAccessRemovesThePassword( t *testing.T ) {
	fixture := newAuthFixture( t )
	fixture.signIn( t , fixture.alice.ID ).setPassword( t , "alice" , "correct horse" , "" )
	revoked , err := models.RevokeAllCredentials( fixture.store , fixture.alice.ID , false )
	if err != nil || revoked.Passwords != 1 { t.Fatalf( "%v %v" , revoked , err ) }
	if status , _ := fixture.browser().login( t , "alice" , "correct horse" ); status != 400 {
		t.Errorf( "login after revoking: %d" , status )
	}
}

// >>> feature:invite_links
func TestInviteClaimAndPasswords( t *testing.T ) {
	fixture := newAuthFixture( t )
	credential , _ , err := models.IssueInviteLink( fixture.store , "Club" , 10 , 24*time.Hour , fixture.owner.ID , "" )
	if err != nil { t.Fatal( err ) }
	bob := fixture.browser()
	if status , reply := bob.post( t , "/api/join/create" , fiber.Map{ "credential": credential , "display_name": "Bob" } ); status != 201 {
		t.Fatalf( "join: %d %v" , status , reply )
	}
	_ , me := bob.do( t , http.MethodGet , "/api/me" , nil )
	bob_id := me[ "id" ]
	claim := func( client *browser , pin string ) ( status int , reply map[ string ]any ) {
		status , reply = client.post( t , "/api/join/claim" , fiber.Map{ "credential": credential , "user_id": bob_id , "pin": pin } )
		return
	}

	if status , reply := claim( fixture.browser() , "" ); status != 200 { t.Fatalf( "claim: %d %v" , status , reply ) }
	bob.setPassword( t , "bob" , "correct horse" , "" )
	if _ , reply := claim( fixture.browser() , "" ); reply[ "error" ] != "use_password" {
		t.Errorf( "a password should close the claim: %v" , reply )
	}
	bob.post( t , "/api/account/pin" , fiber.Map{ "pin": "1234" , "csrf_token": bob.csrf( t ) } )
	if status , reply := claim( fixture.browser() , "1234" ); status != 200 {
		t.Errorf( "a PIN should keep it open: %d %v" , status , reply )
	}

	// >>> feature:two_factor
	secret , _ := bob.enrol( t )
	fresh := fixture.browser()
	_ , reply := claim( fresh , "1234" )
	if reply[ "two_factor" ] != true { t.Fatalf( "a claim did not ask for a code: %v" , reply ) }
	if fresh.signedIn( t ) { t.Errorf( "signed in before the code" ) }
	if status , finished := fresh.post( t , "/api/login/two-factor" , fiber.Map{ "challenge": reply[ "challenge" ] , "code": codeFor( t , secret , 1 ) } ); status != 200 {
		t.Fatalf( "finish: %d %v" , status , finished )
	}
	if fresh.signedIn( t ) == false { t.Errorf( "not signed in after the code" ) }
	// <<< feature:two_factor
}
// <<< feature:invite_links
