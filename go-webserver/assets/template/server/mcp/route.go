// whole-file feature:mcp
package mcp

import (
	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// ForRoute is the tool that stands for a declared route, as the route table
// (routes_generated.go) registers it from the mcp= in the route's @route(...):
//
//	server.Register( mcp.ForRoute( "POST /api/admin/users/{user_id}/delete" , handlers.deleteUser , mcp.Tool{ ... } , nil ) )
//
// Its arguments are the route's request type, URL fields and all, so its
// input schema and the checks on what it is sent are the route's; nobody
// writes them for a tool. With call nil it answers with the route's own
// handler and reply. A tool that adds to the reply (a full URL where the
// route answers a path) passes a call, which gets the checked request and
// the route's handler to run.
func ForRoute[ In any , Out any ]( route string , serve func( c fiber.Ctx , in *In ) ( *Out , error ) , tool Tool , call func( call *Call , in *In , serve func( c fiber.Ctx , in *In ) ( *Out , error ) ) ( any , error ) ) ( result *Tool ) {
	apidoc.DeclareRoute[ In , Out ]( route )
	result = bindRoute( route , tool , func( mcp_call *Call , in *In ) ( reply any , err error ) {
		if call == nil {
			reply , err = Answer( serve( mcp_call.Ctx , in ) )
			return
		}
		reply , err = call( mcp_call , in , serve )
		return
	} )
	return
}

// ForStream is ForRoute for a route that writes its own reply (a download,
// say), which an MCP message cannot carry: call says where to get it instead.
func ForStream[ In any ]( route string , serve func( c fiber.Ctx , in *In ) error , tool Tool , call func( call *Call , in *In ) ( any , error ) ) ( result *Tool ) {
	apidoc.DeclareRoute[ In , apidoc.None ]( route )
	result = bindRoute( route , tool , call )
	return
}

func bindRoute[ In any ]( route string , tool Tool , call func( call *Call , in *In ) ( any , error ) ) ( result *Tool ) {
	result = &tool
	result.route = route
	result.Call = func( mcp_call *Call ) ( reply any , err error ) {
		in := new( In )
		if message , malformed := apidoc.DecodeArguments( route , mcp_call.Args , in ); message != "" {
			if malformed {
				err = &invalidArgs{ message: "invalid arguments: " + message }
			} else {
				err = Refuse( message )
			}
			return
		}
		reply , err = call( mcp_call , in )
		return
	}
	return
}

// Answer is a route handler's result as a tool's: a refusal below 500 is the
// same words as a tool error, anything else wrong is internal.
func Answer[ Out any ]( reply *Out , err error ) ( result any , failure error ) {
	if err != nil {
		if sender , ok := err.( apidoc.Sender ); ok {
			if status , message := sender.Refused(); status < 500 {
				failure = Refuse( message )
				return
			}
		}
		failure = err
		return
	}
	result = reply
	return
}
