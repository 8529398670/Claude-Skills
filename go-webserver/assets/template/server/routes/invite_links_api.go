// whole-file feature:invite_links
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for invite_links.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).ListInviteLinks , apidoc.Op{
	Summary: "Every invite link",
	Tag: "Invite links",
	Role: apidoc.Admin,
	Feature: "invite_links",
	Params: listInviteLinksParamsDoc{},
	Reply: listInviteLinksReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type listInviteLinksParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
	Q string `query:"q" doc:"Keep links whose label or maker contains this, in any case."`
	Sort string `query:"sort" enum:"label|used|expires_at" doc:"Field to sort the whole list by. Rows without it go last."`
	Dir string `query:"dir" enum:"asc|desc" doc:"Default asc."`
}

type listInviteLinksReplyDoc struct {
	InviteLinks []inviteLinkSchema `json:"invite_links"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

var _ = apidoc.Doc( (*Handlers).CreateInviteLink , apidoc.Op{
	Summary: "Create an invite link",
	Tag: "Invite links",
	Role: apidoc.Admin,
	Feature: "invite_links",
	Body: createInviteLinkBodyDoc{},
	Status: 201,
	Reply: createInviteLinkReplyDoc{},
	Errors: []int{ 400 , 401 , 403 },
} )

type createInviteLinkBodyDoc struct {
	Label string `json:"label"`
	MaxUses int `json:"max_uses"`
	Days int `json:"days"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type createInviteLinkReplyDoc struct {
	inviteLinkSchema
	joinPathSchema
}

var _ = apidoc.Doc( (*Handlers).DeleteInviteLink , apidoc.Op{
	Summary: "Delete an invite link",
	Tag: "Invite links",
	Role: apidoc.Admin,
	Feature: "invite_links",
	Body: deleteInviteLinkBodyDoc{},
	Reply: deleteInviteLinkReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type deleteInviteLinkBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type deleteInviteLinkReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).RestoreInviteLink , apidoc.Op{
	Summary: "Undo deleting an invite link",
	Tag: "Invite links",
	Role: apidoc.Admin,
	Feature: "invite_links",
	Body: restoreInviteLinkBodyDoc{},
	Reply: inviteLinkSchema{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type restoreInviteLinkBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).SetInviteLinkLimits , apidoc.Op{
	Summary: "Change a link's places and expiry",
	Tag: "Invite links",
	Role: apidoc.Admin,
	Feature: "invite_links",
	Body: setInviteLinkLimitsBodyDoc{},
	Reply: setInviteLinkLimitsReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type setInviteLinkLimitsBodyDoc struct {
	MaxUses int `json:"max_uses" minimum:"1" maximum:"1000" doc:"Not fewer than have joined."`
	Days int `json:"days,omitempty" minimum:"1" maximum:"365" doc:"Counted from now. Leave out to keep the expiry."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setInviteLinkLimitsReplyDoc struct{}

func ( setInviteLinkLimitsReplyDoc ) APIDoc() ( text string ) { text = "The link, as in the list."; return }

var _ = apidoc.Doc( (*Handlers).RenameInviteLink , apidoc.Op{
	Summary: "Rename a link",
	Tag: "Invite links",
	Role: apidoc.Admin,
	Feature: "invite_links",
	Body: renameInviteLinkBodyDoc{},
	Reply: renameInviteLinkReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type renameInviteLinkBodyDoc struct {
	Label string `json:"label" doc:"1-80 characters."`
	Expect string `json:"expect,omitempty" doc:"The label you think it has. Answers 409 if it has another."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type renameInviteLinkReplyDoc struct {
	Ok bool `json:"ok"`
	Label string `json:"label"`
}

var _ = apidoc.Doc( (*Handlers).SwitchOnInviteLink , apidoc.Op{
	Summary: "Switch a link back on",
	Tag: "Invite links",
	Role: apidoc.Admin,
	Feature: "invite_links",
	Body: switchOnInviteLinkBodyDoc{},
	Reply: switchOnInviteLinkReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type switchOnInviteLinkBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type switchOnInviteLinkReplyDoc struct{}

func ( switchOnInviteLinkReplyDoc ) APIDoc() ( text string ) { text = "The link, as in the list."; return }

var _ = apidoc.Doc( (*Handlers).RevokeInviteLink , apidoc.Op{
	Summary: "Switch an invite link off",
	Tag: "Invite links",
	Role: apidoc.Admin,
	Feature: "invite_links",
	Body: revokeInviteLinkBodyDoc{},
	Reply: revokeInviteLinkReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type revokeInviteLinkBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type revokeInviteLinkReplyDoc struct {
	Ok bool `json:"ok"`
}
