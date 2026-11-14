// Package routes contains one file per group of endpoints, plus this file,
// which is the only place that knows the full URL map.
//
// The API here stays deliberately small: it covers logging in, managing who
// can log in, issuing the API keys that let non-browser callers in, and
// reading the UI's text. Everything else an app does belongs in a new file in
// this package (server side) and in static/js/ (browser side) -- see
// references/architecture.md for where the line sits and why.
package routes

import (
	json "encoding/json" // feature:logs|settings
	errors "errors"

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	config "{{MODULE_PATH}}/server/config"
	db "{{MODULE_PATH}}/server/db"
	errorpage "{{MODULE_PATH}}/server/errorpage" // feature:maintenance
	jobs "{{MODULE_PATH}}/server/jobs" // feature:jobs|metrics
	language "{{MODULE_PATH}}/server/language"
	logs "{{MODULE_PATH}}/server/logs" // feature:logs
	metrics "{{MODULE_PATH}}/server/metrics" // feature:metrics
	mcp "{{MODULE_PATH}}/server/mcp" // feature:mcp
	models "{{MODULE_PATH}}/server/models" // feature:invite_links|two_factor|passwords|files|api_keys|webhooks
	security "{{MODULE_PATH}}/server/security"
	static "{{MODULE_PATH}}/server/static"
	webhooks "{{MODULE_PATH}}/server/webhooks" // feature:webhooks
	stream "{{MODULE_PATH}}/server/stream" // feature:logs
)

// Handlers carries the dependencies every route group needs. Passing this in
// rather than reaching for package-level globals is what makes the handlers
// testable and keeps the wiring visible in main.go.
type Handlers struct {
	Store *db.Store
	// Settings is the running configuration: read it through cfg(), once per
	// request, since config.yaml can change while the server runs.
	Settings *config.Holder
	Guard    *security.Guard
	Language *language.Language
	Static   *static.Server
	Pins     *models.AttemptGuard[ uint64 ] // feature:invite_links
	// >>> feature:two_factor
	// Codes counts wrong two-factor codes per account, wherever they are
	// typed. Six digits with a window of three steps would otherwise fall to
	// a patient guesser.
	Codes *models.AttemptGuard[ uint64 ]
	// <<< feature:two_factor
	// >>> feature:passwords
	// Per name typed, not per account, so a lockout does not say which names
	// are real; and per address, across names.
	PasswordNames     *models.AttemptGuard[ string ]
	PasswordAddresses *models.AttemptGuard[ string ]
	// <<< feature:passwords
	// Version is reported to MCP clients as serverInfo.version.
	Version string
	// >>> feature:logs
	// Logs is the activity log (server/logs), set by main.go after New. Nil
	// or switched off, nothing is logged and its routes are not registered.
	Logs *logs.Logger
	// <<< feature:logs
	// >>> feature:jobs|metrics
	// Jobs is the background job registry (server/jobs), set by main.go.
	Jobs *jobs.Registry
	// <<< feature:jobs|metrics
	// >>> feature:webhooks
	// Webhooks is how a route says something happened (server/webhooks), set
	// by main.go. Nil is fine: nothing is sent.
	Webhooks *webhooks.Dispatcher
	// <<< feature:webhooks
	// >>> feature:metrics
	// Metrics counts requests for /metrics, set by main.go.
	Metrics *metrics.Recorder
	// <<< feature:metrics
	// >>> feature:logs
	// LogStream is who has the Logs screen open with Live on, set by main.go.
	LogStream *stream.Hub
	// <<< feature:logs
	// >>> feature:maintenance
	// ErrorPages is what a browser gets while the app is in maintenance mode,
	// set by main.go. Nil is fine: everyone gets the JSON answer.
	ErrorPages *errorpage.Pages
	// <<< feature:maintenance
	// >>> feature:openapi
	// OpenAPI is config/openapi.json, set by main.go.
	OpenAPI []byte
	// <<< feature:openapi
	// >>> feature:mcp
	// MCPManifest is config/mcp.json, set by main.go.
	MCPManifest []byte
	// MCP is the /mcp server, with every tool registered, set by Register.
	MCP *mcp.Server
	// <<< feature:mcp

	uploads       models.UploadLocks // feature:files
	uiEventBudget eventBudget // feature:logs
	readiness     readiness
}

