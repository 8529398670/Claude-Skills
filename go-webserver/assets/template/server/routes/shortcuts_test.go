// whole-file feature:shortcuts
package routes

import (
	http "net/http"
	testing "testing"

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
)

func myShortcutsFrom( t *testing.T , client *browser ) ( shortcuts map[string]any ) {
	t.Helper()
	_ , reply := client.do( t , http.MethodGet , "/api/me" , nil )
	shortcuts , ok := reply[ "shortcuts" ].( map[string]any )
	if ok == false { t.Fatalf( "/api/me has no shortcuts object: %v" , reply[ "shortcuts" ] ) }
	return
}

func TestShortcutsAreKeptPerAccount( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	owner := fixture.signIn( t , fixture.owner.ID )

	if got := myShortcutsFrom( t , alice ); len( got ) != 0 { t.Fatalf( "a new account starts with %v" , got ) }

	status , reply := send( t , alice , "/api/account/shortcuts" , fiber.Map{
		"shortcuts": map[string]string{ "search": "Mod+K" , "redo": "Mod+Shift+Z" , "help": "" },
	} )
	if status != fiber.StatusOK { t.Fatalf( "%d %v" , status , reply ) }
	got := myShortcutsFrom( t , alice )
	if got[ "search" ] != "Mod+K" || got[ "redo" ] != "Mod+Shift+Z" || got[ "help" ] != "" || len( got ) != 3 {
		t.Fatalf( "read back %v" , got )
	}
	if other := myShortcutsFrom( t , owner ); len( other ) != 0 { t.Fatalf( "the owner got alice's: %v" , other ) }

	// {} puts everything back.
	if status , reply = send( t , alice , "/api/account/shortcuts" , fiber.Map{ "shortcuts": map[string]string{} } ); status != fiber.StatusOK {
		t.Fatalf( "%d %v" , status , reply )
	}
	if got = myShortcutsFrom( t , alice ); len( got ) != 0 { t.Fatalf( "reset left %v" , got ) }
}

func TestShortcutsNeedTheCSRFToken( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	status , _ := alice.post( t , "/api/account/shortcuts" , fiber.Map{ "shortcuts": map[string]string{ "search": "K" } } )
	if status != fiber.StatusForbidden { t.Fatalf( "%d" , status ) }
	if got := myShortcutsFrom( t , alice ); len( got ) != 0 { t.Fatalf( "saved anyway: %v" , got ) }
}

func TestShortcutsRefuseWhatNoKeyboardSends( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	for _ , shortcuts := range []map[string]string{
		{ "Search": "K" },               // names are lower case
		{ "search": "Shift+Mod+K" },     // modifiers in order
		{ "search": "Mod+Mod+K" },
		{ "search": "Mod+" },            // no key
		{ "search": "Hyper+K" },
		{ "search": "Mod+ab" },
		{ "search": " " },
	} {
		if status , reply := send( t , alice , "/api/account/shortcuts" , fiber.Map{ "shortcuts": shortcuts } ); status != fiber.StatusBadRequest {
			t.Fatalf( "%v: %d %v" , shortcuts , status , reply )
		}
	}
	if got := myShortcutsFrom( t , alice ); len( got ) != 0 { t.Fatalf( "saved anyway: %v" , got ) }
}

func TestShortcutBindings( t *testing.T ) {
	for binding , want := range map[string]bool{
		"/": true , "?": true , "Mod+Z": true , "Mod+Shift+Z": true , "Mod+Ctrl+Alt+Shift+F5": true ,
		"Enter": true , "Escape": true , "Space": true , "Shift++": true , "+": true , "": true , "é": true ,
		"Alt+Mod+K": false , "Shift+": false , "enter": false , "\t": false ,
	} {
		if got := models.ValidShortcutBinding( binding ); got != want {
			t.Errorf( "%q: %v, want %v" , binding , got , want )
		}
	}
}
