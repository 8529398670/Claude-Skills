// Package apidoc writes the API reference -- config/openapi.json and
// config/api.md -- from the code, so nobody edits either by hand.
//
// Every route's handler declares its docs once, with Doc, in a *_api.go file
// next to it: a summary, who may call it, and the Go types of what it takes and
// gives back. The struct tags on those types carry the rest (see schema.go).
// Build walks the routes the app really registers and pairs each with its
// handler's docs, so the paths and methods come from the route table and a
// route without docs, or docs without a route, is reported.
//
// Nothing here runs while the server is up. scripts/api_docs.sh runs
// TestAPIDocsAreCurrent with -update, which writes the files; the same test
// fails while a committed file is out of date.
package apidoc

import (
	fmt "fmt"
	reflect "reflect"
	regexp "regexp"
	runtime "runtime"
	sort "sort"
	strconv "strconv"
	strings "strings"
	sync "sync"
)

// Title heads both files.
const Title = "{{PROJECT_NAME}} API"

// Version is info.version in openapi.json.
const Version = "1.0.0"

// About is the opening paragraph of both files.
const About = "Every route this server answers, apart from the pages at /login/* and /join/*. Generated from the code by scripts/api_docs.sh: an app's own routes get their docs the same way, and a test fails until they do."

// Access says who may call a route. The zero value is anyone signed in, by a
// browser session or an API key.
type Access int

const (
	Signed Access = iota
	// Public needs no sign-in at all.
	Public
	// SessionOnly needs a browser session: an API key is turned away.
	SessionOnly
	// KeyOnly needs an API key.
	KeyOnly
)

// Roles for Op.Role. Empty is any signed-in user.
const (
	Admin = "admin"
	Owner = "owner"
)

// Op is one route's docs.
type Op struct {
	// Method picks the route when one handler serves several methods.
	Method      string
	Summary     string
	Description string
	Tag         string
	Access      Access
	Role        string
	// Feature is the optional part of the template the route belongs to; it
	// becomes x-feature on the path, which scaffold.py's prune step reads.
	Feature string
	// MCPHTTP is why a route an API key may call has no MCP tool.
	MCPHTTP string
	// Params is a struct whose fields carry `path:"name"`, `query:"name"` or
	// `header:"name"`. A path parameter it leaves out is a plain string.
	Params any
	// Body is the request body's type. BodyType is its content type when it
	// is not JSON; such a body is raw bytes and Body stays nil.
	Body     any
	BodyType string
	// Status is the success status, 200 when zero. Reply is that reply's
	// type; ReplyType its content type when it is not JSON (then Reply is
	// nil and the reply is text, or bytes for application/octet-stream).
	Status           int
	Reply            any
	ReplyType        string
	ReplyDescription string
	// Errors are the error statuses the route can answer with, besides 429
	// and 503, which every route can.
	Errors []int
}

// Route is one entry of the app's route table.
type Route struct {
	Method  string
	Path    string
	Handler any
}

type entry struct {
	op   Op
	used bool
}

var registry = map[string][]*entry{}

// building keeps two Builds from marking the same registry at once.
var building sync.Mutex

// Doc records a handler's docs. Use it at package level, so it runs before
// any test does:
//
//	var _ = apidoc.Doc( (*Handlers).GetMe , apidoc.Op{ ... } )
func Doc( handler any , op Op ) ( result struct{} ) {
	name := HandlerName( handler )
	registry[ name ] = append( registry[ name ] , &entry{ op: op } )
	return
}

// HandlerName is the function behind a handler: the same for a method value
// (handlers.GetMe, which is what a route holds) and a method expression
// ((*Handlers).GetMe, which is what Doc is given).
func HandlerName( handler any ) ( name string ) {
	value := reflect.ValueOf( handler )
	if value.Kind() != reflect.Func { return }
	function := runtime.FuncForPC( value.Pointer() )
	if function == nil { return }
	name = strings.TrimSuffix( function.Name() , "-fm" )
	return
}

var routeParameter = regexp.MustCompile( `:(\w+)` )
var specParameter = regexp.MustCompile( `\{(\w+)\}` )
var notWord = regexp.MustCompile( `[^a-z0-9]+` )

// SpecPath is a route's path the way the spec writes it: /users/{user_id}.
func SpecPath( path string ) ( result string ) {
	result = routeParameter.ReplaceAllString( path , "{$1}" )
	return
}

