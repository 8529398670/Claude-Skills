#!/usr/bin/env python3
"""Copy the bundled Go project template into a target directory and stamp the
project name and module path into it.

This is only the mechanical part of scaffolding -- copy plus find/replace. It
deliberately does not run `go mod tidy`, build anything, start a server, or
create any users, so nothing touches the network or the outside world until
the person running it decides to.

Usage:
    python3 scaffold.py <target-dir> "<project-name>" [module-path]

If module-path is omitted it is derived from the project name, which is fine
for a private app. Pass a real path (github.com/you/thing) if the project will
ever be imported by something else.
"""
from __future__ import annotations

import re
import shutil
import sys
from pathlib import Path

SKILL_DIR = Path(__file__).resolve().parent.parent
TEMPLATE_DIR = SKILL_DIR / "assets" / "template"

# Substitution is a plain text replace, so it must only touch text files.
TEXT_SUFFIXES = {
    ".go", ".mod", ".sum", ".html", ".css", ".js", ".json",
    ".yaml", ".yml", ".sh", ".md", ".txt", "",
}
TEXT_NAMES = {"Dockerfile", ".dockerignore", ".gitignore"}


def slugify(name: str) -> str:
    """Turn a display name into something valid as a Go module path element."""
    slug = re.sub(r"[^a-z0-9]+", "-", name.strip().lower()).strip("-")
    return slug or "app"


def is_text_file(path: Path) -> bool:
    return path.suffix in TEXT_SUFFIXES or path.name in TEXT_NAMES


def stamp(path: Path, replacements: dict[str, str]) -> None:
    if not is_text_file(path):
        return
    try:
        text = path.read_text(encoding="utf-8")
    except (UnicodeDecodeError, OSError):
        return
    original = text
    for token, value in replacements.items():
        text = text.replace(token, value)
    if text != original:
        path.write_text(text, encoding="utf-8")


def main() -> None:
    if len(sys.argv) not in (3, 4):
        print(__doc__)
        sys.exit(1)

    target_dir = Path(sys.argv[1]).resolve()
    project_name = sys.argv[2]
    module_path = sys.argv[3] if len(sys.argv) == 4 else slugify(project_name)

    # Refusing here rather than merging is the point: scaffolding on top of
    # someone's existing work would silently overwrite files, and the mistake
    # is only obvious later.
    if target_dir.exists() and any(target_dir.iterdir()):
        print(f"Refusing to scaffold into non-empty directory: {target_dir}")
        sys.exit(1)

    # Skip editor/OS cruft rather than copying it into a fresh project. A
    # stray .DS_Store in the template would otherwise end up committed in
    # every scaffolded repo.
    shutil.copytree(
        TEMPLATE_DIR,
        target_dir,
        dirs_exist_ok=True,
        ignore=shutil.ignore_patterns(".DS_Store", "Thumbs.db", "*.swp", "__pycache__"),
    )

    replacements = {
        "{{PROJECT_NAME}}": project_name,
        "{{MODULE_PATH}}": module_path,
        # Docker rejects uppercase repository names, so anything that becomes
        # an image or container name uses the slug rather than the display
        # name. Otherwise a project called "Ledger" produces a dockerRun.sh
        # that fails with "repository name must be lowercase".
        "{{PROJECT_SLUG}}": slugify(project_name),
    }
    for path in sorted(target_dir.rglob("*")):
        if path.is_file():
            stamp(path, replacements)

    for script in (target_dir / "dockerRun.sh", target_dir / "scripts" / "vendor.sh"):
        if script.exists():
            script.chmod(0o755)

    print(f"Scaffolded '{project_name}' into {target_dir}")
    print(f"  module {module_path}")
    print(f"  docker image/container name: {slugify(project_name)}")
    print("")
    print("Next:")
    print(f"  cd {target_dir}")
    print("  go mod tidy && go build ./...")


if __name__ == "__main__":
    main()
