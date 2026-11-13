package routes

import (
	fmt "fmt"
	http "net/http"
	strings "strings" // feature:api_keys
	testing "testing"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
)

// Admins editing other accounts: who may touch whom, and what a change of role
// does to what the account already holds.

func userPath( user *models.User , verb string ) ( path string ) {
	path = fmt.Sprintf( "/api/admin/users/%d/%s" , user.ID , verb )
	return
}

func secondAdmin( t *testing.T , fixture *authFixture ) ( admin *models.User ) {
	t.Helper()
	admin , err := models.CreateUser( fixture.store , "Second admin" , models.RoleAdmin )
	if err != nil { t.Fatal( err ) }
	return
}

func TestRenamingSomeoneElse( t *testing.T ) {
	fixture := newAuthFixture( t )
	owner := fixture.signIn( t , fixture.owner.ID )
	alice := fixture.signIn( t , fixture.alice.ID )
	path := userPath( fixture.alice , "rename" )

	status , reply := send( t , owner , path , fiber.Map{ "display_name": "  Alice B  " } )
	if status != fiber.StatusOK || reply[ "display_name" ] != "Alice B" { t.Fatalf( "%d %v" , status , reply ) }
	if status , _ = send( t , owner , path , fiber.Map{ "display_name": "  " } ); status != fiber.StatusBadRequest { t.Fatalf( "%d" , status ) }
	if status , _ = send( t , alice , path , fiber.Map{ "display_name": "x" } ); status != fiber.StatusForbidden { t.Fatalf( "%d" , status ) }

	// An undo that finds a different name leaves it alone, and says what is there.
	status , reply = send( t , owner , path , fiber.Map{ "display_name": "old" , "expect": "stale" } )
	if status != fiber.StatusConflict || reply[ "display_name" ] != "Alice B" { t.Fatalf( "%d %v" , status , reply ) }

	if status , _ = send( t , owner , "/api/admin/users/9999/rename" , fiber.Map{ "display_name": "x" } ); status != fiber.StatusNotFound { t.Fatalf( "%d" , status ) }
}

func TestOnlyTheOwnerRenamesAnAdmin( t *testing.T ) {
	fixture := newAuthFixture( t )
	second := secondAdmin( t , fixture )
	owner := fixture.signIn( t , fixture.owner.ID )
	admin := fixture.signIn( t , second.ID )

	if status , _ := send( t , admin , userPath( fixture.alice , "rename" ) , fiber.Map{ "display_name": "fine" } ); status != fiber.StatusOK { t.Fatalf( "%d" , status ) }
	if status , _ := send( t , admin , userPath( fixture.owner , "rename" ) , fiber.Map{ "display_name": "x" } ); status != fiber.StatusForbidden { t.Fatalf( "%d" , status ) }
	if status , _ := send( t , admin , userPath( second , "rename" ) , fiber.Map{ "display_name": "x" } ); status != fiber.StatusForbidden { t.Fatalf( "%d" , status ) }
	if status , _ := send( t , owner , userPath( second , "rename" ) , fiber.Map{ "display_name": "Sam" } ); status != fiber.StatusOK { t.Fatalf( "%d" , status ) }
}

func TestBecomingAnAdminEndsWhatWasOpen( t *testing.T ) {
	fixture := newAuthFixture( t )
	owner := fixture.signIn( t , fixture.owner.ID )
	alice := fixture.signIn( t , fixture.alice.ID )
	credential , err := models.IssueLoginToken( fixture.store , fixture.alice.ID , time.Hour )
	if err != nil { t.Fatal( err ) }
	path := userPath( fixture.alice , "role" )

	if status , _ := send( t , owner , path , fiber.Map{ "role": "owner" } ); status != fiber.StatusBadRequest { t.Fatalf( "%d" , status ) }
	if status , _ := send( t , alice , path , fiber.Map{ "role": "admin" } ); status != fiber.StatusForbidden { t.Fatalf( "%d" , status ) }
	if status , reply := send( t , owner , path , fiber.Map{ "role": "admin" } ); status != fiber.StatusOK { t.Fatalf( "%d %v" , status , reply ) }

	user , _ := models.GetUser( fixture.store , fixture.alice.ID )
	if user.IsAdmin() == false { t.Fatalf( "%+v" , user ) }
	if status , _ := alice.do( t , http.MethodGet , "/api/me" , nil ); status != fiber.StatusUnauthorized { t.Fatalf( "%d" , status ) }
	if _ , redeem_err := models.RedeemLoginToken( fixture.store , credential , models.SignInRules{ SessionTTL: time.Hour } ); redeem_err == nil {
		t.Fatal( "a link from before the promotion still works" )
	}
	if status , reply := fixture.signIn( t , fixture.alice.ID ).do( t , http.MethodGet , "/api/me" , nil ); status != fiber.StatusOK || reply[ "role" ] != "admin" {
		t.Fatalf( "%d %v" , status , reply )
	}
}