// Build is openapi.json for these routes, and what is wrong with the docs:
// a route nobody documented, or docs for a route that is not registered.
func Build( routes []Route ) ( spec map[string]any , problems []string ) {
	building.Lock()
	defer building.Unlock()
	for _ , entries := range registry {
		for _ , found := range entries { found.used = false }
	}
	builder := newSchemas()
	paths := map[string]any{}
	tags := map[string]bool{}
	for _ , route := range routes {
		name := HandlerName( route.Handler )
		path := SpecPath( route.Path )
		label := route.Method + " " + path
		if strings.Contains( name , ".func" ) {
			problems = append( problems , label + ": its handler is an unnamed function; give it a name and docs" )
			continue
		}
		var chosen *entry
		for _ , found := range registry[ name ] {
			if found.op.Method == "" || found.op.Method == route.Method {
				chosen = found
				if found.op.Method != "" { break }
			}
		}
		if chosen == nil {
			problems = append( problems , label + ": no docs; add apidoc.Doc( " + shortName( name ) + " , ... ) next to it" )
			continue
		}
		chosen.used = true
		item , _ := paths[ path ].( map[string]any )
		if item == nil {
			item = map[string]any{}
			paths[ path ] = item
		}
		if chosen.op.Feature != "" { item[ "x-feature" ] = chosen.op.Feature }
		operation , operation_problems := builder.operation( route.Method , path , chosen.op )
		for _ , problem := range operation_problems { problems = append( problems , label + ": " + problem ) }
		item[ strings.ToLower( route.Method ) ] = operation
		tags[ chosen.op.Tag ] = true
	}
	for name , entries := range registry {
		for _ , found := range entries {
			if found.used == false {
				problems = append( problems , "docs for " + shortName( name ) + " " + found.op.Method + ", which is no route's handler" )
			}
		}
	}
	sort.Strings( problems )
	builder.named( reflect.TypeOf( ErrorReply{} ) )
	builder.named( reflect.TypeOf( MaintenanceReply{} ) )
	responses := map[string]any{}
	for _ , status := range errorStatuses {
		target := "Error"
		if status == 503 { target = "Maintenance" }
		responses[ "E" + strconv.Itoa( status ) ] = map[string]any{
			"description": errorMeanings[ status ],
			"content":     jsonContent( ref( target ) ),
		}
	}
	tag_list := []any{}
	for _ , name := range tagOrder( tags ) { tag_list = append( tag_list , map[string]any{ "name": name } ) }
	spec = map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       Title,
			"version":     Version,
			"description": About,
		},
		"tags":  tag_list,
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"sessionCookie": map[string]any{ "type": "apiKey" , "in": "cookie" , "name": "session" , "description": "A browser session. Its POSTs carry csrf_token in the body. The cookie's name is server.cookie_name." },
				"apiKey":        map[string]any{ "type": "http" , "scheme": "bearer" , "description": "An API key, as Authorization: Bearer <id>.<secret>." },
			},
			"responses": responses,
			"schemas":   builder.components,
		},
	}
	return
}

func shortName( name string ) ( result string ) {
	result = name
	if index := strings.LastIndex( name , "/" ); index >= 0 { result = name[ index + 1 : ] }
	if index := strings.Index( result , "." ); index >= 0 { result = result[ index + 1 : ] }
	return
}

// TagOrder is the order tags appear in, in both files. A tag not listed here
// comes after these, alphabetically.
var TagOrder = []string{ "Account" , "Sign-in" , "API keys" , "Files" , "Admin" , "Invite links" , "Owner" , "Webhooks" , "Monitoring" }

func tagOrder( used map[string]bool ) ( result []string ) {
	known := map[string]bool{}
	for _ , name := range TagOrder {
		known[ name ] = true
		if used[ name ] { result = append( result , name ) }
	}
	rest := []string{}
	for name := range used {
		if known[ name ] == false { rest = append( rest , name ) }
	}
	sort.Strings( rest )
	result = append( result , rest... )
	return
}

var errorStatuses = []int{ 400 , 401 , 403 , 404 , 409 , 429 , 503 }

var errorMeanings = map[int]string{
	400: "The request was refused as it stands.",
	401: "Not signed in.",
	403: "Not allowed, or a bad csrf_token.",
	404: "No such thing, or this feature is switched off.",
	409: "The record has moved on: an undo too late, or a stale edit.",
	429: "Too many requests.",
	503: "In maintenance mode. Retry-After says when to try again.",
}

// ErrorReply is the body of every error.
type ErrorReply struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty" doc:"On a 5xx: the reference that finds this failure in the log."`
}

func ( ErrorReply ) APIName() ( name string ) { name = "Error"; return }

// MaintenanceReply is the body of a 503 in maintenance mode.
type MaintenanceReply struct {
	Error       string `json:"error"`
	Maintenance string `json:"maintenance" enum:"read_only|full"`
}

func ( MaintenanceReply ) APIName() ( name string ) { name = "Maintenance"; return }

