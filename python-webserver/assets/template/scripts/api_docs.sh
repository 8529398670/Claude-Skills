#!/usr/bin/env bash
# Rewrite the API reference from the code:
#
#   config/openapi.json   every route, from the doc() calls in app/routes/*_api.py
#   config/mcp.json       every MCP tool, from app/routes/mcp_tools.py
#   config/api.md         both, as tables to read
#
# Run it after adding or changing a route or a tool, and commit the files with
# the change. python -m unittest discover app/tests fails while they are out of
# date (test_api_docs.py), and while a route has no docs.
#
#   scripts/api_docs.sh
set -euo pipefail
cd "$( dirname "${BASH_SOURCE[0]}" )/.."
python=.venv/bin/python
[ -x "$python" ] || python=python3
"$python" -m app.apidocs
