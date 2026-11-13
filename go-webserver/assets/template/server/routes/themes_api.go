// whole-file feature:appearance
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for themes.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).SetMyThemes , apidoc.Op{
	Summary: "Replace your own themes",
	Tag: "Account",
	Feature: "appearance",
	Body: setMyThemesBodyDoc{},
	Reply: setMyThemesReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type setMyThemesBodyDoc struct {
	Themes *[]themeSchema `json:"themes,omitempty" doc:"Your whole list, 1 to 40 themes. null goes back to following the app's defaults."`
	ThemeID string `json:"theme_id,omitempty" doc:"The theme you use."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setMyThemesReplyDoc struct {
	Ok bool `json:"ok"`
	Themes *[]themeSchema `json:"themes" doc:"What you now see; null while the built-in themes apply."`
	ThemesCustom bool `json:"themes_custom"`
	ThemeID string `json:"theme_id"`
}

var _ = apidoc.Doc( (*Handlers).SetMyTheme , apidoc.Op{
	Summary: "Pick which theme you use",
	Tag: "Account",
	Feature: "appearance",
	Body: setMyThemeBodyDoc{},
	Reply: setMyThemeReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type setMyThemeBodyDoc struct {
	ThemeID string `json:"theme_id" doc:"A theme's id. Empty for the first in the list."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setMyThemeReplyDoc struct {
	Ok bool `json:"ok"`
	ThemeID string `json:"theme_id"`
}

var _ = apidoc.Doc( (*Handlers).GetThemeDefaults , apidoc.Op{
	Summary: "The themes everyone without their own list sees",
	Tag: "Admin",
	Role: apidoc.Admin,
	Feature: "appearance",
	Reply: getThemeDefaultsReplyDoc{},
	Errors: []int{ 401 , 403 , 404 },
} )

type getThemeDefaultsReplyDoc struct {
	Themes *[]themeSchema `json:"themes" doc:"null while the built-in themes apply."`
}

var _ = apidoc.Doc( (*Handlers).SetThemeDefaults , apidoc.Op{
	Summary: "Replace the default themes",
	Tag: "Admin",
	Role: apidoc.Admin,
	Feature: "appearance",
	Body: setThemeDefaultsBodyDoc{},
	Reply: setThemeDefaultsReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type setThemeDefaultsBodyDoc struct {
	Themes *[]themeSchema `json:"themes,omitempty" doc:"1 to 40 themes. null goes back to the built-in ones."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setThemeDefaultsReplyDoc struct {
	Ok bool `json:"ok"`
	Themes *[]themeSchema `json:"themes"`
}