func TestAskingForTheRoleTheyHaveChangesNothing( t *testing.T ) {
	fixture := newAuthFixture( t )
	owner := fixture.signIn( t , fixture.owner.ID )
	alice := fixture.signIn( t , fixture.alice.ID )
	if status , _ := send( t , owner , userPath( fixture.alice , "role" ) , fiber.Map{ "role": "user" } ); status != fiber.StatusOK { t.Fatalf( "%d" , status ) }
	if status , _ := alice.do( t , http.MethodGet , "/api/me" , nil ); status != fiber.StatusOK { t.Fatalf( "%d" , status ) }
}

func TestLosingAdminRightsAppliesAtOnce( t *testing.T ) {
	fixture := newAuthFixture( t )
	second := secondAdmin( t , fixture )
	owner := fixture.signIn( t , fixture.owner.ID )
	admin := fixture.signIn( t , second.ID )
	// >>> feature:invite_links
	link_credential , _ , err := models.IssueInviteLink( fixture.store , "Club" , 5 , time.Hour , second.ID , "" )
	if err != nil { t.Fatal( err ) }
	// <<< feature:invite_links

	if status , _ := send( t , owner , userPath( second , "role" ) , fiber.Map{ "role": "user" } ); status != fiber.StatusOK { t.Fatalf( "%d" , status ) }

	// Still signed in, but as a user, and any link they made has stopped.
	if status , reply := admin.do( t , http.MethodGet , "/api/me" , nil ); status != fiber.StatusOK || reply[ "role" ] != "user" { t.Fatalf( "%d %v" , status , reply ) }
	if status , _ := admin.do( t , http.MethodGet , "/api/admin/users" , nil ); status != fiber.StatusForbidden { t.Fatalf( "%d" , status ) }
	if _ , load_err := models.LoadInviteLink( fixture.store , link_credential ); load_err == nil { t.Fatal( "their link still works" ) } // feature:invite_links

	// Made an admin again, the link is back, as the screen's question says.
	if status , _ := send( t , owner , userPath( second , "role" ) , fiber.Map{ "role": "admin" } ); status != fiber.StatusOK { t.Fatalf( "%d" , status ) }
	if _ , load_err := models.LoadInviteLink( fixture.store , link_credential ); load_err != nil { t.Fatal( load_err ) } // feature:invite_links
}

func TestWhoMayChangeWhoseRole( t *testing.T ) {
	fixture := newAuthFixture( t )
	second := secondAdmin( t , fixture )
	third := secondAdmin( t , fixture )
	owner := fixture.signIn( t , fixture.owner.ID )
	admin := fixture.signIn( t , second.ID )

	cases := []struct {
		client *browser
		target *models.User
		role   string
		want   int
	}{
		{ owner , fixture.owner , "user" , fiber.StatusBadRequest }, // yourself
		{ admin , second , "user" , fiber.StatusBadRequest },        // yourself
		{ admin , fixture.owner , "user" , fiber.StatusForbidden },  // the owner
		{ admin , third , "user" , fiber.StatusForbidden },          // another admin
		{ admin , fixture.alice , "admin" , fiber.StatusOK },        // anyone may make one
	}
	for index , item := range cases {
		if status , reply := send( t , item.client , userPath( item.target , "role" ) , fiber.Map{ "role": item.role } ); status != item.want {
			t.Fatalf( "case %d: %d %v" , index , status , reply )
		}
	}
	if status , _ := send( t , owner , "/api/admin/users/9999/role" , fiber.Map{ "role": "admin" } ); status != fiber.StatusNotFound { t.Fatalf( "%d" , status ) }
}

