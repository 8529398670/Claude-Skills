// whole-file feature:appearance
package routes

import (
	http "net/http"
	testing "testing"

	fiber "github.com/gofiber/fiber/v3"
)

func themeList( names ...string ) ( result []fiber.Map ) {
	for _ , name := range names {
		result = append( result , fiber.Map{ "id": name , "name": name , "mode": "light" , "accent": "#1F5FD6" , "second": "#0b7a75" } )
	}
	return
}

func meThemes( t *testing.T , client *browser ) ( themes []any , custom bool , theme_id string ) {
	t.Helper()
	_ , reply := client.do( t , http.MethodGet , "/api/me" , nil )
	themes , _ = reply[ "themes" ].( []any )
	custom , _ = reply[ "themes_custom" ].( bool )
	theme_id , _ = reply[ "theme_id" ].( string )
	return
}

func themeNames( themes []any ) ( names []string ) {
	for _ , theme := range themes {
		names = append( names , theme.( map[string]any )[ "name" ].( string ) )
	}
	return
}

func TestThemesFollowTheDefaultsUntilChanged( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	owner := fixture.signIn( t , fixture.owner.ID )

	// Nothing saved anywhere: the browser's built-in themes.
	if themes , custom , _ := meThemes( t , alice ); themes != nil || custom {
		t.Fatalf( "a new account starts with %v %v" , themes , custom )
	}

	// An admin's defaults reach everyone without their own list.
	if status , reply := send( t , owner , "/api/admin/theme-defaults" , fiber.Map{ "themes": themeList( "one" , "two" ) } ); status != fiber.StatusOK {
		t.Fatalf( "%d %v" , status , reply )
	}
	if themes , custom , _ := meThemes( t , alice ); len( themes ) != 2 || custom {
		t.Fatalf( "alice sees %v %v" , themes , custom )
	}

	// Her own list is hers; colors come back in lower case.
	status , reply := send( t , alice , "/api/account/themes" , fiber.Map{ "themes": themeList( "mine" ) , "theme_id": "mine" } )
	if status != fiber.StatusOK { t.Fatalf( "%d %v" , status , reply ) }
	themes , custom , theme_id := meThemes( t , alice )
	if len( themes ) != 1 || custom == false || theme_id != "mine" {
		t.Fatalf( "read back %v %v %q" , themes , custom , theme_id )
	}
	if accent := themes[ 0 ].( map[string]any )[ "accent" ]; accent != "#1f5fd6" { t.Fatalf( "accent %v" , accent ) }

	// Later defaults no longer reach her.
	send( t , owner , "/api/admin/theme-defaults" , fiber.Map{ "themes": themeList( "three" ) } )
	if themes , _ , _ := meThemes( t , alice ); themeNames( themes )[ 0 ] != "mine" { t.Fatalf( "defaults replaced hers: %v" , themes ) }

	// Picking keeps the list.
	if status , reply := send( t , alice , "/api/account/theme" , fiber.Map{ "theme_id": "other" } ); status != fiber.StatusOK {
		t.Fatalf( "%d %v" , status , reply )
	}
	if themes , _ , theme_id := meThemes( t , alice ); len( themes ) != 1 || theme_id != "other" { t.Fatalf( "%v %q" , themes , theme_id ) }

	// null goes back to the defaults.
	send( t , alice , "/api/account/themes" , fiber.Map{ "themes": nil } )
	if themes , custom , _ := meThemes( t , alice ); custom || themeNames( themes )[ 0 ] != "three" { t.Fatalf( "%v %v" , themes , custom ) }

	// And null defaults go back to the built-in ones.
	send( t , owner , "/api/admin/theme-defaults" , fiber.Map{ "themes": nil } )
	if themes , _ , _ := meThemes( t , alice ); themes != nil { t.Fatalf( "%v" , themes ) }
}

