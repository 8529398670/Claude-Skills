package routes

import (
	http "net/http"
	strings "strings"
	testing "testing"

	fiber "github.com/gofiber/fiber/v3"

	models "{{MODULE_PATH}}/server/models"
	webhooks "{{MODULE_PATH}}/server/webhooks"
)

// send is a POST with the session's csrf token.
func send( t *testing.T , client *browser , path string , body fiber.Map ) ( status int , reply map[ string ]any ) {
	t.Helper()
	body[ "csrf_token" ] = client.csrf( t )
	status , reply = client.post( t , path , body )
	return
}

func createdWebhook( t *testing.T , owner *browser ) ( id string , secret string ) {
	t.Helper()
	status , reply := send( t , owner , "/api/owner/webhooks" , fiber.Map{
		"name": "Chat" , "url": "https://hooks.example.com/in" , "events": []string{ "*" },
	} )
	if status != fiber.StatusCreated { t.Fatalf( "%d %v" , status , reply ) }
	id , _ = reply[ "id" ].( string )
	secret , _ = reply[ "secret" ].( string )
	return
}

func TestTheOwnerManagesWebhooksAndTheSecretShowsOnce( t *testing.T ) {
	fixture := newAuthFixture( t )
	owner := fixture.signIn( t , fixture.owner.ID )
	id , secret := createdWebhook( t , owner )
	if strings.HasPrefix( secret , "whsec_" ) == false { t.Fatalf( "%q" , secret ) }

	status , page := owner.do( t , http.MethodGet , "/api/owner/webhooks" , nil )
	rows , _ := page[ "webhooks" ].( []any )
	if status != 200 || len( rows ) != 1 { t.Fatalf( "%d %v" , status , page ) }
	if _ , present := rows[ 0 ].( map[ string ]any )[ "secret" ]; present { t.Fatal( "the list shows the secret" ) }

	if status , _ = send( t , owner , "/api/owner/webhooks/"+id+"/delete" , fiber.Map{} ); status != 200 { t.Fatalf( "delete %d" , status ) }
	if _ , page = owner.do( t , http.MethodGet , "/api/owner/webhooks" , nil ); page[ "total" ] != float64( 0 ) { t.Fatalf( "%v" , page ) }
	if status , _ = send( t , owner , "/api/owner/webhooks/"+id+"/restore" , fiber.Map{} ); status != 200 { t.Fatalf( "restore %d" , status ) }
	if status , _ = send( t , owner , "/api/owner/webhooks/"+id+"/enabled" , fiber.Map{ "enabled": false } ); status != 200 { t.Fatalf( "enabled %d" , status ) }
	hook , err := models.GetWebhook( fixture.store , id )
	if err != nil || hook.Enabled { t.Fatalf( "%+v %v" , hook , err ) }
}

func TestWebhookRoutesAreTheOwnersAlone( t *testing.T ) {
	fixture := newAuthFixture( t )
	alice := fixture.signIn( t , fixture.alice.ID )
	if status , _ := alice.do( t , http.MethodGet , "/api/owner/webhooks" , nil ); status != fiber.StatusNotFound {
		t.Fatalf( "%d" , status )
	}
}

func TestCreatingAWebhookChecksTheAddress( t *testing.T ) {
	fixture := newAuthFixture( t )
	owner := fixture.signIn( t , fixture.owner.ID )
	for _ , target := range []string{ "http://hooks.example.com/in" , "https://127.0.0.1/in" , "https://localhost/in" , "https://169.254.169.254/" } {
		status , reply := send( t , owner , "/api/owner/webhooks" , fiber.Map{ "name": "x" , "url": target , "events": []string{ "*" } } )
		if status != fiber.StatusBadRequest { t.Fatalf( "%s: %d %v" , target , status , reply ) }
	}
	status , reply := send( t , owner , "/api/owner/webhooks" , fiber.Map{ "name": "x" , "url": "https://a.example" , "events": []string{ "made.up" } } )
	if status != fiber.StatusBadRequest { t.Fatalf( "%d %v" , status , reply ) }
}

func TestATestEventQueuesADeliveryAndListsIt( t *testing.T ) {
	fixture := newAuthFixture( t )
	fixture.handlers.Webhooks = webhooks.New( fixture.store , fixture.handlers.Settings )
	owner := fixture.signIn( t , fixture.owner.ID )
	id , _ := createdWebhook( t , owner )
	if status , reply := send( t , owner , "/api/owner/webhooks/"+id+"/test" , fiber.Map{} ); status != fiber.StatusAccepted {
		t.Fatalf( "%d %v" , status , reply )
	}
	status , page := owner.do( t , http.MethodGet , "/api/owner/webhooks/"+id+"/deliveries?limit=1" , nil )
	rows , _ := page[ "deliveries" ].( []any )
	if status != 200 || len( rows ) != 1 { t.Fatalf( "%d %v" , status , page ) }
	row := rows[ 0 ].( map[ string ]any )
	if row[ "event" ] != models.EventWebhookTest || row[ "status" ] != models.WebhookPending { t.Fatalf( "%v" , row ) }
	if _ , present := row[ "body" ]; present { t.Fatal( "the list carries the body" ) }
	if status , _ = owner.do( t , http.MethodGet , "/api/owner/webhooks/nope/deliveries" , nil ); status != fiber.StatusNotFound { t.Fatalf( "%d" , status ) }
}

// The two doors reach the same rule, so an event from either should go out.
func TestDisablingAUserEmitsFromTheSharedHelper( t *testing.T ) {
	fixture := newAuthFixture( t )
	fixture.handlers.Webhooks = webhooks.New( fixture.store , fixture.handlers.Settings )
	_ , hook , err := models.CreateWebhook( fixture.store , fixture.cfg.SecretKey , "x" , "https://a.example" , []string{ models.EventUserDisabled } , fixture.owner.ID )
	if err != nil { t.Fatal( err ) }
	if refused := fixture.handlers.setUserDisabled( fixture.owner.ID , fixture.alice.ID , true ); refused != nil { t.Fatalf( "%+v" , refused ) }
	rows , _ := models.ListWebhookDeliveries( fixture.store , hook.ID )
	if len( rows ) != 1 || strings.Contains( rows[ 0 ].Body , `"disabled":true` ) == false { t.Fatalf( "%+v" , rows ) }
}
