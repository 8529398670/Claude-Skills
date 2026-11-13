// whole-file feature:logs
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for logs.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).RecordUIEvents , apidoc.Op{
	Summary: "Record what happened on the page",
	Tag: "Account",
	Feature: "logs",
	MCPHTTP: "The browser's own activity reports (track.js), not an action.",
	Body: recordUIEventsBodyDoc{},
	Reply: recordUIEventsReplyDoc{},
	Errors: []int{ 400 , 401 , 403 , 404 },
} )

type recordUIEventsBodyDoc struct {
	Tab string `json:"tab"`
	Events []map[string]any `json:"events"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type recordUIEventsReplyDoc struct {
	Accepted int `json:"accepted"`
}

var _ = apidoc.Doc( (*Handlers).ListLogDays , apidoc.Op{
	Summary: "The days on record",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "logs",
	Params: listLogDaysParamsDoc{},
	Reply: logDaysSchema{},
	Errors: []int{ 404 },
} )

type listLogDaysParamsDoc struct {
	Q string `query:"q" doc:"The Logs screen's search."`
}

var _ = apidoc.Doc( (*Handlers).GetLogDay , apidoc.Op{
	Summary: "One day's entries",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "logs",
	Params: getLogDayParamsDoc{},
	Reply: logDaySchema{},
	Errors: []int{ 400 , 404 },
} )

type getLogDayParamsDoc struct {
	Q string `query:"q" doc:"The Logs screen's search."`
	Before int `query:"before"`
	Limit int `query:"limit"`
}

var _ = apidoc.Doc( (*Handlers).DeleteLogDay , apidoc.Op{
	Summary: "Delete one day",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "logs",
	Body: deleteLogDayBodyDoc{},
	Reply: deleteLogDayReplyDoc{},
	Errors: []int{ 400 , 403 , 404 , 409 },
} )

type deleteLogDayBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type deleteLogDayReplyDoc struct {
	Deleted bool `json:"deleted"`
}

var _ = apidoc.Doc( (*Handlers).DownloadLogDay , apidoc.Op{
	Summary: "One day's entries as a file",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "logs",
	ReplyType: "application/x-ndjson",
	Errors: []int{ 400 , 404 },
} )

var _ = apidoc.Doc( (*Handlers).PruneLogs , apidoc.Op{
	Summary: "Apply the retention rules now",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "logs",
	Body: pruneLogsBodyDoc{},
	Reply: pruneResultSchema{},
	Errors: []int{ 400 , 403 , 404 },
} )

type pruneLogsBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

var _ = apidoc.Doc( (*Handlers).StreamLogs , apidoc.Op{
	Summary: "New log entries as they happen",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "logs",
	Params: streamLogsParamsDoc{},
	ReplyType: "text/event-stream",
	Errors: []int{ 404 },
} )

type streamLogsParamsDoc struct {
	Q string `query:"q" doc:"The Logs screen's search."`
}

var _ = apidoc.Doc( (*Handlers).ListIPNames , apidoc.Op{
	Summary: "What the owner calls addresses",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "logs",
	Reply: ipNamesSchema{},
	Errors: []int{ 403 , 404 },
} )

var _ = apidoc.Doc( (*Handlers).SetIPName , apidoc.Op{
	Summary: "Name an address or a range, or rename one",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "logs",
	Body: setIPNameBodyDoc{},
	Reply: setIPNameReplyDoc{},
	Errors: []int{ 400 , 403 , 404 },
} )

type setIPNameBodyDoc struct {
	Target string `json:"target" example:"10.0.0.0/24" doc:"An address, or a range such as 10.0.0.0/24."`
	Name string `json:"name" maxLength:"80"`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type setIPNameReplyDoc struct {
	Target string `json:"target,omitempty"`
	Name string `json:"name,omitempty"`
}

var _ = apidoc.Doc( (*Handlers).DeleteIPName , apidoc.Op{
	Summary: "Take a name off an address or a range",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "logs",
	Body: deleteIPNameBodyDoc{},
	Reply: deleteIPNameReplyDoc{},
	Errors: []int{ 400 , 403 , 404 },
} )

type deleteIPNameBodyDoc struct {
	Target string `json:"target" example:"10.0.0.0/24" doc:"An address, or a range such as 10.0.0.0/24."`
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type deleteIPNameReplyDoc struct {
	Deleted bool `json:"deleted,omitempty"`
}
