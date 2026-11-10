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
	payload , refused := handlers.setMyShortcuts( c , body.Shortcuts )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// setMyShortcuts is SetMyShortcuts after the request is read, for it and the
// MCP set_shortcuts tool.
func ( handlers *Handlers ) setMyShortcuts( c fiber.Ctx , shortcuts map[string]string ) ( payload fiber.Map , refused *refusal ) {
	if shortcuts == nil { shortcuts = map[string]string{} }
	if invalid := models.ValidShortcuts( shortcuts ); invalid != nil {
		refused = refuse( fiber.StatusBadRequest , invalid.Error() )
		return
	}
	user := security.UserFrom( c )
	if set_err := models.SetShortcuts( handlers.Store , user.ID , shortcuts ); set_err != nil {
		refused = internalFailure( set_err )
		return
	}
	handlers.audit( c , logs.KindAuth , "changed own shortcuts" , fiber.Map{ "count": len( shortcuts ) } )
	payload = fiber.Map{ "ok": true , "shortcuts": shortcuts }
	return
}