func ( builder *schemas ) operation( method string , path string , op Op ) ( operation map[string]any , problems []string ) {
	operation = map[string]any{
		"summary":     op.Summary,
		"operationId": strings.Trim( notWord.ReplaceAllString( strings.ToLower( method + " " + path ) , "_" ) , "_" ),
		"tags":        []any{ op.Tag },
	}
	if op.Summary == "" { problems = append( problems , "no Summary" ) }
	if op.Tag == "" { problems = append( problems , "no Tag" ) }
	if op.Description != "" { operation[ "description" ] = op.Description }
	switch op.Access {
	case Public:
		operation[ "security" ] = []any{}
	case SessionOnly:
		operation[ "security" ] = []any{ map[string]any{ "sessionCookie": []any{} } }
	case KeyOnly:
		operation[ "security" ] = []any{ map[string]any{ "apiKey": []any{} } }
	default:
		operation[ "security" ] = []any{ map[string]any{ "sessionCookie": []any{} } , map[string]any{ "apiKey": []any{} } }
	}
	if op.Role != "" { operation[ "x-required-role" ] = op.Role }
	if op.MCPHTTP != "" { operation[ "x-mcp-http" ] = op.MCPHTTP }
	parameters , parameter_problems := builder.parameters( path , op.Params )
	problems = append( problems , parameter_problems... )
	if len( parameters ) > 0 { operation[ "parameters" ] = parameters }
	switch {
	case op.BodyType != "":
		operation[ "requestBody" ] = map[string]any{ "required": true , "content": map[string]any{ op.BodyType: map[string]any{ "schema": rawSchema( op.BodyType ) } } }
	case op.Body != nil:
		operation[ "requestBody" ] = map[string]any{ "required": true , "content": jsonContent( builder.of( reflect.TypeOf( op.Body ) ) ) }
	}
	status := op.Status
	if status == 0 { status = 200 }
	description := op.ReplyDescription
	if description == "" { description = "Success" }
	reply := map[string]any{ "description": description }
	switch {
	case op.ReplyType != "":
		reply[ "content" ] = map[string]any{ op.ReplyType: map[string]any{ "schema": rawSchema( op.ReplyType ) } }
	case op.Reply != nil:
		reply[ "content" ] = jsonContent( builder.of( reflect.TypeOf( op.Reply ) ) )
	default:
		problems = append( problems , "no Reply" )
	}
	responses := map[string]any{ strconv.Itoa( status ): reply }
	statuses := append( []int{ 429 , 503 } , op.Errors... )
	for _ , code := range statuses {
		if _ , known := errorMeanings[ code ]; known == false {
			problems = append( problems , fmt.Sprintf( "error status %d has no shared response" , code ) )
			continue
		}
		responses[ strconv.Itoa( code ) ] = ref( "E" + strconv.Itoa( code ) , "responses" )
	}
	operation[ "responses" ] = responses
	return
}

func ( builder *schemas ) parameters( path string , params any ) ( result []any , problems []string ) {
	declared := map[string]map[string]any{}
	order := []string{}
	if params != nil {
		kind := reflect.TypeOf( params )
		for index := 0 ; index < kind.NumField() ; index++ {
			field := kind.Field( index )
			place , name := "" , ""
			for _ , candidate := range []string{ "path" , "query" , "header" } {
				if value := field.Tag.Get( candidate ); value != "" { place , name = candidate , value }
			}
			if place == "" {
				problems = append( problems , "parameter field " + field.Name + " has no path, query or header tag" )
				continue
			}
			parameter := map[string]any{ "name": name , "in": place , "schema": builder.field( field , false ) }
			if doc := field.Tag.Get( "doc" ); doc != "" { parameter[ "description" ] = doc }
			if place == "path" { parameter[ "required" ] = true }
			declared[ place + " " + name ] = parameter
			order = append( order , place + " " + name )
		}
	}
	for _ , match := range specParameter.FindAllStringSubmatch( path , -1 ) {
		key := "path " + match[ 1 ]
		if parameter , found := declared[ key ]; found {
			result = append( result , parameter )
			delete( declared , key )
			continue
		}
		result = append( result , map[string]any{ "name": match[ 1 ] , "in": "path" , "required": true , "schema": map[string]any{ "type": "string" } } )
	}
	for _ , key := range order {
		if parameter , found := declared[ key ]; found {
			if strings.HasPrefix( key , "path " ) {
				problems = append( problems , "path parameter " + key[ 5 : ] + " is not in the path" )
				continue
			}
			result = append( result , parameter )
		}
	}
	return
}

func jsonContent( schema map[string]any ) ( content map[string]any ) {
	content = map[string]any{ "application/json": map[string]any{ "schema": schema } }
	return
}

func rawSchema( content_type string ) ( schema map[string]any ) {
	schema = map[string]any{ "type": "string" }
	if content_type == "application/octet-stream" { schema[ "format" ] = "binary" }
	return
}

func ref( name string , section ...string ) ( result map[string]any ) {
	where := "schemas"
	if len( section ) > 0 { where = section[ 0 ] }
	result = map[string]any{ "$ref": "#/components/" + where + "/" + name }
	return
}
