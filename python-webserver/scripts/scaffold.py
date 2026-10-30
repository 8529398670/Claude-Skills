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
import socket
import sys
from pathlib import Path

SKILL_DIR = Path(__file__).resolve().parent.parent
TEMPLATE_DIR = SKILL_DIR / "assets" / "template"

# Substitution is a plain text replace, so it must only touch text files.
TEXT_SUFFIXES = {".py", ".html", ".css", ".js", ".json", ".yaml", ".yml", ".sh", ".bat", ".md", ".txt", ""}
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
        # Bytes, not read_text(): universal newlines would turn run.bat's CRLF
        # line endings into LF, and cmd.exe misreads labels in an LF batch file.
        text = path.read_bytes().decode("utf-8")
    except (UnicodeDecodeError, OSError):
        return
    original = text
    for token, value in replacements.items():
        text = text.replace(token, value)
    if text != original:
        path.write_bytes(text.encode("utf-8"))


# The one line in defaults.yaml that config.yaml must differ on. Mirrors
# new_file_content in app/core/config.py, which does the same thing for
# `config init` and for the server's first run.
SECRET_KEY_LINE = re.compile(r'^(\s*secret_key:\s*)""', re.MULTILINE)


def write_config(target_dir: Path) -> Path:
    """Write <target>/config/config.yaml: the stamped defaults plus a fresh key.

    config.yaml is the project's one configuration file -- server and Docker
    -- and it holds the secret key, so it is created at 0600 and is already in
    the template's .gitignore and .dockerignore.
    """
    defaults = (target_dir / "config" / "defaults.yaml").read_text(encoding="utf-8")
    content, count = SECRET_KEY_LINE.subn(lambda match: f'{match.group(1)}"{secrets.token_hex(32)}"', defaults)
    if count != 1:
        raise SystemExit(f'defaults.yaml must contain exactly one `secret_key: ""` line, found {count}')
    config_path = target_dir / "config" / "config.yaml"
    descriptor = os.open(config_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
        handle.write(content)
    return config_path


def write_example(target_dir: Path) -> Path:
    """Write <target>/config/config.example.yaml: the example header plus the stamped
    defaults, secret key left empty.

    It is the committed counterpart of config.yaml -- every setting with its
    default, nothing secret -- so a fresh clone has something to copy. It is
    regenerated here on every scaffold from the same two files that
    `python -m app config example` uses, so it always matches the defaults
    it was scaffolded with; the copy in the template is only there to be
    read.
    """
    config_dir = target_dir / "config"
    header = (config_dir / "example_header.yaml").read_text(encoding="utf-8")
    defaults = (config_dir / "defaults.yaml").read_text(encoding="utf-8")
    example_path = config_dir / "config.example.yaml"
    example_path.write_text(header + defaults, encoding="utf-8")
    return example_path


# Every project gets its own port, chosen once here and written into the
# project's defaults.yaml, so several local projects can run side by side and
# each keeps its own browser tab, theme, and sign-in. (The session cookie is
# named per project too -- browsers share cookies across ports on one host.)
# Writing defaults.yaml rather than only config.yaml means config.yaml,
# config.example.yaml, `config init`, and a fresh clone all agree on it.
PORT_RANGE = (20000, 39999)
PORT_LINES = (
    re.compile(r"^(\s*port:\s*)\d+[ \t]*$", re.MULTILINE),
    re.compile(r"^(\s*host_port:\s*)\d+[ \t]*$", re.MULTILINE),
)


def pick_port() -> int:
    """A random port in PORT_RANGE that nothing on this machine is listening on."""
    for _ in range(50):
        port = secrets.choice(range(PORT_RANGE[0], PORT_RANGE[1] + 1))
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
            try:
                probe.bind(("127.0.0.1", port))
            except OSError:
                continue
        return port
    raise SystemExit(f"Could not find a free port in {PORT_RANGE[0]}-{PORT_RANGE[1]}")


def stamp_port(target_dir: Path, port: int) -> None:
    """Set server.port and docker.host_port in <target>/config/defaults.yaml."""
    defaults_path = target_dir / "config" / "defaults.yaml"
    text = defaults_path.read_text(encoding="utf-8")
    for pattern in PORT_LINES:
        text, count = pattern.subn(lambda match: f"{match.group(1)}{port}", text)
        if count != 1:
            raise SystemExit(f"defaults.yaml must contain exactly one `{pattern.pattern}` line, found {count}")
    defaults_path.write_text(text, encoding="utf-8")


# What a scripts/run.sh or docker/run.sh run inside the template itself can
# leave behind: runtime state and the secret-bearing config/config.yaml. None
# of it belongs in a new project. Paths are relative to the template root.
RUN_LEFTOVERS = {"config/config.yaml", ".run", ".venv", "data"}
CRUFT = shutil.ignore_patterns(".DS_Store", "Thumbs.db", "*.swp", "__pycache__", "*.pyc")


def ignore_cruft(directory: str, names: list[str]) -> set[str]:
    ignored = set(CRUFT(directory, names))
    here = Path(directory).resolve()
    if here == TEMPLATE_DIR or TEMPLATE_DIR in here.parents:
        relative = here.relative_to(TEMPLATE_DIR)
        ignored |= {name for name in names if (relative / name).as_posix() in RUN_LEFTOVERS}
    return ignored


# scripts/run.sh and scripts/run.bat carry a block, between these two marker
# lines, that turns scripts/run.sh inside the template into a demo (see demo.py). A scaffolded
# project is not the template, so the block is cut out of it.
DEMO_BLOCK = re.compile(r"^[^\n]*>>> template demo[^\n]*\n.*?^[^\n]*<<< template demo[^\n]*\n", re.MULTILINE | re.DOTALL)


def strip_demo_block(path: Path) -> None:
    if not path.exists():
        return
    text = path.read_bytes().decode("utf-8")
    stripped, count = DEMO_BLOCK.subn("", text)
    if count:
        path.write_bytes(stripped.encode("utf-8"))


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

    # Skip editor/OS cruft rather than copying it into a fresh project. A
    # stray .DS_Store in the template would otherwise end up committed in
    # every scaffolded repo. RUN_LEFTOVERS go too, at the top level only.
    shutil.copytree(
        TEMPLATE_DIR,
        target_dir,
        dirs_exist_ok=True,
        ignore=ignore_cruft,
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
    for name in ("run.sh", "run.bat", "login.sh", "login.bat"):
        strip_demo_block(target_dir / "scripts" / name)

    # lib.sh files are sourced, not run, so they stay non-executable.
    for folder in ("scripts", "docker", "misc"):
        for script in (target_dir / folder).glob("*.sh"):
            if script.name != "lib.sh":
                script.chmod(0o755)

    port = pick_port()
    stamp_port(target_dir, port)
    config_path = write_config(target_dir)
    example_path = write_example(target_dir)

    print(f"Scaffolded '{project_name}' into {target_dir}")
    print(f"  slug (state dir, docker image/container name): {slug}")
    print(f"  port: {port} (random per project, so local projects never collide)")
    print(f"  config: {config_path} (mode 0600, fresh secret key, gitignored)")
    print(f"  example: {example_path} (no secret key, commit it -- `cp` it to config/config.yaml on a fresh clone)")
    print("")
    print("Next:")
    print(f"  cd {target_dir}")
    print("  scripts/run.sh            # sets up .venv if needed, (re)starts the server, prints its URL")
    print("  scripts/login.sh          # a login link for the owner (user 1), wherever it runs")
    print("  scripts/run.sh manage ... # admin CLI, works while it is running")
    print("  (scripts\\run.bat and scripts\\login.bat on Windows, same arguments)")


if __name__ == "__main__":
    main()
