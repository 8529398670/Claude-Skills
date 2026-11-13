// whole-file feature:metrics
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for metrics.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).GetMetrics , apidoc.Op{
	Summary: "Prometheus metrics",
	Tag: "Monitoring",
	Access: apidoc.KeyOnly,
	Role: apidoc.Admin,
	Feature: "metrics",
	MCPHTTP: "Scraped by Prometheus over HTTP.",
	ReplyType: "text/plain",
	Errors: []int{ 401 , 403 , 404 },
} )
