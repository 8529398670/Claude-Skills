package routes

import (
	encoding_json "encoding/json"
	io "io"
	http "net/http"
	strings "strings"
	testing "testing"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
)

// mintFor is POST /api/admin/keys from a browser.
func mintFor( t *testing.T , client *browser , user_id uint64 , role string ) ( status int , reply map[ string ]any ) {
	t.Helper()
	status , reply = send( t , client , "/api/admin/keys" , fiber.Map{ "user_id": user_id , "name": "deploy" , "role": role } )
	return
}

func TestWhoMayMintAKeyForWhom( t *testing.T ) {
	fixture := newAuthFixture( t )
	second , _ := models.CreateUser( fixture.store , "Second admin" , models.RoleAdmin )
	third , _ := models.CreateUser( fixture.store , "Third admin" , models.RoleAdmin )
	owner := fixture.signIn( t , fixture.owner.ID )
	admin := fixture.signIn( t , second.ID )
	alice := fixture.signIn( t , fixture.alice.ID )

	cases := []struct {
		name   string
		client *browser
		target uint64
		role   string
		status int
	}{
		{ "an admin for a user" , admin , fixture.alice.ID , "user" , fiber.StatusCreated },
		{ "the owner for a user" , owner , fixture.alice.ID , "user" , fiber.StatusCreated },
		{ "the owner for an admin" , owner , second.ID , "admin" , fiber.StatusCreated },
		{ "an admin for themselves" , admin , second.ID , "admin" , fiber.StatusCreated },
		{ "an admin for another admin" , admin , third.ID , "user" , fiber.StatusForbidden },
		{ "an admin for the owner" , admin , fixture.owner.ID , "user" , fiber.StatusForbidden },
		{ "an admin role for a user" , owner , fixture.alice.ID , "admin" , fiber.StatusForbidden },
		{ "a user for anyone" , alice , second.ID , "user" , fiber.StatusForbidden },
		{ "nobody there" , admin , 99999 , "user" , fiber.StatusNotFound },
	}
	for _ , item := range cases {
		status , reply := mintFor( t , item.client , item.target , item.role )
		if status != item.status {
			t.Fatalf( "%s: status %d, wanted %d: %v" , item.name , status , item.status , reply )
		}
	}
}

func TestAKeyMadeForSomeoneActsAsThemAndSaysWhoMadeIt( t *testing.T ) {
	fixture := newAuthFixture( t )
	owner := fixture.signIn( t , fixture.owner.ID )
	status , reply := mintFor( t , owner , fixture.alice.ID , "user" )
	if status != fiber.StatusCreated { t.Fatalf( "%d %v" , status , reply ) }
	if reply[ "created_by" ] != float64( fixture.owner.ID ) { t.Fatalf( "%v" , reply ) }

	request , _ := http.NewRequest( http.MethodGet , "/api/me" , nil )
	request.Header.Set( "Authorization" , "Bearer " + reply[ "credential" ].( string ) )
	response , err := fixture.app.Test( request , fiber.TestConfig{ Timeout: 10 * time.Second } )
	if err != nil { t.Fatal( err ) }
	raw , _ := io.ReadAll( response.Body )
	me := map[ string ]any{}
	encoding_json.Unmarshal( raw , &me )
	if me[ "id" ] != float64( fixture.alice.ID ) { t.Fatalf( "%s" , raw ) }

	alice := fixture.signIn( t , fixture.alice.ID )
	status , page := alice.do( t , http.MethodGet , "/api/keys" , nil )
	if status != fiber.StatusOK { t.Fatalf( "%d %v" , status , page ) }
	rows := page[ "keys" ].( []any )
	if len( rows ) != 1 { t.Fatalf( "%v" , rows ) }
	row := rows[ 0 ].( map[ string ]any )
	if row[ "created_by_name" ] != "Owner" { t.Fatalf( "%v" , row ) }
	if strings.Contains( string( raw ) , "secret_hash" ) { t.Fatalf( "%s" , raw ) }
}

func TestAKeyCannotMintAKeyForAnyone( t *testing.T ) {
	fixture := newAuthFixture( t )
	key , _ , err := models.IssueAPIKey( fixture.store , fixture.owner.ID , "script" , models.RoleAdmin , time.Hour )
	if err != nil { t.Fatal( err ) }
	request , _ := http.NewRequest( http.MethodPost , "/api/admin/keys" , strings.NewReader( `{"user_id":2,"name":"x"}` ) )
	request.Header.Set( "Content-Type" , "application/json" )
	request.Header.Set( "Authorization" , "Bearer " + key )
	response , err := fixture.app.Test( request , fiber.TestConfig{ Timeout: 10 * time.Second } )
	if err != nil { t.Fatal( err ) }
	if response.StatusCode != fiber.StatusForbidden {
		t.Fatalf( "status %d" , response.StatusCode )
	}
	keys , _ := models.ListAPIKeysForUser( fixture.store , fixture.alice.ID )
	if len( keys ) != 0 { t.Fatalf( "%v" , keys ) }
}

func TestAShellStillMintsForTheOwner( t *testing.T ) {
	fixture := newAuthFixture( t )
	_ , key , err := models.IssueAPIKey( fixture.store , fixture.owner.ID , "script" , models.RoleAdmin , time.Hour )
	if err != nil { t.Fatal( err ) }
	if key.CreatedBy != 0 { t.Fatalf( "%d" , key.CreatedBy ) }
}
