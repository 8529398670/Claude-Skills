// whole-file feature:logs
package routes

import (
	bufio "bufio"
	json "encoding/json"
	errors "errors"
	strings "strings"
	sync "sync"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
	stream "{{MODULE_PATH}}/server/stream"
)

// The activity log's endpoints (see server/logs). Reading it is the owner's
// alone, under /api/owner/logs (Guard.RequireOwner); writing to it from the
// page -- what track.js sends -- is any signed-in user's, at
// /api/logs/events, since what they do is what gets logged.

// RequireLogs makes the log's routes answer 404 while the log is off
// (logs.enabled, or no log at all). Per request, like RequireWebhooks.
func ( handlers *Handlers ) RequireLogs( c fiber.Ctx ) ( err error ) {
	if handlers.Logs.Enabled() == false {
		err = notFound( c , "not found" )
		return
	}
	err = c.Next()
	return
}

const (
	// Page events a browser may send per call, and per session per minute.
	logEventsPerCall   = 50
	logEventsPerMinute = 600
	logDetailMaxBytes  = 2048
)

// The page actions track.js sends. Anything else is refused, so the log
// cannot be filled with made-up kinds.
var logUIActions = map[string]bool{
	"session_start": true , "nav": true , "click": true , "submit": true ,
	"undo": true , "redo": true , "error": true , "fetch_fail": true ,
}

func ( handlers *Handlers ) logSettings() ( settings fiber.Map ) {
	current := handlers.Logs.Settings()
	settings = fiber.Map{
		"enabled":             current.Enabled,
		"retention_days":      current.RetentionDays,
		"max_total_mb":        current.MaxTotalBytes / 1024 / 1024,
		"max_file_mb":         current.MaxFileBytes / 1024 / 1024,
		"compress_after_days": current.CompressAfterDays,
		"static_assets":       current.StaticAssets,
		"ui_clicks":           current.UIClicks,
		"encrypted":           handlers.Logs.Encrypted(),
	}
	return
}

// logQuery is a search that knows what the owner calls each address, which
// reading a day, searching and downloading all need.
func ( handlers *Handlers ) logQuery( text string ) ( query logs.Query , err error ) {
	namer , err := models.LoadIPNamer( handlers.Store )
	if err != nil { return }
	query = logs.ParseQuery( text ).WithNames( namer.Name )
	return
}

// logSearch is the Logs screen's search, which the routes that read the log
// take.
type logSearch struct {
	Q string `json:"q,omitempty" query:"q" jsonschema_description:"The Logs screen's search."`
}

type logDaysReply struct {
	Days       []map[string]any `json:"days"`
	TotalBytes int64            `json:"total_bytes"`
	Dropped    int64            `json:"dropped"`
	Settings   map[string]any   `json:"settings"`
}

func ( logDaysReply ) APIName() ( name string ) { name = "LogDays"; return }

// @route("GET /api/owner/logs/days", summary="The days on record", tag="Owner", role=OWNER, feature="logs")
func ( handlers *Handlers ) listLogDays( c fiber.Ctx , in *logSearch ) ( reply *logDaysReply , err error ) {
	query , err := handlers.logQuery( in.Q )
	if err != nil { return }
	days , total , err := handlers.Logs.Days( query )
	if err != nil { return }
	reply = &logDaysReply{ TotalBytes: total , Dropped: handlers.Logs.Dropped() , Settings: handlers.logSettings() }
	reply.Days , err = jsonObjects( days )
	return
}

// logDayPath is the day a route under .../days/{date} reads.
type logDayPath struct {
	Date string `json:"date" path:"date" jsonschema_description:"The day's date, from GET /api/owner/logs/days."`
}

type logDayRequest struct {
	logDayPath
	logSearch
	Before *int `json:"before,omitempty" query:"before" jsonschema:"minimum=0" error:"before must be a line number"`
	Limit  int  `json:"limit,omitempty" query:"limit" jsonschema:"minimum=1,maximum=1000,default=200" jsonschema_description:"Rows to return. Default 200." error:"limit must be 1-1000"`
}

type logDayReply struct {
	Date    string           `json:"date"`
	Entries []map[string]any `json:"entries"`
	Next    *int             `json:"next"`
}

