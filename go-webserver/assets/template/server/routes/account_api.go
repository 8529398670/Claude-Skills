package routes

import (
	time "time"

	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for account.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).CreateMyLoginLink , apidoc.Op{
	Summary: "A login link for yourself",
	Tag: "Account",
	Body: createMyLoginLinkBodyDoc{},
	Reply: createMyLoginLinkReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type createMyLoginLinkBodyDoc struct {
	MaxUses int `json:"max_uses,omitempty" minimum:"0" maximum:"1000" doc:"Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1."`
	ExpiresInMinutes int `json:"expires_in_minutes,omitempty" minimum:"0" maximum:"525600" doc:"0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type createMyLoginLinkReplyDoc struct {
	LoginPath string `json:"login_path" doc:"Path of the login link. A credential: treat it like a password."`
	ExpiresInSeconds int `json:"expires_in_seconds" doc:"0 when it never expires."`
	ExpiresAt *time.Time `json:"expires_at" doc:"null when it never expires."`
	MaxUses int `json:"max_uses" doc:"0 when unlimited."`
	Unlimited bool `json:"unlimited"`
}

var _ = apidoc.Doc( (*Handlers).CancelMyLoginLink , apidoc.Op{
	Summary: "Stop your own login link working",
	Tag: "Account",
	Body: cancelMyLoginLinkBodyDoc{},
	Reply: cancelMyLoginLinkReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type cancelMyLoginLinkBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type cancelMyLoginLinkReplyDoc struct {
	Ok bool `json:"ok"`
	Removed int `json:"removed" doc:"Links removed: 0 or 1."`
}

// >>> feature:invite_links
var _ = apidoc.Doc( (*Handlers).SetPin , apidoc.Op{
	Summary: "Set or clear your PIN",
	Tag: "Account",
	Access: apidoc.SessionOnly,
	Feature: "invite_links",
	Body: setPinBodyDoc{},
	Reply: setPinReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type setPinBodyDoc struct {
	PIN string `json:"pin"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setPinReplyDoc struct {
	Ok bool `json:"ok"`
	HasPIN bool `json:"has_pin"`
}
// <<< feature:invite_links

var _ = apidoc.Doc( (*Handlers).RenameAccount , apidoc.Op{
	Summary: "Change your display name",
	Tag: "Account",
	Body: renameAccountBodyDoc{},
	Reply: renameAccountReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 409 },
} )

type renameAccountBodyDoc struct {
	DisplayName string `json:"display_name"`
	Expect *string `json:"expect,omitempty"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type renameAccountReplyDoc struct {
	Ok bool `json:"ok"`
	DisplayName string `json:"display_name"`
}

var _ = apidoc.Doc( (*Handlers).Logout , apidoc.Op{
	Summary: "Sign out",
	Tag: "Account",
	MCPHTTP: "Ends a browser session; a key has none to end.",
	Body: logoutBodyDoc{},
	Reply: logoutReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type logoutBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type logoutReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).GetMe , apidoc.Op{
	Summary: "Who am I",
	Tag: "Account",
	Reply: meSchema{},
	Errors: []int{ 401 },
} )
