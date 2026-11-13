// whole-file feature:shortcuts
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for shortcuts.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).SetMyShortcuts , apidoc.Op{
	Summary: "Change your keyboard shortcuts",
	Tag: "Account",
	Feature: "shortcuts",
	Body: setMyShortcutsBodyDoc{},
	Reply: setMyShortcutsReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type setMyShortcutsBodyDoc struct {
	Shortcuts map[string]string `json:"shortcuts" doc:"Action name to binding, such as \"search\": \"Mod+K\". Mod is Cmd on a Mac and Ctrl elsewhere. Only changed shortcuts are listed; an empty binding switches that key off. Replaces the whole set; {} puts every key back to its default."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setMyShortcutsReplyDoc struct {
	Ok bool `json:"ok"`
	Shortcuts map[string]string `json:"shortcuts"`
}
