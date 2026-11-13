// whole-file feature:api_keys
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for api_keys.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).ListAllAPIKeys , apidoc.Op{
	Summary: "Every key in the app",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "api_keys",
	Params: listAllAPIKeysParamsDoc{},
	Reply: listAllAPIKeysReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type listAllAPIKeysParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
	Q string `query:"q" doc:"Keep keys whose name or owner contains this, in any case."`
	Live string `query:"live" enum:"true" doc:"Leave out revoked and expired keys."`
	Sort string `query:"sort" enum:"display_name|name|role|last_used_at|expires_at|created_at" doc:"Field to sort the whole list by. Rows without it go last."`
	Dir string `query:"dir" enum:"asc|desc" doc:"Default asc."`
}

type listAllAPIKeysReplyDoc struct {
	Keys []apiKeySchema `json:"keys"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

var _ = apidoc.Doc( (*Handlers).CreateAPIKeyForUser , apidoc.Op{
	Summary: "Create a key that acts as someone else",
	Description: "Any admin may do this for an ordinary user, and only the owner for another admin. Nobody may do it for the owner. The key records who made it and shows that to the person it acts as.",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "api_keys",
	Body: createAPIKeyForUserBodyDoc{},
	Status: 201,
	Reply: newAPIKeySchema{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type createAPIKeyForUserBodyDoc struct {
	UserID int `json:"user_id"`
	Name string `json:"name" minLength:"1" maxLength:"80"`
	Role string `json:"role,omitempty" enum:"admin|user" default:"user"`
	ExpiresInDays *int `json:"expires_in_days,omitempty" minimum:"0" maximum:"3650" doc:"Absent or null: the configured default. 0: never expires."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).DeleteAnyAPIKey , apidoc.Op{
	Summary: "Delete anyone's key",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "api_keys",
	Body: deleteAnyAPIKeyBodyDoc{},
	Reply: deleteAnyAPIKeyReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type deleteAnyAPIKeyBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type deleteAnyAPIKeyReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).RestoreAnyAPIKey , apidoc.Op{
	Summary: "Undo deleting anyone's key",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "api_keys",
	Body: restoreAnyAPIKeyBodyDoc{},
	Reply: apiKeySchema{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type restoreAnyAPIKeyBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).SetAnyAPIKeyExpiry , apidoc.Op{
	Summary: "Change when anyone's key expires",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "api_keys",
	Body: setAnyAPIKeyExpiryBodyDoc{},
	Reply: setAnyAPIKeyExpiryReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type setAnyAPIKeyExpiryBodyDoc struct {
	ExpiresInDays int `json:"expires_in_days" minimum:"0" maximum:"3650" doc:"Counted from now. 0 means it never expires."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setAnyAPIKeyExpiryReplyDoc struct{}

func ( setAnyAPIKeyExpiryReplyDoc ) APIDoc() ( text string ) { text = "The key, as in the list."; return }

var _ = apidoc.Doc( (*Handlers).RenameAnyAPIKey , apidoc.Op{
	Summary: "Rename anyone's key",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "api_keys",
	Body: renameAnyAPIKeyBodyDoc{},
	Reply: renameAnyAPIKeyReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type renameAnyAPIKeyBodyDoc struct {
	Name string `json:"name" doc:"1-80 characters."`
	Expect string `json:"expect,omitempty" doc:"The name you think it has. Answers 409 if it has another."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type renameAnyAPIKeyReplyDoc struct {
	Ok bool `json:"ok"`
	Name string `json:"name"`
}

var _ = apidoc.Doc( (*Handlers).RevokeAnyAPIKey , apidoc.Op{
	Summary: "Revoke anyone's key",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Role: apidoc.Admin,
	Feature: "api_keys",
	Body: revokeAnyAPIKeyBodyDoc{},
	Reply: revokeAnyAPIKeyReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type revokeAnyAPIKeyBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type revokeAnyAPIKeyReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).ListMyAPIKeys , apidoc.Op{
	Summary: "Your API keys",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Feature: "api_keys",
	Params: listMyAPIKeysParamsDoc{},
	Reply: listMyAPIKeysReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type listMyAPIKeysParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
	Q string `query:"q" doc:"Keep keys whose name contains this, in any case."`
	Live string `query:"live" enum:"true" doc:"Leave out revoked and expired keys."`
	Sort string `query:"sort" enum:"name|role|last_used_at|expires_at|created_at" doc:"Field to sort the whole list by. Rows without it go last."`
	Dir string `query:"dir" enum:"asc|desc" doc:"Default asc."`
}

type listMyAPIKeysReplyDoc struct {
	Keys []apiKeySchema `json:"keys"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

var _ = apidoc.Doc( (*Handlers).CreateMyAPIKey , apidoc.Op{
	Summary: "Create an API key",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Feature: "api_keys",
	Body: createMyAPIKeyBodyDoc{},
	Status: 201,
	Reply: newAPIKeySchema{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type createMyAPIKeyBodyDoc struct {
	Name string `json:"name" minLength:"1" maxLength:"80"`
	Role string `json:"role,omitempty" enum:"admin|user" default:"user"`
	ExpiresInDays *int `json:"expires_in_days,omitempty" minimum:"0" maximum:"3650" doc:"Absent or null: the configured default. 0: never expires."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).DeleteMyAPIKey , apidoc.Op{
	Summary: "Delete one of your keys",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Feature: "api_keys",
	Body: deleteMyAPIKeyBodyDoc{},
	Reply: deleteMyAPIKeyReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type deleteMyAPIKeyBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type deleteMyAPIKeyReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).RestoreMyAPIKey , apidoc.Op{
	Summary: "Undo deleting one of your keys",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Feature: "api_keys",
	Body: restoreMyAPIKeyBodyDoc{},
	Reply: apiKeySchema{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type restoreMyAPIKeyBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).SetMyAPIKeyExpiry , apidoc.Op{
	Summary: "Change when one of your keys expires",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Feature: "api_keys",
	Body: setMyAPIKeyExpiryBodyDoc{},
	Reply: setMyAPIKeyExpiryReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type setMyAPIKeyExpiryBodyDoc struct {
	ExpiresInDays int `json:"expires_in_days" minimum:"0" maximum:"3650" doc:"Counted from now. 0 means it never expires."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setMyAPIKeyExpiryReplyDoc struct{}

func ( setMyAPIKeyExpiryReplyDoc ) APIDoc() ( text string ) { text = "The key, as in the list."; return }

var _ = apidoc.Doc( (*Handlers).RenameMyAPIKey , apidoc.Op{
	Summary: "Rename one of your keys",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Feature: "api_keys",
	Body: renameMyAPIKeyBodyDoc{},
	Reply: renameMyAPIKeyReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type renameMyAPIKeyBodyDoc struct {
	Name string `json:"name" doc:"1-80 characters."`
	Expect string `json:"expect,omitempty" doc:"The name you think it has. Answers 409 if it has another."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type renameMyAPIKeyReplyDoc struct {
	Ok bool `json:"ok"`
	Name string `json:"name"`
}

var _ = apidoc.Doc( (*Handlers).RevokeMyAPIKey , apidoc.Op{
	Summary: "Revoke one of your keys",
	Tag: "API keys",
	Access: apidoc.SessionOnly,
	Feature: "api_keys",
	Body: revokeMyAPIKeyBodyDoc{},
	Reply: revokeMyAPIKeyReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type revokeMyAPIKeyBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type revokeMyAPIKeyReplyDoc struct {
	Ok bool `json:"ok"`
}
