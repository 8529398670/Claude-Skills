package routes

import (
	encoding_json "encoding/json"
	io "io"
	http "net/http"
	strings "strings"
	testing "testing"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	db "{{MODULE_PATH}}/server/db"
	jobs "{{MODULE_PATH}}/server/jobs"
	models "{{MODULE_PATH}}/server/models"
)

// devices is a browser's GET /api/account/sessions, which answers a list.
func ( client *browser ) devices( t *testing.T ) ( rows []map[ string ]any ) {
	t.Helper()
	request , _ := http.NewRequest( http.MethodGet , "/api/account/sessions" , nil )
	request.Header.Set( "Cookie" , client.cookie )
	request.Header.Set( "User-Agent" , "test-agent" )
	response , err := client.fixture.app.Test( request , fiber.TestConfig{ Timeout: 10 * time.Second } )
	if err != nil { t.Fatal( err ) }
	raw , _ := io.ReadAll( response.Body )
	if response.StatusCode != http.StatusOK {
		t.Fatalf( "status %d: %s" , response.StatusCode , raw )
	}
	if strings.Contains( string( raw ) , "secret_hash" ) || strings.Contains( string( raw ) , "csrf" ) {
		t.Fatalf( "the list leaks a credential: %s" , raw )
	}
	if err = encoding_json.Unmarshal( raw , &rows ); err != nil { t.Fatal( err ) }
	return
}

func other( t *testing.T , rows []map[ string ]any ) ( id string ) {
	t.Helper()
	for _ , row := range rows {
		if row[ "current" ] == false {
			id , _ = row[ "id" ].( string )
			return
		}
	}
	t.Fatal( "no other session listed" )
	return
}

func TestDevicesListMarksThisOneAndRecordsTheSignIn( t *testing.T ) {
	fixture := newAuthFixture( t )
	first := fixture.signIn( t , fixture.alice.ID )
	second := fixture.signIn( t , fixture.alice.ID )
	fixture.signIn( t , fixture.owner.ID )

	rows := first.devices( t )
	if len( rows ) != 2 {
		t.Fatalf( "someone else's session is listed, or one is missing: %v" , rows )
	}
	current := 0
	for _ , row := range rows {
		if row[ "current" ] == true { current += 1 }
		if row[ "last_seen_at" ] == nil || row[ "ip" ] == nil {
			t.Fatalf( "sign-in details were not recorded: %v" , row )
		}
	}
	if current != 1 {
		t.Fatalf( "%d sessions marked current" , current )
	}
	if second.signedIn( t ) == false {
		t.Fatal( "listing ended a session" )
	}
}

func TestTwoFactorSignInRecordsTheDevice( t *testing.T ) {
	fixture := newAuthFixture( t )
	client := fixture.signIn( t , fixture.alice.ID )
	secret , _ := client.enrol( t )
	challenge := fixture.challenge( t , fixture.alice.ID )
	status , _ := fixture.finish( t , challenge , codeFor( t , secret , 1 ) )
	if status != http.StatusOK {
		t.Fatalf( "status %d" , status )
	}
	sessions , err := models.ListSessionsForUser( fixture.store , fixture.alice.ID )
	if err != nil { t.Fatal( err ) }
	recorded := 0
	for _ , session := range sessions {
		if session.LastSeenAt != nil { recorded += 1 }
	}
	if recorded != len( sessions ) || recorded == 0 {
		t.Fatalf( "%d of %d sessions were recorded" , recorded , len( sessions ) )
	}
}

func TestRevokeOneDevice( t *testing.T ) {
	fixture := newAuthFixture( t )
	here := fixture.signIn( t , fixture.alice.ID )
	there := fixture.signIn( t , fixture.alice.ID )
	id := other( t , here.devices( t ) )

	if status , _ := here.post( t , "/api/account/sessions/"+id+"/revoke" , fiber.Map{} ); status != http.StatusForbidden {
		t.Fatalf( "no CSRF token: status %d" , status )
	}
	status , _ := here.post( t , "/api/account/sessions/"+id+"/revoke" , fiber.Map{ "csrf_token": here.csrf( t ) } )
	if status != http.StatusOK {
		t.Fatalf( "status %d" , status )
	}
	if there.signedIn( t ) {
		t.Fatal( "the revoked device is still signed in" )
	}
	if here.signedIn( t ) == false {
		t.Fatal( "revoking the other device ended this one" )
	}
}

func TestSomeoneElsesSessionLooksLikeNoSession( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	owner := fixture.signIn( t , fixture.owner.ID )
	alice_second := fixture.signIn( t , fixture.alice.ID )
	id := other( t , alice.devices( t ) )

	csrf := owner.csrf( t )
	theirs , foreign := owner.post( t , "/api/account/sessions/"+id+"/revoke" , fiber.Map{ "csrf_token": csrf } )
	unknown_status , unknown := owner.post( t , "/api/account/sessions/nothing-here/revoke" , fiber.Map{ "csrf_token": csrf } )
	if theirs != http.StatusNotFound || unknown_status != http.StatusNotFound || foreign[ "error" ] != unknown[ "error" ] {
		t.Fatalf( "%d %v vs %d %v" , theirs , foreign , unknown_status , unknown )
	}
	if alice_second.signedIn( t ) == false {
		t.Fatal( "another account's session was ended" )
	}
}

func TestThisDeviceIsNotRevokedHere( t *testing.T ) {
	fixture := newAuthFixture( t )
	client := fixture.signIn( t , fixture.alice.ID )
	id , _ := client.devices( t )[ 0 ][ "id" ].( string )
	status , reply := client.post( t , "/api/account/sessions/"+id+"/revoke" , fiber.Map{ "csrf_token": client.csrf( t ) } )
	if status != http.StatusBadRequest || reply[ "error" ] != "use sign out for this device" {
		t.Fatalf( "%d %v" , status , reply )
	}
	if client.signedIn( t ) == false {
		t.Fatal( "this session was ended" )
	}
}