func New( store *db.Store , settings *config.Holder , guard *security.Guard , lang *language.Language , static_server *static.Server , version string ) ( handlers *Handlers ) {
	handlers = &Handlers{
		Store:    store,
		Settings: settings,
		Guard:    guard,
		Language: lang,
		Static:   static_server,
		Pins:     models.NewAttemptGuard[ uint64 ]( models.AttemptMaxFailures ), // feature:invite_links
		Codes:    models.NewAttemptGuard[ uint64 ]( models.AttemptMaxFailures ), // feature:two_factor
		PasswordNames:     models.NewAttemptGuard[ string ]( models.AttemptMaxFailures ), // feature:passwords
		PasswordAddresses: models.NewAttemptGuard[ string ]( passwordAddressMaxFailures ), // feature:passwords
		Version:  version,
	}
	return
}

// Register wires every route. Read it top to bottom to see the whole surface
// of this server.
func ( handlers *Handlers ) Register( app *fiber.App ) {
	// Resolve the session once, before anything else looks at the request.
	app.Use( handlers.Guard.LoadUser )
	apidoc.CheckCSRF = handlers.Guard.CheckCSRF

	// >>> feature:maintenance
	// Next, because it needs to know who is asking and has to come before
	// every route below. See maintenance.go.
	app.Use( handlers.Maintenance )
	// <<< feature:maintenance

	// Public: the login link's page and the call it makes, and the UI's text
	// (needed to render the signed-out page at all). Opening a login link
	// only shows a page; the link is spent by the POST that page sends when
	// the person confirms. See auth.go.
	app.Get( "/login/*" , handlers.ShowLoginPage )
	apidoc.Post( app , "/api/login" , handlers.RedeemLoginLink , sameOrigin( loginElsewhere ) )
	apidoc.Get( app , "/api/login/options" , handlers.GetLoginOptions ) // feature:passwords
	apidoc.Post( app , "/api/login/two-factor" , handlers.FinishSignIn , sameOrigin( loginElsewhere ) ) // feature:two_factor
	apidoc.Post( app , "/api/login/password" , handlers.PasswordLogin , handlers.RequirePasswords , sameOrigin( loginElsewhere ) ) // feature:passwords
	apidoc.Get( app , "/api/language" , handlers.GetLanguage )
	apidoc.Get( app , "/api/health" , handlers.GetHealth )
	apidoc.Get( app , "/api/ready" , handlers.GetReady )

	// >>> feature:invite_links
	// Public: invite links (see join.go). Holding the link is the permission,
	// so these sit before the signed-in "/api" group below -- registered
	// after it, they would pick up RequireLogin from the shared prefix.
	app.Get( "/join/*" , handlers.ShowJoinPage )
	apidoc.Post( app , "/api/join/info" , handlers.JoinInfo , handlers.RequireInviteLinks , sameOrigin( joinElsewhere ) )
	apidoc.Post( app , "/api/join/match" , handlers.JoinMatch , handlers.RequireInviteLinks , sameOrigin( joinElsewhere ) )
	apidoc.Post( app , "/api/join/claim" , handlers.JoinClaim , handlers.RequireInviteLinks , sameOrigin( joinElsewhere ) )
	apidoc.Post( app , "/api/join/create" , handlers.JoinCreate , handlers.RequireInviteLinks , sameOrigin( joinElsewhere ) )
	// <<< feature:invite_links

	// Any signed-in user.
	account := app.Group( "/api" , handlers.Guard.RequireLogin )
	apidoc.Get( account , "/me" , handlers.GetMe )
	apidoc.Post( account , "/account/rename" , handlers.RenameAccount )
	apidoc.Post( account , "/account/shortcuts" , handlers.SetMyShortcuts ) // feature:shortcuts
	apidoc.Post( account , "/account/themes" , handlers.SetMyThemes ) // feature:appearance
	apidoc.Post( account , "/account/theme" , handlers.SetMyTheme ) // feature:appearance
	// >>> feature:invite_links
	// A session only: a PIN lets its holder sign in through an invite link,
	// so a key that could set one could turn itself into a session.
	apidoc.Post( account , "/account/pin" , handlers.SetPin , handlers.RequireInviteLinks , handlers.Guard.RequireSession )
	// <<< feature:invite_links
	// A key may make its owner a login link, but only a key carrying the
	// account's full role (see CreateMyLoginLink), and revoking the key voids
	// the link if it is still unused (see models.RevokeAPIKey).
	apidoc.Post( account , "/account/login-link" , handlers.CreateMyLoginLink )
	apidoc.Post( account , "/account/login-link/cancel" , handlers.CancelMyLoginLink )
	// Passwords and 2FA change how an account signs in, so setting them needs // feature:passwords|two_factor
	// a session, like the PIN: a key that could set a password could turn // feature:passwords|two_factor
	// itself into a session. // feature:passwords|two_factor
	apidoc.Post( account , "/account/password" , handlers.SetPassword , handlers.RequirePasswords , handlers.Guard.RequireSession ) // feature:passwords
	apidoc.Post( account , "/account/two-factor/start" , handlers.StartTwoFactor , handlers.RequireTwoFactor , handlers.Guard.RequireSession ) // feature:two_factor
	apidoc.Post( account , "/account/two-factor/confirm" , handlers.ConfirmTwoFactor , handlers.RequireTwoFactor , handlers.Guard.RequireSession ) // feature:two_factor
	apidoc.Post( account , "/account/two-factor/disable" , handlers.DisableTwoFactor , handlers.RequireTwoFactor , handlers.Guard.RequireSession ) // feature:two_factor
	// Your own signed-in devices. A key may list and end them too; through // feature:sessions
	// a key there is no current device, so revoke-others ends them all. // feature:sessions
	apidoc.Get( account , "/account/sessions" , handlers.ListMySessions ) // feature:sessions
	apidoc.Get( account , "/account/storage" , handlers.MyStorage , handlers.RequireFiles ) // feature:files
	apidoc.Post( account , "/account/sessions/revoke-others" , handlers.RevokeMyOtherSessions ) // feature:sessions
	apidoc.Post( account , "/account/sessions/:session_id/label" , handlers.LabelMySession ) // feature:sessions
	apidoc.Post( account , "/account/sessions/:session_id/revoke" , handlers.RevokeMySession ) // feature:sessions
	apidoc.Post( account , "/logout" , handlers.Logout )
	apidoc.Get( account , "/team" , handlers.ListTeam )
	apidoc.Get( account , "/openapi.json" , handlers.GetOpenAPI ) // feature:openapi
	apidoc.Get( account , "/mcp.json" , handlers.GetMCPManifest , handlers.RequireMCP ) // feature:mcp
	// >>> feature:logs
	// What someone did on the page, sent by track.js. Theirs to send, the
	// owner's to read (below).
	if handlers.Logs.Enabled() {
		apidoc.Post( account , "/logs/events" , handlers.RecordUIEvents )
	}
	// <<< feature:logs

	// >>> feature:api_keys
	// API keys: any signed-in user manages their own.
	//
	// Note the group prefix. A group's middleware is mounted on a path
	// *prefix*, not on the routes written under it, so it applies to every
	// route registered after it whose path starts with that prefix. Mounting
	// these two on "/api" would put RequireSession in front of /api/admin/users
	// as well, and an admin-scoped API key would then be refused there. That is
	// also why the prefix is exactly "/api/keys": a future "/api/keysets" would
	// pick up this middleware by sharing the string, so keep new routes off
	// these prefixes unless they want these rules.
	keys := app.Group( "/api/keys" , handlers.RequireAPIKeys , handlers.Guard.RequireLogin , handlers.Guard.RequireSession )
	apidoc.Get( keys , "" , handlers.ListMyAPIKeys )
	apidoc.Post( keys , "" , handlers.CreateMyAPIKey )
	apidoc.Post( keys , "/:key_id/rename" , handlers.RenameMyAPIKey )
	apidoc.Post( keys , "/:key_id/expiry" , handlers.SetMyAPIKeyExpiry )
	apidoc.Post( keys , "/:key_id/revoke" , handlers.RevokeMyAPIKey )
	apidoc.Post( keys , "/:key_id/delete" , handlers.DeleteMyAPIKey )
	apidoc.Post( keys , "/:key_id/restore" , handlers.RestoreMyAPIKey )
	// <<< feature:api_keys

	// >>> feature:files
	// Uploaded files: everyone their own, API keys included. See files.go.
	// Always registered, like /mcp: files.enabled can change while the server
	// runs, so RequireFiles checks it per request.
	files := app.Group( "/api/files" , handlers.RequireFiles , handlers.Guard.RequireLogin )
	apidoc.Get( files , "" , handlers.ListMyFiles )
	apidoc.Post( files , "" , handlers.CreateFile )
	apidoc.Get( files , "/:file_id" , handlers.GetMyFile )
	apidoc.Post( files , "/:file_id/chunks/:index" , handlers.PutFileChunk )
	apidoc.Post( files , "/:file_id/finish" , handlers.FinishFile )
	apidoc.Get( files , "/:file_id/download" , handlers.DownloadMyFile )
	apidoc.Post( files , "/:file_id/rename" , handlers.RenameMyFile )
	apidoc.Post( files , "/:file_id/delete" , handlers.DeleteMyFile )
	apidoc.Post( files , "/:file_id/restore" , handlers.RestoreMyFile )
	// <<< feature:files

	// Admin only: deciding who gets in. Note this is the *only* thing admin
	// rights gate in the base template -- see architecture.md on defaulting
	// new features to "any signed-in user".
	//
	// An admin-scoped key may do all of it, creating accounts and handing out
	// login links included. Every link records the key that made it, and
	// revoking or deleting that key voids the links it made that can still be
	// used (models.RevokeAPIKey), so revoking a leaked key still ends what it
	// could hand out. Who may delete or demote whom (the owner rules) is
	// enforced in the models.
	admin := app.Group( "/api/admin" , handlers.Guard.RequireAdmin )
	apidoc.Get( admin , "/users" , handlers.ListUsers )
	apidoc.Post( admin , "/users" , handlers.CreateUser )
	apidoc.Post( admin , "/users/:user_id/reissue-login" , handlers.ReissueLogin )
	apidoc.Post( admin , "/users/:user_id/cancel-login-link" , handlers.CancelLoginLink )
	apidoc.Post( admin , "/users/:user_id/rename" , handlers.RenameUser )
	apidoc.Post( admin , "/users/:user_id/role" , handlers.SetUserRole )
	apidoc.Post( admin , "/users/:user_id/disabled" , handlers.SetUserDisabled )
	apidoc.Post( admin , "/users/:user_id/revoke-access" , handlers.RevokeAccess )
	apidoc.Post( admin , "/users/:user_id/delete" , handlers.DeleteUser )
	apidoc.Post( admin , "/users/:user_id/reset-two-factor" , handlers.ResetTwoFactor ) // feature:two_factor
	// An account's devices, like your own list above. // feature:sessions
	apidoc.Get( admin , "/users/:user_id/sessions" , handlers.ListUserSessions ) // feature:sessions
	apidoc.Post( admin , "/users/:user_id/sessions/:session_id/revoke" , handlers.RevokeUserSession ) // feature:sessions
	apidoc.Get( admin , "/storage" , handlers.AllStorage , handlers.RequireFiles ) // feature:files
	// The themes everyone starts from. Not a credential, so a key may. // feature:appearance
	apidoc.Get( admin , "/theme-defaults" , handlers.GetThemeDefaults ) // feature:appearance
	apidoc.Post( admin , "/theme-defaults" , handlers.SetThemeDefaults ) // feature:appearance

	// >>> feature:invite_links
	// Invite links: one link for a group, each person who opens it joins as a
	// new user (see invite_links.go). An admin key may manage them; like a
	// login link, one a key made is switched off when that key is revoked.
	apidoc.Get( admin , "/invite-links" , handlers.ListInviteLinks , handlers.RequireInviteLinks )
	apidoc.Post( admin , "/invite-links" , handlers.CreateInviteLink , handlers.RequireInviteLinks )
	apidoc.Post( admin , "/invite-links/:link_id/rename" , handlers.RenameInviteLink , handlers.RequireInviteLinks )
	apidoc.Post( admin , "/invite-links/:link_id/limits" , handlers.SetInviteLinkLimits , handlers.RequireInviteLinks )
	apidoc.Post( admin , "/invite-links/:link_id/switch-on" , handlers.SwitchOnInviteLink , handlers.RequireInviteLinks )
	apidoc.Post( admin , "/invite-links/:link_id/revoke" , handlers.RevokeInviteLink , handlers.RequireInviteLinks )
	apidoc.Post( admin , "/invite-links/:link_id/delete" , handlers.DeleteInviteLink , handlers.RequireInviteLinks )
	apidoc.Post( admin , "/invite-links/:link_id/restore" , handlers.RestoreInviteLink , handlers.RequireInviteLinks )
	// <<< feature:invite_links

	// >>> feature:api_keys
	// Admin view of every key in the app -- "who has standing access" -- and
	// the ability to revoke or delete someone else's. Its own prefix for the
	// same reason as above: mounted on "/api/admin" it would gate the account
	// routes too.
	admin_keys := app.Group( "/api/admin/keys" , handlers.RequireAPIKeys , handlers.Guard.RequireAdmin , handlers.Guard.RequireSession )
	apidoc.Get( admin_keys , "" , handlers.ListAllAPIKeys )
	apidoc.Post( admin_keys , "" , handlers.CreateAPIKeyForUser )
	apidoc.Post( admin_keys , "/:key_id/rename" , handlers.RenameAnyAPIKey )
	apidoc.Post( admin_keys , "/:key_id/expiry" , handlers.SetAnyAPIKeyExpiry )
	apidoc.Post( admin_keys , "/:key_id/revoke" , handlers.RevokeAnyAPIKey )
	apidoc.Post( admin_keys , "/:key_id/delete" , handlers.DeleteAnyAPIKey )
	apidoc.Post( admin_keys , "/:key_id/restore" , handlers.RestoreAnyAPIKey )
	// <<< feature:api_keys

	// >>> feature:files
	// Every account's files. An admin key may list them, but reading or
	// deleting someone else's needs a session: a leaked key must not be a way
	// to carry off the contents of every account.
	admin_files := app.Group( "/api/admin/files" , handlers.RequireFiles , handlers.Guard.RequireAdmin )
	apidoc.Get( admin_files , "" , handlers.ListAllFiles )
	apidoc.Get( admin_files , "/:file_id/download" , handlers.DownloadAnyFile , handlers.Guard.RequireSession )
	apidoc.Post( admin_files , "/:file_id/rename" , handlers.RenameAnyFile , handlers.Guard.RequireSession )
	apidoc.Post( admin_files , "/:file_id/delete" , handlers.DeleteAnyFile , handlers.Guard.RequireSession )
	apidoc.Post( admin_files , "/:file_id/restore" , handlers.RestoreAnyFile , handlers.Guard.RequireSession )
	// <<< feature:files

	// >>> feature:settings
	// config.yaml, as the owner's Settings screen: the owner only, from a
	// browser session, like the activity log below. See settings.go.
	owner_settings := app.Group( "/api/owner/settings" , handlers.Guard.RequireOwner )
	apidoc.Get( owner_settings , "" , handlers.GetSettings )
	apidoc.Post( owner_settings , "" , handlers.SaveSettings )
	// <<< feature:settings

	// >>> feature:passwords
	// The per-account password switch. One exact path, so nothing else picks
	// up these guards. See passwords.go.
	apidoc.Post( app , "/api/owner/users/:user_id/password-access" , handlers.SetPasswordAccess , handlers.RequirePasswords , handlers.Guard.RequireOwner )
	// <<< feature:passwords

	// >>> feature:logs
	// The activity log: the owner only, from a browser session -- not other
	// admins, not any API key (see Guard.RequireOwner). Its own prefix, so
	// nothing else picks up RequireOwner by sharing the string.
	if handlers.Logs.Enabled() {
		owner_logs := app.Group( "/api/owner/logs" , handlers.Guard.RequireOwner )
		apidoc.Get( owner_logs , "/stream" , handlers.StreamLogs )
		apidoc.Get( owner_logs , "/days" , handlers.ListLogDays )
		apidoc.Get( owner_logs , "/days/:date" , handlers.GetLogDay )
		apidoc.Get( owner_logs , "/days/:date/download" , handlers.DownloadLogDay )
		apidoc.Post( owner_logs , "/days/:date/delete" , handlers.DeleteLogDay )
		apidoc.Post( owner_logs , "/prune" , handlers.PruneLogs )
		apidoc.Get( owner_logs , "/ip-names" , handlers.ListIPNames )
		apidoc.Post( owner_logs , "/ip-names" , handlers.SetIPName )
		apidoc.Post( owner_logs , "/ip-names/delete" , handlers.DeleteIPName )
	}
	// <<< feature:logs

	// >>> feature:jobs
	// Background jobs: the owner only, from a browser session, like the log.
	owner_jobs := app.Group( "/api/owner/jobs" , handlers.Guard.RequireOwner )
	apidoc.Get( owner_jobs , "" , handlers.ListJobs )
	apidoc.Post( owner_jobs , "/:name/run" , handlers.RunJob )
	// <<< feature:jobs

	// Outbound webhooks: the owner only, from a browser session, like the log. // feature:webhooks
	owner_webhooks := app.Group( "/api/owner/webhooks" , handlers.RequireWebhooks , handlers.Guard.RequireOwner ) // feature:webhooks
	apidoc.Get( owner_webhooks , "" , handlers.ListWebhooks ) // feature:webhooks
	apidoc.Post( owner_webhooks , "" , handlers.CreateWebhook ) // feature:webhooks
	apidoc.Get( owner_webhooks , "/events" , handlers.ListWebhookEvents ) // feature:webhooks
	apidoc.Post( owner_webhooks , "/:webhook_id/enabled" , handlers.SetWebhookEnabled ) // feature:webhooks
	apidoc.Post( owner_webhooks , "/:webhook_id/rename" , handlers.RenameWebhook ) // feature:webhooks
	apidoc.Post( owner_webhooks , "/:webhook_id/target" , handlers.SetWebhookTarget ) // feature:webhooks
	apidoc.Post( owner_webhooks , "/:webhook_id/rotate-secret" , handlers.RotateWebhookSecret ) // feature:webhooks
	apidoc.Post( owner_webhooks , "/:webhook_id/delete" , handlers.DeleteWebhook ) // feature:webhooks
	apidoc.Post( owner_webhooks , "/:webhook_id/restore" , handlers.RestoreWebhook ) // feature:webhooks
	apidoc.Post( owner_webhooks , "/:webhook_id/test" , handlers.TestWebhook ) // feature:webhooks
	apidoc.Get( owner_webhooks , "/:webhook_id/deliveries" , handlers.ListWebhookDeliveries ) // feature:webhooks
	apidoc.Post( owner_webhooks , "/:webhook_id/deliveries/:delivery_id/resend" , handlers.ResendWebhookDelivery ) // feature:webhooks

	// >>> feature:mcp
	// MCP: agents calling the app with an API key (see server/mcp and
	// mcp_tools.go). Keys only -- RequireAPIKey turns a browser session away
	// -- and every tool runs at the key's effective role, so this is the API
	// above in another shape, not a wider one. One exact path, not a group:
	// a group on "/mcp" would also catch any later "/mcp..." route.
	//
	// Always registered: server.mcp and auth.api_keys can be switched while
	// the server runs, so RequireMCP checks them per request and answers 404
	// when either is off -- exactly what an unregistered route would say.
	mcp_server := mcp.New( handlers.Settings.Boot().ProjectName , handlers.Version )
	handlers.registerMCPTools( mcp_server )
	handlers.MCP = mcp_server
	apidoc.Post( app , "/mcp" , handlers.ServeMCP , handlers.RequireMCP , handlers.Guard.RequireAPIKey )
	apidoc.Get( app , "/mcp" , handlers.MCPStream , handlers.RequireMCP )
	apidoc.Delete( app , "/mcp" , handlers.MCPSession , handlers.RequireMCP )
	// <<< feature:mcp

	// >>> feature:metrics
	// Prometheus. One exact path outside /api, so no group's guards reach it,
	// and always registered so metrics.enabled can change while the server
	// runs. An admin key only: the numbers say how the server is doing, and a
	// browser session has no business scraping.
	apidoc.Get( app , "/metrics" , handlers.GetMetrics , handlers.RequireMetrics , handlers.Guard.RequireAPIKey , handlers.Guard.RequireAdmin )
	// <<< feature:metrics

	// Any other /api/ path is a JSON 404, not a static page.
	app.Get( "/api/*" , func( c fiber.Ctx ) ( err error ) {
		err = c.Status( fiber.StatusNotFound ).JSON( fiber.Map{ "error": "not found" } )
		return
	} )

	// Catch-all static serving. Must be registered last -- it matches every
	// remaining path, so anything added after this line never runs.
	app.Get( "/*" , handlers.Static.Handler )
}

