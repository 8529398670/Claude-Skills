#!/usr/bin/env bash
# Set up the venv and run the server from this checkout, outside Docker.
#
#   ./run.sh                    create/update .venv if needed, stop the copy
#                               this checkout started last, start a fresh one
#   ./run.sh manage <command>   run an admin command against it (see below)
#   ./run.sh <anything else>    same: passed straight to `python -m app`
#
# The server runs in the foreground: its log streams here and Ctrl-C stops it.
# Running ./run.sh again from another terminal stops this one and takes over,
# so "edit, ./run.sh, reload the page" is the whole loop.
#
# With arguments it is a one-off instead -- the same venv, then `python -m
# app` with those arguments -- and the running server is left alone. `manage`
# opens the database directly, which SQLite's WAL mode makes safe alongside
# the live server.
#
#   ./run.sh manage list-users
#   ./run.sh manage reissue-login --user-id 1    fresh login link (locked out)
#   ./run.sh manage create-login --name "Grace" --role user
#   ./run.sh manage create-invite --user-id 1 --label "Book club"
#   ./run.sh manage create-key --user-id 1 --name "CI"
#   ./run.sh manage paths                        where the state lives
#   ./run.sh config check
#   ./run.sh version
#
# This is for development and internal boxes. It is a plain process with your
# permissions -- none of dockerRun.sh's read-only filesystem, dropped
# capabilities, or pid and memory limits apply -- so use Docker for anything
# long-lived or internet-facing.
set -euo pipefail
cd "$( dirname "${BASH_SOURCE[0]}" )"

# >>> template demo -- scaffold.py removes this block from every project.
# This is the skill's template, not a project: its {{PLACEHOLDERS}} are not
# filled in, so it cannot run as it is. Stamp a demo project from it instead
# (../../scripts/demo.py, kept in a cache directory with its config and
# database) and hand over to that project's own run.sh.
if [ -f ../../scripts/demo.py ] && [ -f ../../scripts/scaffold.py ]; then
  DEMO_DIR="$( python3 ../../scripts/demo.py )"
  exec "${DEMO_DIR}/run.sh" "$@"
fi
# <<< template demo

VENV="$( pwd )/.venv"
PYTHON="${VENV}/bin/python"
RUN_DIR="$( pwd )/.run"
PID_FILE="${RUN_DIR}/server.pid"
# Touched after a successful install. requirements.txt being newer than it is
# the signal to install again, so editing the file is all it takes.
STAMP="${VENV}/.requirements.stamp"

# The venv's own python is called by path rather than activating the venv:
# it finds its packages either way, and nothing here depends on PATH.
#
# All of this happens before anything is stopped, so a failed install leaves
# the server that was already running untouched.
#
# A venv whose creation was interrupted has a python but no pip, and would
# fail every install from then on; --clear rebuilds it from scratch.
if [ ! -x "$PYTHON" ] \
   || { [ ! -f "$STAMP" ] && ! "$PYTHON" -m pip --version >/dev/null 2>&1; }; then
  echo "Creating .venv..."
  python3 -m venv --clear "$VENV"
fi
if [ ! -f "$STAMP" ] || [ requirements.txt -nt "$STAMP" ]; then
  echo "Installing requirements.txt into .venv..."
  "$PYTHON" -m pip install --quiet --upgrade pip
  "$PYTHON" -m pip install --quiet -r requirements.txt
  touch "$STAMP"
fi

if [ $# -gt 0 ]; then
  exec "$PYTHON" -m app "$@"
fi

# First run on this checkout: config.yaml is gitignored, so a fresh clone has
# none. Without one here the app would quietly create
# ~/.config/{{PROJECT_SLUG}}/config.yaml instead, which is not the file this
# checkout's dockerRun.sh reads. Write it here and stop, like dockerRun.sh, so
# it gets looked at before the first start.
if [ ! -f config.yaml ]; then
  "$PYTHON" -m app config init --file ./config.yaml
  cat <<NOTE

For plain http://localhost set server.secure_cookies: false in config.yaml,
or the browser drops the session cookie and login silently does nothing.
Then run ./run.sh again.
NOTE
  exit 0
fi

# Stop the copy this checkout started last. The pid file says which process
# that was; is_ours makes sure it still is, because a pid left behind by a
# server that has since exited can belong to anything by now.
#
# The command line alone cannot say so: on a macOS framework build,
# .venv/bin/python re-executes itself as Python.app/.../Python, so the venv
# path never shows up in ps. "-m app" running in this directory does.
is_ours() {
  local cwd
  case "$( ps -p "$1" -o command= 2>/dev/null )" in
    *" -m app") ;;
    *) return 1 ;;
  esac
  if [ -e "/proc/$1/cwd" ]; then
    cwd="$( readlink "/proc/$1/cwd" )"
  else
    cwd="$( lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' )"
  fi
  [ "$cwd" = "$( pwd -P )" ]
}

if [ -f "$PID_FILE" ]; then
  OLD_PID="$( cat "$PID_FILE" )"
  if [ -n "$OLD_PID" ] && kill -0 "$OLD_PID" 2>/dev/null && is_ours "$OLD_PID"; then
    echo "Stopping the previous server (pid ${OLD_PID})..."
    kill -TERM "$OLD_PID" 2>/dev/null || true
    # uvicorn shuts down gracefully within 10 seconds (app/main.py). Past
    # that, something is stuck, and waiting longer will not unstick it.
    for _ in $( seq 1 60 ); do
      kill -0 "$OLD_PID" 2>/dev/null || break
      sleep 0.25
    done
    if kill -0 "$OLD_PID" 2>/dev/null; then
      echo "It did not exit in 15 seconds; killing it."
      kill -KILL "$OLD_PID" 2>/dev/null || true
    fi
  fi
  rm -f "$PID_FILE"
fi

mkdir -p "$RUN_DIR"

cat <<NOTE
Starting {{PROJECT_SLUG}} from $( pwd )
  config   ./config.yaml
  state    ~/.config/{{PROJECT_SLUG}}/ unless server.app_dir says otherwise
           (./run.sh manage paths)
  code     Python changes need ./run.sh again; static/ edits only a reload

On a fresh database the first-run login link is printed below. Locked out
later? ./run.sh manage reissue-login --user-id 1 from another terminal.

NOTE

# exec keeps this shell's pid, so the one written here is the server's.
echo $$ > "$PID_FILE"
exec "$PYTHON" -m app
