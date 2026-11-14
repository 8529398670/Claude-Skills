// whole-file feature:webhooks
package routes

import (
	errors "errors"
	strings "strings"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	db "{{MODULE_PATH}}/server/db"
	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
	webhooks "{{MODULE_PATH}}/server/webhooks"
)

// The owner's Webhooks screen. Owner only, from a browser session, like the
// Logs and Settings screens: an endpoint sends what happens here to somewhere
// else, so a leaked key must not be able to add one. There are no MCP tools
// for this, for the same reason. See server/webhooks.

// RequireWebhooks makes the Webhooks screen's routes answer 404 while
// webhooks.enabled is off. Per request, like RequireFiles.
func ( handlers *Handlers ) RequireWebhooks( c fiber.Ctx ) ( err error ) {
	if handlers.cfg().Webhooks.Enabled == false {
		err = notFound( c , "not found" )
		return
	}
	err = c.Next()
	return
}

// webhookIDPath is the webhook a route under .../webhooks/{webhook_id} acts on.
type webhookIDPath struct {
	WebhookID string `json:"webhook_id" path:"webhook_id" jsonschema_description:"The webhook's id, from GET /api/owner/webhooks."`
}

// webhookRow picks the fields by hand, like sessionRow, so the sealed secret
// cannot reach a response by way of a change to the model.
type webhookRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func ( webhookRow ) APIName() ( name string ) { name = "Webhook"; return }

func newWebhookRow( hook *models.Webhook ) ( row webhookRow ) {
	row = webhookRow{
		ID:        hook.ID,
		Name:      hook.Name,
		URL:       hook.URL,
		Events:    hook.Events,
		Enabled:   hook.Enabled,
		CreatedAt: hook.CreatedAt,
	}
	return
}

// deliveryRow leaves out the body: the screen is for "did it arrive".
type deliveryRow struct {
	ID            string    `json:"id"`
	Event         string    `json:"event"`
	Status        string    `json:"status"`
	Attempts      int       `json:"attempts"`
	CreatedAt     time.Time `json:"created_at"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	LastStatus    int       `json:"last_status,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	FinishedAt    time.Time `json:"finished_at,omitzero"`
}

func ( deliveryRow ) APIName() ( name string ) { name = "WebhookDelivery"; return }

func newDeliveryRow( delivery *models.WebhookDelivery ) ( row deliveryRow ) {
	row = deliveryRow{
		ID:            delivery.ID,
		Event:         delivery.Event,
		Status:        delivery.Status,
		Attempts:      delivery.Attempts,
		CreatedAt:     delivery.CreatedAt,
		NextAttemptAt: delivery.NextAttemptAt,
		LastStatus:    delivery.LastStatus,
		LastError:     delivery.LastError,
	}
	if delivery.FinishedAt != nil { row.FinishedAt = *delivery.FinishedAt }
	return
}

type webhookEventsReply struct {
	Events []string `json:"events"`
}

// ListWebhookEvents is what the form offers.
func ( handlers *Handlers ) ListWebhookEvents() *apidoc.Endpoint[ apidoc.None , webhookEventsReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Events a webhook can ask for" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Errors: []int{ 404 } },
		func( c fiber.Ctx , in *apidoc.None ) ( reply *webhookEventsReply , err error ) {
			reply = &webhookEventsReply{ Events: models.WebhookEvents }
			return
		} )
}

type listWebhooksRequest struct {
	Page
	Q    string `json:"q,omitempty" query:"q" jsonschema_description:"Keep webhooks whose name or address contains this, in any case."`
	Sort string `json:"sort,omitempty" query:"sort" jsonschema:"enum=name,enum=url,enum=enabled" jsonschema_description:"Field to sort the whole list by. Rows without it go last."`
	Direction
}

type webhooksReply struct {
	Webhooks []webhookRow `json:"webhooks"`
	Paged
}

func ( handlers *Handlers ) ListWebhooks() *apidoc.Endpoint[ listWebhooksRequest , webhooksReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Webhooks" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Errors: []int{ 400 , 404 } },
		func( c fiber.Ctx , in *listWebhooksRequest ) ( reply *webhooksReply , err error ) {
			hooks , err := models.ListWebhooks( handlers.Store )
			if err != nil { return }
			rows := []webhookRow{}
			for _ , hook := range hooks {
				rows = append( rows , newWebhookRow( hook ) )
			}
			reply = &webhooksReply{}
			reply.Webhooks , reply.Paged = sortedPage( matchRows( rows , in.Q , "name" , "url" ) , in.Sort , in.Direction , in.Page )
			return
		} )
}

