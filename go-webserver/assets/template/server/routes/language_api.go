package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for language.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).GetHealth , apidoc.Op{
	Summary: "Is the process up",
	Tag: "Monitoring",
	Access: apidoc.Public,
	Reply: getHealthReplyDoc{},
} )

type getHealthReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).GetLanguage , apidoc.Op{
	Summary: "The UI's text",
	Tag: "Account",
	Access: apidoc.Public,
	Reply: getLanguageReplyDoc{},
} )

type getLanguageReplyDoc struct{}
