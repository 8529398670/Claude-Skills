// whole-file feature:jobs
package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for jobs.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).ListJobs , apidoc.Op{
	Summary: "Background jobs",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "jobs",
	Reply: listJobsReplyDoc{},
	Errors: []int{ 404 },
} )

type listJobsReplyDoc struct {
	Jobs []jobSchema `json:"jobs"`
}

var _ = apidoc.Doc( (*Handlers).RunJob , apidoc.Op{
	Summary: "Run a job now",
	Tag: "Owner",
	Access: apidoc.SessionOnly,
	Role: apidoc.Owner,
	Feature: "jobs",
	Body: runJobBodyDoc{},
	Status: 202,
	Reply: runJobReplyDoc{},
	Errors: []int{ 400 , 403 , 404 , 409 },
} )

type runJobBodyDoc struct {
	CSRFToken string `json:"csrf_token,omitempty" doc:"From GET /api/me. Browser sessions only."`
}

type runJobReplyDoc struct {
	Ok bool `json:"ok"`
}
