// whole-file feature:mcp
package routes

import (
	json "encoding/json"
	io "io"
	http "net/http"
	filepath "path/filepath"
	strings "strings"
	testing "testing"
	fstest "testing/fstest"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	config "{{MODULE_PATH}}/server/config"
	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
	static "{{MODULE_PATH}}/server/static"
)

// These run /mcp end to end -- real database, real keys, the real route map --
// because the properties worth testing (a user key never sees an admin tool,
// a session is turned away, the owner rules still hold) live in the wiring
// between packages, not in any one of them.

type mcpFixture struct {
	app       *fiber.App
	store     *db.Store
	cfg       *config.Config
	owner     *models.User
	member    *models.User
	admin_key string
	user_key  string
}

func newMCPFixture( t *testing.T ) ( fixture *mcpFixture ) {
	t.Helper()
	cfg := &config.Config{
		ProjectName:    "Test",
		DatabasePath:   filepath.Join( t.TempDir() , "test.db" ),
		SecretKey:      strings.Repeat( "ab" , 32 ),
		CookieName:     "session",
		APIKeysEnabled: true,
		MCPEnabled:     true,
		InviteLinksEnabled: true, // feature:invite_links
		Webhooks:       config.WebhooksConfig{ Enabled: true }, // feature:webhooks
		SessionTTL:     time.Hour,
	}
	store , err := db.Open( cfg )
	if err != nil { t.Fatal( err ) }
	t.Cleanup( func() { store.Close() } )

	owner , _ := models.CreateUser( store , "Owner" , models.RoleAdmin )
	if _ , err = models.EnsureOwner( store ); err != nil { t.Fatal( err ) }
	member , _ := models.CreateUser( store , "Member" , models.RoleUser )
	admin_key , _ , err := models.IssueAPIKey( store , owner.ID , "admin" , models.RoleAdmin , time.Hour )
	if err != nil { t.Fatal( err ) }
	// An admin's user-scoped key: the ceiling is what is being tested.
	user_key , _ , err := models.IssueAPIKey( store , owner.ID , "user" , models.RoleUser , time.Hour )
	if err != nil { t.Fatal( err ) }

	static_server , _ := static.New( fstest.MapFS{ "index.html": { Data: []byte( "index" ) } } )
	settings := config.NewStaticHolder( cfg )
	handlers := New( store , settings , security.New( store , settings ) , nil , static_server , "test" )
	app := fiber.New()
	app.Use( checkReplies( t ) )
	handlers.Register( app )

	fixture = &mcpFixture{ app: app , store: store , cfg: cfg , owner: owner , member: member , admin_key: admin_key , user_key: user_key }
	return
}

func ( fixture *mcpFixture ) post( t *testing.T , key string , body string , headers ...string ) ( status int , reply map[ string ]any ) {
	t.Helper()
	request , _ := http.NewRequest( http.MethodPost , "/mcp" , strings.NewReader( body ) )
	request.Header.Set( "Content-Type" , "application/json" )
	request.Header.Set( "Accept" , "application/json, text/event-stream" )
	if key != "" {
		request.Header.Set( "Authorization" , "Bearer "+key )
	}
	for index := 0; index+1 < len( headers ); index += 2 {
		request.Header.Set( headers[ index ] , headers[ index+1 ] )
	}
	response , err := fixture.app.Test( request )
	if err != nil { t.Fatal( err ) }
	raw , _ := io.ReadAll( response.Body )
	status = response.StatusCode
	if len( raw ) > 0 {
		json.Unmarshal( raw , &reply )
	}
	return
}

func rpc( method string , params string ) ( body string ) {
	body = `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":` + params + `}`
	return
}

func callTool( name string , args string ) ( body string ) {
	body = rpc( "tools/call" , `{"name":"`+name+`","arguments":`+args+`}` )
	return
}

func toolNames( reply map[ string ]any ) ( names []string ) {
	result , _ := reply[ "result" ].( map[ string ]any )
	tools , _ := result[ "tools" ].( []any )
	for _ , tool := range tools {
		names = append( names , tool.( map[ string ]any )[ "name" ].( string ) )
	}
	return
}

func toolFailed( t *testing.T , reply map[ string ]any ) ( failed bool , text string ) {
	t.Helper()
	result , ok := reply[ "result" ].( map[ string ]any )
	if ok == false { t.Fatalf( "no result in %v" , reply ) }
	failed , _ = result[ "isError" ].( bool )
	content := result[ "content" ].( []any )
	text = content[ 0 ].( map[ string ]any )[ "text" ].( string )
	return
}

