// whole-file feature:appearance
package routes

import (
	errors "errors"

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
	BuiltIns int           `json:"builtins,omitempty" jsonschema_description:"Which built-in themes this list has seen: the highest since in Theme.builtIn (static/js/theme.js) when it was saved. Built-in themes added after that join the list the browser shows. Left out: 0, the first ones."`
}

type setMyThemesReply struct {
	Ok           bool           `json:"ok"`
	Themes       []models.Theme `json:"themes" jsonschema:"nullable" jsonschema_description:"What you now see; null while the built-in themes apply."`
	ThemesCustom bool           `json:"themes_custom"`
	ThemesBuiltIns int          `json:"themes_builtins" jsonschema_description:"Which built-in themes that list has seen (see builtins on POST /api/account/themes)."`
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
	BuiltIns int          `json:"builtins" jsonschema_description:"Which built-in themes that list has seen (see builtins on POST /api/account/themes)."`
}

type setThemeDefaultsRequest struct {
	// nil (or left out) means "the built-in themes".
	Themes []models.Theme `json:"themes,omitempty" jsonschema:"nullable" jsonschema_description:"1 to 40 themes. null goes back to the built-in ones."`
	BuiltIns int          `json:"builtins,omitempty" jsonschema_description:"Which built-in themes this list has seen: the highest since in Theme.builtIn (static/js/theme.js) when it was saved. Built-in themes added after that join the list the browser shows. Left out: 0, the first ones."`
}

type setThemeDefaultsReply struct {
	Ok     bool           `json:"ok"`
	Themes []models.Theme `json:"themes" jsonschema:"nullable"`
	BuiltIns int          `json:"builtins" jsonschema_description:"Which built-in themes that list has seen (see builtins on POST /api/account/themes)."`
}

type setUserThemeRequest struct {
	userIDPath
	Theme models.Theme `json:"theme" jsonschema_description:"The theme to put on the account and make its pick, in the Theme schema. One with the same id replaces it."`
}

type setUserThemeReply struct {
	Ok      bool   `json:"ok"`
	ThemeID string `json:"theme_id" jsonschema_description:"The theme the account now uses."`
}

// myThemes is what /api/me says about themes. themes is null when neither the
// person nor an admin has saved a list: the browser has the built-in ones.
// builtins is which built-in themes that list has seen.
func ( handlers *Handlers ) myThemes( user *models.User ) ( themes []models.Theme , custom bool , builtins int , err error ) {
	if user.Themes != nil {
		themes = user.Themes
		custom = true
		builtins = user.ThemesBuiltIns
		return
	}
	themes , builtins , err = models.ThemeDefaults( handlers.Store )
	return
}

// setMyThemes replaces the caller's own list and pick.
//
// @route("POST /api/account/themes", summary="Replace your own themes",
//     tag="Account", feature="appearance",
//     description="Replace your whole list of color themes (1 to 40) and pick the one you use. themes: "
//         "null goes back to following the app's defaults. A theme is the Theme schema in the API "
//         "reference (config/api.json): id, name, mode light|dark, accent, second, and optional colors "
//         "as #rrggbb.",
//     mcp_note="whoami shows the current list.",  // feature:mcp
//     mcp="set_themes",  // feature:mcp
// )
func ( handlers *Handlers ) setMyThemes( c fiber.Ctx , in *setMyThemesRequest ) ( reply *setMyThemesReply , err error ) {
	user := security.UserFrom( c )
	if set_err := models.SetUserThemes( handlers.Store , user.ID , in.Themes , in.BuiltIns , in.ThemeID ); set_err != nil {
		err = refuse( fiber.StatusBadRequest , set_err.Error() )
		return
	}
	user , err = models.GetUser( handlers.Store , user.ID )
	if err != nil { return }
	themes , custom , builtins , err := handlers.myThemes( user )
	if err != nil { return }
	handlers.audit( c , logs.KindAuth , "changed own themes" , fiber.Map{ "count": len( in.Themes ) , "custom": custom } )
	reply = &setMyThemesReply{ Ok: true , Themes: themes , ThemesCustom: custom , ThemesBuiltIns: builtins , ThemeID: user.ThemeID }
	return
}

