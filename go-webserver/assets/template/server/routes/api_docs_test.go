package routes

import (
	bytes "bytes"
	json "encoding/json" // feature:mcp
	flag "flag"
	http "net/http"
	os "os"
	filepath "path/filepath"
	strings "strings"
	sync "sync"
	testing "testing"
	time "time" // feature:files

	fiber "github.com/gofiber/fiber/v3"

	config "{{MODULE_PATH}}/server/config" // feature:files
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API reference is written from the code: config/openapi.json and
// config/api.md from the docs each handler declares with apidoc.Doc (the
// *_api.go files), and config/mcp.json from the registered MCP tools.
// scripts/api_docs.sh runs TestAPIDocsAreCurrent with -update to rewrite
// them; without it, the test fails while a committed file is out of date.
var updateAPIDocs = flag.Bool( "update" , false , "rewrite config/openapi.json, config/mcp.json and config/api.md from the code" )

// notDocumented is what the route table has that is not an API: the pages a
// link opens, and the catch-alls.
var notDocumented = map[string]bool{
	"GET /login/*": true,
	"GET /join/*":  true,
	"GET /api/*":   true,
	"GET /*":       true,
}

// newParityFixture is the app with every switch on, so that no feature's
// routes or tools hide behind a 404 and every route is there to be called.
func newParityFixture( t *testing.T ) ( fixture *monitored ) {
	t.Helper()
	fixture = newMonitored( t , true )
	fixture.cfg.MCPEnabled = true // feature:mcp
	// >>> feature:files
	directory := t.TempDir()
	fixture.cfg.Files = config.FilesConfig{
		Enabled: true , MaxFileBytes: 1 << 20 , QuotaBytesPerUser: 1 << 20 , AbandonedAfter: time.Hour,
		Dir: filepath.Join( directory , "files" ) , UploadsDir: filepath.Join( directory , "uploads" ),
	}
	// <<< feature:files
	return
}

// apiSpec is openapi.json as the code describes it, built from the route
// table of the app with everything on. A route without docs, or docs without
// a route, fails the test that asked.
func apiSpec( t *testing.T ) ( spec map[string]any , fixture *monitored ) {
	t.Helper()
	fixture = newParityFixture( t )
	routes := documentedRoutes( fixture.app )
	built , problems := apidoc.Build( routes )
	if len( problems ) > 0 { t.Fatal( strings.Join( problems , "\n" ) ) }
	spec , err := apidoc.Plain( built )
	if err != nil { t.Fatal( err ) }
	return
}

// documentedRoutes is the app's route table, less what is not an API.
func documentedRoutes( app *fiber.App ) ( routes []apidoc.Route ) {
	for _ , route := range app.GetRoutes( true ) {
		if route.Method != http.MethodGet && route.Method != http.MethodPost && route.Method != http.MethodDelete { continue }
		if route.Path == "/fail" || route.Path == "/panic" { continue } // newMonitored's own
		if notDocumented[ route.Method + " " + route.Path ] { continue }
		routes = append( routes , apidoc.Route{ Method: route.Method , Path: route.Path , Handler: route.Handlers[ len( route.Handlers ) - 1 ] } )
	}
	return
}

// checkReplies holds every success reply a test gets against the API docs,
// so docs that say something a handler does not do fail a test. Every test
// app has it in front of its routes.
func checkReplies( t *testing.T ) ( handler fiber.Handler ) {
	var once sync.Once
	var spec map[string]any
	handler = func( c fiber.Ctx ) ( err error ) {
		err = c.Next()
		status := c.Response().StatusCode()
		if err != nil || status < 200 || status > 299 { return }
		if strings.HasPrefix( string( c.Response().Header.ContentType() ) , fiber.MIMEApplicationJSON ) == false { return }
		once.Do( func() {
			built , _ := apidoc.Build( documentedRoutes( c.App() ) )
			spec , _ = apidoc.Plain( built )
		} )
		route := c.Route()
		path := apidoc.SpecPath( route.Path )
		for _ , problem := range apidoc.Check( spec , route.Method , path , status , c.Response().Body() ) {
			t.Errorf( "%s %s: %s" , route.Method , path , problem )
		}
		return
	}
	return
}

// Every route has docs, and every doc has a route.
func TestEveryRouteIsDocumented( t *testing.T ) {
	apiSpec( t )
}

func TestAPIDocsAreCurrent( t *testing.T ) {
	spec , fixture := apiSpec( t ) // feature:mcp
	spec , _ := apiSpec( t ) // feature:!mcp
	files := map[string][]byte{}
	var err error
	// >>> feature:openapi
	if files[ "openapi.json" ] , err = apidoc.Encode( spec ); err != nil { t.Fatal( err ) }
	// <<< feature:openapi
	var manifest map[string]any
	// >>> feature:mcp
	if files[ "mcp.json" ] , err = mcpManifest( fixture.handlers.MCP.Tools() , specOperations( t ) ); err != nil { t.Fatal( err ) }
	if err = json.Unmarshal( files[ "mcp.json" ] , &manifest ); err != nil { t.Fatal( err ) }
	// <<< feature:mcp
	files[ "api.md" ] = apidoc.Markdown( spec , manifest )
	for name , want := range files {
		path := filepath.Join( "../../config" , name )
		if *updateAPIDocs {
			if err = os.WriteFile( path , want , 0o644 ); err != nil { t.Fatal( err ) }
			continue
		}
		have , read_err := os.ReadFile( path )
		if read_err != nil || bytes.Equal( have , want ) == false {
			t.Errorf( "config/%s is out of date: run scripts/api_docs.sh" , name )
		}
	}
}
