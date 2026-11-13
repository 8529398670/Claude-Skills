// whole-file feature:sessions
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for sessions.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).ListMySessions , apidoc.Op{
	Summary: "Your signed-in devices",
	Tag: "Account",
	Feature: "sessions",
	Params: listMySessionsParamsDoc{},
	Reply: listMySessionsReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type listMySessionsParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
}

type listMySessionsReplyDoc struct {
	Sessions []sessionSchema `json:"sessions"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

var _ = apidoc.Doc( (*Handlers).RevokeMyOtherSessions , apidoc.Op{
	Summary: "Sign out every other device",
	Tag: "Account",
	Feature: "sessions",
	Body: revokeMyOtherSessionsBodyDoc{},
	Reply: revokeMyOtherSessionsReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type revokeMyOtherSessionsBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type revokeMyOtherSessionsReplyDoc struct {
	Ok bool `json:"ok"`
	Removed int `json:"removed"`
}

var _ = apidoc.Doc( (*Handlers).LabelMySession , apidoc.Op{
	Summary: "Name one of your devices",
	Description: "This device included. Empty clears the name.",
	Tag: "Account",
	Feature: "sessions",
	Body: labelMySessionBodyDoc{},
	Reply: labelMySessionReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type labelMySessionBodyDoc struct {
	Label string `json:"label" doc:"At most 80 characters."`
	Expect string `json:"expect,omitempty" doc:"The name you think it has. Answers 409 if it has another."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type labelMySessionReplyDoc struct {
	Ok bool `json:"ok"`
	Label string `json:"label"`
}

var _ = apidoc.Doc( (*Handlers).RevokeMySession , apidoc.Op{
	Summary: "Sign out one device",
	Tag: "Account",
	Feature: "sessions",
	Body: revokeMySessionBodyDoc{},
	Reply: revokeMySessionReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type revokeMySessionBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type revokeMySessionReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).ListUserSessions , apidoc.Op{
	Summary: "An account's signed-in devices",
	Description: "The owner's are the owner's alone, and only the owner sees addresses.",
	Tag: "Admin",
	Role: apidoc.Admin,
	Feature: "sessions",
	Params: listUserSessionsParamsDoc{},
	Reply: listUserSessionsReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type listUserSessionsParamsDoc struct {
	UserID int `path:"user_id"`
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
}

type listUserSessionsReplyDoc struct {
	Sessions []sessionSchema `json:"sessions"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

var _ = apidoc.Doc( (*Handlers).RevokeUserSession , apidoc.Op{
	Summary: "Sign out one of an account's devices",
	Tag: "Admin",
	Role: apidoc.Admin,
	Feature: "sessions",
	Params: revokeUserSessionParamsDoc{},
	Body: revokeUserSessionBodyDoc{},
	Reply: revokeUserSessionReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type revokeUserSessionParamsDoc struct {
	UserID int `path:"user_id"`
}

type revokeUserSessionBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type revokeUserSessionReplyDoc struct {
	Ok bool `json:"ok"`
}
