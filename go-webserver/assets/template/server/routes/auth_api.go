package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for auth.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).RedeemLoginLink , apidoc.Op{
	Summary: "Redeem a login link",
	Tag: "Sign-in",
	Access: apidoc.Public,
	Body: redeemLoginLinkBodyDoc{},
	Reply: signInSchema{},
	Errors: []int{ 400 , 403 },
} )

type redeemLoginLinkBodyDoc struct {
	Credential string `json:"credential"`
}