type createWebhookRequest struct {
	Name   string   `json:"name" jsonschema:"minLength=1,maxLength=80" error:"name must be 1-80 characters, the url at most 2000, and at least one known event picked"`
	URL    string   `json:"url" jsonschema:"minLength=1,maxLength=2000" error:"name must be 1-80 characters, the url at most 2000, and at least one known event picked"`
	Events []string `json:"events" jsonschema:"minItems=1" error:"name must be 1-80 characters, the url at most 2000, and at least one known event picked"`
}

type createdWebhookReply struct {
	webhookRow
	Secret string `json:"secret" jsonschema_description:"Shown in this response only."`
}

// CreateWebhook answers with the signing secret, the only time it is shown.
func ( handlers *Handlers ) CreateWebhook() *apidoc.Endpoint[ createWebhookRequest , createdWebhookReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Create a webhook" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Status: 201 , Errors: []int{ 400 , 403 , 404 } },
		func( c fiber.Ctx , in *createWebhookRequest ) ( reply *createdWebhookReply , err error ) {
			cfg := handlers.cfg()
			if models.ValidWebhook( in.Name , in.URL , in.Events ) == false {
				err = refuse( fiber.StatusBadRequest , models.ErrBadWebhook.Error() )
				return
			}
			if check_err := webhooks.CheckURL( in.URL , cfg.Webhooks.AllowPrivateTargets ); check_err != nil {
				err = refuse( fiber.StatusBadRequest , check_err.Error() )
				return
			}
			secret , hook , err := models.CreateWebhook( handlers.Store , cfg.SecretKey , in.Name , in.URL , in.Events , security.UserFrom( c ).ID )
			if err != nil { return }
			// The name only: the url can hold a token of its own.
			handlers.audit( c , logs.KindAdmin , "webhook created" , fiber.Map{ "webhook_id": hook.ID , "name": hook.Name } )
			reply = &createdWebhookReply{ webhookRow: newWebhookRow( hook ) , Secret: secret }
			return
		} )
}

type webhookEnabledRequest struct {
	webhookIDPath
	Enabled bool `json:"enabled"`
}

func ( handlers *Handlers ) SetWebhookEnabled() *apidoc.Endpoint[ webhookEnabledRequest , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Switch a webhook on or off" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Errors: []int{ 400 , 403 , 404 } },
		func( c fiber.Ctx , in *webhookEnabledRequest ) ( reply *okReply , err error ) {
			if models.SetWebhookEnabled( handlers.Store , in.WebhookID , in.Enabled ) != nil {
				err = refuse( fiber.StatusNotFound , "no such webhook" )
				return
			}
			handlers.audit( c , logs.KindAdmin , "webhook enabled set" , fiber.Map{ "webhook_id": in.WebhookID , "enabled": in.Enabled } )
			reply = &okReply{ Ok: true }
			return
		} )
}

type renameWebhookRequest struct {
	webhookIDPath
	Name   string  `json:"name" jsonschema:"minLength=1,maxLength=80" jsonschema_description:"1-80 characters." error:"name must be 1-80 characters"`
	Expect *string `json:"expect,omitempty" jsonschema_description:"The name you think it has. Answers 409 if it has another."`
}

type renamedWebhookReply struct {
	Ok   bool   `json:"ok"`
	Name string `json:"name"`
}

func ( handlers *Handlers ) RenameWebhook() *apidoc.Endpoint[ renameWebhookRequest , renamedWebhookReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Rename a webhook" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Errors: []int{ 400 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *renameWebhookRequest ) ( reply *renamedWebhookReply , err error ) {
			if models.ValidWebhookName( in.Name ) == false {
				err = refuse( fiber.StatusBadRequest , models.ErrBadWebhookName.Error() )
				return
			}
			rename_err := models.RenameWebhook( handlers.Store , in.WebhookID , in.Name , in.Expect )
			var conflict_err *models.RenameConflictError
			switch {
			case errors.As( rename_err , &conflict_err ):
				err = refuse( fiber.StatusConflict , conflict_err.Error() , fiber.Map{ "name": conflict_err.Current } )
				return
			case errors.Is( rename_err , db.ErrNotFound ):
				err = refuse( fiber.StatusNotFound , "no such webhook" )
				return
			case rename_err != nil:
				err = rename_err
				return
			}
			name := strings.TrimSpace( in.Name )
			handlers.audit( c , logs.KindAdmin , "webhook renamed" , fiber.Map{ "webhook_id": in.WebhookID , "name": name } )
			reply = &renamedWebhookReply{ Ok: true , Name: name }
			return
		} )
}