// >>> feature:api_keys
func TestAnAdminKeyManagesAccounts( t *testing.T ) {
	fixture := newAuthFixture( t )
	key , _ , err := models.IssueAPIKey( fixture.store , fixture.owner.ID , "ops" , models.RoleAdmin , time.Hour )
	if err != nil { t.Fatal( err ) }

	status , reply := fixture.withKey( t , key , http.MethodPost , "/api/admin/users" , fiber.Map{ "display_name": "Bob" , "role": "user" } )
	if status != fiber.StatusCreated { t.Fatalf( "create: %d %v" , status , reply ) }
	if fixture.loginWith( t , reply[ "login_path" ].( string ) ) == false { t.Fatal( "the new account's link did not sign in" ) }

	if status , reply = fixture.withKey( t , key , http.MethodPost , userPath( fixture.alice , "reissue-login" ) , fiber.Map{} ); status != fiber.StatusOK { t.Fatalf( "reissue: %d %v" , status , reply ) }
	if fixture.loginWith( t , reply[ "login_path" ].( string ) ) == false { t.Fatal( "the reissued link did not sign in" ) }

	if status , reply = fixture.withKey( t , key , http.MethodPost , userPath( fixture.alice , "rename" ) , fiber.Map{ "display_name": "Alicia" } ); status != fiber.StatusOK { t.Fatalf( "rename: %d %v" , status , reply ) }
	if status , reply = fixture.withKey( t , key , http.MethodPost , userPath( fixture.alice , "role" ) , fiber.Map{ "role": "admin" } ); status != fiber.StatusOK { t.Fatalf( "role: %d %v" , status , reply ) }
	// The owner rules hold through a key as through a browser.
	if status , _ = fixture.withKey( t , key , http.MethodPost , userPath( fixture.owner , "delete" ) , fiber.Map{} ); status != fiber.StatusBadRequest { t.Fatalf( "deleting yourself: %d" , status ) }
	if status , reply = fixture.withKey( t , key , http.MethodPost , userPath( fixture.alice , "delete" ) , fiber.Map{} ); status != fiber.StatusOK { t.Fatalf( "delete: %d %v" , status , reply ) }

	// A user-scoped key on the same admin account is not an admin.
	user_key , _ , err := models.IssueAPIKey( fixture.store , fixture.owner.ID , "narrow" , models.RoleUser , time.Hour )
	if err != nil { t.Fatal( err ) }
	if status , _ = fixture.withKey( t , user_key , http.MethodPost , "/api/admin/users" , fiber.Map{ "display_name": "Eve" , "role": "admin" } ); status != fiber.StatusForbidden { t.Fatalf( "narrowed key made an account: %d" , status ) }
}

// A login link becomes a full-role session, so only a key carrying the
// account's full role may make one for its own account.
func TestOnlyAFullRoleKeyMakesItsOwnLoginLink( t *testing.T ) {
	fixture := newAuthFixture( t )
	narrow , _ , err := models.IssueAPIKey( fixture.store , fixture.owner.ID , "narrow" , models.RoleUser , time.Hour )
	if err != nil { t.Fatal( err ) }
	if status , reply := fixture.withKey( t , narrow , http.MethodPost , "/api/account/login-link" , fiber.Map{} ); status != fiber.StatusForbidden || strings.Contains( reply[ "error" ].( string ) , "less than your account's role" ) == false {
		t.Fatalf( "narrowed key: %d %v" , status , reply )
	}
	full , _ , err := models.IssueAPIKey( fixture.store , fixture.alice.ID , "full" , models.RoleUser , time.Hour )
	if err != nil { t.Fatal( err ) }
	status , reply := fixture.withKey( t , full , http.MethodPost , "/api/account/login-link" , fiber.Map{} )
	if status != fiber.StatusOK || fixture.loginWith( t , reply[ "login_path" ].( string ) ) == false { t.Fatalf( "full key: %d %v" , status , reply ) }
}

// Revoking or deleting a key voids the login links it made that are still
// unused; a link a person made in the browser is left alone.
func TestRevokingAKeyVoidsTheLinksItMade( t *testing.T ) {
	for _ , how := range []string{ "revoke" , "delete" } {
		fixture := newAuthFixture( t )
		key , record , err := models.IssueAPIKey( fixture.store , fixture.owner.ID , "ops" , models.RoleAdmin , time.Hour )
		if err != nil { t.Fatal( err ) }
		_ , made_by_key := fixture.withKey( t , key , http.MethodPost , "/api/admin/users" , fiber.Map{ "display_name": "Bob" , "role": "user" } )
		_ , made_by_person := send( t , fixture.signIn( t , fixture.owner.ID ) , userPath( fixture.alice , "reissue-login" ) , fiber.Map{} )

		if how == "revoke" {
			err = models.RevokeAPIKey( fixture.store , record.ID )
		} else {
			err = models.DeleteAPIKey( fixture.store , record.ID , fixture.owner.ID )
		}
		if err != nil { t.Fatal( err ) }
		if fixture.loginWith( t , made_by_key[ "login_path" ].( string ) ) { t.Fatalf( "%s: the key's link still signs in" , how ) }
		if fixture.loginWith( t , made_by_person[ "login_path" ].( string ) ) == false { t.Fatalf( "%s: a person's link stopped working" , how ) }
	}
}
// <<< feature:api_keys