// A saved list keeps which built-in themes it has seen, so theme.js can add
// the newer ones; going back to the defaults (or the built-ins) drops it.
func TestThemeListsKeepTheBuiltInsTheyHaveSeen( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	owner := fixture.signIn( t , fixture.owner.ID )
	seen := func( client *browser , path string , key string ) ( result float64 ) {
		t.Helper()
		_ , reply := client.do( t , http.MethodGet , path , nil )
		result , _ = reply[ key ].( float64 )
		return
	}

	// A list saved before builtins existed, or without it: 0.
	send( t , alice , "/api/account/themes" , fiber.Map{ "themes": themeList( "mine" ) } )
	if got := seen( alice , "/api/me" , "themes_builtins" ); got != 0 { t.Fatalf( "%v" , got ) }
	if status , reply := send( t , alice , "/api/account/themes" , fiber.Map{ "themes": themeList( "mine" ) , "builtins": 3 } ); status != fiber.StatusOK || reply[ "themes_builtins" ] != float64( 3 ) {
		t.Fatalf( "%d %v" , status , reply )
	}
	if got := seen( alice , "/api/me" , "themes_builtins" ); got != 3 { t.Fatalf( "%v" , got ) }

	// The admin's defaults keep their own, which those following them see.
	if status , reply := send( t , owner , "/api/admin/theme-defaults" , fiber.Map{ "themes": themeList( "one" ) , "builtins": 2 } ); status != fiber.StatusOK || reply[ "builtins" ] != float64( 2 ) {
		t.Fatalf( "%d %v" , status , reply )
	}
	if got := seen( owner , "/api/admin/theme-defaults" , "builtins" ); got != 2 { t.Fatalf( "%v" , got ) }
	send( t , alice , "/api/account/themes" , fiber.Map{ "themes": nil } )
	if got := seen( alice , "/api/me" , "themes_builtins" ); got != 2 { t.Fatalf( "following the defaults: %v" , got ) }

	// Back to the built-in themes: nothing seen.
	send( t , owner , "/api/admin/theme-defaults" , fiber.Map{ "themes": nil } )
	if got := seen( owner , "/api/admin/theme-defaults" , "builtins" ); got != 0 { t.Fatalf( "%v" , got ) }

	// Out of range is refused.
	for _ , bad := range []int{ -1 , 1001 } {
		if status , _ := send( t , alice , "/api/account/themes" , fiber.Map{ "themes": themeList( "mine" ) , "builtins": bad } ); status != fiber.StatusBadRequest {
			t.Fatalf( "builtins %d: %d" , bad , status )
		}
	}
}

func TestThemeDefaultsAreForAdmins( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	if status , _ := alice.do( t , http.MethodGet , "/api/admin/theme-defaults" , nil ); status == fiber.StatusOK {
		t.Fatalf( "a user read the defaults" )
	}
	if status , _ := send( t , alice , "/api/admin/theme-defaults" , fiber.Map{ "themes": themeList( "x" ) } ); status == fiber.StatusOK {
		t.Fatalf( "a user set the defaults" )
	}
}

func TestThemesNeedTheCSRFToken( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	if status , _ := alice.post( t , "/api/account/themes" , fiber.Map{ "themes": themeList( "x" ) } ); status != fiber.StatusForbidden {
		t.Fatalf( "%d" , status )
	}
	if _ , custom , _ := meThemes( t , alice ); custom { t.Fatalf( "saved anyway" ) }
}

