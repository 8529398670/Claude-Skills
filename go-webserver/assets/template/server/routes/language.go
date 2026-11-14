package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// GetLanguage hands the browser the whole language.yaml tree as JSON. It is
// deliberately public: the signed-out page has text too, and none of it is
// secret. Do not put anything sensitive in language.yaml -- it is, by design,
// readable by anyone who can reach the server.
func ( handlers *Handlers ) GetLanguage() *apidoc.Endpoint[ apidoc.None , apidoc.None ] {
	return apidoc.Stream( apidoc.Op{ Summary: "The UI's text" , Tag: "Account" , Access: apidoc.Public , ReplyType: fiber.MIMEApplicationJSON },
		func( c fiber.Ctx , in *apidoc.None ) ( err error ) {
			c.Set( fiber.HeaderContentType , fiber.MIMEApplicationJSON )
			c.Set( fiber.HeaderCacheControl , "no-store" )
			err = c.Send( handlers.Language.JSON() )
			return
		} )
}

// GetHealth is what the Dockerfile HEALTHCHECK hits. It stays trivial on
// purpose -- if it touched the database, a slow query would get the container
// killed and restarted, turning a small problem into an outage.
func ( handlers *Handlers ) GetHealth() *apidoc.Endpoint[ apidoc.None , okReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Is the process up" , Tag: "Monitoring" , Access: apidoc.Public },
		func( c fiber.Ctx , in *apidoc.None ) ( reply *okReply , err error ) {
			reply = &okReply{ Ok: true }
			return
		} )
}
