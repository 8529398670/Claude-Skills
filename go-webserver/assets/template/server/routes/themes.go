// whole-file feature:appearance
package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// The Appearance screen's routes: someone's own themes, and the defaults an
// admin sets for everyone who has not made their own. See models/themes.go.
// Each save sends the whole list, so an undo is the same call with the list
// from before.

type themesRequest struct {
	// nil (or left out) means "follow the defaults" for your own list, and
	// "the built-in themes" for the defaults.
	Themes    []models.Theme `json:"themes"`
	ThemeID   string         `json:"theme_id"`
	CSRFToken string         `json:"csrf_token"`
}

// myThemes is what /api/me says about themes. themes is null when neither the
// person nor an admin has saved a list: the browser has the built-in ones.
func ( handlers *Handlers ) myThemes( user *models.User ) ( themes []models.Theme , custom bool , err error ) {
	if user.Themes != nil {
		themes = user.Themes
		custom = true
		return
	}
	themes , err = models.ThemeDefaults( handlers.Store )
	return
}

// SetMyThemes replaces the caller's own list and pick.
func ( handlers *Handlers ) SetMyThemes( c fiber.Ctx ) ( err error ) {
	var body themesRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	user := security.UserFrom( c )
	if set_err := models.SetUserThemes( handlers.Store , user.ID , body.Themes , body.ThemeID ); set_err != nil {
		err = badRequest( c , set_err.Error() )
		return
	}
	user , get_err := models.GetUser( handlers.Store , user.ID )
	if get_err != nil {
		err = serverError( c , get_err )
		return
	}
	themes , custom , themes_err := handlers.myThemes( user )
	if themes_err != nil {
		err = serverError( c , themes_err )
		return
	}
	handlers.audit( c , logs.KindAuth , "changed own themes" , fiber.Map{ "count": len( body.Themes ) , "custom": custom } )
	err = c.JSON( fiber.Map{ "ok": true , "themes": themes , "themes_custom": custom , "theme_id": user.ThemeID } )
	return
}

// SetMyTheme changes only which theme the caller uses.
func ( handlers *Handlers ) SetMyTheme( c fiber.Ctx ) ( err error ) {
	var body themesRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	user := security.UserFrom( c )
	if set_err := models.SetUserThemeID( handlers.Store , user.ID , body.ThemeID ); set_err != nil {
		err = badRequest( c , set_err.Error() )
		return
	}
	err = c.JSON( fiber.Map{ "ok": true , "theme_id": body.ThemeID } )
	return
}

// GetThemeDefaults: themes is null while the built-in ones apply.
func ( handlers *Handlers ) GetThemeDefaults( c fiber.Ctx ) ( err error ) {
	themes , get_err := models.ThemeDefaults( handlers.Store )
	if get_err != nil {
		err = serverError( c , get_err )
		return
	}
	err = c.JSON( fiber.Map{ "themes": themes } )
	return
}

// SetThemeDefaults replaces the list everyone without their own list sees.
func ( handlers *Handlers ) SetThemeDefaults( c fiber.Ctx ) ( err error ) {
	var body themesRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	if set_err := models.SetThemeDefaults( handlers.Store , body.Themes ); set_err != nil {
		err = badRequest( c , set_err.Error() )
		return
	}
	themes , get_err := models.ThemeDefaults( handlers.Store )
	if get_err != nil {
		err = serverError( c , get_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "changed the default themes" , fiber.Map{ "count": len( themes ) , "built_in": themes == nil } )
	err = c.JSON( fiber.Map{ "ok": true , "themes": themes } )
	return
}
