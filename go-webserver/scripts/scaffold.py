#!/usr/bin/env python3
"""Copy the bundled Go project template into a target directory and stamp the
project name and module path into it.

This is only the mechanical part of scaffolding -- copy plus find/replace, and
writing the project's config.yaml with a freshly generated secret key. It
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

import os
import re
import secrets
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


# The one line in defaults.yaml that config.yaml must differ on. Mirrors
# NewFileContent in server/config/config.go, which does the same thing for
# `config init` and for a binary's first run.
SECRET_KEY_LINE = re.compile(r'^(\s*secret_key:\s*)""', re.MULTILINE)


def write_config(target_dir: Path) -> Path:
    """Write <target>/config.yaml: the stamped defaults plus a fresh key.

    config.yaml is the project's one configuration file -- server, Docker,
    and build -- and it holds the secret key, so it is created at 0600 and is
    already in the template's .gitignore and .dockerignore.
    """
    defaults = (target_dir / "server" / "config" / "defaults.yaml").read_text(encoding="utf-8")
    content, count = SECRET_KEY_LINE.subn(
        lambda match: f'{match.group(1)}"{secrets.token_hex(32)}"', defaults
    )
    if count != 1:
        raise SystemExit(f"defaults.yaml must contain exactly one `secret_key: \"\"` line, found {count}")
    config_path = target_dir / "config.yaml"
    descriptor = os.open(config_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
        handle.write(content)
    return config_path


def write_example(target_dir: Path) -> Path:
    """Write <target>/config.example.yaml: the example header plus the stamped
    defaults, secret key left empty.

    It is the committed counterpart of config.yaml -- every setting with its
    default, nothing secret -- so a fresh clone has something to copy. It is
    regenerated here on every scaffold from the same two files that
    `go run . config example` uses, so it always matches the defaults it was
    scaffolded with; the copy in the template is only there to be read.
    """
    config_dir = target_dir / "server" / "config"
    header = (config_dir / "example_header.yaml").read_text(encoding="utf-8")
    defaults = (config_dir / "defaults.yaml").read_text(encoding="utf-8")
    example_path = target_dir / "config.example.yaml"
    example_path.write_text(header + defaults, encoding="utf-8")
    return example_path


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

    for script in (
        target_dir / "run.sh",
        target_dir / "dockerRun.sh",
        target_dir / "dockerBuild.sh",
        target_dir / "build.sh",
        target_dir / "scripts" / "vendor.sh",
    ):
        if script.exists():
            script.chmod(0o755)

    config_path = write_config(target_dir)
    example_path = write_example(target_dir)

    print(f"Scaffolded '{project_name}' into {target_dir}")
    print(f"  module {module_path}")
    print(f"  docker image/container name: {slugify(project_name)}")
    print(f"  config: {config_path} (mode 0600, fresh secret key, gitignored)")
    print(f"  example: {example_path} (no secret key, commit it -- `cp` it to config.yaml on a fresh clone)")
    print("")
    print("Next:")
    print(f"  cd {target_dir}")
    print("  go mod tidy")
    print("  ./run.sh            # builds if needed, (re)starts the server")
    print("  ./run.sh manage ... # admin CLI, works while it is running")


if __name__ == "__main__":
    main()