func ( logDayReply ) APIName() ( name string ) { name = "LogDay"; return }

// @route("GET /api/owner/logs/days/{date}", summary="One day's entries",
//     tag="Owner", role=OWNER, feature="logs",
// )
func ( handlers *Handlers ) getLogDay( c fiber.Ctx , in *logDayRequest ) ( reply *logDayReply , err error ) {
	if logs.ValidDay( in.Date ) == false {
		err = refuse( fiber.StatusBadRequest , "bad_day" )
		return
	}
	before , limit := -1 , pageDefault
	if in.Before != nil { before = *in.Before }
	if in.Limit != 0 { limit = in.Limit }
	query , err := handlers.logQuery( in.Q )
	if err != nil { return }
	records , next , err := handlers.Logs.ReadDay( in.Date , query , before , limit )
	if err != nil { return }
	reply = &logDayReply{ Date: in.Date , Next: next }
	reply.Entries , err = jsonObjects( records )
	return
}

// downloadLogDay streams a day as plain JSON lines, decrypted: the file a
// person can grep, or hand to whatever reads logs. Each line has ip_name beside
// the address, as named when this was asked for.
//
// @route("GET /api/owner/logs/days/{date}/download", summary="One day's entries as a file",
//     tag="Owner", role=OWNER, feature="logs", reply_type="application/x-ndjson",
// )
func ( handlers *Handlers ) downloadLogDay( c fiber.Ctx , in *logDayPath ) ( err error ) {
	day := in.Date
	if logs.ValidDay( day ) == false {
		err = refuse( fiber.StatusBadRequest , "bad_day" )
		return
	}
	query , err := handlers.logQuery( "" )
	if err != nil { return }
	c.Set( fiber.HeaderContentType , "application/x-ndjson" )
	c.Set( fiber.HeaderContentDisposition , `attachment; filename="logs-`+day+`.jsonl"` )
	c.Response().SetBodyStreamWriter( func( writer *bufio.Writer ) {
		handlers.Logs.Download( day , query , writer )
		writer.Flush()
	} )
	return
}

// streamLogs tells the Logs screen when there is something new, so Live needs
// no timer. It sends a nudge, not the entries: the page reads them the way it
// always does, which is what keeps its search, paging and decryption in one
// place. Watching cannot feed itself, since nothing under /api/owner/logs is
// logged.
//
// @route("GET /api/owner/logs/stream", summary="New log entries as they happen",
//     tag="Owner", role=OWNER, feature="logs", reply_type="text/event-stream",
// )
func ( handlers *Handlers ) streamLogs( c fiber.Ctx , in *logSearch ) ( err error ) {
	credential , cookie_err := handlers.Guard.DecryptCookie( c.Cookies( handlers.cfg().CookieName ) )
	if cookie_err != nil {
		err = refuse( fiber.StatusNotFound , "not found" )
		return
	}
	err = stream.Serve( c , handlers.LogStream , func() bool {
		return handlers.Guard.StillOwner( credential )
	} )
	return
}

type logDayDeletedReply struct {
	Deleted bool `json:"deleted"`
}

// @route("POST /api/owner/logs/days/{date}/delete", summary="Delete one day",
//     tag="Owner", role=OWNER, feature="logs", errors=(409,),
// )
func ( handlers *Handlers ) deleteLogDay( c fiber.Ctx , in *logDayPath ) ( reply *logDayDeletedReply , err error ) {
	delete_err := handlers.Logs.DeleteDay( in.Date )
	switch {
	case errors.Is( delete_err , logs.ErrBadDay ):
		err = refuse( fiber.StatusBadRequest , "bad_day" )
	case errors.Is( delete_err , logs.ErrToday ):
		err = refuse( fiber.StatusConflict , logs.ErrToday.Error() )
	case delete_err != nil:
		err = delete_err
	default:
		handlers.audit( c , logs.KindAdmin , "log day deleted" , fiber.Map{ "date": in.Date } )
		reply = &logDayDeletedReply{ Deleted: true }
	}
	return
}

// pruneResult is what a prune did, as the model says it.
type pruneResult logs.PruneResult

func ( pruneResult ) APIName() ( name string ) { name = "PruneResult"; return }

