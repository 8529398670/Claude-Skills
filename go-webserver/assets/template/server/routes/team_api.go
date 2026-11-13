package routes

import (
	apidoc "{{MODULE_PATH}}/server/apidoc"
)

// The API docs for team.go's routes: config/openapi.json and config/api.md
// are written from these by scripts/api_docs.sh (see server/apidoc).

var _ = apidoc.Doc( (*Handlers).ListTeam , apidoc.Op{
	Summary: "Everyone's names",
	Tag: "Account",
	Params: listTeamParamsDoc{},
	Reply: listTeamReplyDoc{},
	Errors: []int{ 400 , 401 },
} )

type listTeamParamsDoc struct {
	Limit int `query:"limit" minimum:"1" maximum:"1000" doc:"Rows to return. Default 200."`
	Offset int `query:"offset" minimum:"0"`
}

type listTeamReplyDoc struct {
	Members []memberSchema `json:"members"`
	Next *int `json:"next" doc:"Offset of the next page, or null on the last one."`
	Total int `json:"total"`
}
