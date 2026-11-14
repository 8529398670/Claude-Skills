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

// Handlers carries the dependencies every route needs. Passing this in
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
	// or switched off, nothing is logged and its routes answer 404
	// (RequireLogs).
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

//go:generate go run ../apidoc/gen

// Register wires every route. The API routes are declared beside their
// handlers, each with its // @route(...) declaration, and mountDeclared
// (routes_generated.go, written by go generate) mounts them, the most
// specific path first, each behind the guards its declaration says:
//
//   - Anyone signed in, by a browser session or an API key, is the default.
//     role=Admin is the admins, and role=Owner the owner from a browser
//     session; everyone else gets the 404 an unknown route gets.
//   - access=SessionOnly turns an API key away. It guards what a key must
//     not reach: minting and managing API keys, your own password, PIN and
//     two-factor, other people's files. A key that could do those could
//     turn itself into a session or carry off every account's files.
//   - An admin-scoped key may manage accounts and hand out login and invite
//     links. Every link records the key that made it, and revoking or
//     deleting that key voids the links it made that can still be used
//     (models.RevokeAPIKey), so revoking a leaked key still ends what it
//     could hand out. Who may delete or demote whom is in the models.
//   - access=Public needs no sign-in: logging in, the UI's text, health.
//   - A feature that can be switched off while the server runs has a switch
//     in featureGuards: its routes answer 404 while it is off, before
//     anything else is asked.
//
// Read routes_generated.go to see the whole surface of this server. Only the
// pages a link opens and the catch-alls are written out here.
func ( handlers *Handlers ) Register( app *fiber.App ) {
	// Resolve the session once, before anything else looks at the request.
	app.Use( handlers.Guard.LoadUser )
	apidoc.CheckCSRF = handlers.Guard.CheckCSRF

	// >>> feature:maintenance
	// Next, because it needs to know who is asking and has to come before
	// every route below. See maintenance.go.
	app.Use( handlers.Maintenance )
	// <<< feature:maintenance

	// The pages a login link and an invite link open. Opening one only shows
	// a page; the link is spent by the POST that page sends when the person
	// confirms. See auth.go and join.go.
	app.Get( "/login/*" , handlers.ShowLoginPage )
	app.Get( "/join/*" , handlers.ShowJoinPage ) // feature:invite_links

	// >>> feature:mcp
	// MCP: agents calling the app with an API key (see server/mcp and
	// mcp_tools.go). Every tool runs at the key's effective role, so this is
	// the API in another shape, not a wider one.
	mcp_server := mcp.New( handlers.Settings.Boot().ProjectName , handlers.Version )
	handlers.registerMCPTools( mcp_server )
	handlers.MCP = mcp_server
	// <<< feature:mcp

	handlers.mountDeclared( app )

	// Any other /api/ path is a JSON 404, not a static page.
	app.Get( "/api/*" , func( c fiber.Ctx ) ( err error ) {
		err = c.Status( fiber.StatusNotFound ).JSON( fiber.Map{ "error": "not found" } )
		return
	} )

	// Catch-all static serving. Must be registered last -- it matches every
	// remaining path, so anything added after this line never runs.
	app.Get( "/*" , handlers.Static.Handler )
}

// featureGuards are the switches of the features that can be off while the
// server runs, by feature name: a route whose op says feature=x answers 404
// while x is off, exactly what an unregistered route would say. The route
// table puts the switch in front of every other guard. apidoc/gen reads
// this map's keys.
func ( handlers *Handlers ) featureGuards() ( guards map[string]fiber.Handler ) {
	guards = map[string]fiber.Handler{
		"invite_links": handlers.RequireInviteLinks, // feature:invite_links
		"passwords":    handlers.RequirePasswords, // feature:passwords
		"two_factor":   handlers.RequireTwoFactor, // feature:two_factor
		"files":        handlers.RequireFiles, // feature:files
		"webhooks":     handlers.RequireWebhooks, // feature:webhooks
		"api_keys":     handlers.RequireAPIKeys, // feature:api_keys
		"mcp":          handlers.RequireMCP, // feature:mcp
		"metrics":      handlers.RequireMetrics, // feature:metrics
		"logs":         handlers.RequireLogs, // feature:logs
	}
	return
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

// notFound keeps an error body the shape every refusal has, so the frontend
// has one shape to handle and no guard accidentally leaks internal detail.
func notFound( c fiber.Ctx , message string ) ( err error ) {
	err = c.Status( fiber.StatusNotFound ).JSON( fiber.Map{ "error": message } )
	return
}

func forbidden( c fiber.Ctx , message string ) ( err error ) {
	err = c.Status( fiber.StatusForbidden ).JSON( fiber.Map{ "error": message } )
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
