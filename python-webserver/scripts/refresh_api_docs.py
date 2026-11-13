#!/usr/bin/env python3
"""Rewrite the template's config/openapi.json, config/mcp.json and config/api.md.

They are written from the code, but the template itself cannot run: it is full
of {{PLACEHOLDERS}} that only scaffold.py fills in. So this scaffolds a project
with every part into a temporary folder, writes its API reference the way its
scripts/api_docs.sh does, and copies the three files back with the project's
name put back as {{PROJECT_NAME}}.

Run it after changing a route's docs (app/routes/*_api.py), an MCP tool, or
app/apidocs. webserver-upgrade/scripts/check_features.py fails while the
template's copies are out of date. The go-webserver skill has the same script,
and the two templates' files are identical.

    python3 refresh_api_docs.py [--python PATH]

--python is an interpreter with the app's requirements and httpx2; the default
is the skill's demo virtualenv.
"""
from __future__ import annotations

import argparse
import subprocess
import sys
import tempfile
from pathlib import Path

SKILL_DIR = Path(__file__).resolve().parent.parent
TEMPLATE = SKILL_DIR / "assets" / "template"
NAME = "Refresh"
FILES = ("openapi.json", "mcp.json", "api.md")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--python", default=str(Path.home() / ".cache" / "python-webserver-demo" / ".venv" / "bin" / "python"))
    arguments = parser.parse_args()
    with tempfile.TemporaryDirectory() as folder:
        project = Path(folder) / "refresh"
        subprocess.run([sys.executable, "-B", str(SKILL_DIR / "scripts" / "scaffold.py"), str(project), NAME, "--prune", "none"], check=True, stdout=subprocess.DEVNULL)
        subprocess.run([arguments.python, "-m", "app.apidocs"], cwd=project, check=True)
        for name in FILES:
            text = (project / "config" / name).read_text(encoding="utf-8")
            text = text.replace(f'"title": "{NAME} API"', '"title": "{{PROJECT_NAME}} API"').replace(f"# {NAME} API\n", "# {{PROJECT_NAME}} API\n", 1)
            (TEMPLATE / "config" / name).write_text(text, encoding="utf-8", newline="\n")
            print(f"wrote {TEMPLATE / 'config' / name}")


if __name__ == "__main__":
    main()
