// whole-file feature:webhooks
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for webhooks.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).ListWebhooks , apidoc.Op{
	Summary: "Webhooks",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Params: listWebhooksParamsDoc{},
	Reply: listWebhooksReplyDoc{},
	Errors: []int{ 400 , 404 },
} )

type listWebhooksParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
	Q string `query:"q" doc:"Keep webhooks whose name or address contains this, in any case."`
	Sort string `query:"sort" enum:"name|url|enabled" doc:"Field to sort the whole list by. Rows without it go last."`
	Dir string `query:"dir" enum:"asc|desc" doc:"Default asc."`
}

type listWebhooksReplyDoc struct {
	Webhooks []webhookSchema `json:"webhooks"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

var _ = apidoc.Doc( (*Handlers).CreateWebhook , apidoc.Op{
	Summary: "Create a webhook",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Body: createWebhookBodyDoc{},
	Status: 201,
	Reply: createWebhookReplyDoc{},
	Errors: []int{ 400 , 403 , 404 },
} )

type createWebhookBodyDoc struct {
	Name string `json:"name"`
	URL string `json:"url"`
	Events []string `json:"events"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type createWebhookReplyDoc struct {
	webhookSchema
	Secret string `json:"secret" doc:"Shown in this response only."`
}

var _ = apidoc.Doc( (*Handlers).ListWebhookEvents , apidoc.Op{
	Summary: "Events a webhook can ask for",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Reply: listWebhookEventsReplyDoc{},
	Errors: []int{ 404 },
} )

type listWebhookEventsReplyDoc struct {
	Events []string `json:"events"`
}

var _ = apidoc.Doc( (*Handlers).DeleteWebhook , apidoc.Op{
	Summary: "Delete a webhook",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Body: deleteWebhookBodyDoc{},
	Reply: deleteWebhookReplyDoc{},
	Errors: []int{ 400 , 403 , 404 },
} )

type deleteWebhookBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type deleteWebhookReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).ListWebhookDeliveries , apidoc.Op{
	Summary: "Recent deliveries",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Params: listWebhookDeliveriesParamsDoc{},
	Reply: listWebhookDeliveriesReplyDoc{},
	Errors: []int{ 400 , 404 },
} )

type listWebhookDeliveriesParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
}

type listWebhookDeliveriesReplyDoc struct {
	Deliveries []webhookDeliverySchema `json:"deliveries"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}

var _ = apidoc.Doc( (*Handlers).SetWebhookEnabled , apidoc.Op{
	Summary: "Switch a webhook on or off",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Body: setWebhookEnabledBodyDoc{},
	Reply: setWebhookEnabledReplyDoc{},
	Errors: []int{ 400 , 403 , 404 },
} )

type setWebhookEnabledBodyDoc struct {
	Enabled bool `json:"enabled"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setWebhookEnabledReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).RenameWebhook , apidoc.Op{
	Summary: "Rename a webhook",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Body: renameWebhookBodyDoc{},
	Reply: renameWebhookReplyDoc{},
	Errors: []int{ 400 , 403 , 404 , 409 },
} )

type renameWebhookBodyDoc struct {
	Name string `json:"name" doc:"1-80 characters."`
	Expect string `json:"expect,omitempty" doc:"The name you think it has. Answers 409 if it has another."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type renameWebhookReplyDoc struct {
	Ok bool `json:"ok"`
	Name string `json:"name"`
}

var _ = apidoc.Doc( (*Handlers).RestoreWebhook , apidoc.Op{
	Summary: "Undo deleting a webhook",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Body: restoreWebhookBodyDoc{},
	Reply: webhookSchema{},
	Errors: []int{ 400 , 403 , 404 , 409 },
} )

type restoreWebhookBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).RotateWebhookSecret , apidoc.Op{
	Summary: "Replace a webhook's signing secret",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Body: rotateWebhookSecretBodyDoc{},
	Reply: rotateWebhookSecretReplyDoc{},
	Errors: []int{ 400 , 403 , 404 },
} )

type rotateWebhookSecretBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type rotateWebhookSecretReplyDoc struct {
	Secret string `json:"secret" doc:"Shown only here. The old one has stopped working."`
}

var _ = apidoc.Doc( (*Handlers).SetWebhookTarget , apidoc.Op{
	Summary: "Change a webhook's address and events",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Body: setWebhookTargetBodyDoc{},
	Reply: setWebhookTargetReplyDoc{},
	Errors: []int{ 400 , 403 , 404 , 409 },
} )

type setWebhookTargetBodyDoc struct {
	URL string `json:"url" doc:"Checked like a new webhook's."`
	Events []string `json:"events"`
	Expect setWebhookTargetBodyDocExpect `json:"expect,omitempty" doc:"What you think it has now. Answers 409 if it has something else."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setWebhookTargetBodyDocExpect struct {
	URL string `json:"url"`
	Events []string `json:"events"`
}

type setWebhookTargetReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).TestWebhook , apidoc.Op{
	Summary: "Send a test event",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Body: testWebhookBodyDoc{},
	Status: 202,
	Reply: testWebhookReplyDoc{},
	Errors: []int{ 400 , 403 , 404 },
} )

type testWebhookBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type testWebhookReplyDoc struct {
	Ok bool `json:"ok"`
}

var _ = apidoc.Doc( (*Handlers).ResendWebhookDelivery , apidoc.Op{
	Summary: "Send a failed delivery again",
	Description: "A new delivery with the same body. The failed one stays listed.",
	Tag: "Webhooks",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "webhooks",
	Body: resendWebhookDeliveryBodyDoc{},
	Status: 202,
	Reply: resendWebhookDeliveryReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 , 409 },
} )

type resendWebhookDeliveryBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type resendWebhookDeliveryReplyDoc struct {
	Ok bool `json:"ok"`
}
