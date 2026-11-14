#!/usr/bin/env bash
# Rewrite the API reference from the code:
#
#   config/openapi.json   every route, from its declaration beside its handler (apidoc.Handle)
#   config/mcp.json       every MCP tool, from server/routes/mcp_tools.go
#   config/api.md         both, as tables to read
#
# Run it after adding or changing a route or a tool, and commit the files with
# the change. go test ./server/routes fails while they are out of date
# (TestAPIDocsAreCurrent), and while a route is mounted without apidoc.Get/Post/Delete.
#
#   scripts/api_docs.sh
set -euo pipefail
cd "$( dirname "${BASH_SOURCE[0]}" )/.."
go test ./server/routes -run '^TestAPIDocsAreCurrent$' -count=1 -update
echo "wrote config/api.md and the JSON beside it"
