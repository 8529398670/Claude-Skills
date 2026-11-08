// whole-file feature:logs
package routes

import (
	http "net/http"
	testing "testing"

	fiber "github.com/gofiber/fiber/v3"
)

func TestNamingAnAddressIsTheOwnersAlone( t *testing.T ) {
	fixture := newMonitored( t , true )
	owner := fixture.signIn( t , fixture.owner.ID )
	alice := fixture.signIn( t , fixture.alice.ID )

	status , body := alice.do( t , http.MethodPost , "/api/owner/logs/ip-names" , fiber.Map{ "target": "0.0.0.0/0" , "name": "Mine" , "csrf_token": alice.csrf( t ) } )
	if status != http.StatusNotFound { t.Fatalf( "someone else named an address: %d %v" , status , body ) }

	status , body = owner.post( t , "/api/owner/logs/ip-names" , fiber.Map{ "target": "0.0.0.0/0" , "name": "  Everywhere " , "csrf_token": owner.csrf( t ) } )
	if status != http.StatusOK || body[ "name" ] != "Everywhere" {
		t.Fatalf( "%d %v" , status , body )
	}
	if status , body = owner.post( t , "/api/owner/logs/ip-names" , fiber.Map{ "target": "laptop" , "name": "x" , "csrf_token": owner.csrf( t ) } ); status != http.StatusBadRequest {
		t.Fatalf( "a bad target was taken: %d %v" , status , body )
	}
	if status , body = owner.post( t , "/api/owner/logs/ip-names" , fiber.Map{ "target": "10.0.0.1" , "name": " " , "csrf_token": owner.csrf( t ) } ); status != http.StatusBadRequest {
		t.Fatalf( "an empty name was taken: %d %v" , status , body )
	}
	// >>> feature:sessions
	if rows := owner.devices( t ); rows[ 0 ][ "ip_name" ] != "Everywhere" {
		t.Fatalf( "the owner's devices have no name: %v" , rows )
	}
	if rows := alice.devices( t ); rows[ 0 ][ "ip_name" ] != nil {
		t.Fatalf( "a name reached someone else: %v" , rows )
	}
	// <<< feature:sessions
	if status , body = owner.post( t , "/api/owner/logs/ip-names/delete" , fiber.Map{ "target": "0.0.0.0/0" , "csrf_token": owner.csrf( t ) } ); status != http.StatusOK {
		t.Fatalf( "%d %v" , status , body )
	}
	// >>> feature:sessions
	if rows := owner.devices( t ); rows[ 0 ][ "ip_name" ] != nil {
		t.Fatalf( "a deleted name stayed: %v" , rows )
	}
	// <<< feature:sessions
}
