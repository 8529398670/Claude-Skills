#!/usr/bin/env python3
"""Rewrite the template's config/openapi.json, config/mcp.json and config/api.md.

They are written from the code, but the template itself cannot build: it is full
of {{PLACEHOLDERS}} that only scaffold.py fills in. So this scaffolds a project
with every part into a temporary folder, runs its scripts/api_docs.sh, and copies
the three files back with the project's name put back as {{PROJECT_NAME}}.

Run it after changing a route's declaration (apidoc.Handle), an MCP tool, or
server/apidoc. webserver-upgrade/scripts/check_features.py fails while the
template's copies are out of date. The python-webserver skill has the same
script, and the two templates' files are identical.

    python3 refresh_api_docs.py
"""
from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

SKILL_DIR = Path(__file__).resolve().parent.parent
TEMPLATE = SKILL_DIR / "assets" / "template"
NAME = "Refresh"
FILES = ("openapi.json", "mcp.json", "api.md")


def main() -> None:
    with tempfile.TemporaryDirectory() as folder:
        project = Path(folder) / "refresh"
        subprocess.run([sys.executable, "-B", str(SKILL_DIR / "scripts" / "scaffold.py"), str(project), NAME, "--prune", "none"], check=True, stdout=subprocess.DEVNULL)
        subprocess.run(["go", "mod", "tidy"], cwd=project, check=True)
        subprocess.run(["bash", "scripts/api_docs.sh"], cwd=project, check=True)
        for name in FILES:
            text = (project / "config" / name).read_text(encoding="utf-8")
            text = text.replace(f'"title": "{NAME} API"', '"title": "{{PROJECT_NAME}} API"').replace(f"# {NAME} API\n", "# {{PROJECT_NAME}} API\n", 1)
            (TEMPLATE / "config" / name).write_text(text, encoding="utf-8", newline="\n")
            print(f"wrote {TEMPLATE / 'config' / name}")


if __name__ == "__main__":
    if shutil.which("go") is None:
        sys.exit("needs Go")
    main()