func TestThemesRefuseBadShapes( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	good := func() fiber.Map { return themeList( "a" )[ 0 ] }
	cases := []func( theme fiber.Map ) []fiber.Map{
		func( theme fiber.Map ) []fiber.Map { return []fiber.Map{} },
		func( theme fiber.Map ) []fiber.Map { theme[ "id" ] = "Has Space" ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { return []fiber.Map{ theme , theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "name" ] = "  " ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "mode" ] = "dim" ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "accent" ] = "red" ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "surface" ] = "#fff" ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "overrides" ] = fiber.Map{ "glow": "#000000" } ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "overrides" ] = fiber.Map{ "link": "blue" } ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "button" ] = fiber.Map{ "shape": "blob" } ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "button" ] = fiber.Map{ "depth": "deep" } ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "dark" ] = fiber.Map{ "accent": "gold" , "second": "#ffffff" } ; return []fiber.Map{ theme } },
		func( theme fiber.Map ) []fiber.Map { theme[ "dark" ] = fiber.Map{ "accent": "#000000" , "second": "#ffffff" , "overrides": fiber.Map{ "glow": "#000000" } } ; return []fiber.Map{ theme } },
	}
	for index , build := range cases {
		themes := build( good() )
		if status , reply := send( t , alice , "/api/account/themes" , fiber.Map{ "themes": themes } ); status != fiber.StatusBadRequest {
			t.Fatalf( "case %d: %d %v" , index , status , reply )
		}
	}
	if _ , custom , _ := meThemes( t , alice ); custom { t.Fatalf( "saved anyway" ) }

	// Overrides by name are kept.
	theme := good()
	theme[ "overrides" ] = fiber.Map{ "link": "#ABCDEF" , "infoBorder": "#000000" }
	if status , reply := send( t , alice , "/api/account/themes" , fiber.Map{ "themes": []fiber.Map{ theme } } ); status != fiber.StatusOK {
		t.Fatalf( "%d %v" , status , reply )
	}
	themes , _ , _ := meThemes( t , alice )
	overrides := themes[ 0 ].( map[string]any )[ "overrides" ].( map[string]any )
	if overrides[ "link" ] != "#abcdef" || overrides[ "infoBorder" ] != "#000000" { t.Fatalf( "%v" , overrides ) }

	// A button style, and a button variant's color, are kept; an empty button
	// is the default and is not stored.
	theme = good()
	theme[ "overrides" ] = fiber.Map{ "btnDanger": "#AA0000" , "btnWarningFg": "#000000" }
	theme[ "button" ] = fiber.Map{ "shape": "pill" , "depth": "glossy" }
	plain := themeList( "b" )[ 0 ]
	plain[ "button" ] = fiber.Map{}
	if status , reply := send( t , alice , "/api/account/themes" , fiber.Map{ "themes": []fiber.Map{ theme , plain } } ); status != fiber.StatusOK {
		t.Fatalf( "%d %v" , status , reply )
	}
	themes , _ , _ = meThemes( t , alice )
	first := themes[ 0 ].( map[string]any )
	button := first[ "button" ].( map[string]any )
	if button[ "shape" ] != "pill" || button[ "depth" ] != "glossy" || len( button ) != 2 { t.Fatalf( "%v" , button ) }
	if first[ "overrides" ].( map[string]any )[ "btnDanger" ] != "#aa0000" { t.Fatalf( "%v" , first[ "overrides" ] ) }
	if _ , there := themes[ 1 ].( map[string]any )[ "button" ]; there { t.Fatalf( "empty button stored: %v" , themes[ 1 ] ) }

	// A light theme keeps its dark colors (switching mode swaps them in); colors
	// kept for the mode already in use mean nothing and are dropped.
	theme = good()
	theme[ "dark" ] = fiber.Map{ "accent": "#CBA052" , "second": "#F8E08E" , "background": "#101820" , "overrides": fiber.Map{ "link": "#FFFFFF" } }
	theme[ "light" ] = fiber.Map{ "accent": "#000000" , "second": "#000000" }
	if status , reply := send( t , alice , "/api/account/themes" , fiber.Map{ "themes": []fiber.Map{ theme } } ); status != fiber.StatusOK {
		t.Fatalf( "%d %v" , status , reply )
	}
	themes , _ , _ = meThemes( t , alice )
	first = themes[ 0 ].( map[string]any )
	dark := first[ "dark" ].( map[string]any )
	if dark[ "accent" ] != "#cba052" || dark[ "background" ] != "#101820" || dark[ "overrides" ].( map[string]any )[ "link" ] != "#ffffff" { t.Fatalf( "%v" , dark ) }
	if _ , there := first[ "light" ]; there { t.Fatalf( "light kept on a light theme: %v" , first ) }
}

