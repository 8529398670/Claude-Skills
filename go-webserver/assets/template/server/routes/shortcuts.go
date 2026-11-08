// whole-file feature:shortcuts
package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

type shortcutsRequest struct {
	Shortcuts map[string]string `json:"shortcuts"`
	CSRFToken string            `json:"csrf_token"`
}

// SetMyShortcuts replaces the caller's changed keyboard shortcuts, from the
// Shortcuts screen. The whole set is sent each time, so an undo is the same
// call with the set from before; {} puts every key back to its default. See
// models/shortcuts.go.
func ( handlers *Handlers ) SetMyShortcuts( c fiber.Ctx ) ( err error ) {
	var body shortcutsRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	if body.Shortcuts == nil { body.Shortcuts = map[string]string{} }
	if invalid := models.ValidShortcuts( body.Shortcuts ); invalid != nil {
		err = badRequest( c , invalid.Error() )
		return
	}
	user := security.UserFrom( c )
	if set_err := models.SetShortcuts( handlers.Store , user.ID , body.Shortcuts ); set_err != nil {
		err = serverError( c , set_err )
		return
	}
	handlers.audit( c , logs.KindAuth , "changed own shortcuts" , fiber.Map{ "count": len( body.Shortcuts ) } )
	err = c.JSON( fiber.Map{ "ok": true , "shortcuts": body.Shortcuts } )
	return
}