func TestRevokeOthersKeepsThisOne( t *testing.T ) {
	fixture := newAuthFixture( t )
	here := fixture.signIn( t , fixture.alice.ID )
	there := fixture.signIn( t , fixture.alice.ID )
	third := fixture.signIn( t , fixture.alice.ID )
	owner := fixture.signIn( t , fixture.owner.ID )

	status , reply := here.post( t , "/api/account/sessions/revoke-others" , fiber.Map{ "csrf_token": here.csrf( t ) } )
	if status != http.StatusOK || reply[ "removed" ] != float64( 2 ) {
		t.Fatalf( "%d %v" , status , reply )
	}
	if there.signedIn( t ) || third.signedIn( t ) {
		t.Fatal( "another device is still signed in" )
	}
	if here.signedIn( t ) == false || owner.signedIn( t ) == false {
		t.Fatal( "the wrong session ended" )
	}
}

func TestAnAPIKeyCannotManageDevices( t *testing.T ) {
	fixture := newAuthFixture( t )
	key , _ , err := models.IssueAPIKey( fixture.store , fixture.alice.ID , "script" , models.RoleUser , time.Hour )
	if err != nil { t.Fatal( err ) }
	request , _ := http.NewRequest( http.MethodGet , "/api/account/sessions" , nil )
	request.Header.Set( "Authorization" , "Bearer "+key )
	response , err := fixture.app.Test( request , fiber.TestConfig{ Timeout: 10 * time.Second } )
	if err != nil { t.Fatal( err ) }
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf( "status %d" , response.StatusCode )
	}
}

func TestTouchIsThrottledAndCannotBringASessionBack( t *testing.T ) {
	fixture := newAuthFixture( t )
	client := fixture.signIn( t , fixture.alice.ID )
	sessions , _ := models.ListSessionsForUser( fixture.store , fixture.alice.ID )
	session := sessions[ 0 ]
	stamp := *session.LastSeenAt

	models.TouchSession( fixture.store , session )
	if session.LastSeenAt.Equal( stamp ) == false {
		t.Fatal( "touched inside the interval" )
	}
	stale := stamp.Add( -time.Hour )
	session.LastSeenAt = &stale
	models.TouchSession( fixture.store , session )
	if session.LastSeenAt.Before( stamp ) {
		t.Fatal( "not touched after the interval" )
	}

	if err := models.RevokeSession( fixture.store , fixture.alice.ID , session.ID ); err != nil { t.Fatal( err ) }
	session.LastSeenAt = &stale
	models.TouchSession( fixture.store , session )
	if client.signedIn( t ) {
		t.Fatal( "a touch brought back a revoked session" )
	}
}

func TestSessionFromBeforeTheseFieldsStillLoads( t *testing.T ) {
	fixture := newAuthFixture( t )
	client := fixture.signIn( t , fixture.alice.ID )
	sessions , _ := models.ListSessionsForUser( fixture.store , fixture.alice.ID )
	old := *sessions[ 0 ]
	old.IP , old.UserAgent , old.LastSeenAt = "" , "" , nil
	if err := fixture.store.Put( db.BucketSessions , []byte( old.ID ) , &old ); err != nil { t.Fatal( err ) }
	rows := client.devices( t )
	if len( rows ) != 1 || rows[ 0 ][ "ip" ] != nil {
		t.Fatalf( "rows %v" , rows )
	}
}

func TestJobsAreTheOwnersAlone( t *testing.T ) {
	fixture := newAuthFixture( t )
	handlers_jobs := jobs.New( fixture.store , nil )
	release := make( chan struct{} )
	handlers_jobs.Register( "slow" , time.Hour , func() ( note string , err error ) {
		<-release
		return
	} )
	fixture.handlers.Jobs = handlers_jobs
	owner := fixture.signIn( t , fixture.owner.ID )
	alice := fixture.signIn( t , fixture.alice.ID )

	if status , _ := alice.do( t , http.MethodGet , "/api/owner/jobs" , nil ); status != http.StatusNotFound {
		t.Fatalf( "a user reached the jobs: %d" , status )
	}
	status , reply := owner.do( t , http.MethodGet , "/api/owner/jobs" , nil )
	listed , _ := reply[ "jobs" ].( []any )
	if status != http.StatusOK || len( listed ) != 1 {
		t.Fatalf( "%d %v" , status , reply )
	}
	if status , _ = owner.post( t , "/api/owner/jobs/slow/run" , fiber.Map{} ); status != http.StatusForbidden {
		t.Fatalf( "no CSRF token: %d" , status )
	}
	csrf := owner.csrf( t )
	if status , _ = owner.post( t , "/api/owner/jobs/nope/run" , fiber.Map{ "csrf_token": csrf } ); status != http.StatusNotFound {
		t.Fatalf( "unknown job: %d" , status )
	}
	if status , _ = owner.post( t , "/api/owner/jobs/slow/run" , fiber.Map{ "csrf_token": csrf } ); status != http.StatusAccepted {
		t.Fatalf( "run: %d" , status )
	}
	status , reply = owner.post( t , "/api/owner/jobs/slow/run" , fiber.Map{ "csrf_token": csrf } )
	close( release )
	if status != http.StatusConflict || reply[ "error" ] != "job already running" {
		t.Fatalf( "%d %v" , status , reply )
	}
	if handlers_jobs.Wait( 5 * time.Second ) == false {
		t.Fatal( "the job did not finish" )
	}
}
