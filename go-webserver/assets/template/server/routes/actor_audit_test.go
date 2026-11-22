// whole-file feature:logs
package routes

import (
	json "encoding/json"
	http "net/http" // feature:api_keys
	os "os"
	filepath "path/filepath"
	strings "strings"
	testing "testing"
	time "time" // feature:api_keys

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models" // feature:api_keys|webhooks
	webhooks "{{MODULE_PATH}}/server/webhooks" // feature:webhooks
)

// Every admin action says who did it (portal contract v3, item 6): the
// account, and when it was a key, which key -- what the service account
// exists for. The Logs screen shows it, and webhooks carry it.

// auditEntries are the entries written so far, by message, closing the log.
func auditEntries( t *testing.T , fixture *monitored ) ( found map[ string ][]map[ string ]any ) {
	t.Helper()
	fixture.logger.Close()
	found = map[ string ][]map[ string ]any{}
	files , _ := filepath.Glob( filepath.Join( fixture.logDir , "*.jsonl" ) )
	for _ , file := range files {
		raw , _ := os.ReadFile( file )
		for _ , line := range strings.Split( string( raw ) , "\n" ) {
			entry := map[ string ]any{}
			if json.Unmarshal( []byte( line ) , &entry ) != nil { continue }
			message , _ := entry[ "msg" ].( string )
			found[ message ] = append( found[ message ] , entry )
		}
	}
	return
}

func TestAnAdminActionSaysWhoDidIt( t *testing.T ) {
	fixture := newMonitored( t , true )
	var written map[ string ][]map[ string ]any
	// >>> feature:webhooks
	fixture.handlers.Webhooks = webhooks.New( fixture.store , fixture.handlers.Settings )
	_ , hook , err := models.CreateWebhook( fixture.store , fixture.cfg.SecretKey , "x" , "https://a.example" , []string{ models.EventUserCreated , models.EventUserDisabled } , fixture.owner.ID )
	if err != nil { t.Fatal( err ) }
	// webhookActor is the "actor" of the hook's deliveries for one event.
	webhookActor := func( event string ) ( actor map[ string ]any ) {
		rows , _ := models.ListWebhookDeliveries( fixture.store , hook.ID )
		for _ , row := range rows {
			body := map[ string ]any{}
			json.Unmarshal( []byte( row.Body ) , &body )
			if body[ "event" ] == event { actor , _ = body[ "actor" ].( map[ string ]any ) }
		}
		return
	}
	// <<< feature:webhooks
	owner := fixture.signIn( t , fixture.owner.ID )
	if status , reply := send( t , owner , "/api/admin/users/" + idText( fixture.alice.ID ) + "/disabled" , fiber.Map{ "disabled": true } ); status != fiber.StatusOK { t.Fatalf( "%d %v" , status , reply ) }
	// >>> feature:webhooks
	if actor := webhookActor( models.EventUserDisabled ); actor == nil || actor[ "actor_user_id" ] != float64( fixture.owner.ID ) || actor[ "actor_key_id" ] != nil {
		t.Fatalf( "a session's webhook actor: %v" , actor )
	}
	// <<< feature:webhooks
	// >>> feature:api_keys
	credential , key , user , err := models.ConnectPortal( fixture.store , fixture.owner.ID , models.DefaultPortalName , 24 * time.Hour )
	if err != nil { t.Fatal( err ) }
	status , reply := fixture.withKey( t , credential , http.MethodPost , "/api/admin/users" , fiber.Map{ "display_name": "Cy" , "role": "user" } )
	if status != fiber.StatusCreated { t.Fatalf( "%d %v" , status , reply ) }
	// >>> feature:webhooks
	if actor := webhookActor( models.EventUserCreated ); actor == nil || actor[ "actor_user_id" ] != float64( user.ID ) || actor[ "actor_name" ] != "ProjectPortal" || actor[ "actor_key_id" ] != key.ID || actor[ "actor_key_name" ] != key.Name {
		t.Fatalf( "webhook actor: %v" , actor )
	}
	// <<< feature:webhooks
	written = auditEntries( t , fixture )
	entries := written[ "user created" ]
	if len( entries ) != 1 { t.Fatalf( "%v" , entries ) }
	detail := entries[ 0 ][ "detail" ].( map[ string ]any )
	if detail[ "actor_user_id" ] != float64( user.ID ) || detail[ "actor_name" ] != "ProjectPortal" || detail[ "actor_key_id" ] != key.ID || detail[ "actor_key_name" ] != key.Name {
		t.Fatalf( "audit detail: %v" , detail )
	}
	if entries[ 0 ][ "actor" ].( map[ string ]any )[ "key_name" ] != key.Name { t.Fatalf( "actor: %v" , entries[ 0 ][ "actor" ] ) }
	// <<< feature:api_keys
	written = auditEntries( t , fixture ) // feature:!api_keys

	// A session: the account, and no key.
	if len( written[ "user disabled set" ] ) != 1 { t.Fatalf( "%v" , written ) }
	for _ , entry := range written[ "user disabled set" ] {
		detail := entry[ "detail" ].( map[ string ]any )
		if detail[ "actor_user_id" ] != float64( fixture.owner.ID ) || detail[ "actor_name" ] != "Owner" { t.Fatalf( "%v" , detail ) }
		if _ , has := detail[ "actor_key_id" ]; has { t.Fatalf( "a session has no key: %v" , detail ) }
	}
}
