package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for ready.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).GetReady , apidoc.Op{
	Summary: "Can the server do real work",
	Tag: "Monitoring",
	Access: apidoc.Public,
	Reply: getReadyReplyDoc{},
} )

type getReadyReplyDoc struct {
	Ok bool `json:"ok"`
}