type webhookTargetRequest struct {
	webhookIDPath
	URL    string                `json:"url" jsonschema:"minLength=1,maxLength=2000" jsonschema_description:"Checked like a new webhook's." error:"the url must be at most 2000 characters, with at least one known event picked"`
	Events []string              `json:"events" jsonschema:"minItems=1" error:"the url must be at most 2000 characters, with at least one known event picked"`
	Expect *models.WebhookTarget `json:"expect,omitempty" jsonschema_description:"What you think it has now. Answers 409 if it has something else."`
}

// SetWebhookTarget changes the address and the events. The address gets the
// same check as a new endpoint's, every time: an events-only edit of one that
// has since been ruled out is refused too, and the sender would refuse it anyway.
func ( handlers *Handlers ) SetWebhookTarget() *apidoc.Endpoint[ webhookTargetRequest , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Change a webhook's address and events" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Errors: []int{ 400 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *webhookTargetRequest ) ( reply *okReply , err error ) {
			target := models.WebhookTarget{ URL: in.URL , Events: in.Events }
			if models.ValidWebhookTarget( target ) == false {
				err = refuse( fiber.StatusBadRequest , models.ErrBadWebhookTarget.Error() )
				return
			}
			if check_err := webhooks.CheckURL( in.URL , handlers.cfg().Webhooks.AllowPrivateTargets ); check_err != nil {
				err = refuse( fiber.StatusBadRequest , check_err.Error() )
				return
			}
			set_err := models.SetWebhookTarget( handlers.Store , in.WebhookID , target , in.Expect )
			switch {
			case errors.Is( set_err , models.ErrWebhookChanged ):
				err = refuse( fiber.StatusConflict , set_err.Error() )
				return
			case errors.Is( set_err , db.ErrNotFound ):
				err = refuse( fiber.StatusNotFound , "no such webhook" )
				return
			case set_err != nil:
				err = set_err
				return
			}
			// The events but not the url, which can hold a token of its own.
			handlers.audit( c , logs.KindAdmin , "webhook target changed" , fiber.Map{ "webhook_id": in.WebhookID , "events": in.Events } )
			reply = &okReply{ Ok: true }
			return
		} )
}

type webhookSecretReply struct {
	Secret string `json:"secret" jsonschema_description:"Shown only here. The old one has stopped working."`
}

// RotateWebhookSecret answers with the new signing secret, the only time it
// is shown.
func ( handlers *Handlers ) RotateWebhookSecret() *apidoc.Endpoint[ webhookIDPath , webhookSecretReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Replace a webhook's signing secret" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Errors: []int{ 400 , 403 , 404 } },
		func( c fiber.Ctx , in *webhookIDPath ) ( reply *webhookSecretReply , err error ) {
			secret , rotate_err := models.RotateWebhookSecret( handlers.Store , handlers.cfg().SecretKey , in.WebhookID )
			switch {
			case errors.Is( rotate_err , db.ErrNotFound ):
				err = refuse( fiber.StatusNotFound , "no such webhook" )
				return
			case rotate_err != nil:
				err = rotate_err
				return
			}
			handlers.audit( c , logs.KindAdmin , "webhook secret rotated" , fiber.Map{ "webhook_id": in.WebhookID } )
			reply = &webhookSecretReply{ Secret: secret }
			return
		} )
}

func ( handlers *Handlers ) DeleteWebhook() *apidoc.Endpoint[ webhookIDPath , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Delete a webhook" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Errors: []int{ 400 , 403 , 404 } },
		func( c fiber.Ctx , in *webhookIDPath ) ( reply *okReply , err error ) {
			if models.DeleteWebhook( handlers.Store , in.WebhookID , security.UserFrom( c ).ID ) != nil {
				err = refuse( fiber.StatusNotFound , "no such webhook" )
				return
			}
			handlers.audit( c , logs.KindAdmin , "webhook deleted" , fiber.Map{ "webhook_id": in.WebhookID } )
			reply = &okReply{ Ok: true }
			return
		} )
}

