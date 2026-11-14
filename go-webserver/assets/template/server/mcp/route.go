// whole-file feature:mcp
package mcp

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// ForRoute is the tool that stands for a declared route (see apidoc.Handle):
//
//	server.Register( mcp.ForRoute( handlers.DeleteUser , mcp.Tool{ Name: "admin_delete_user" , ... } , nil ) )
//
// Its arguments are the route's request type, URL fields and all, so its
// input schema and the checks on what it is sent are the route's; nobody
// writes them for a tool. Leave call nil to answer with the route's own
// handler and reply. A tool that adds to the reply (a full URL where the
// route answers a path) passes a call that does, given the checked request.
func ForRoute[ In any , Out any ]( route func() *apidoc.Endpoint[ In , Out ] , tool Tool , call func( call *Call , in *In ) ( any , error ) ) ( result *Tool ) {
	name := apidoc.Declare( route )
	endpoint := route()
	if call == nil {
		call = func( call *Call , in *In ) ( reply any , err error ) {
			reply , err = Answer( endpoint.Serve( call.Ctx , in ) )
			return
		}
	}
	result = &tool
	result.route = name
	result.Call = func( mcp_call *Call ) ( reply any , err error ) {
		in := new( In )
		if message , malformed := apidoc.DecodeArguments( name , mcp_call.Args , in ); message != "" {
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