// @route("POST /api/owner/logs/prune", summary="Apply the retention rules now",
//     tag="Owner", role=OWNER, feature="logs",
// )
func ( handlers *Handlers ) pruneLogs( c fiber.Ctx , in *apidoc.None ) ( reply *pruneResult , err error ) {
	result , err := handlers.Logs.Prune()
	if err != nil { return }
	handlers.audit( c , logs.KindAdmin , "logs pruned" , fiber.Map{ "compressed": result.Compressed , "deleted": result.Deleted } )
	pruned := pruneResult( result )
	reply = &pruned
	return
}

type ipNamesReply struct {
	Names []*models.IPName `json:"names"`
}

func ( ipNamesReply ) APIName() ( name string ) { name = "IPNames"; return }

// listIPNames is everything the owner has named. It is short by nature, so
// unlike the lists people's activity grows, it comes whole.
//
// @route("GET /api/owner/logs/ip-names", summary="What the owner calls addresses",
//     tag="Owner", role=OWNER, feature="logs", errors=(403,),
// )
func ( handlers *Handlers ) listIPNames( c fiber.Ctx , in *apidoc.None ) ( reply *ipNamesReply , err error ) {
	names , err := models.ListIPNames( handlers.Store )
	if err != nil { return }
	reply = &ipNamesReply{ Names: names }
	return
}

// ipTarget is the address or range an IP name is for. It rides in the body,
// because a range has a slash in it.
type ipTarget struct {
	Target string `json:"target" jsonschema:"example=10.0.0.0/24" jsonschema_description:"An address, or a range such as 10.0.0.0/24." error:"bad_ip_target"`
}

type ipNameRequest struct {
	ipTarget
	Name string `json:"name" jsonschema:"minLength=1,maxLength=80" error:"bad_ip_name"`
}

type ipNameReply struct {
	Target string `json:"target"`
	Name   string `json:"name"`
}

// setIPName names an address or a range, or renames one. The answer gives the
// target as stored, which may not be how it was typed.
//
// @route("POST /api/owner/logs/ip-names", summary="Name an address or a range, or rename one",
//     tag="Owner", role=OWNER, feature="logs",
// )
func ( handlers *Handlers ) setIPName( c fiber.Ctx , in *ipNameRequest ) ( reply *ipNameReply , err error ) {
	target , ok := models.CleanIPTarget( in.Target )
	if ok == false {
		err = refuse( fiber.StatusBadRequest , "bad_ip_target" )
		return
	}
	if models.ValidIPName( in.Name ) == false {
		err = refuse( fiber.StatusBadRequest , "bad_ip_name" )
		return
	}
	if err = models.SetIPName( handlers.Store , target , in.Name ); err != nil { return }
	handlers.audit( c , logs.KindAdmin , "ip name set" , fiber.Map{ "target": target } )
	reply = &ipNameReply{ Target: target , Name: strings.TrimSpace( in.Name ) }
	return
}

type ipNameDeletedReply struct {
	Deleted bool `json:"deleted"`
}

// deleteIPName takes a name away.
//
// @route("POST /api/owner/logs/ip-names/delete", summary="Take a name off an address or a range",
//     tag="Owner", role=OWNER, feature="logs",
// )
func ( handlers *Handlers ) deleteIPName( c fiber.Ctx , in *ipTarget ) ( reply *ipNameDeletedReply , err error ) {
	target , ok := models.CleanIPTarget( in.Target )
	if ok == false {
		err = refuse( fiber.StatusBadRequest , "bad_ip_target" )
		return
	}
	if err = models.DeleteIPName( handlers.Store , target ); err != nil { return }
	handlers.audit( c , logs.KindAdmin , "ip name removed" , fiber.Map{ "target": target } )
	reply = &ipNameDeletedReply{ Deleted: true }
	return
}

// uiEvent is one thing track.js saw. Everything in it came from a browser,
// so every field is capped before it is written.
type uiEvent struct {
	Action string         `json:"action"`
	Screen string         `json:"screen"`
	Target string         `json:"target"`
	Fields []string       `json:"fields"`
	Detail map[string]any `json:"detail"`
	At     string         `json:"at"`
	Client map[string]any `json:"client"`
}

