#!/usr/bin/env python3
"""Stamp a demo project from the template, for `scripts/run.sh` inside the template.

The template itself cannot build: it is full of {{PLACEHOLDERS}} that only
scaffold.py fills in. So when scripts/run.sh (or run.bat) finds itself in the
template, it calls this, which scaffolds a project called DEMO_NAME into a
cache directory and prints that directory's path. run.sh then hands over to
the demo's own run.sh, so what runs is exactly what a scaffold produces.

Every call re-scaffolds, so template edits show up on the next scripts/run.sh. What
a scaffold does not produce is kept: config/config.yaml (and with it the secret key
that reads the database), the .run/ pid file, and the .venv with its
installed packages. The demo therefore keeps its accounts between runs, like
any other project would.

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
# relative to the demo root. config/config.yaml holds the secret key that
# reads the demo's database, so losing it would lose every account.
KEEP = {"config/config.yaml", ".run", ".venv", "data"}

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
            [sys.executable, str(SCRIPTS_DIR / "scaffold.py"), str(fresh), DEMO_NAME],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            sys.stderr.write(result.stdout + result.stderr)
            sys.exit(result.returncode)

        # Sync in place rather than swapping directories: a running server's
        # working directory has to stay the same directory for run.sh to
        # recognize and stop it.
        sync(fresh, target)

    if first_run:
        text, count = SECURE_COOKIES_LINE.subn(r"\1false", config.read_text(encoding="utf-8"))
        if count == 1:
            config.write_text(text, encoding="utf-8")
        print(f"Demo project: {target} (secure_cookies off, for http://localhost)", file=sys.stderr)

    print(target)


if __name__ == "__main__":
    main()
