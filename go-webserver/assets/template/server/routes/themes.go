// whole-file feature:appearance
package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// The Appearance screen's routes: someone's own themes, and the defaults an
// admin sets for everyone who has not made their own. See models/themes.go.
// Each save sends the whole list, so an undo is the same call with the list
// from before.

type setMyThemesRequest struct {
	// nil (or left out) means "follow the defaults".
	Themes  []models.Theme `json:"themes,omitempty" jsonschema:"nullable" jsonschema_description:"Your whole list, 1 to 40 themes. null goes back to following the app's defaults."`
	ThemeID string         `json:"theme_id,omitempty" jsonschema_description:"The theme you use."`
}

type setMyThemesReply struct {
	Ok           bool           `json:"ok"`
	Themes       []models.Theme `json:"themes" jsonschema:"nullable" jsonschema_description:"What you now see; null while the built-in themes apply."`
	ThemesCustom bool           `json:"themes_custom"`
	ThemeID      string         `json:"theme_id"`
}

type setMyThemeRequest struct {
	ThemeID string `json:"theme_id" jsonschema_description:"A theme's id. Empty for the first in the list."`
}

type setMyThemeReply struct {
	Ok      bool   `json:"ok"`
	ThemeID string `json:"theme_id"`
}

type themeDefaultsReply struct {
	Themes []models.Theme `json:"themes" jsonschema:"nullable" jsonschema_description:"null while the built-in themes apply."`
}

type setThemeDefaultsRequest struct {
	// nil (or left out) means "the built-in themes".
	Themes []models.Theme `json:"themes,omitempty" jsonschema:"nullable" jsonschema_description:"1 to 40 themes. null goes back to the built-in ones."`
}

type setThemeDefaultsReply struct {
	Ok     bool           `json:"ok"`
	Themes []models.Theme `json:"themes" jsonschema:"nullable"`
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
func ( handlers *Handlers ) SetMyThemes() *apidoc.Endpoint[ setMyThemesRequest , setMyThemesReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Replace your own themes" , Tag: "Account" , Feature: "appearance" , Errors: []int{ 400 , 401 , 403 } },
		func( c fiber.Ctx , in *setMyThemesRequest ) ( reply *setMyThemesReply , err error ) {
			user := security.UserFrom( c )
			if set_err := models.SetUserThemes( handlers.Store , user.ID , in.Themes , in.ThemeID ); set_err != nil {
				err = refuse( fiber.StatusBadRequest , set_err.Error() )
				return
			}
			user , err = models.GetUser( handlers.Store , user.ID )
			if err != nil { return }
			themes , custom , err := handlers.myThemes( user )
			if err != nil { return }
			handlers.audit( c , logs.KindAuth , "changed own themes" , fiber.Map{ "count": len( in.Themes ) , "custom": custom } )
			reply = &setMyThemesReply{ Ok: true , Themes: themes , ThemesCustom: custom , ThemeID: user.ThemeID }
			return
		} )
}

// SetMyTheme changes only which theme the caller uses.
func ( handlers *Handlers ) SetMyTheme() *apidoc.Endpoint[ setMyThemeRequest , setMyThemeReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Pick which theme you use" , Tag: "Account" , Feature: "appearance" , Errors: []int{ 400 , 401 , 403 } },
		func( c fiber.Ctx , in *setMyThemeRequest ) ( reply *setMyThemeReply , err error ) {
			if set_err := models.SetUserThemeID( handlers.Store , security.UserFrom( c ).ID , in.ThemeID ); set_err != nil {
				err = refuse( fiber.StatusBadRequest , set_err.Error() )
				return
			}
			reply = &setMyThemeReply{ Ok: true , ThemeID: in.ThemeID }
			return
		} )
}

// GetThemeDefaults: themes is null while the built-in ones apply.
func ( handlers *Handlers ) GetThemeDefaults() *apidoc.Endpoint[ apidoc.None , themeDefaultsReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "The themes everyone without their own list sees" , Tag: "Admin" , Role: apidoc.Admin , Feature: "appearance" , Errors: []int{ 401 , 403 , 404 } },
		func( c fiber.Ctx , in *apidoc.None ) ( reply *themeDefaultsReply , err error ) {
			themes , err := models.ThemeDefaults( handlers.Store )
			if err != nil { return }
			reply = &themeDefaultsReply{ Themes: themes }
			return
		} )
}

// SetThemeDefaults replaces the list everyone without their own list sees.
func ( handlers *Handlers ) SetThemeDefaults() *apidoc.Endpoint[ setThemeDefaultsRequest , setThemeDefaultsReply ] {
	return apidoc.Handle( apidoc.Op{ Summary: "Replace the default themes" , Tag: "Admin" , Role: apidoc.Admin , Feature: "appearance" , Errors: []int{ 400 , 401 , 403 , 404 } },
		func( c fiber.Ctx , in *setThemeDefaultsRequest ) ( reply *setThemeDefaultsReply , err error ) {
			if set_err := models.SetThemeDefaults( handlers.Store , in.Themes ); set_err != nil {
				err = refuse( fiber.StatusBadRequest , set_err.Error() )
				return
			}
			themes , err := models.ThemeDefaults( handlers.Store )
			if err != nil { return }
			handlers.audit( c , logs.KindAdmin , "changed the default themes" , fiber.Map{ "count": len( themes ) , "built_in": themes == nil } )
			reply = &setThemeDefaultsReply{ Ok: true , Themes: themes }
			return
		} )
}
