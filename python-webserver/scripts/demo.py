#!/usr/bin/env python3
"""Stamp a demo project from the template, for `scripts/run.sh` inside the template.

The template itself cannot build: it is full of {{PLACEHOLDERS}} that only
scaffold.py fills in. So when scripts/run.sh (or run.bat) finds itself in the
template, it calls this, which scaffolds a project called DEMO_NAME into a
cache directory and prints that directory's path. run.sh then hands over to
the demo's own run.sh, so what runs is exactly what a scaffold produces.

Every call re-scaffolds, so template edits show up on the next scripts/run.sh. What
a scaffold does not produce is kept: config/config.yaml, config/secret.key (the key
that reads the database), the .run/ pid file, and the .venv with its
installed packages. The demo therefore keeps its accounts between runs, like
any other project would. A demo from before secret.key existed has its key
inside config.yaml; its next run moves it out (scripts/run.sh runs `config
migrate`), like any other project.

The first scaffold also sets server.secure_cookies to false, because the demo
is only ever opened at plain http://localhost, where a Secure cookie is
dropped and login silently does nothing.

Usage (normally only from run.sh / run.bat):
    python3 demo.py
"""
from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

SCRIPTS_DIR = Path(__file__).resolve().parent
DEMO_NAME = "Python Webserver Demo"
DEMO_SLUG = "python-webserver-demo"

# Everything a running demo owns that a fresh scaffold must not replace,
# relative to the demo root. config/secret.key is the key that reads the
# demo's database, so losing it would lose every account.
KEEP = {"config/config.yaml", "config/secret.key", ".run", ".venv", "data"}

# The demo's own port, the same on every run: a re-scaffold would otherwise
# pick a new random one each time, and the browser tab, its saved theme and
# its sign-in would be left behind on the old one. Outside scaffold.py's
# random range, so no scaffolded project lands on it, and different from the
# other webserver skill's demo so both can run at once.
DEMO_PORT = 18492
PORT_LINES = (
    re.compile(r"^(  port:[ \t]*)\d+[ \t]*$", re.MULTILINE),
    re.compile(r"^(  host_port:[ \t]*)\d+[ \t]*$", re.MULTILINE),
)

SECURE_COOKIES_LINE = re.compile(r"^(\s*secure_cookies:\s*)true\b", re.MULTILINE)


def demo_dir() -> Path:
    if os.name == "nt":
        base = Path(os.environ.get("LOCALAPPDATA", Path.home() / "AppData" / "Local"))
    else:
        base = Path(os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache"))
    return base / DEMO_SLUG


def sync(fresh: Path, target: Path, relative: Path = Path(".")) -> None:
    """Make target match fresh, file by file, leaving KEEP alone.

    Directories are updated in place, never deleted and recreated: a running
    server holds static/ open (an os.Root), and swapping the directory out
    from under it would leave it serving a deleted tree -- every request a
    404 until it restarts.
    """
    src_dir, dst_dir = fresh / relative, target / relative
    dst_dir.mkdir(parents=True, exist_ok=True)
    wanted = {entry.name for entry in src_dir.iterdir()}
    for entry in dst_dir.iterdir():
        if (relative / entry.name).as_posix() in KEEP or entry.name in wanted:
            continue
        if entry.is_dir() and not entry.is_symlink():
            shutil.rmtree(entry)
        else:
            entry.unlink()
    for entry in src_dir.iterdir():
        rel = relative / entry.name
        out = target / rel
        if rel.as_posix() in KEEP and out.exists():
            continue
        if entry.is_dir():
            if out.exists() and not out.is_dir():
                out.unlink()
            sync(fresh, target, rel)
        else:
            if out.is_dir():
                shutil.rmtree(out)
            if not out.exists() or out.read_bytes() != entry.read_bytes():
                shutil.copy2(entry, out)
            else:
                shutil.copymode(entry, out)


def pin_port(config: Path) -> None:
    """Point a demo's config.yaml at DEMO_PORT. A demo made before the port was
    pinned has whatever port it was first scaffolded with written in there,
    and config.yaml wins over defaults.yaml."""
    if not config.is_file():
        return
    before = config.read_text(encoding="utf-8")
    after = before
    for pattern in PORT_LINES:
        after = pattern.sub(lambda match: f"{match.group(1)}{DEMO_PORT}", after)
    if after != before:
        config.write_text(after, encoding="utf-8")
        print(f"Demo port is now {DEMO_PORT}", file=sys.stderr)


def main() -> None:
    target = demo_dir()
    target.mkdir(parents=True, exist_ok=True)
    config = target / "config" / "config.yaml"
    # Demos made before config/ existed kept config.yaml at the top level.
    # Move it rather than generating a new key the old database cannot read.
    legacy = target / "config.yaml"
    if legacy.is_file() and not config.exists():
        config.parent.mkdir(parents=True, exist_ok=True)
        legacy.rename(config)
        print(f"Moved {legacy} to {config}", file=sys.stderr)
    first_run = not config.exists()

    with tempfile.TemporaryDirectory(prefix=DEMO_SLUG + "-") as scratch:
        fresh = Path(scratch) / "project"
        # scaffold.py's "next steps" are for a person scaffolding a real
        # project, not for this, so they are only shown if it fails.
        result = subprocess.run(
            [sys.executable, str(SCRIPTS_DIR / "scaffold.py"), str(fresh), DEMO_NAME, "--prune", "none", "--port", str(DEMO_PORT)],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            sys.stderr.write(result.stdout + result.stderr)
            sys.exit(result.returncode)

        # An existing demo keeps its key. Copying the scaffold's fresh one in
        # beside a config.yaml from before secret.key existed would leave two
        # different keys, and the migration rightly refuses to pick one.
        if not first_run:
            (fresh / "config" / "secret.key").unlink(missing_ok=True)

        # Sync in place rather than swapping directories: a running server's
        # working directory has to stay the same directory for run.sh to
        # recognize and stop it.
        sync(fresh, target)

    pin_port(config)

    if first_run:
        text, count = SECURE_COOKIES_LINE.subn(r"\1false", config.read_text(encoding="utf-8"))
        if count == 1:
            config.write_text(text, encoding="utf-8")
        print(f"Demo project: {target} (secure_cookies off, for http://localhost)", file=sys.stderr)

    print(target)


if __name__ == "__main__":
    main()