// An admin (ProjectPortal's shared theme) puts one theme on an account: onto
// the list it sees, replacing the same id, and makes it the pick.
func TestAnAdminGivesAnAccountATheme( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	owner := fixture.signIn( t , fixture.owner.ID )
	path := "/api/admin/users/" + idText( fixture.alice.ID ) + "/theme"
	give := func( name string , accent string ) ( status int , reply map[string]any ) {
		status , reply = send( t , owner , path , fiber.Map{ "theme": fiber.Map{ "id": name , "name": name , "mode": "dark" , "accent": accent , "second": "#0b7a75" } } )
		return
	}
	builtins := func() ( seen float64 ) {
		_ , reply := alice.do( t , http.MethodGet , "/api/me" , nil )
		seen , _ = reply[ "themes_builtins" ].( float64 )
		return
	}

	// Following the built-in themes: the one theme, and every built-in joins it in the browser.
	status , reply := give( "shared" , "#AA0000" )
	if status != fiber.StatusOK || reply[ "ok" ] != true || reply[ "theme_id" ] != "shared" { t.Fatalf( "%d %v" , status , reply ) }
	themes , custom , theme_id := meThemes( t , alice )
	if len( themes ) != 1 || custom == false || theme_id != "shared" || builtins() != -1 { t.Fatalf( "%v %v %q %v" , themes , custom , theme_id , builtins() ) }

	// Her own list: the same id is replaced in place, a new one goes at the end.
	send( t , alice , "/api/account/themes" , fiber.Map{ "themes": themeList( "a" , "shared" , "b" ) , "theme_id": "a" , "builtins": 3 } )
	give( "shared" , "#00aa00" )
	themes , _ , theme_id = meThemes( t , alice )
	if names := themeNames( themes ); len( names ) != 3 || names[ 1 ] != "shared" || theme_id != "shared" || builtins() != 3 { t.Fatalf( "%v %q" , names , theme_id ) }
	if accent := themes[ 1 ].( map[string]any )[ "accent" ]; accent != "#00aa00" { t.Fatalf( "accent %v" , accent ) }
	give( "new" , "#0000aa" )
	if names := themeNames( onlyThemes( meThemes( t , alice ) ) ); len( names ) != 4 || names[ 3 ] != "new" { t.Fatalf( "%v" , names ) }

	// Following the admin's defaults: those, plus the theme, become hers.
	send( t , alice , "/api/account/themes" , fiber.Map{ "themes": nil } )
	send( t , owner , "/api/admin/theme-defaults" , fiber.Map{ "themes": themeList( "d1" , "d2" ) , "builtins": 2 } )
	give( "shared" , "#aa0000" )
	themes , custom , _ = meThemes( t , alice )
	if names := themeNames( themes ); len( names ) != 3 || custom == false || names[ 2 ] != "shared" || builtins() != 2 { t.Fatalf( "%v %v" , names , custom ) }

	// At most 40; replacing one in a full list still works.
	full := []string{}
	for index := 0; index < 40; index++ { full = append( full , "t" + idText( uint64( index ) ) ) }
	send( t , alice , "/api/account/themes" , fiber.Map{ "themes": themeList( full... ) } )
	if status , reply := give( "one-more" , "#aa0000" ); status != fiber.StatusBadRequest { t.Fatalf( "a 41st theme: %d %v" , status , reply ) }
	if status , reply := give( "t5" , "#aa0000" ); status != fiber.StatusOK { t.Fatalf( "replacing in a full list: %d %v" , status , reply ) }

	// A bad theme is refused like on POST /api/account/themes; a missing account is 404.
	if status , reply := give( "Bad Id" , "#aa0000" ); status != fiber.StatusBadRequest { t.Fatalf( "bad id: %d %v" , status , reply ) }
	if status , reply := give( "x" , "red" ); status != fiber.StatusBadRequest { t.Fatalf( "bad color: %d %v" , status , reply ) }
	if status , reply := send( t , owner , "/api/admin/users/99999/theme" , fiber.Map{ "theme": themeList( "x" )[ 0 ] } ); status != fiber.StatusNotFound { t.Fatalf( "missing account: %d %v" , status , reply ) }

	// Not for ordinary users.
	if status , reply := send( t , alice , "/api/admin/users/" + idText( fixture.owner.ID ) + "/theme" , fiber.Map{ "theme": themeList( "x" )[ 0 ] } ); status != fiber.StatusForbidden { t.Fatalf( "a user: %d %v" , status , reply ) }
}

func onlyThemes( themes []any , _ bool , _ string ) ( result []any ) { result = themes; return }
