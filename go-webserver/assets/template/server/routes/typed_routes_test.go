package routes

import (
	http "net/http"
	strings "strings"
	testing "testing"

	fiber "github.com/gofiber/fiber/v3"
)

// A declared route checks its request against its type before the handler
// runs (see server/apidoc/route.go). Each fault answers the same 400 whichever
// route it is, and python-webserver answers the same words.

const linkLimits = "max_uses must be 0 (unlimited) or 1-1000, expires_in_minutes 0 (never) or 1-525600"

func TestADeclaredRouteChecksItsBody( t *testing.T ) {
	fixture := newAuthFixture( t )
	owner := fixture.signIn( t , fixture.owner.ID )
	cases := []struct {
		name    string
		body    fiber.Map
		message string
	}{
		{ "a missing field" , fiber.Map{ "role": "user" } , "display name must be 1-80 characters" },
		{ "another missing field" , fiber.Map{ "display_name": "Bea" } , "role must be admin or user" },
		{ "a wrong type" , fiber.Map{ "display_name": 5 , "role": "user" } , "malformed request body" },
		{ "a value not in the enum" , fiber.Map{ "display_name": "Bea" , "role": "boss" } , "role must be admin or user" },
		{ "too long" , fiber.Map{ "display_name": strings.Repeat( "x" , 81 ) , "role": "user" } , "display name must be 1-80 characters" },
		{ "out of range" , fiber.Map{ "display_name": "Bea" , "role": "user" , "max_uses": 5000 } , linkLimits },
		{ "below range" , fiber.Map{ "display_name": "Bea" , "role": "user" , "expires_in_minutes": -1 } , linkLimits },
		{ "an unknown field" , fiber.Map{ "display_name": "Bea" , "role": "user" , "colour": "red" } , "unknown field: colour" },
	}
	for _ , each := range cases {
		status , reply := send( t , owner , "/api/admin/users" , each.body )
		if status != fiber.StatusBadRequest || reply[ "error" ] != each.message {
			t.Errorf( "%s: %d %v, want 400 %q" , each.name , status , reply , each.message )
		}
	}
	if status , _ := send( t , owner , "/api/admin/users" , fiber.Map{ "display_name": "Bea" , "role": "user" , "max_uses": nil } ); status != fiber.StatusCreated {
		t.Errorf( "a null optional field: %d" , status )
	}
}

func TestADeclaredRouteChecksItsURL( t *testing.T ) {
	fixture := newAuthFixture( t )
	owner := fixture.signIn( t , fixture.owner.ID )
	cases := []struct {
		path    string
		message string
	}{
		{ "/api/admin/users?limit=0" , "limit must be 1-1000" },
		{ "/api/admin/users?limit=lots" , "limit must be 1-1000" },
		{ "/api/admin/users?offset=-1" , "offset must be 0 or more" },
		{ "/api/admin/users?dir=up" , "dir must be asc or desc" },
		{ "/api/admin/users?sort=shoe_size" , "sort must be one of: display_name, role, disabled, created_at" },
	}
	for _ , each := range cases {
		status , reply := owner.do( t , http.MethodGet , each.path , nil )
		if status != fiber.StatusBadRequest || reply[ "error" ] != each.message {
			t.Errorf( "%s: %d %v, want 400 %q" , each.path , status , reply , each.message )
		}
	}
	status , reply := send( t , owner , "/api/admin/users/abc/rename" , fiber.Map{ "display_name": "Bea" } )
	if status != fiber.StatusBadRequest || reply[ "error" ] != "invalid user id" {
		t.Errorf( "a path id that is not a number: %d %v" , status , reply )
	}
}

func TestADeclaredRouteChecksTheSessionsCSRFToken( t *testing.T ) {
	fixture := newAuthFixture( t )
	owner := fixture.signIn( t , fixture.owner.ID )
	status , reply := owner.post( t , "/api/admin/users" , fiber.Map{ "display_name": "Bea" , "role": "user" } )
	if status != fiber.StatusForbidden || reply[ "error" ] != "invalid csrf token" {
		t.Errorf( "no csrf token: %d %v" , status , reply )
	}
	// The token is checked before the body, as before.
	status , reply = owner.post( t , "/api/admin/users" , fiber.Map{ "colour": "red" } )
	if status != fiber.StatusForbidden {
		t.Errorf( "no csrf token and a bad body: %d %v" , status , reply )
	}
}

// >>> feature:appearance
// A value inside a list of named types is checked too: a theme's mode or button
// choice off its enum is the field's 400, like any other limit.
func TestADeclaredRouteChecksInsideAList( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	for _ , theme := range []fiber.Map{
		{ "id": "a" , "name": "A" , "mode": "dim" , "accent": "#1f5fd6" , "second": "#0b7a75" },
		{ "id": "a" , "name": "A" , "mode": "light" , "accent": "#1f5fd6" , "second": "#0b7a75" , "button": fiber.Map{ "shape": "Pill" } },
		{ "id": "a" , "name": "A" , "mode": "light" , "second": "#0b7a75" },
	} {
		status , reply := send( t , alice , "/api/account/themes" , fiber.Map{ "themes": []fiber.Map{ theme } } )
		if status != fiber.StatusBadRequest || reply[ "error" ] != "themes is invalid" {
			t.Errorf( "%v: %d %v" , theme , status , reply )
		}
	}
}
// <<< feature:appearance

// >>> feature:mcp
func TestAToolChecksItsArgumentsLikeItsRoute( t *testing.T ) {
	fixture := newMCPFixture( t )
	_ , reply := fixture.post( t , fixture.admin_key , callTool( "admin_create_user" , `{"display_name":"Bea","role":"user","max_uses":5000}` ) )
	if failed , text := toolFailed( t , reply ); failed == false || strings.Contains( text , linkLimits ) == false {
		t.Errorf( "out of range: %v" , reply )
	}
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_create_user" , `{"role":"user"}` ) )
	if failed , text := toolFailed( t , reply ); failed == false || strings.Contains( text , "display name must be 1-80 characters" ) == false {
		t.Errorf( "a missing argument: %v" , reply )
	}
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_create_user" , `{"display_name":5,"role":"user"}` ) )
	if reply[ "error" ] == nil {
		t.Errorf( "a wrong type: %v" , reply )
	}
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_list_users" , `{"sort":"shoe_size"}` ) )
	if failed , text := toolFailed( t , reply ); failed == false || strings.Contains( text , "sort must be one of" ) == false {
		t.Errorf( "a sort the route refuses: %v" , reply )
	}
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_list_users" , `{"sort":"display_name","dir":"desc","q":"mem"}` ) )
	if failed , text := toolFailed( t , reply ); failed {
		t.Errorf( "the route's own query, through MCP: %s" , text )
	}
}
// <<< feature:mcp
