package middleware

import (
	errors "errors"
	log "log"

	fiber "github.com/gofiber/fiber/v3"

	logs "{{MODULE_PATH}}/server/logs"
)

// ErrorHandler answers every error a handler returns. An internal error
// message is never handed to a client: the detail goes to stdout and the
// activity log, and the client gets a generic message and the reference that
// finds it there.
func ErrorHandler( c fiber.Ctx , handler_err error ) ( response_err error ) {
	code := errorStatus( handler_err )
	body := fiber.Map{ "error": statusText( code ) }
	if code >= 500 {
		id := RequestIDFrom( c )
		// Redacted like the log: the path of a login link is the credential.
		log.Printf( "[error] %s %s %s: %v" , id , c.Method() , logs.Redact( c.Path() , "" ) , handler_err )
		body[ "request_id" ] = id
	}
	response_err = c.Status( code ).JSON( body )
	return
}

// errorStatus is the status an error becomes.
func errorStatus( err error ) ( code int ) {
	code = fiber.StatusInternalServerError
	var fiber_err *fiber.Error
	if errors.As( err , &fiber_err ) {
		code = fiber_err.Code
	}
	return
}

func statusText( code int ) ( result string ) {
	switch {
	case code == fiber.StatusNotFound:
		result = "not found"
	case code == fiber.StatusUnauthorized:
		result = "not authenticated"
	case code == fiber.StatusForbidden:
		result = "forbidden"
	case code == fiber.StatusTooManyRequests:
		result = "too many requests"
	case code >= 500:
		result = "internal error"
	default:
		result = "bad request"
	}
	return
}
