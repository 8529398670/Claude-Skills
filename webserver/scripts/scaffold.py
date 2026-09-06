#!/usr/bin/env python3
"""Copy the bundled project template into a target directory and stamp the
project name into it. This is the mechanical part of scaffolding (copy +
find/replace) -- it does not start the server or create any users, so the
person running the skill stays in control of that.

Usage:
    python3 scaffold.py <target-dir> <project-name>
"""
from __future__ import annotations

import shutil
import sys
from pathlib import Path

SKILL_DIR = Path(__file__).resolve().parent.parent
TEMPLATE_DIR = SKILL_DIR / "assets" / "template"

# Extensions where a literal find/replace of the token is safe. Binary
# assets should never go through text substitution.
TEXT_EXTENSIONS = {".py", ".html", ".css", ".js", ".sh", ".txt", ".md", ""}


def stamp(path: Path, project_name: str) -> None:
    if path.suffix not in TEXT_EXTENSIONS:
        return
    try:
        text = path.read_text()
    except UnicodeDecodeError:
        return
    if "{{PROJECT_NAME}}" not in text:
        return
    path.write_text(text.replace("{{PROJECT_NAME}}", project_name))


def main() -> None:
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(1)
    target_dir = Path(sys.argv[1]).resolve()
    project_name = sys.argv[2]

    if target_dir.exists() and any(target_dir.iterdir()):
        print(f"Refusing to scaffold into non-empty directory: {target_dir}")
        sys.exit(1)

    shutil.copytree(TEMPLATE_DIR, target_dir, dirs_exist_ok=True)

    for path in target_dir.rglob("*"):
        if path.is_file():
            stamp(path, project_name)

    dockerrun = target_dir / "dockerRun.sh"
    if dockerrun.exists():
        dockerrun.chmod(0o755)

    print(f"Scaffolded '{project_name}' into {target_dir}")


if __name__ == "__main__":
    main()
