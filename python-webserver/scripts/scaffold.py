#!/usr/bin/env python3
"""Copy the bundled project template into a target directory and stamp the
project name into it.

This is only the mechanical part of scaffolding -- copy plus find/replace, and
writing the project's config.yaml with a freshly generated secret key. It
deliberately does not create a venv, install anything, start a server, or
create any users, so nothing touches the network or the outside world until
the person running it decides to.

Usage:
    python3 scaffold.py <target-dir> "<project-name>"
"""
from __future__ import annotations

import os
import re
import secrets
import shutil
import sys
from pathlib import Path

SKILL_DIR = Path(__file__).resolve().parent.parent
TEMPLATE_DIR = SKILL_DIR / "assets" / "template"

# Substitution is a plain text replace, so it must only touch text files.
TEXT_SUFFIXES = {".py", ".html", ".css", ".js", ".json", ".yaml", ".yml", ".sh", ".md", ".txt", ""}
TEXT_NAMES = {"Dockerfile", ".dockerignore", ".gitignore"}

# The name lands inside Python and YAML string literals, so the characters
# that would end or escape one are refused up front rather than producing a
# project that does not parse.
UNSAFE_NAME = re.compile(r'["\\\n\r]')


def slugify(name: str) -> str:
    """The lowercase, filesystem- and Docker-safe form of the display name."""
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


# The one line in defaults.yaml that config.yaml must differ on. Mirrors
# new_file_content in app/core/config.py, which does the same thing for
# `config init` and for the server's first run.
SECRET_KEY_LINE = re.compile(r'^(\s*secret_key:\s*)""', re.MULTILINE)


def write_config(target_dir: Path) -> Path:
    """Write <target>/config.yaml: the stamped defaults plus a fresh key.

    config.yaml is the project's one configuration file -- server and Docker
    -- and it holds the secret key, so it is created at 0600 and is already in
    the template's .gitignore and .dockerignore.
    """
    defaults = (target_dir / "app" / "core" / "defaults.yaml").read_text(encoding="utf-8")
    content, count = SECRET_KEY_LINE.subn(lambda match: f'{match.group(1)}"{secrets.token_hex(32)}"', defaults)
    if count != 1:
        raise SystemExit(f'defaults.yaml must contain exactly one `secret_key: ""` line, found {count}')
    config_path = target_dir / "config.yaml"
    descriptor = os.open(config_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
        handle.write(content)
    return config_path


def main() -> None:
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(1)

    target_dir = Path(sys.argv[1]).resolve()
    project_name = sys.argv[2].strip()
    if not project_name or UNSAFE_NAME.search(project_name):
        print('The project name must be non-empty and contain no double quote, backslash, or newline.')
        sys.exit(1)

    # Refusing here rather than merging is the point: scaffolding on top of
    # someone's existing work would silently overwrite files, and the mistake
    # is only obvious later.
    if target_dir.exists() and any(target_dir.iterdir()):
        print(f"Refusing to scaffold into non-empty directory: {target_dir}")
        sys.exit(1)

    # Skip editor/OS cruft rather than copying it into a fresh project. A stray
    # .DS_Store in the template would otherwise end up committed in every
    # scaffolded repo.
    shutil.copytree(
        TEMPLATE_DIR,
        target_dir,
        dirs_exist_ok=True,
        ignore=shutil.ignore_patterns(".DS_Store", "Thumbs.db", "*.swp", "__pycache__", "*.pyc"),
    )

    slug = slugify(project_name)
    replacements = {
        "{{PROJECT_NAME}}": project_name,
        # Docker rejects uppercase repository names, and the slug also names
        # the app's state directory (~/.config/<slug>/), so anything that
        # becomes a path or an image/container name uses it rather than the
        # display name.
        "{{PROJECT_SLUG}}": slug,
    }
    for path in sorted(target_dir.rglob("*")):
        if path.is_file():
            stamp(path, replacements)

    for script in (
        target_dir / "dockerRun.sh",
        target_dir / "dockerBuild.sh",
        target_dir / "scripts" / "vendor.sh",
    ):
        if script.exists():
            script.chmod(0o755)

    config_path = write_config(target_dir)

    print(f"Scaffolded '{project_name}' into {target_dir}")
    print(f"  slug (state dir, docker image/container name): {slug}")
    print(f"  config: {config_path} (mode 0600, fresh secret key, gitignored)")
    print("")
    print("Next:")
    print(f"  cd {target_dir}")
    print("  python3 -m venv .venv && ./.venv/bin/pip install -r requirements.txt")
    print("  ./.venv/bin/python -m app")


if __name__ == "__main__":
    main()