type uiEventsRequest struct {
	Tab    string           `json:"tab"`
	Events []map[string]any `json:"events"`
}

type uiEventsReply struct {
	Accepted int `json:"accepted"`
}

// recordUIEvents takes a batch from track.js. The server stamps each with
// its own time, the signed-in account, the address and the user agent, so a
// browser can describe what it did but not who did it or when it arrived.
//
// @route("POST /api/logs/events", summary="Record what happened on the page",
//     tag="Account", feature="logs",
//     mcp_http="The browser's own activity reports (track.js), not an action.",
// )
func ( handlers *Handlers ) recordUIEvents( c fiber.Ctx , in *uiEventsRequest ) ( reply *uiEventsReply , err error ) {
	events := []uiEvent{}
	encoded , err := json.Marshal( in.Events )
	if err != nil { return }
	if json.Unmarshal( encoded , &events ) != nil {
		err = refuse( fiber.StatusBadRequest , "malformed request body" )
		return
	}
	if len( events ) > logEventsPerCall {
		events = events[ :logEventsPerCall ]
	}
	actor := security.ActorFrom( c )
	allowed := handlers.uiEventBudget.take( actor.User.ID , len( events ) )
	tab := logs.Truncate( in.Tab , 32 )
	accepted := 0
	for index , event := range events {
		if index >= allowed || logUIActions[ event.Action ] == false { continue }
		entry := logs.Entry{
			Kind:  logs.KindUI,
			Level: logs.LevelInfo,
			Actor: security.LogActor( actor ),
			IP:    c.IP(),
			UA:    logs.Truncate( c.Get( fiber.HeaderUserAgent ) , 300 ),
			UI: &logs.UI{
				Action: event.Action,
				Screen: logs.Truncate( event.Screen , 120 ),
				Target: logs.Truncate( event.Target , 120 ),
				Fields: capFields( event.Fields ),
				Detail: capMap( event.Detail ),
				At:     logs.Truncate( event.At , 40 ),
			},
			Client: map[string]any{ "tab": tab },
		}
		if event.Action == "error" || event.Action == "fetch_fail" {
			entry.Level = logs.LevelError
		}
		if event.Action == "session_start" {
			for name , value := range capMap( event.Client ) {
				entry.Client[ name ] = value
			}
		}
		handlers.Logs.Log( entry )
		accepted += 1
	}
	reply = &uiEventsReply{ Accepted: accepted }
	return
}

func capFields( fields []string ) ( result []string ) {
	for _ , field := range fields {
		if len( result ) == 30 { break }
		result = append( result , logs.Truncate( field , 60 ) )
	}
	return
}

// capMap keeps a browser-sent object only if it is small, with its strings
// capped. Too big, and it is dropped whole rather than cut mid-structure.
func capMap( value map[string]any ) ( result map[string]any ) {
	if len( value ) == 0 { return }
	for name , item := range value {
		if text , ok := item.( string ); ok {
			value[ name ] = logs.Truncate( text , 200 )
		}
	}
	encoded , err := json.Marshal( value )
	if err != nil || len( encoded ) > logDetailMaxBytes { return }
	result = value
	return
}

// eventBudget is the per-account cap on page events: a minute's worth, in a
// sliding window, so a stuck loop in a browser cannot fill the disk.
type eventBudget struct {
	lock sync.Mutex
	seen map[uint64][]time.Time
}

func ( budget *eventBudget ) take( user_id uint64 , wanted int ) ( allowed int ) {
	budget.lock.Lock()
	defer budget.lock.Unlock()
	if budget.seen == nil {
		budget.seen = map[uint64][]time.Time{}
	}
	now := time.Now()
	recent := budget.seen[ user_id ][ :0 ]
	for _ , at := range budget.seen[ user_id ] {
		if now.Sub( at ) < time.Minute {
			recent = append( recent , at )
		}
	}
	allowed = logEventsPerMinute - len( recent )
	if allowed > wanted { allowed = wanted }
	if allowed < 0 { allowed = 0 }
	for index := 0; index < allowed; index += 1 {
		recent = append( recent , now )
	}
	if len( recent ) == 0 {
		delete( budget.seen , user_id )
	} else {
		budget.seen[ user_id ] = recent
	}
	return
}
