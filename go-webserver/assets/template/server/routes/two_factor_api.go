// whole-file feature:two_factor
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for two_factor.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).ConfirmTwoFactor , apidoc.Op{
	Summary: "Turn two-factor on",
	Tag: "Account",
	Access: apidoc.SessionOnly,
	Feature: "two_factor",
	Body: confirmTwoFactorBodyDoc{},
	Reply: confirmTwoFactorReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type confirmTwoFactorBodyDoc struct {
	Code string `json:"code"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type confirmTwoFactorReplyDoc struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

var _ = apidoc.Doc( (*Handlers).DisableTwoFactor , apidoc.Op{
	Summary: "Turn two-factor off",
	Tag: "Account",
	Access: apidoc.SessionOnly,
	Feature: "two_factor",
	Body: disableTwoFactorBodyDoc{},
	Reply: disableTwoFactorReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type disableTwoFactorBodyDoc struct {
	Code string `json:"code"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type disableTwoFactorReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).StartTwoFactor , apidoc.Op{
	Summary: "Begin two-factor setup",
	Tag: "Account",
	Access: apidoc.SessionOnly,
	Feature: "two_factor",
	Body: startTwoFactorBodyDoc{},
	Reply: startTwoFactorReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type startTwoFactorBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type startTwoFactorReplyDoc struct {
	Secret string `json:"secret"`
}

var _ = apidoc.Doc( (*Handlers).FinishSignIn , apidoc.Op{
	Summary: "Finish a sign-in with a code",
	Tag: "Sign-in",
	Access: apidoc.Public,
	Feature: "two_factor",
	Body: finishSignInBodyDoc{},
	Reply: finishSignInReplyDoc{},
	Errors: []int{ 400 , 403 },
} )

type finishSignInBodyDoc struct {
	Challenge string `json:"challenge"`
	Code string `json:"code"`
}

type finishSignInReplyDoc struct {
	Ok bool `json:"ok"`
}
