package static

import (
	io "io"
	http "net/http"
	testing "testing"
	fstest "testing/fstest"

	fiber "github.com/gofiber/fiber/v3"
)

// S2: the static handler never serves a dotfile, whatever directory it was
// pointed at. .well-known/ is public by definition and still served.
func TestDotfilesAreNeverServed( t *testing.T ) {
	server , _ := New( fstest.MapFS{
		"index.html":                 { Data: []byte( "index" ) },
		".env":                       { Data: []byte( "SECRET=1" ) },
		".git/config":                { Data: []byte( "[core]" ) },
		"css/.secret.css":            { Data: []byte( "x" ) },
		".well-known/security.txt":   { Data: []byte( "Contact: x" ) },
	} )
	app := fiber.New()
	app.Get( "/*" , server.Handler )
	get := func( path string ) ( status int , body string ) {
		response , err := app.Test( httptestRequest( path ) )
		if err != nil { t.Fatal( err ) }
		raw , _ := io.ReadAll( response.Body )
		status , body = response.StatusCode , string( raw )
		return
	}
	for _ , path := range []string{ "/.env" , "/.git/config" , "/css/.secret.css" , "/%2Eenv" , "/css/../.env" } {
		if status , body := get( path ); status != http.StatusNotFound {
			t.Errorf( "%s: %d %q" , path , status , body )
		}
	}
	if status , body := get( "/.well-known/security.txt" ); status != http.StatusOK || body != "Contact: x" {
		t.Errorf( ".well-known: %d %q" , status , body )
	}
	if status , _ := get( "/" ); status != http.StatusOK { t.Errorf( "index: %d" , status ) }
}

func httptestRequest( path string ) ( request *http.Request ) {
	request , _ = http.NewRequest( http.MethodGet , path , nil )
	return
}
