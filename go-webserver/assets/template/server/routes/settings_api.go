// whole-file feature:settings
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for settings.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).GetSettings , apidoc.Op{
	Summary: "config.yaml, as a form",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "settings",
	Reply: settingsSnapshotSchema{},
	Errors: []int{ 404 },
} )

var _ = apidoc.Doc( (*Handlers).SaveSettings , apidoc.Op{
	Summary: "Save settings",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "settings",
	Body: saveSettingsBodyDoc{},
	Reply: settingsResultSchema{},
	Errors: []int{ 400 , 403 , 404 , 409 },
} )

type saveSettingsBodyDoc struct {
	Version string `json:"version"`
	Changes map[string]any `json:"changes" doc:"Setting paths and their new values."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}
