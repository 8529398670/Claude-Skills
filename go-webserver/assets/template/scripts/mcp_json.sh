#!/usr/bin/env bash
# whole-file feature:mcp
# Rewrite config/mcp.json -- the MCP reference: every tool POST /mcp offers,
# its arguments, the route it stands for, and the routes that stay HTTP only --
# from the tools registered in server/routes/mcp_tools.go.
#
# Run it after adding or changing a tool, and commit the file with the change.
# go test ./server/routes fails while it is out of date
# (TestMCPManifestIsCurrent), the way it does for config/openapi.json.
#
#   scripts/mcp_json.sh
set -euo pipefail
cd "$( dirname "${BASH_SOURCE[0]}" )/.."
go test ./server/routes -run '^TestMCPManifestIsCurrent$' -count=1 -update
echo "wrote config/mcp.json"