func TestMCPInitializeNegotiatesVersion( t *testing.T ) {
	fixture := newMCPFixture( t )
	_ , reply := fixture.post( t , fixture.user_key , rpc( "initialize" , `{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}` ) )
	result := reply[ "result" ].( map[ string ]any )
	if result[ "protocolVersion" ] != "2025-06-18" {
		t.Errorf( "known version not echoed: %v" , result[ "protocolVersion" ] )
	}
	_ , reply = fixture.post( t , fixture.user_key , rpc( "initialize" , `{"protocolVersion":"1999-01-01"}` ) )
	if reply[ "result" ].( map[ string ]any )[ "protocolVersion" ] != "2025-11-25" {
		t.Errorf( "unknown version should get our newest" )
	}
	if status , _ := fixture.post( t , fixture.user_key , rpc( "ping" , `{}` ) , "MCP-Protocol-Version" , "1999-01-01" ); status != 400 {
		t.Errorf( "unsupported MCP-Protocol-Version header: status %d, want 400" , status )
	}
}

func TestMCPTransportRules( t *testing.T ) {
	fixture := newMCPFixture( t )
	if status , _ := fixture.post( t , fixture.user_key , `{"jsonrpc":"2.0","method":"notifications/initialized"}` ); status != 202 {
		t.Errorf( "notification: status %d, want 202" , status )
	}
	if _ , reply := fixture.post( t , fixture.user_key , rpc( "nope" , `{}` ) ); reply[ "error" ].( map[ string ]any )[ "code" ].( float64 ) != -32601 {
		t.Errorf( "unknown method: %v" , reply )
	}
	if status , _ := fixture.post( t , fixture.user_key , `[`+rpc( "ping" , `{}` )+`]` ); status != 400 {
		t.Errorf( "batch: status %d, want 400" , status )
	}
	if status , _ := fixture.post( t , fixture.user_key , `{not json` ); status != 400 {
		t.Errorf( "parse error: status %d, want 400" , status )
	}
	if status , _ := fixture.post( t , fixture.user_key , rpc( "ping" , `{}` ) , "Origin" , "https://evil.example" ); status != 403 {
		t.Errorf( "cross-origin: status %d, want 403" , status )
	}
	request , _ := http.NewRequest( http.MethodGet , "/mcp" , nil )
	if response , _ := fixture.app.Test( request ); response.StatusCode != 405 {
		t.Errorf( "GET /mcp: status %d, want 405" , response.StatusCode )
	}
}

func TestMCPAuthentication( t *testing.T ) {
	fixture := newMCPFixture( t )
	if status , _ := fixture.post( t , "" , rpc( "ping" , `{}` ) ); status != 401 {
		t.Errorf( "no key: status %d, want 401" , status )
	}
	if status , _ := fixture.post( t , "nope.nope" , rpc( "ping" , `{}` ) ); status != 401 {
		t.Errorf( "bad key: status %d, want 401" , status )
	}

	// A browser session is refused, even an admin's.
	login , _ := models.IssueLoginToken( fixture.store , fixture.owner.ID , time.Hour )
	signed , err := models.RedeemLoginToken( fixture.store , login , models.SignInRules{ SessionTTL: time.Hour } )
	if err != nil { t.Fatal( err ) }
	cookie , _ := encryption.ChaChaEncryptString( fixture.cfg.SecretKey , signed.SessionCredential )
	if status , _ := fixture.post( t , "" , rpc( "ping" , `{}` ) , "Cookie" , "session="+cookie ); status != 403 {
		t.Errorf( "session: status %d, want 403" , status )
	}
}

func TestMCPToolsFollowTheKeyRole( t *testing.T ) {
	fixture := newMCPFixture( t )

	_ , reply := fixture.post( t , fixture.user_key , rpc( "tools/list" , `{}` ) )
	for _ , name := range toolNames( reply ) {
		if strings.HasPrefix( name , "admin_" ) {
			t.Errorf( "user-scoped key was shown %s" , name )
		}
	}
	_ , reply = fixture.post( t , fixture.admin_key , rpc( "tools/list" , `{}` ) )
	if strings.Contains( strings.Join( toolNames( reply ) , "," ) , "admin_list_users" ) == false {
		t.Errorf( "admin key not shown admin tools: %v" , toolNames( reply ) )
	}

	// whoami reports the narrowed role, not the account's.
	_ , reply = fixture.post( t , fixture.user_key , callTool( "whoami" , `{}` ) )
	if _ , text := toolFailed( t , reply ); strings.Contains( text , `"role":"user"` ) == false {
		t.Errorf( "whoami with a user-scoped admin key: %s" , text )
	}

	// An admin tool asked for by a user key does not exist, as far as it knows.
	_ , reply = fixture.post( t , fixture.user_key , callTool( "admin_list_users" , `{}` ) )
	if reply[ "error" ] == nil {
		t.Errorf( "user key called admin_list_users: %v" , reply )
	}
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_list_users" , `{}` ) )
	if failed , text := toolFailed( t , reply ); failed || strings.Contains( text , "Member" ) == false {
		t.Errorf( "admin_list_users: %s" , text )
	}
}

