package routes

import (
	time "time"

	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for admin.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).ListUsers , apidoc.Op{
	Summary: "Every account",
	Tag: "Admin",
	Role: apidoc.Admin,
	Params: listUsersParamsDoc{},
	Reply: listUsersReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type listUsersParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
	Q string `query:"q" doc:"Keep accounts whose name or username contains this, in any case."`
	Sort string `query:"sort" enum:"display_name|role|disabled|created_at" doc:"Field to sort the whole list by. Rows without it go last."`
	Dir string `query:"dir" enum:"asc|desc" doc:"Default asc."`
}

type listUsersReplyDoc struct {
	Users []userSchema `json:"users"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

var _ = apidoc.Doc( (*Handlers).CreateUser , apidoc.Op{
	Summary: "Create an account and its login link",
	Tag: "Admin",
	Role: apidoc.Admin,
	Body: createUserBodyDoc{},
	Status: 201,
	Reply: createUserReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type createUserBodyDoc struct {
	DisplayName string `json:"display_name"`
	Role string `json:"role,omitempty" enum:"admin|user"`
	MaxUses int `json:"max_uses,omitempty" minimum:"0" maximum:"1000" doc:"Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1."`
	ExpiresInMinutes int `json:"expires_in_minutes,omitempty" minimum:"0" maximum:"525600" doc:"0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type createUserReplyDoc struct {
	UserID int `json:"user_id"`
	LoginPath string `json:"login_path" doc:"Path of the login link. A credential: treat it like a password."`
	ExpiresInSeconds int `json:"expires_in_seconds" doc:"0 when it never expires."`
	ExpiresAt *time.Time `json:"expires_at" doc:"null when it never expires."`
	MaxUses int `json:"max_uses" doc:"0 when unlimited."`
	Unlimited bool `json:"unlimited"`
}

var _ = apidoc.Doc( (*Handlers).DeleteUser , apidoc.Op{
	Summary: "Delete an account",
	Tag: "Admin",
	Role: apidoc.Admin,
	Body: deleteUserBodyDoc{},
	Reply: deleteUserReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type deleteUserBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type deleteUserReplyDoc struct {
	Ok bool `json:"ok"`
	Removed revokedSchema `json:"removed"`
}

var _ = apidoc.Doc( (*Handlers).RenameUser , apidoc.Op{
	Summary: "Rename someone else",
	Description: "Only the owner can rename an admin.",
	Tag: "Admin",
	Role: apidoc.Admin,
	Body: renameUserBodyDoc{},
	Reply: renameUserReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type renameUserBodyDoc struct {
	DisplayName string `json:"display_name" doc:"1-80 characters."`
	Expect string `json:"expect,omitempty" doc:"The name you think they have. Answers 409 if it is another."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type renameUserReplyDoc struct {
	Ok bool `json:"ok"`
	DisplayName string `json:"display_name"`
}

var _ = apidoc.Doc( (*Handlers).SetUserRole , apidoc.Op{
	Summary: "Change an account's role",
	Description: "Only the owner can take an admin's rights away. Becoming an admin ends the account's sessions and live login link.",
	Tag: "Admin",
	Role: apidoc.Admin,
	Body: setUserRoleBodyDoc{},
	Reply: setUserRoleReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type setUserRoleBodyDoc struct {
	Role string `json:"role" enum:"admin|user"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setUserRoleReplyDoc struct {
	Ok bool `json:"ok"`
	Role string `json:"role"`
}

var _ = apidoc.Doc( (*Handlers).SetUserDisabled , apidoc.Op{
	Summary: "Disable or enable an account",
	Tag: "Admin",
	Role: apidoc.Admin,
	Body: setUserDisabledBodyDoc{},
	Reply: setUserDisabledReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type setUserDisabledBodyDoc struct {
	Disabled bool `json:"disabled"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setUserDisabledReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).ReissueLogin , apidoc.Op{
	Summary: "A new login link for an account",
	Tag: "Admin",
	Role: apidoc.Admin,
	Body: reissueLoginBodyDoc{},
	Reply: reissueLoginReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type reissueLoginBodyDoc struct {
	MaxUses int `json:"max_uses,omitempty" minimum:"0" maximum:"1000" doc:"Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1."`
	ExpiresInMinutes int `json:"expires_in_minutes,omitempty" minimum:"0" maximum:"525600" doc:"0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type reissueLoginReplyDoc struct {
	LoginPath string `json:"login_path" doc:"Path of the login link. A credential: treat it like a password."`
	ExpiresInSeconds int `json:"expires_in_seconds" doc:"0 when it never expires."`
	ExpiresAt *time.Time `json:"expires_at" doc:"null when it never expires."`
	MaxUses int `json:"max_uses" doc:"0 when unlimited."`
	Unlimited bool `json:"unlimited"`
}

var _ = apidoc.Doc( (*Handlers).CancelLoginLink , apidoc.Op{
	Summary: "Stop an account's login link working",
	Tag: "Admin",
	Role: apidoc.Admin,
	Body: cancelLoginLinkBodyDoc{},
	Reply: cancelLoginLinkReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type cancelLoginLinkBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type cancelLoginLinkReplyDoc struct {
	Ok bool `json:"ok"`
	Removed int `json:"removed" doc:"Links removed: 0 or 1."`
}

// >>> feature:two_factor
var _ = apidoc.Doc( (*Handlers).ResetTwoFactor , apidoc.Op{
	Summary: "Turn off an account's two-factor",
	Tag: "Admin",
	Role: apidoc.Admin,
	Feature: "two_factor",
	Body: resetTwoFactorBodyDoc{},
	Reply: resetTwoFactorReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type resetTwoFactorBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type resetTwoFactorReplyDoc struct {
	Ok bool `json:"ok"`
}
// <<< feature:two_factor

var _ = apidoc.Doc( (*Handlers).RevokeAccess , apidoc.Op{
	Summary: "End every credential an account holds",
	Tag: "Admin",
	Role: apidoc.Admin,
	Body: revokeAccessBodyDoc{},
	Reply: revokeAccessReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type revokeAccessBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type revokeAccessReplyDoc struct {
	Ok bool `json:"ok"`
	Revoked revokedSchema `json:"revoked"`
}