func ( handlers *Handlers ) cfg() ( cfg *config.Config ) {
	cfg = handlers.Settings.Get()
	return
}

// >>> feature:mcp
// RequireMCP makes /mcp answer 404 while server.mcp or auth.api_keys is off.
func ( handlers *Handlers ) RequireMCP( c fiber.Ctx ) ( err error ) {
	cfg := handlers.cfg()
	if cfg.MCPEnabled == false || cfg.APIKeysEnabled == false {
		err = notFound( c , "not found" )
		return
	}
	err = c.Next()
	return
}
// <<< feature:mcp

// badRequest / notFound keep error bodies uniform, so the frontend has one
// shape to handle and no handler accidentally leaks internal detail.
func badRequest( c fiber.Ctx , message string ) ( err error ) {
	err = c.Status( fiber.StatusBadRequest ).JSON( fiber.Map{ "error": message } )
	return
}

func notFound( c fiber.Ctx , message string ) ( err error ) {
	err = c.Status( fiber.StatusNotFound ).JSON( fiber.Map{ "error": message } )
	return
}

func forbidden( c fiber.Ctx , message string ) ( err error ) {
	err = c.Status( fiber.StatusForbidden ).JSON( fiber.Map{ "error": message } )
	return
}

// conflict is 409: the request was fine but the record has moved on -- an
// undo whose window has passed, or a rename that expected a name someone else
// has since changed. extra carries the current state, so the caller can show
// it.
func conflict( c fiber.Ctx , message string , extra fiber.Map ) ( err error ) {
	body := fiber.Map{ "error": message }
	for name , value := range extra {
		body[ name ] = value
	}
	err = c.Status( fiber.StatusConflict ).JSON( body )
	return
}