func TestMCPAdminToolsKeepTheRules( t *testing.T ) {
	fixture := newMCPFixture( t )
	owner_id := `{"user_id":` + itoa( fixture.owner.ID )
	member_id := `{"user_id":` + itoa( fixture.member.ID )

	// The caller is the owner here, so "yourself" is what refuses first.
	_ , reply := fixture.post( t , fixture.admin_key , callTool( "admin_set_user_disabled" , owner_id+`,"disabled":true}` ) )
	if failed , text := toolFailed( t , reply ); failed == false || strings.Contains( text , "your own account" ) == false {
		t.Errorf( "disabling yourself: %s" , text )
	}
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_revoke_access" , owner_id+`}` ) )
	if failed , _ := toolFailed( t , reply ); failed == false {
		t.Errorf( "revoking yourself was allowed" )
	}

	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_set_user_disabled" , member_id+`,"disabled":true}` ) )
	if failed , text := toolFailed( t , reply ); failed {
		t.Errorf( "disabling a member: %s" , text )
	}
	if user , _ := models.GetUser( fixture.store , fixture.member.ID ); user.Disabled() == false {
		t.Errorf( "member not disabled" )
	}

	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_set_user_disabled" , `{"user_id":999,"disabled":true}` ) )
	if failed , text := toolFailed( t , reply ); failed == false || strings.Contains( text , "no such user" ) == false {
		t.Errorf( "missing user: %s" , text )
	}

	// A misspelt argument is reported, not ignored.
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_set_user_disabled" , member_id+`,"disable":true}` ) )
	if reply[ "error" ] == nil {
		t.Errorf( "unknown argument accepted: %v" , reply )
	}
}

func TestMCPAccountTools( t *testing.T ) {
	fixture := newMCPFixture( t )

	// An admin key makes an account and gets a full address to hand over.
	_ , reply := fixture.post( t , fixture.admin_key , callTool( "admin_create_user" , `{"display_name":"Bob","role":"user","max_uses":2}` ) )
	failed , text := toolFailed( t , reply )
	if failed || strings.Contains( text , `"login_url":"http` ) == false || strings.Contains( text , "/login/" ) == false {
		t.Fatalf( "admin_create_user: %s" , text )
	}

	// The narrowed key on the owner's admin account may not make itself a
	// login link: the session would be an admin's.
	_ , reply = fixture.post( t , fixture.user_key , callTool( "create_my_login_link" , `{}` ) )
	if failed , text = toolFailed( t , reply ); failed == false || strings.Contains( text , "less than your account's role" ) == false {
		t.Errorf( "narrowed key's own link: %s" , text )
	}
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "create_my_login_link" , `{}` ) )
	if failed , text = toolFailed( t , reply ); failed || strings.Contains( text , "login_url" ) == false {
		t.Errorf( "full key's own link: %s" , text )
	}

	// The owner rules hold through the tools.
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_set_user_role" , `{"user_id":`+itoa( fixture.owner.ID )+`,"role":"user"}` ) )
	if failed , _ = toolFailed( t , reply ); failed == false { t.Errorf( "changed your own role" ) }
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_rename_user" , `{"user_id":`+itoa( fixture.member.ID )+`,"display_name":"Mem"}` ) )
	if failed , text = toolFailed( t , reply ); failed { t.Errorf( "rename: %s" , text ) }
}

func itoa( value uint64 ) ( result string ) {
	encoded , _ := json.Marshal( value )
	result = string( encoded )
	return
}

