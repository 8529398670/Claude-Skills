// whole-file feature:mcp
package routes

import (
	bytes "bytes"
	json "encoding/json"
	flag "flag"
	io "io"
	http "net/http"
	os "os"
	filepath "path/filepath" // feature:files
	regexp "regexp"
	sort "sort"
	strings "strings"
	testing "testing"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	config "{{MODULE_PATH}}/server/config" // feature:files
	mcp "{{MODULE_PATH}}/server/mcp"
)

// The rule these hold: everything a person can do in the app, an API key can
// do over HTTP and an agent can do through /mcp -- except managing users, keys
// and sign-in, which need a browser session. config/openapi.json says which
// routes take a key (their security lists apiKey); each of those has an MCP
// tool naming it in Route, or an "x-mcp-http" reason for staying HTTP only.

// updateMCPManifest is how scripts/mcp_json.sh rewrites config/mcp.json:
// go test ./server/routes -run TestMCPManifestIsCurrent -update
var updateMCPManifest = flag.Bool( "update" , false , "rewrite config/mcp.json from the registered tools" )

const mcpManifestPath = "../../config/mcp.json"

type specOperation struct {
	Method   string
	Path     string
	KeyOK    bool
	// Public is a route anyone may call: no security at all.
	Public   bool
	HTTPOnly string
}

func ( operation specOperation ) name() ( result string ) {
	result = operation.Method + " " + operation.Path
	return
}

// specOperations is every operation in config/openapi.json. Without the spec
// (a project scaffolded --without openapi) there is nothing to hold the tools
// to, and the tests that need it skip.
func specOperations( t *testing.T ) ( operations map[string]specOperation ) {
	t.Helper()
	raw , err := os.ReadFile( "../../config/openapi.json" )
	if os.IsNotExist( err ) { t.Skip( "no config/openapi.json" ) }
	if err != nil { t.Fatal( err ) }
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal( raw , &document ); err != nil { t.Fatal( err ) }
	operations = map[string]specOperation{}
	for path , item := range document.Paths {
		for method , raw_operation := range item {
			if strings.HasPrefix( method , "x-" ) { continue }
			var operation struct {
				Security []map[string]any `json:"security"`
				HTTPOnly string           `json:"x-mcp-http"`
			}
			if err = json.Unmarshal( raw_operation , &operation ); err != nil { t.Fatal( err ) }
			entry := specOperation{ Method: strings.ToUpper( method ) , Path: path , HTTPOnly: operation.HTTPOnly , Public: len( operation.Security ) == 0 }
			for _ , scheme := range operation.Security {
				if _ , ok := scheme[ "apiKey" ]; ok { entry.KeyOK = true }
			}
			operations[ entry.name() ] = entry
		}
	}
	return
}

// newParityFixture is the app with every switch on, so that no feature's
// tools hide behind a 404 and every route is there to be called.
func newParityFixture( t *testing.T ) ( fixture *monitored ) {
	t.Helper()
	fixture = newMonitored( t , true )
	fixture.cfg.MCPEnabled = true
	// >>> feature:files
	directory := t.TempDir()
	fixture.cfg.Files = config.FilesConfig{
		Enabled: true , MaxFileBytes: 1 << 20 , QuotaBytesPerUser: 1 << 20 , AbandonedAfter: time.Hour,
		Dir: filepath.Join( directory , "files" ) , UploadsDir: filepath.Join( directory , "uploads" ),
	}
	// <<< feature:files
	return
}

func TestEveryKeyRouteIsAnMCPTool( t *testing.T ) {
	operations := specOperations( t )
	fixture := newParityFixture( t )
	problems := []string{}
	covered := map[string]bool{}
	for _ , tool := range fixture.handlers.MCP.Tools() {
		operation , found := operations[ tool.Route ]
		switch {
		case tool.Route == "":
			problems = append( problems , "tool " + tool.Name + " has no Route" )
		case found == false:
			problems = append( problems , "tool " + tool.Name + " names " + tool.Route + ", which is not in config/openapi.json" )
		case operation.KeyOK == false:
			problems = append( problems , "tool " + tool.Name + " names " + tool.Route + ", which an API key may not call" )
		case operation.HTTPOnly != "":
			problems = append( problems , tool.Route + " has a tool (" + tool.Name + ") and an x-mcp-http reason; drop one" )
		}
		covered[ tool.Route ] = true
	}
	for name , operation := range operations {
		if operation.KeyOK && covered[ name ] == false && operation.HTTPOnly == "" {
			problems = append( problems , name + " takes an API key but has no MCP tool: add one in mcp_tools.go, or an x-mcp-http reason in config/openapi.json" )
		}
		if operation.KeyOK == false && operation.HTTPOnly != "" {
			problems = append( problems , name + " has an x-mcp-http reason but no key may call it; drop the reason" )
		}
	}
	sort.Strings( problems )
	if len( problems ) > 0 { t.Fatal( strings.Join( problems , "\n" ) ) }
}