// setMyTheme changes only which theme the caller uses.
//
// @route("POST /api/account/theme", summary="Pick your theme",
//     tag="Account", feature="appearance",
//     description="Switch to another theme from your list (or the defaults), by its id.",
//     mcp="set_theme",  // feature:mcp
// )
func ( handlers *Handlers ) setMyTheme( c fiber.Ctx , in *setMyThemeRequest ) ( reply *setMyThemeReply , err error ) {
	if set_err := models.SetUserThemeID( handlers.Store , security.UserFrom( c ).ID , in.ThemeID ); set_err != nil {
		err = refuse( fiber.StatusBadRequest , set_err.Error() )
		return
	}
	reply = &setMyThemeReply{ Ok: true , ThemeID: in.ThemeID }
	return
}

// getThemeDefaults: themes is null while the built-in ones apply.
//
// @route("GET /api/admin/theme-defaults", summary="Default themes",
//     tag="Admin", role=ADMIN, feature="appearance", errors=(404,),
//     description="The theme list everyone without their own list sees. themes is null while the built-in "
//         "ones apply.",
//     mcp="admin_get_theme_defaults", read_only=True,  // feature:mcp
// )
func ( handlers *Handlers ) getThemeDefaults( c fiber.Ctx , in *apidoc.None ) ( reply *themeDefaultsReply , err error ) {
	themes , builtins , err := models.ThemeDefaults( handlers.Store )
	if err != nil { return }
	reply = &themeDefaultsReply{ Themes: themes , BuiltIns: builtins }
	return
}

// setThemeDefaults replaces the list everyone without their own list sees.
//
// @route("POST /api/admin/theme-defaults", summary="Replace the default themes",
//     tag="Admin", role=ADMIN, feature="appearance", errors=(404,),
//     description="Replace the theme list everyone without their own list sees, 1 to 40 themes in the "
//         "Theme schema of the API reference. null puts the built-in themes back.",
//     mcp="admin_set_theme_defaults",  // feature:mcp
// )
func ( handlers *Handlers ) setThemeDefaults( c fiber.Ctx , in *setThemeDefaultsRequest ) ( reply *setThemeDefaultsReply , err error ) {
	if set_err := models.SetThemeDefaults( handlers.Store , in.Themes , in.BuiltIns ); set_err != nil {
		err = refuse( fiber.StatusBadRequest , set_err.Error() )
		return
	}
	themes , builtins , err := models.ThemeDefaults( handlers.Store )
	if err != nil { return }
	handlers.audit( c , logs.KindAdmin , "changed the default themes" , fiber.Map{ "count": len( themes ) , "built_in": themes == nil } )
	reply = &setThemeDefaultsReply{ Ok: true , Themes: themes , BuiltIns: builtins }
	return
}

// setUserTheme puts one theme on someone else's account and makes it their
// pick: ProjectPortal's shared theme (portal contract capability user_theme).
// The list rules live in models.SetUserThemeAs, the ladder in mayActOnTx.
//
// @route("POST /api/admin/users/{user_id}/theme", summary="Give an account a theme",
//     tag="Admin", role=ADMIN, feature="appearance", errors=(404,),
//     description="Put one theme (the Theme schema) on another account and make it the one it uses. It starts "
//         "from the list the account sees now (its own, else the defaults, else the built-in ones), replaces "
//         "the theme with the same id or adds it at the end (at most 40), and saves that as the account's own "
//         "list. Only the owner does this to an admin, and nobody to the owner.",
//     mcp="admin_set_user_theme",  // feature:mcp
// )
func ( handlers *Handlers ) setUserTheme( c fiber.Ctx , in *setUserThemeRequest ) ( reply *setUserThemeReply , err error ) {
	if _ , clean_err := models.CleanThemes( []models.Theme{ in.Theme } ); clean_err != nil {
		err = refuse( fiber.StatusBadRequest , clean_err.Error() )
		return
	}
	theme_id , set_err := models.SetUserThemeAs( handlers.Store , security.UserFrom( c ).ID , in.UserID , in.Theme )
	if errors.Is( set_err , models.ErrThemesFull ) {
		err = refuse( fiber.StatusBadRequest , set_err.Error() )
		return
	}
	if refused := actOnRefusal( set_err , "change the theme of" ); refused != nil {
		err = refused
		return
	}
	handlers.audit( c , logs.KindAdmin , "gave an account a theme" , fiber.Map{ "user_id": in.UserID , "theme_id": theme_id } )
	reply = &setUserThemeReply{ Ok: true , ThemeID: theme_id }
	return
}
