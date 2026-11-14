#!/usr/bin/env bash
# Rewrite the route table and the API reference from the code:
#
#   server/routes/routes_generated.go   the route table, from the // @route(...) declaration of each handler
#   config/openapi.json                 every route, from those declarations
#   config/mcp.json                     every MCP tool, from the mcp= in their routes' declarations
#   config/api.md                       both, as tables to read
#
# Run it after adding or changing a route or a tool, and commit the files with
# the change. go test ./server/routes fails while any of them is out of date
# (TestTheRouteTableIsCurrent, TestAPIDocsAreCurrent).
#
#   scripts/api_docs.sh
set -euo pipefail
cd "$( dirname "${BASH_SOURCE[0]}" )/.."
go generate ./server/routes
go test ./server/routes -run '^TestAPIDocsAreCurrent$' -count=1 -update
echo "wrote the route table, config/api.md and the JSON beside it"