func ( handlers *Handlers ) RestoreWebhook() *apidoc.Endpoint[ webhookIDPath , webhookRow ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Undo deleting a webhook" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Errors: []int{ 400 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *webhookIDPath ) ( reply *webhookRow , err error ) {
			actor := security.UserFrom( c )
			hook , restore_err := models.RestoreWebhook( handlers.Store , in.WebhookID , actor.ID , actor.IsAdmin() )
			if restore_err != nil {
				err = restoreRefusal( restore_err )
				return
			}
			handlers.audit( c , logs.KindAdmin , "webhook restored" , fiber.Map{ "webhook_id": in.WebhookID } )
			row := newWebhookRow( hook )
			reply = &row
			return
		} )
}

// liveWebhook loads an endpoint the screen can act on: "no such webhook" for
// one that is gone.
func ( handlers *Handlers ) liveWebhook( webhook_id string ) ( hook *models.Webhook , err error ) {
	hook , err = models.GetWebhook( handlers.Store , webhook_id )
	if err == nil && hook.Deleted() {
		hook , err = nil , db.ErrNotFound
	}
	if errors.Is( err , db.ErrNotFound ) { err = refuse( fiber.StatusNotFound , "no such webhook" ) }
	return
}

// TestWebhook queues a webhook.test event. For an endpoint that is switched
// off, the delivery says so rather than the event going out.
func ( handlers *Handlers ) TestWebhook() *apidoc.Endpoint[ webhookIDPath , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Send a test event" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Status: 202 , Errors: []int{ 400 , 403 , 404 } },
		func( c fiber.Ctx , in *webhookIDPath ) ( reply *okReply , err error ) {
			hook , err := handlers.liveWebhook( in.WebhookID )
			if err != nil { return }
			if err = handlers.Webhooks.SendTest( hook ); err != nil { return }
			reply = &okReply{ Ok: true }
			return
		} )
}

type webhookDeliveryPath struct {
	webhookIDPath
	DeliveryID string `json:"delivery_id" path:"delivery_id" jsonschema_description:"The delivery's id, from GET /api/owner/webhooks/{webhook_id}/deliveries."`
}

// ResendWebhookDelivery queues a delivery that was given up on.
func ( handlers *Handlers ) ResendWebhookDelivery() *apidoc.Endpoint[ webhookDeliveryPath , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Send a failed delivery again" , Description: "A new delivery with the same body. The failed one stays listed." , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Status: 202 , Errors: []int{ 400 , 401 , 403 , 404 , 409 } },
		func( c fiber.Ctx , in *webhookDeliveryPath ) ( reply *okReply , err error ) {
			hook , err := handlers.liveWebhook( in.WebhookID )
			if err != nil { return }
			send_err := handlers.Webhooks.Resend( hook.ID , in.DeliveryID )
			switch {
			case errors.Is( send_err , db.ErrNotFound ):
				err = refuse( fiber.StatusNotFound , "no such delivery" )
			case errors.Is( send_err , models.ErrDeliveryNotFailed ):
				err = refuse( fiber.StatusConflict , "delivery_not_failed" )
			case send_err != nil:
				err = send_err
			default:
				handlers.audit( c , logs.KindAdmin , "webhook delivery resent" , fiber.Map{ "webhook_id": hook.ID , "delivery_id": in.DeliveryID } )
				reply = &okReply{ Ok: true }
			}
			return
		} )
}

type listDeliveriesRequest struct {
	webhookIDPath
	Page
}

type deliveriesReply struct {
	Deliveries []deliveryRow `json:"deliveries"`
	Paged
}

func ( handlers *Handlers ) ListWebhookDeliveries() *apidoc.Endpoint[ listDeliveriesRequest , deliveriesReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Recent deliveries" , Tag: "Webhooks" , Access: apidoc.SessionOnly , Role: apidoc.Owner , Feature: "webhooks" , Errors: []int{ 400 , 404 } },
		func( c fiber.Ctx , in *listDeliveriesRequest ) ( reply *deliveriesReply , err error ) {
			hook , err := handlers.liveWebhook( in.WebhookID )
			if err != nil { return }
			deliveries , err := models.ListWebhookDeliveries( handlers.Store , hook.ID )
			if err != nil { return }
			rows := []deliveryRow{}
			for _ , delivery := range deliveries {
				rows = append( rows , newDeliveryRow( delivery ) )
			}
			reply = &deliveriesReply{}
			reply.Deliveries , reply.Paged = cutPage( rows , in.Page )
			return
		} )
}
