// whole-file feature:passwords
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for passwords.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).SetPassword , apidoc.Op{
	Summary: "Set or clear your password",
	Tag: "Account",
	Access: apidoc.SessionOnly,
	Feature: "passwords",
	Body: setPasswordBodyDoc{},
	Reply: setPasswordReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type setPasswordBodyDoc struct {
	Username string `json:"username"`
	PasswordDigest string `json:"password_digest"`
	Code string `json:"code,omitempty"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setPasswordReplyDoc struct {
	Ok bool `json:"ok"`
	HasPassword bool `json:"has_password"`
}

var _ = apidoc.Doc( (*Handlers).GetLoginOptions , apidoc.Op{
	Summary: "What sign-in offers",
	Tag: "Sign-in",
	Access: apidoc.Public,
	Feature: "passwords",
	Reply: getLoginOptionsReplyDoc{},
} )

type getLoginOptionsReplyDoc struct {
	Passwords bool `json:"passwords"`
	TwoFactor bool `json:"two_factor"` // feature:two_factor
	Salt string `json:"salt,omitempty"`
}

var _ = apidoc.Doc( (*Handlers).PasswordLogin , apidoc.Op{
	Summary: "Sign in with a password",
	Tag: "Sign-in",
	Access: apidoc.Public,
	Feature: "passwords",
	Body: passwordLoginBodyDoc{},
	Reply: signInSchema{},
	Errors: []int{ 400 , 403 , 404 },
} )

type passwordLoginBodyDoc struct {
	Username string `json:"username"`
	PasswordDigest string `json:"password_digest"`
}

var _ = apidoc.Doc( (*Handlers).SetPasswordAccess , apidoc.Op{
	Summary: "Allow or block an account's password",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "passwords",
	Body: setPasswordAccessBodyDoc{},
	Reply: setPasswordAccessReplyDoc{},
	Errors: []int{ 400 , 403 , 404 },
} )

type setPasswordAccessBodyDoc struct {
	Allowed bool `json:"allowed"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setPasswordAccessReplyDoc struct {
	Ok bool `json:"ok"`
	PasswordBlocked bool `json:"password_blocked"`
}
