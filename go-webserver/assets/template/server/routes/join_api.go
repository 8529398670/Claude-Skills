// whole-file feature:invite_links
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for join.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).JoinClaim , apidoc.Op{
	Summary: "Sign in as an existing account",
	Tag: "Invite links",
	Access: apidoc.Public,
	Feature: "invite_links",
	Body: joinClaimBodyDoc{},
	Reply: joinedSchema{},
	Errors: []int{ 400 , 403 , 404 },
} )

type joinClaimBodyDoc struct {
	Credential string `json:"credential"`
	DisplayName string `json:"display_name,omitempty"`
	UserID int `json:"user_id,omitempty"`
	PIN string `json:"pin,omitempty"`
}

var _ = apidoc.Doc( (*Handlers).JoinCreate , apidoc.Op{
	Summary: "Join as a new account",
	Tag: "Invite links",
	Access: apidoc.Public,
	Feature: "invite_links",
	Body: joinCreateBodyDoc{},
	Status: 201,
	Reply: joinedSchema{},
	Errors: []int{ 400 , 404 , 409 },
} )

type joinCreateBodyDoc struct {
	Credential string `json:"credential"`
	DisplayName string `json:"display_name,omitempty"`
	UserID int `json:"user_id,omitempty"`
	PIN string `json:"pin,omitempty"`
}

var _ = apidoc.Doc( (*Handlers).JoinInfo , apidoc.Op{
	Summary: "What an invite link is",
	Tag: "Invite links",
	Access: apidoc.Public,
	Feature: "invite_links",
	Body: joinInfoBodyDoc{},
	Reply: joinInfoReplyDoc{},
	Errors: []int{ 404 },
} )

type joinInfoBodyDoc struct {
	Credential string `json:"credential"`
	DisplayName string `json:"display_name,omitempty"`
	UserID int `json:"user_id,omitempty"`
	PIN string `json:"pin,omitempty"`
}

type joinInfoReplyDoc struct {
	Label string `json:"label"`
	Remaining int `json:"remaining"`
}

var _ = apidoc.Doc( (*Handlers).JoinMatch , apidoc.Op{
	Summary: "Accounts matching a name",
	Tag: "Invite links",
	Access: apidoc.Public,
	Feature: "invite_links",
	Body: joinMatchBodyDoc{},
	Reply: joinMatchReplyDoc{},
	Errors: []int{ 400 , 404 },
} )

type joinMatchBodyDoc struct {
	Credential string `json:"credential"`
	DisplayName string `json:"display_name,omitempty"`
	UserID int `json:"user_id,omitempty"`
	PIN string `json:"pin,omitempty"`
}

type joinMatchReplyDoc struct {
	Matches []memberSchema `json:"matches"`
	CanCreate bool `json:"can_create"`
}
