// whole-file feature:shortcuts
package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

type shortcutsRequest struct {
	Shortcuts map[string]string `json:"shortcuts" jsonschema_description:"Action name to binding, such as \"search\": \"Mod+K\". Mod is Cmd on a Mac and Ctrl elsewhere. Only changed shortcuts are listed; an empty binding switches that key off. Replaces the whole set; {} puts every key back to its default."`
}

type shortcutsReply struct {
	Ok        bool              `json:"ok"`
	Shortcuts map[string]string `json:"shortcuts"`
}

// setMyShortcuts replaces the caller's changed keyboard shortcuts, from the
// Shortcuts screen. The whole set is sent each time, so an undo is the same
// call with the set from before; {} puts every key back to its default. See
// models/shortcuts.go.
//
// @route("POST /api/account/shortcuts", summary="Change your keyboard shortcuts",
//     tag="Account", feature="shortcuts",
//     description='Replace your changed keyboard shortcuts: action name to binding, such as "search": '
//         '"Mod+K" (Mod is Cmd on a Mac, Ctrl elsewhere). Send the whole set; an empty binding switches a '
//         'key off, and {} puts every key back to its default.',
//     mcp_note="whoami shows the current set.",  // feature:mcp
//     mcp="set_shortcuts",  // feature:mcp
// )
func ( handlers *Handlers ) setMyShortcuts( c fiber.Ctx , in *shortcutsRequest ) ( reply *shortcutsReply , err error ) {
	shortcuts := in.Shortcuts
	if shortcuts == nil { shortcuts = map[string]string{} }
	if invalid := models.ValidShortcuts( shortcuts ); invalid != nil {
		err = refuse( fiber.StatusBadRequest , invalid.Error() )
		return
	}
	user := security.UserFrom( c )
	if err = models.SetShortcuts( handlers.Store , user.ID , shortcuts ); err != nil { return }
	handlers.audit( c , logs.KindAuth , "changed own shortcuts" , fiber.Map{ "count": len( shortcuts ) } )
	reply = &shortcutsReply{ Ok: true , Shortcuts: shortcuts }
	return
}
