// whole-file feature:mcp
package mcp

import (
	json "encoding/json"
	fmt "fmt"

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	security "{{MODULE_PATH}}/server/security"
)

// Tool is one thing an agent can do through /mcp.
//
// Adding one is the MCP version of adding a route: write it next to the
// others in server/routes/mcp_tools.go with ForRoute, so it runs the route's
// own handler and a rule enforced there holds whichever door a change comes
// through. The rules match the rest of the template:
//
//   - One tool per route an API key may call. ForRoute ties it to its route,
//     and mcp_parity_test.go fails until every key-callable route in the API
//     docs has a tool, or an MCPHTTP reason in its declaration for staying
//     HTTP only (file bytes, for one: MCP messages are JSON, and clients cap
//     their size, so uploads and downloads go over /api/files with the same
//     key and the tool hands back the addresses).
//   - Any signed-in key, unless the route is admin only. AdminOnly hides the
//     tool from user-scoped keys entirely.
//   - Nothing a key cannot do over HTTP. Managing API keys, your own password,
//     PIN and two-factor, other people's files and the owner's screens need a
//     browser session (see RequireSession). A tool is a key-authenticated
//     route; the same rule keeps those out. Accounts, login links, devices and
//     invite links are open to keys (mcp_tools_accounts.go).
//   - Mark the hints honestly. ReadOnly and Destructive become the spec's
//     readOnlyHint / destructiveHint, which a client uses to decide whether to
//     ask the person before calling. They are advice to the client, not
//     protection -- the protection is the role check and the model rules.
type Tool struct {
	Name        string
	Title       string
	Description string
	ReadOnly    bool
	Destructive bool
	AdminOnly   bool
	// route is the declared route ForRoute made this tool for: its input
	// schema, and the HTTP route it is the MCP door to.
	route string
	// Enabled, when set, is asked on every call: a tool for a feature that is
	// switched off answers exactly like one that does not exist. Nil is always.
	Enabled func() bool
	Call        func( call *Call ) ( result any , err error )
}

// Call is what a tool gets: the request (for anything the handler would
// read), who is calling at which effective role, and its raw arguments,
// which ForRoute checks and decodes into the route's request type.
type Call struct {
	Ctx   fiber.Ctx
	Actor *security.Actor
	Args  json.RawMessage
}

// Refusal is a tool saying no for a reason the caller should read -- "no such
// user", "the owner account cannot be disabled". It becomes a tool result
// with isError set, worded like the HTTP route's error, so the model sees why
// and can try something else. Any other error is logged and reported only as
// "internal error".
type Refusal struct {
	Message string
}

func ( refusal *Refusal ) Error() ( result string ) {
	result = refusal.Message
	return
}

func Refuse( message string ) ( err error ) {
	err = &Refusal{ Message: message }
	return
}

type invalidArgs struct {
	message string
}

func ( invalid *invalidArgs ) Error() ( result string ) {
	result = invalid.message
	return
}

// Register adds a tool made with ForRoute. A duplicate name is a programming
// error and panics at startup, where it is noticed, rather than one tool
// silently shadowing another.
func ( server *Server ) Register( tool *Tool ) {
	if tool.route == "" {
		panic( fmt.Sprintf( "mcp: tool %q was not made with ForRoute" , tool.Name ) )
	}
	if _ , exists := server.by_name[ tool.Name ]; exists {
		panic( fmt.Sprintf( "mcp: tool %q registered twice" , tool.Name ) )
	}
	server.tools = append( server.tools , tool )
	server.by_name[ tool.Name ] = tool
}

func ( tool *Tool ) visibleTo( actor *security.Actor ) ( result bool ) {
	if tool.Enabled != nil && tool.Enabled() == false { return }
	result = tool.AdminOnly == false || ( actor != nil && actor.IsAdmin() )
	return
}

// Tools is every registered tool, whoever may call it, in registration
// order: what config/mcp.json lists.
func ( server *Server ) Tools() ( tools []*Tool ) {
	tools = server.tools
	return
}

// Listing is the tool as tools/list describes it.
func ( tool *Tool ) Listing() ( listed fiber.Map ) {
	declared , err := apidoc.InputSchema( tool.route )
	if err != nil { panic( err ) }
	schema := fiber.Map( declared )
	listed = fiber.Map{
		"name":        tool.Name,
		"title":       tool.Title,
		"description": tool.Description,
		"inputSchema": schema,
		"annotations": fiber.Map{
			"title":           tool.Title,
			"readOnlyHint":    tool.ReadOnly,
			"destructiveHint": tool.Destructive,
			// Every tool here acts on this server's own records, never
			// the open internet.
			"openWorldHint": false,
		},
	}
	return
}

// RouteName is the HTTP route the tool stands for, as "METHOD /path/{param}"
// the way config/openapi.json writes it.
func ( tool *Tool ) RouteName() ( route string ) {
	route = apidoc.RouteOf( tool.route )
	return
}

// visible is tools/list for one actor: the tools its effective role may call,
// in registration order, so a user-scoped key is never even told the admin
// tools exist.
func ( server *Server ) visible( actor *security.Actor ) ( listed []fiber.Map ) {
	listed = []fiber.Map{}
	for _ , tool := range server.tools {
		if tool.visibleTo( actor ) == false { continue }
		listed = append( listed , tool.Listing() )
	}
	return
}
