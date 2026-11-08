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

// What the tests in this package share: an app wired through the real routes,
// and a browser to drive it with. Anything one feature's tests alone use stays
// in that feature's test file, since a project can leave the feature out.

type authFixture struct {
	app   *fiber.App
	store *db.Store
	cfg   *config.Config
	owner *models.User
	alice *models.User
	handlers *Handlers
}

// browser is one cookie jar.
type browser struct {
	fixture *authFixture
	cookie  string
}

func newAuthFixture( t *testing.T ) ( fixture *authFixture ) {
	t.Helper()
	cfg := &config.Config{
		ProjectName:      "Test",
		DatabasePath:     filepath.Join( t.TempDir() , "test.db" ),
		SecretKey:        strings.Repeat( "ab" , 32 ),
		CookieName:       "session",
		APIKeysEnabled:   true, // feature:api_keys
		PasswordsEnabled: true, // feature:passwords
		TwoFactorEnabled: true, // feature:two_factor
		InviteLinksEnabled: true, // feature:invite_links
		Webhooks:         config.WebhooksConfig{ Enabled: true }, // feature:webhooks
		SessionTTL:       time.Hour,
	}
	store , err := db.Open( cfg )
	if err != nil { t.Fatal( err ) }
	t.Cleanup( func() { store.Close() } )
	owner , _ := models.CreateUser( store , "Owner" , models.RoleAdmin )
	if _ , err = models.EnsureOwner( store ); err != nil { t.Fatal( err ) }
	alice , _ := models.CreateUser( store , "Alice" , models.RoleUser )

	static_server , _ := static.New( fstest.MapFS{ "index.html": { Data: []byte( "index" ) } } )
	settings := config.NewStaticHolder( cfg )
	handlers := New( store , settings , security.New( store , settings ) , nil , static_server , "test" )
	app := fiber.New()
	handlers.Register( app )
	fixture = &authFixture{ app: app , store: store , cfg: cfg , owner: owner , alice: alice , handlers: handlers }
	return
}

func ( fixture *authFixture ) browser() ( result *browser ) {
	result = &browser{ fixture: fixture }
	return
}

func ( client *browser ) do( t *testing.T , method string , path string , body fiber.Map ) ( status int , reply map[ string ]any ) {
	t.Helper()
	raw , _ := json.Marshal( body )
	request , _ := http.NewRequest( method , path , strings.NewReader( string( raw ) ) )
	request.Header.Set( "Content-Type" , "application/json" )
	if client.cookie != "" {
		request.Header.Set( "Cookie" , client.cookie )
	}
	response , err := client.fixture.app.Test( request , fiber.TestConfig{ Timeout: 10 * time.Second } )
	if err != nil { t.Fatal( err ) }
	for _ , cookie := range response.Cookies() {
		client.cookie = cookie.Name + "=" + cookie.Value
	}
	text , _ := io.ReadAll( response.Body )
	status = response.StatusCode
	json.Unmarshal( text , &reply )
	return
}

func ( client *browser ) post( t *testing.T , path string , body fiber.Map ) ( status int , reply map[ string ]any ) {
	t.Helper()
	status , reply = client.do( t , http.MethodPost , path , body )
	return
}

// send is a POST with the session's csrf token.
func send( t *testing.T , client *browser , path string , body fiber.Map ) ( status int , reply map[ string ]any ) {
	t.Helper()
	body[ "csrf_token" ] = client.csrf( t )
	status , reply = client.post( t , path , body )
	return
}

func ( client *browser ) csrf( t *testing.T ) ( token string ) {
	t.Helper()
	_ , reply := client.do( t , http.MethodGet , "/api/me" , nil )
	token , _ = reply[ "csrf_token" ].( string )
	return
}

func ( client *browser ) signedIn( t *testing.T ) ( result bool ) {
	t.Helper()
	status , _ := client.do( t , http.MethodGet , "/api/me" , nil )
	result = status == 200
	return
}