// What config/openapi.json says about each route's callers is what the guards
// do: a route listed for API keys is not turned away for being called with
// one, and a session-only route is. Called with the owner's admin key, so
// only RequireSession and RequireOwner can say no to the key itself.
func TestTheSpecSaysWhoMayCallEachRoute( t *testing.T ) {
	operations := specOperations( t )
	fixture := newParityFixture( t )
	parameter := regexp.MustCompile( `\{\w+\}` )
	names := []string{}
	for name , operation := range operations {
		if operation.Public { continue }
		if operation.Path == "/api/logout" || operation.Path == "/api/owner/logs/stream" { continue } // would end or hold the test
		names = append( names , name )
	}
	sort.Strings( names )
	problems := []string{}
	for _ , name := range names {
		operation := operations[ name ]
		path := parameter.ReplaceAllString( operation.Path , "999" )
		request , _ := http.NewRequest( operation.Method , path , bytes.NewReader( []byte( "{}" ) ) )
		request.Header.Set( "Content-Type" , "application/json" )
		request.Header.Set( "Authorization" , "Bearer " + fixture.adminKey )
		response , err := fixture.app.Test( request , fiber.TestConfig{ Timeout: 10 * time.Second } )
		if err != nil { t.Fatal( err ) }
		raw , _ := io.ReadAll( response.Body )
		var reply struct {
			Error string `json:"error"`
		}
		json.Unmarshal( raw , &reply )
		refused := ( response.StatusCode == fiber.StatusForbidden && strings.Contains( reply.Error , "requires a signed-in session" ) ) ||
			( response.StatusCode == fiber.StatusNotFound && reply.Error == "not_found" )
		if operation.KeyOK && refused {
			problems = append( problems , name + ": config/openapi.json lists apiKey, but a key is turned away" )
		}
		if operation.KeyOK == false && refused == false {
			problems = append( problems , name + ": config/openapi.json says session only, but a key got " + response.Status )
		}
	}
	if len( problems ) > 0 { t.Fatal( strings.Join( problems , "\n" ) ) }
}

// mcpManifest is config/mcp.json: what tools/list says, for every tool and
// any key, with the route each stands for and the routes that stay HTTP. It
// names no features: scaffold.py's prune step cuts a tool out by its route,
// using the x-feature openapi.json has in the template.
func mcpManifest( tools []*mcp.Tool , operations map[string]specOperation ) ( raw []byte , err error ) {
	listed := []fiber.Map{}
	for _ , tool := range tools {
		entry := tool.Listing()
		entry[ "route" ] = tool.Route
		entry[ "adminOnly" ] = tool.AdminOnly
		listed = append( listed , entry )
	}
	http_only := []fiber.Map{}
	names := []string{}
	for name , operation := range operations {
		if operation.HTTPOnly != "" { names = append( names , name ) }
	}
	sort.Strings( names )
	for _ , name := range names {
		http_only = append( http_only , fiber.Map{ "route": name , "reason": operations[ name ].HTTPOnly } )
	}
	document := fiber.Map{
		"description": "Every tool POST /mcp offers. Written by scripts/mcp_json.sh from the registered tools; do not edit by hand. A key sees the tools its role allows (adminOnly ones need an admin key), and a tool whose feature is switched off answers like one that does not exist. route is the HTTP route the tool stands for, with the same rules; http_only are the routes a key may call that have no tool, and why.",
		"endpoint":    "/mcp",
		"transport":   "Streamable HTTP, stateless: one JSON-RPC message per POST, a JSON reply",
		"auth":        "Authorization: Bearer <API key>",
		"protocolVersions": mcp.SupportedVersions,
		"tools":       listed,
		"http_only":   http_only,
	}
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder( buffer )
	encoder.SetEscapeHTML( false )
	encoder.SetIndent( "" , "  " )
	err = encoder.Encode( document )
	raw = buffer.Bytes()
	return
}

func TestMCPManifestIsCurrent( t *testing.T ) {
	operations := specOperations( t )
	fixture := newParityFixture( t )
	want , err := mcpManifest( fixture.handlers.MCP.Tools() , operations )
	if err != nil { t.Fatal( err ) }
	if *updateMCPManifest {
		if err = os.WriteFile( mcpManifestPath , want , 0o644 ); err != nil { t.Fatal( err ) }
		return
	}
	have , err := os.ReadFile( mcpManifestPath )
	if err != nil { t.Fatal( err ) }
	if bytes.Equal( have , want ) == false {
		t.Fatal( "config/mcp.json is out of date: run scripts/mcp_json.sh" )
	}
}

func TestTheMCPManifestIsForSignedInCallers( t *testing.T ) {
	fixture := newParityFixture( t )
	fixture.handlers.MCPManifest = []byte( `{"endpoint":"/mcp"}` )
	if response , _ := fixture.get( t , "/api/mcp.json" , nil ); response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf( "signed out: status %d" , response.StatusCode )
	}
	response , body := fixture.get( t , "/api/mcp.json" , map[string]string{ "Authorization": "Bearer " + fixture.userKey } )
	if response.StatusCode != fiber.StatusOK || strings.Contains( body , "/mcp" ) == false {
		t.Fatalf( "%d %s" , response.StatusCode , body )
	}
}

// /api/me says where the MCP reference is, as it does for the API one.
func TestMeSaysWhereTheMCPReferenceIs( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	if _ , me := alice.do( t , http.MethodGet , "/api/me" , nil ); me[ "mcp_url" ] != "/api/mcp.json" {
		t.Fatalf( "without a repo: %v" , me[ "mcp_url" ] )
	}
	fixture.cfg.Repo = "https://github.com/you/app"
	if _ , me := alice.do( t , http.MethodGet , "/api/me" , nil ); me[ "mcp_url" ] != "https://github.com/you/app/blob/HEAD/config/mcp.json" {
		t.Fatalf( "with a repo: %v" , me[ "mcp_url" ] )
	}
}