// >>> feature:api_keys|invite_links|files|webhooks
// restoreRefusal answers each way a restore can be refused.
func restoreRefusal( cause error ) ( refused *refusal ) {
	switch {
	case errors.Is( cause , models.ErrRestoreGone ):
		refused = refuse( fiber.StatusNotFound , cause.Error() )
	case errors.Is( cause , models.ErrRestoreForbidden ):
		refused = refuse( fiber.StatusForbidden , cause.Error() )
	case errors.Is( cause , models.ErrRestoreExpired ):
		refused = refuse( fiber.StatusConflict , cause.Error() )
	case errors.Is( cause , models.ErrAPIKeyLimit ): // feature:api_keys
		refused = refuse( fiber.StatusConflict , "its owner already has the maximum number of live API keys" ) // feature:api_keys
	case errors.Is( cause , models.ErrOwnerProtected ):
		refused = refuse( fiber.StatusForbidden , "nobody can restore the owner account's keys" )
	case errors.Is( cause , models.ErrOwnerRequired ):
		refused = refuse( fiber.StatusForbidden , "only the owner can restore an admin account's keys" )
	default:
		refused = internalFailure( cause )
	}
	return
}
// <<< feature:api_keys|invite_links|files|webhooks

// serverError ends a request that failed on our side. It writes nothing:
// handing cause back up is what gets it to the log, and to the ErrorHandler
// (server/middleware/errors.go), which answers with the generic message and a
// reference the person can quote. The cause itself never reaches them.
func serverError( _ fiber.Ctx , cause error ) ( err error ) {
	err = cause
	if err == nil {
		err = errors.New( "internal error" )
	}
	return
}

// >>> feature:logs|settings
// jsonObjects is rows as the JSON objects they encode to, for a reply that
// passes on what another package keeps without saying its shape.
func jsonObjects[ T any ]( rows []T ) ( objects []map[string]any , err error ) {
	objects = []map[string]any{}
	encoded , err := json.Marshal( rows )
	if err != nil { return }
	err = json.Unmarshal( encoded , &objects )
	if objects == nil { objects = []map[string]any{} }
	return
}
// <<< feature:logs|settings
