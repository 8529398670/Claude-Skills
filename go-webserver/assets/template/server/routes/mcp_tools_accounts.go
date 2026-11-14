// whole-file feature:mcp
package routes

import (
	json "encoding/json"

	fiber "github.com/gofiber/fiber/v3"

	mcp "{{MODULE_PATH}}/server/mcp"
)

// The MCP side of accounts and sign-in: login links, accounts, devices and
// invite links. Each tool is its HTTP route (mcp= in its @route(...)): the same handler,
// the same checks on what it is sent, so the rules (who may touch the owner,
// what a narrowed key may not do) hold whichever door a request comes through.
//
// A login link or invite link made here records the key that asked, and
// revoking or deleting that key voids it while it can still be used (see
// models.VoidLinksMintedByKeyTx). That is what keeps revoking a leaked key
// meaningful now that a key can hand out ways in.

// withLoginLinkURL is the call of a tool for a route that mints a login link:
// the route's reply with login_url added, the full address, from the address
// this agent reached /mcp at. The HTTP routes answer with login_path only
// (see createUser for why).
func withLoginLinkURL[ In any , Out any ]( call *mcp.Call , in *In , serve func( fiber.Ctx , *In ) ( *Out , error ) ) ( result any , err error ) {
	result , err = withFullURL( call , in , serve , "login_path" , "login_url" )
	return
}

// >>> feature:invite_links
// withJoinURL is withLoginLinkURL for an invite link: join_url beside
// join_path.
func withJoinURL[ In any , Out any ]( call *mcp.Call , in *In , serve func( fiber.Ctx , *In ) ( *Out , error ) ) ( result any , err error ) {
	result , err = withFullURL( call , in , serve , "join_path" , "join_url" )
	return
}
// <<< feature:invite_links

// withFullURL is the route's reply with url_field added beside path_field,
// the full address, from the address this agent reached /mcp at.
func withFullURL[ In any , Out any ]( call *mcp.Call , in *In , serve func( fiber.Ctx , *In ) ( *Out , error ) , path_field string , url_field string ) ( result any , err error ) {
	reply , serve_err := serve( call.Ctx , in )
	if result , err = mcp.Answer( reply , serve_err ); err != nil { return }
	encoded , err := json.Marshal( reply )
	if err != nil { return }
	answer := map[string]any{}
	if err = json.Unmarshal( encoded , &answer ); err != nil { return }
	if path , ok := answer[ path_field ].( string ); ok { answer[ url_field ] = call.Ctx.BaseURL() + path }
	result = answer
	return
}

// >>> feature:two_factor
// twoFactorOn is whether admin_reset_two_factor is offered.
func ( handlers *Handlers ) twoFactorOn() ( result bool ) {
	result = handlers.cfg().TwoFactorEnabled
	return
}
// <<< feature:two_factor
