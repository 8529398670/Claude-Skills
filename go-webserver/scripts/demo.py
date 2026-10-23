#!/usr/bin/env python3
"""Stamp a demo project from the template, for `./run.sh` inside the template.

The template itself cannot build: it is full of {{PLACEHOLDERS}} that only
scaffold.py fills in. So when run.sh (or run.bat) finds itself in the
template, it calls this, which scaffolds a project called DEMO_NAME into a
cache directory and prints that directory's path. run.sh then hands over to
the demo's own run.sh, so what runs is exactly what a scaffold produces.

Every call re-scaffolds, so template edits show up on the next ./run.sh. What
a scaffold does not produce is kept: config.yaml (and with it the secret key
that reads the database), the .run/ pid file, and the built binary. The demo
therefore keeps its accounts between runs, like any other project would.

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
DEMO_NAME = "Go Webserver Demo"
DEMO_SLUG = "go-webserver-demo"

# Everything a running demo owns that a fresh scaffold must not replace.
KEEP = {"config.yaml", ".run", "data", "dist", DEMO_SLUG, DEMO_SLUG + ".exe"}

SECURE_COOKIES_LINE = re.compile(r"^(\s*secure_cookies:\s*)true\b", re.MULTILINE)


def demo_dir() -> Path:
    if os.name == "nt":
        base = Path(os.environ.get("LOCALAPPDATA", Path.home() / "AppData" / "Local"))
    else:
        base = Path(os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache"))
    return base / DEMO_SLUG


def main() -> None:
    target = demo_dir()
    target.mkdir(parents=True, exist_ok=True)
    first_run = not (target / "config.yaml").exists()

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
        for entry in target.iterdir():
            if entry.name in KEEP:
                continue
            if entry.is_dir() and not entry.is_symlink():
                shutil.rmtree(entry)
            else:
                entry.unlink()
        for entry in fresh.iterdir():
            if entry.name in KEEP and (target / entry.name).exists():
                continue
            if entry.is_dir():
                shutil.copytree(entry, target / entry.name)
            else:
                shutil.copy2(entry, target / entry.name)

    if first_run:
        config = target / "config.yaml"
        text, count = SECURE_COOKIES_LINE.subn(r"\1false", config.read_text(encoding="utf-8"))
        if count == 1:
            config.write_text(text, encoding="utf-8")
        print(f"Demo project: {target} (secure_cookies off, for http://localhost)", file=sys.stderr)

    print(target)


if __name__ == "__main__":
    main()