func ( fixture *authFixture ) link( t *testing.T , user_id uint64 ) ( credential string ) {
	t.Helper()
	credential , err := models.IssueLoginToken( fixture.store , user_id , time.Hour )
	if err != nil { t.Fatal( err ) }
	return
}

func ( fixture *authFixture ) signIn( t *testing.T , user_id uint64 ) ( client *browser ) {
	t.Helper()
	client = fixture.browser()
	if status , reply := client.post( t , "/api/login" , fiber.Map{ "credential": fixture.link( t , user_id ) } ); status != 200 {
		t.Fatalf( "login link: %d %v" , status , reply )
	}
	return
}

func sessionCookie( t *testing.T , store *db.Store , cfg *config.Config , user_id uint64 ) ( cookie string ) {
	t.Helper()
	login , err := models.IssueLoginToken( store , user_id , time.Hour )
	if err != nil { t.Fatal( err ) }
	signed , err := models.RedeemLoginToken( store , login , models.SignInRules{ SessionTTL: time.Hour } )
	if err != nil { t.Fatal( err ) }
	sealed , _ := encryption.ChaChaEncryptString( cfg.SecretKey , signed.SessionCredential )
	cookie = cfg.CookieName + "=" + sealed
	return
}

// >>> feature:passwords
// digest stands in for what password.js derives: 43 base64url characters.
func digest( text string ) ( result string ) {
	result = encryption.Sha256Hex( text )[ :43 ]
	return
}

func ( client *browser ) setPassword( t *testing.T , username string , text string , code string ) ( status int , reply map[ string ]any ) {
	t.Helper()
	status , reply = client.post( t , "/api/account/password" , fiber.Map{
		"username": username , "password_digest": digest( text ) , "code": code , "csrf_token": client.csrf( t ) ,
	} )
	return
}

func ( client *browser ) login( t *testing.T , username string , text string ) ( status int , reply map[ string ]any ) {
	t.Helper()
	status , reply = client.post( t , "/api/login/password" , fiber.Map{ "username": username , "password_digest": digest( text ) } )
	return
}
// <<< feature:passwords

// >>> feature:two_factor
func codeFor( t *testing.T , secret string , offset int64 ) ( code string ) {
	t.Helper()
	code , err := encryption.TOTPCode( secret , uint64( time.Now().Unix()/encryption.TOTPPeriodSeconds+offset ) )
	if err != nil { t.Fatal( err ) }
	return
}

func ( client *browser ) enrol( t *testing.T ) ( secret string , recovery []string ) {
	t.Helper()
	token := client.csrf( t )
	_ , started := client.post( t , "/api/account/two-factor/start" , fiber.Map{ "csrf_token": token } )
	secret = started[ "secret" ].( string )
	status , reply := client.post( t , "/api/account/two-factor/confirm" , fiber.Map{ "code": codeFor( t , secret , 0 ) , "csrf_token": token } )
	if status != 200 { t.Fatalf( "confirm: %d %v" , status , reply ) }
	for _ , code := range reply[ "recovery_codes" ].( []any ) {
		recovery = append( recovery , code.( string ) )
	}
	return
}

func ( fixture *authFixture ) challenge( t *testing.T , user_id uint64 ) ( challenge string ) {
	t.Helper()
	_ , reply := fixture.browser().post( t , "/api/login" , fiber.Map{ "credential": fixture.link( t , user_id ) } )
	challenge , ok := reply[ "challenge" ].( string )
	if ok == false { t.Fatalf( "a login link did not ask for a code: %v" , reply ) }
	return
}

func ( fixture *authFixture ) finish( t *testing.T , challenge string , code string ) ( status int , reply map[ string ]any ) {
	t.Helper()
	status , reply = fixture.browser().post( t , "/api/login/two-factor" , fiber.Map{ "challenge": challenge , "code": code } )
	return
}
// <<< feature:two_factor