// >>> feature:files
// A file goes up and comes back the way the tools describe: start and finish
// through /mcp, the bytes over plain HTTP with the same key.
func TestMCPFilesTravelOverHTTP( t *testing.T ) {
	fixture := newMCPFixture( t )
	directory := t.TempDir()
	fixture.cfg.Files = config.FilesConfig{
		Enabled: true , MaxFileBytes: 16 << 20 , QuotaBytesPerUser: 16 << 20 , AbandonedAfter: time.Hour,
		Dir: filepath.Join( directory , "files" ) , UploadsDir: filepath.Join( directory , "uploads" ),
	}
	key := fixture.user_key
	content := strings.Repeat( "agent upload " , 100 )
	keyed := func( method string , url string , body string , content_type string ) ( status int , text string ) {
		request , _ := http.NewRequest( method , url , strings.NewReader( body ) )
		request.Header.Set( "Authorization" , "Bearer " + key )
		if content_type != "" { request.Header.Set( "Content-Type" , content_type ) }
		response , err := fixture.app.Test( request )
		if err != nil { t.Fatal( err ) }
		raw , _ := io.ReadAll( response.Body )
		status , text = response.StatusCode , string( raw )
		return
	}

	_ , reply := fixture.post( t , key , callTool( "files_start_upload" , `{"name":"notes.txt","size":`+itoa( uint64( len( content ) ) )+`}` ) )
	failed , text := toolFailed( t , reply )
	if failed { t.Fatalf( "start: %s" , text ) }
	var started struct {
		ID       string `json:"id"`
		Chunks   int    `json:"chunks"`
		ChunkURL string `json:"chunk_url"`
	}
	json.Unmarshal( []byte( text ) , &started )
	if started.Chunks != 1 || strings.Contains( started.ChunkURL , "/api/files/"+started.ID+"/chunks/{index}" ) == false {
		t.Fatalf( "start answered %s" , text )
	}
	chunk_path := strings.Replace( started.ChunkURL[ strings.Index( started.ChunkURL , "/api/" ): ] , "{index}" , "0" , 1 )
	if status , body := keyed( http.MethodPost , chunk_path , content , "application/octet-stream" ); status != 200 {
		t.Fatalf( "chunk: %d %s" , status , body )
	}

	_ , reply = fixture.post( t , key , callTool( "files_finish_upload" , `{"file_id":"`+started.ID+`"}` ) )
	if failed , text := toolFailed( t , reply ); failed || strings.Contains( text , `"state":"ready"` ) == false {
		t.Fatalf( "finish: %s" , text )
	}
	_ , reply = fixture.post( t , key , callTool( "files_download_url" , `{"file_id":"`+started.ID+`"}` ) )
	failed , text = toolFailed( t , reply )
	var where struct {
		URL string `json:"url"`
	}
	json.Unmarshal( []byte( text ) , &where )
	if failed || strings.HasSuffix( where.URL , "/api/files/"+started.ID+"/download" ) == false {
		t.Fatalf( "download url: %s" , text )
	}
	if status , body := keyed( http.MethodGet , where.URL[ strings.Index( where.URL , "/api/" ): ] , "" , "" ); status != 200 || body != content {
		t.Fatalf( "download: %d, %d bytes" , status , len( body ) )
	}

	// The same refusals as the route: someone else's file is no file.
	member_key , _ , err := models.IssueAPIKey( fixture.store , fixture.member.ID , "member" , models.RoleUser , time.Hour )
	if err != nil { t.Fatal( err ) }
	_ , reply = fixture.post( t , member_key , callTool( "files_rename" , `{"file_id":"`+started.ID+`","name":"x.txt"}` ) )
	if failed , text := toolFailed( t , reply ); failed == false || strings.Contains( text , "no such file" ) == false {
		t.Fatalf( "renaming another key's file: %s" , text )
	}
	_ , reply = fixture.post( t , key , callTool( "files_delete" , `{"file_id":"`+started.ID+`"}` ) )
	if failed , text := toolFailed( t , reply ); failed {
		t.Fatalf( "delete: %s" , text )
	}
	_ , reply = fixture.post( t , key , callTool( "files_restore" , `{"file_id":"`+started.ID+`"}` ) )
	if failed , text := toolFailed( t , reply ); failed {
		t.Fatalf( "restore: %s" , text )
	}

	// Switched off, the tools are gone like the routes.
	fixture.cfg.Files.Enabled = false
	_ , reply = fixture.post( t , key , rpc( "tools/list" , `{}` ) )
	if strings.Contains( strings.Join( toolNames( reply ) , "," ) , "files_" ) {
		t.Errorf( "files tools listed with files off: %v" , toolNames( reply ) )
	}
}
// <<< feature:files
