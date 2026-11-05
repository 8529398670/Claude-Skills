# Docker & deployment reference

## Two scripts, one config file

Everything Docker lives in `docker/`. `docker/build.sh` builds the image;
`docker/run.sh` builds through the same function (`docker/lib.sh`) and then
runs the container. They are split so building and running are separately
runnable -- CI can build without starting anything. The common case is still
one command: `docker/run.sh`. Both `cd` to the project root first, which is
the build context: `docker build -f docker/Dockerfile .`, with
`.dockerignore` at the project root (read by both BuildKit and the legacy
builder; a per-Dockerfile `docker/Dockerfile.dockerignore` is read only by
BuildKit, so the legacy builder would ignore it) keeping the secret, the scripts and local state out of it.

Every value either script uses comes from `config/config.yaml`: the `docker:`
section (image and container names, `bind_address`, `host_port`, `data_dir`,
`restart`, `memory`, `cpus`, `pids_limit`, `tmpfs_size`) and `server.port` for
the container side of the published port. Neither reads an environment
variable, and the container is started with no `-e` flags -- it reads the
same `config.yaml`, mounted writable so the owner's Settings screen can save
it, and the key in `config/secret.key`, mounted read-only.

The scripts do not parse YAML themselves. `build_image` builds the image,
then runs *its* app (`config shell docker`) in a throwaway container with no
network, a read-only root, and no capabilities, and `eval`s the single-quoted
assignments it prints. So the deploy gets the server's own defaults and
strict validation, and needs nothing on the host but Docker -- not even
Python. The image name lives in `config.yaml` too, which is why it is applied
with `docker tag` after the build rather than passed to it. An extra tag is a
positional argument: `docker/build.sh v1.2.3`. The build says which
Dockerfile and which config file it used (or that there is none yet and the
image is named from the defaults), and stops early with a plain message when
`docker` is missing or its daemon is not answering.

Both stamp version metadata into the image (the version is the UTC build
time, `2026.10.23-1412`, plus the commit hash), so `docker exec <name> python -m app version` says which build is
running.

## Builders: BuildKit (buildx) or the legacy builder

Either builds this image. Prefer BuildKit: Docker Desktop and colima ship it,
and on a Linux server it is the `docker-buildx-plugin` package (Docker's own
apt repo, `docker-ce`) or `docker-buildx` (the distro's `docker.io`); check
with `docker buildx version`. Without it, `docker build` falls back to the
legacy builder, prints a deprecation warning, and still works -- which is why
the ignore file is the root `.dockerignore` that both read. Install buildx
before the legacy builder is removed rather than after.

## Why there is no docker-compose

This project is exactly one container. Compose's value is orchestrating
several services; with one, it adds a second configuration format that hides
the same flags behind YAML keys you have to look up. A single `docker run` you
can read top to bottom is easier to audit, and every hardening choice below is
visible on the line where it takes effect.

If the project later genuinely grows a second service, that is the moment to
reconsider -- not before.

## Dockerfile choices

- **Multi-stage.** Only the builder stage runs pip. The runtime stage copies
  in the installed packages (`--prefix=/install` → `/usr/local`), the `app/`
  package, `static/`, and `language.yaml` -- nothing else from the build.
- **`--only-binary=:all:`.** Every dependency must arrive as a prebuilt
  wheel. All of them ship musllinux wheels for Alpine, so nothing is compiled
  and the image needs no gcc or headers. If a future dependency does not, the
  build fails loudly at that line rather than quietly growing a compiler
  toolchain. Plain `uvicorn` rather than `uvicorn[standard]` keeps it that
  way -- uvloop and httptools are speed this app does not need.
- **pip removed from the runtime image.** Nothing installs packages at
  runtime, and a package manager is one more tool for anyone who gets code
  execution inside.
- **No `config.yaml` or `secret.key` in the image.** `.dockerignore` keeps
  them out -- an image layer is forever, and a key in one is everyone's --
  and they are mounted at run time instead, so one image serves any
  deployment. Most settings apply while it runs; the rest need a restart,
  never a rebuild.
- **No `ENV`, no `EXPOSE`.** The image's fixed layout is pinned by a file the
  Dockerfile writes (`app/core/layout_pinned.py`: config at
  `/app/config/config.yaml`, state in `/app/data`, listen on `0.0.0.0`; see
  `configuration.md`). The port is `server.port`, so a hardcoded `EXPOSE`
  would be stale the first time it changed. Even the interpreter flags that
  are usually set with `ENV` are on the entrypoint instead: `python -u` for
  unbuffered output, so the first-run login link reaches `docker logs` at
  once.
- **Bytecode compiled at build time.** `compileall` runs in the image because
  `--read-only` means the process can never write `.pyc` files later.
- **Non-root user with a fixed uid/gid of 10001.** `app` is created with
  `/sbin/nologin`, and `USER app` drops root before the entrypoint runs. The
  number is fixed and high on purpose: `docker/run.sh` grants this uid the
  host's data directory, and this gid read-write access to `config.yaml` and
  read access to `secret.key`, and a
  low system uid is likely to belong to a real account on a Linux host, which
  would get the same access.
- **Only `/app/data` is writable, and only by `app`.** Everything else stays
  root-owned and read-only to the process, so code execution inside the
  container still cannot rewrite the app or the frontend.
- **Healthcheck is `python -m app health`,** which reads `server.port` from
  the mounted config and asks `/api/health`, an endpoint that touches nothing.
  A healthcheck that queried the database would let a slow query get a
  working container killed and restarted -- turning a small problem into an
  outage. Doing it in the app also means a port change cannot leave a stale
  URL in the image.

The Python version in the two `FROM` lines should move together, and match
what you develop on; `app/requirements.txt` is pinned so the first build is
reproducible and does not silently resolve to newer versions.

## docker/run.sh choices

- **Stops on first run for review.** A fresh clone has no `config/config.yaml` (it
  is gitignored), so the first `docker/run.sh` writes one from the image's
  defaults, and `config/secret.key` with a new key, and exits, naming the settings that depend on what
  sits in front of this. Starting on unreviewed defaults would mean guessing
  at `secure_cookies` and `trust_proxy`.
- **Names whatever holds the host port.** With its own old container
  removed, anything still listening on `docker.host_port` is something else,
  and `docker run` would fail with a bind error after creating the
  container. The script names the process (and the container, if it is one)
  and stops instead.
- **Waits until it is really up.** After `docker run -d` it polls the image's
  own healthcheck (`python -m app health`) for up to 30 seconds. If the
  container exits or never answers, it prints the status, the exit code and
  the last 40 log lines, and exits non-zero; only a healthy container gets
  `Ready: <url>`. On a fresh database it then prints the owner's first-run
  login link straight from the log, and points at `scripts/login.sh --docker`
  for a new one later.
- **Moves the key out of an older `config.yaml`** before anything else
  (`config migrate`, run through the image with `config/` mounted, as you).
  It is silent unless there was a `server.secret_key` to move. A missing
  `secret.key` with no database yet gets a new one; with a database it stops
  and says to restore it from backup.
- **Checks that Docker can see the project at all.** Docker Desktop and
  colima run the daemon in a VM that shares only some host paths (colima
  shares your home directory). Mount anything outside those and Docker
  silently mounts an empty directory from inside the VM: `config.yaml` arrives
  as a directory, and the database would live in the VM where no host backup
  ever finds it. The script checks that `config.yaml` and `secret.key` arrive
  as files and
  that a probe written to the data directory appears on the host, and stops
  with an explanation if not.
- **Publishes to `docker.bind_address`, `127.0.0.1` by default.** This app
  does **not** terminate TLS. Put nginx, Caddy, or Traefik in front and let it
  hold the certificate.
- **`server.secure_cookies` stays `true`** whenever a proxy is serving HTTPS.
  Set it to `false` *only* for a plain-`http://localhost` run. Getting this
  wrong is the single most common cause of "login does nothing": a `Secure`
  cookie is silently dropped over plain http, so the sign-in appears to
  succeed and the session never sticks.
- **`server.trust_proxy: true` plus `server.trusted_proxies`** make the rate
  limiter count requests by the address in `server.proxy_header` (default
  `X-Forwarded-For`) rather than the socket's peer -- but only when that peer
  is in `trusted_proxies`. The header is read right to left, skipping
  addresses that are themselves trusted proxies, and the first one that is not
  is the client; anything further left came from the client and can say
  anything, and an entry that is not an IP is never used as a key. Checking
  the peer is what makes this safe: without it, any client that reaches the
  backend directly picks its own rate-limit key. The config check refuses
  `trust_proxy: true` with an empty list. For the Docker setup the proxy
  reaches the container from the bridge gateway, so `[172.16.0.0/12]` covers
  the default bridge -- confirm with `docker network inspect bridge`. With a
  single-value header from a CDN (`CF-Connecting-IP`, say), list the CDN's
  ranges and make sure it overwrites what the client sent. `docker/run.sh`
  reminds you while it is off.
- **`config.yaml` is mounted writable and `secret.key` read-only, both
  separately from the data directory,** so the key is never inside
  `docker.data_dir`. The script gives the container's group read-write on
  `config.yaml` (0660) and read on `secret.key` (0640) -- you stay the owner
  of both -- and tests that it worked, the same way it handles the data
  directory below. If the container can read `config.yaml` but not write it,
  it still starts, and the Settings screen is read-only.
- **`config.yaml` is rewritten in place inside the container.** A single file
  bind-mounted into a container cannot be replaced by renaming a new file over
  it, and `/app/config` is root's and read-only anyway. So a save from the
  Settings screen (or `docker exec <name> python -m app config set ...`)
  copies the previous version to `<data_dir>/config.yaml.bak` and then writes
  the file from the start, truncating the tail last, so it is never empty. On
  the host, edit it with something that saves in place too (vi, nano, VS Code
  do). An editor that writes a new file and renames it over the old one
  leaves the container holding the old file, because a bind mount follows the
  file, not the name. The server notices (the file's link count drops to 0),
  the Settings screen says to restart the container, and saves are refused
  until then.
- **Guards `docker.data_dir`.** `config.yaml` is writable from inside the
  container now, and the script hands `data_dir` to the container's user
  (`chown -R`). So the Settings screen refuses to change `docker:` from
  inside the container, and the script refuses a `data_dir` of `/` or a home
  directory, or one that is not empty and is not this app's (no `app.db`, no
  `.<slug>-data` marker, which it leaves in every data directory it sets up).
  It also prints any `docker:` setting that changed since the last run
  (`.run/docker.last`).
- **`--read-only`** plus a `noexec,nosuid` tmpfs for `/tmp`: the container
  cannot write to its own filesystem. This is what turns the Dockerfile's
  ownership choices into an actual guarantee.
- **`--cap-drop ALL`** and **`--security-opt no-new-privileges:true`**: no
  Linux capabilities, and no process inside can gain more rights than it
  started with, even via a setuid binary.
- **`docker.pids_limit`, `docker.memory`, `docker.cpus`** (128, 256m, 1):
  blast-radius limits, so a runaway loop degrades one container instead of
  the host. Raise these deliberately for real load rather than discovering
  them under traffic.

## Paths inside the container

The image pins the app directory to `/app/data` and the config file to
`/app/config/config.yaml` (with `secret.key` beside it), overriding `server.app_dir` and the `~/.config/<slug>`
default that apply outside a container. A container has no meaningful home
directory, and the point of the mounts is that state lives on the host.

Everything the app persists is therefore in the mounted data directory --
database, `storage/`, and the `config.yaml.bak` a Settings save leaves. The
exception is the secret key, which lives in `config/secret.key` in the project
directory and is mounted separately, so the key stays *outside* the data
volume. See `configuration.md` for why that
distinction is worth keeping.

## The data directory and container ownership

`docker/run.sh` bind-mounts `docker.data_dir` (`./data`) so the database stays
visible on the host for backups. A bind mount ignores the image's ownership,
though, so the script makes sure the container's unprivileged user can
actually write there -- and read `secret.key` and `config.yaml` -- before
starting anything.

It has to, because platforms disagree:

- **Linux**: the host's ownership is real. The container user (uid 10001)
  needs to be granted it, so the chown/chgrp the script attempts matters.
- **Docker Desktop on macOS**: the file-sharing layer usually ignores guest
  ownership -- any uid can read and write -- and the chown fails harmlessly.
- **colima with sshfs mounts**: host file modes are enforced *and* root in the
  container cannot change them. A 0600 `secret.key` is unreadable to uid
  10001, and the script stops with the exact `sudo chgrp`/`sudo chown` to run.

So the changes are attempted, their failure is tolerated, and the result is
then actually tested. If a test fails the script stops with the command to
run, rather than starting a container that crash-loops on "permission
denied" -- which looks like a bug in the app and is not.

## The secret key

The key is `config/secret.key`, generated once (`scaffold.py`, `config
init`, or the first `docker/run.sh` on a fresh clone) and mounted read-only
into the container on every run. An older project's `server.secret_key` is
moved into it by `docker/run.sh` before the container starts; the container
itself never writes `config/`. It must stay stable:

- It encrypts session cookie payloads -- changing it logs everyone out.
- With `server.encrypt_at_rest: true` (default) it encrypts every record in
  the database -- **rotating or losing it makes the data permanently
  unreadable.**

So `secret.key` is as important as `data/`, and `manage backup` leaves it out
on purpose: see [Backups](#backups).

To change it, run the rotation, never an edit of the file:

```bash
docker/rotate-key.sh --yes
```

That stops the container, moves the database, the two-factor secrets and the
activity log onto a new key, and starts it again. The container cannot do this
alone, because `secret.key` is mounted read-only, so the work is split: the
script writes `config/secret.key.next` on the host, a throwaway container moves
the data, and the script swaps the key file. A marker in `data/.rotate/` holds
the two halves together. If anything stops it part way, the server refuses to
start rather than run on a key that does not match the data, and running the
script again finishes the same rotation.

Everyone has to sign in again. The old key stays as
`config/secret.key.retired-<time>`, because backups made before it still need
it: `docker/restore.sh backup.tar.gz config/secret.key.retired-<time>`. Delete
it once those backups no longer matter. Both names are in `.gitignore` and
`.dockerignore`.

See `configuration.md` for what a rotation does and does not fix after a leak.

## First login

On a fresh database the server creates an admin and prints a one-time link to
its own logs:

```bash
docker logs <container-name>
```

Prepend the real scheme and host: `https://yourapp.example.com/login/<credential>`.
The server prints a path rather than a URL because behind a proxy it sees an
internal address, and a confidently wrong URL is worse than an obviously
partial one.

The link is shown once and is not recoverable. If it scrolls away or expires
(30 minutes by default), mint another:

```bash
docker exec -it <container-name> python -m app manage list-users
docker exec -it <container-name> python -m app manage reissue-login --user-id 1
```

## Operating the container

`docker exec ... manage` works against the running container: SQLite's WAL
mode lets the CLI open the database alongside the live server, and it goes
through the same model code, so a change made this way is identical to one
made in the admin panel.

Day-to-day account management still belongs in the admin panel; the CLI is the
recovery path for when nobody can get in.

API keys go through the same door, which is what lets a deploy script hand a
CI system a credential without anyone signing in first:

```bash
docker exec -it <name> python -m app manage create-key --user-id 1 --name "CI" --days 90
docker exec -it <name> python -m app manage list-keys
docker exec -it <name> python -m app manage revoke-key --key-id <id>
```

The key is printed once and is not recoverable -- capture it where the script
needs it, or revoke and mint another. `list-keys` is the standing answer to
"what has programmatic access to this deployment", and worth reading before
assuming the answer is nothing. Set `auth.api_keys: false` (Settings screen,
or `config.yaml` on the host) to close that surface entirely -- it applies at
once.

For a login link, `scripts/login.sh --docker` does the `docker exec` for you
and says which container and address the link is for.

If the container is stopped, run a one-off with the same two mounts:

```bash
docker run --rm -v "$(pwd)/data:/app/data" \
  -v "$(pwd)/config/config.yaml:/app/config/config.yaml:ro" \
  -v "$(pwd)/config/secret.key:/app/config/secret.key:ro" \
  <image> manage reissue-login --user-id 1
```

The `secret.key` mount is what carries the key; without it the stored
records cannot be decrypted.

## Running without Docker

For development, an internal box with no container runtime, or a quick demo,
run from a checkout:

```bash
scripts/run.sh                       # (re)start the server
scripts/login.sh                     # the owner's login link, from whatever is live
scripts/run.sh manage list-users     # any arguments: the CLI, server untouched
```

`scripts/run.sh` creates `.venv` if it is missing and runs `pip install -r
app/requirements.txt` whenever that file is newer than `.venv/.requirements.stamp`
-- all before stopping anything, so a failed install leaves the old server up.
With no arguments it then stops the server it started last and runs a new one
in the foreground: it records its own pid in `.run/server.pid` just before
`exec`ing `python -m app`, and only signals that pid if it is still a
`... -m app` process whose working directory is this checkout (SIGTERM, then
SIGKILL after 15 seconds), so a stale pid is never mistaken for it. The venv
path is not part of that check on purpose: a macOS framework Python
re-executes `.venv/bin/python` as `Python.app/.../Python`, so it never shows
up in `ps`. Running it again from
another terminal is how you restart. With arguments it runs `python -m app`
with them and leaves the server alone; `manage` is safe alongside it because
SQLite runs in WAL mode.

Before starting, it reads `config shell server` -- every path and the URL,
resolved by the app exactly as the server will -- and:

- moves `server.secret_key` out of an older `config/config.yaml` into
  `config/secret.key` (`config migrate`; silent when there is nothing to move),
  and refuses to start when `secret.key` is missing but the database exists;
- on a fresh clone with no `config/config.yaml`, writes it and
  `config/secret.key` (`config init`) and stops, as `docker/run.sh` does, saying which URL it will serve and,
  when that is plain `http://`, the line to change so `secure_cookies` does
  not silently break sign-in;
- refuses to start, naming the process (via `lsof`), when anything else is
  listening on `server.port` -- including this project's docker container;
- warns when `secure_cookies` is on but the URL is plain `http://`;
- prints the URL and where it came from, the config file, the app dir, the
  database (existing, or new with the first login link to follow), and
  whether static/ and language come from disk or the bundled copies;
- prints `Ready: <url>` once `/api/health` answers (or says it is not
  answering after 20 seconds).

The shared helpers are in `scripts/lib.sh`.

`scripts/login.sh [user-id] [--docker|--local]` prints a one-time login link
(user 1, the owner, by default) as a full URL. It decides where from: this
checkout's local database when its server is running, otherwise the docker
container when it is running (`docker exec`), otherwise the local database
offline, saying the link waits until the server starts. The local and
container deployments keep separate databases, so it always says which one
the link is for; when both run, it uses the local one and says so. Errors are
specific: no `.venv` or config yet, no database yet, no such user (with the
command that lists them), container not running (with its status).

On Windows, `scripts\run.bat` and `scripts\login.bat` take the same
arguments and do the same, reading `config shell --format bat` and using
PowerShell for the port owner and the readiness check. `run.bat` uses
`.venv\Scripts\python.exe` (the venv is created with the `py` launcher when
there is one, since a bare `python` is often the Microsoft Store stub). Three
things differ. It reinstalls when `app\requirements.txt`'s contents differ from
the copy saved as `.venv\.requirements.stamp`, since batch cannot compare file
times. It finds the old server as any process running this checkout's
`.venv\Scripts\python.exe` with exactly `-m app` -- on Windows that launcher
does show up, and it starts the real interpreter as a child -- instead of a
pid file. And with no SIGTERM to send another console's process, it kills the
whole tree (`taskkill /T /F`) instead of draining it, which WAL-mode SQLite
survives. PowerShell (built into Windows) does the process lookup.

It runs from the project directory, so `./config/config.yaml` is the one found (and
`app` is importable). State goes to `~/.config/<slug>/` unless
`server.app_dir` says otherwise; `python -m app manage paths` reports where.
Two things to pass on to whoever runs it this way:

- **It has no sandbox.** It is a normal process with the invoking user's
  permissions -- none of Docker's read-only filesystem, dropped capabilities,
  or pid and memory limits apply. Use Docker for anything long-lived or
  internet-facing.
- **`server.secure_cookies` must be true anywhere real**, behind an HTTPS
  proxy. Only `false` for plain `http://localhost`.

Python cannot produce one self-contained executable that runs with no
interpreter installed, the way the Go template's `build.sh` does. If that is
the requirement, `go-webserver` is the template for it.

## Backups

The state is `data/` plus `config/secret.key` and `config/config.yaml`.
`manage backup` writes one file holding the database, `config.yaml` and
`storage/`, **while the server runs**: `VACUUM INTO` copies one fixed moment,
including what is still in `app.db-wal`, which copying `app.db` by hand would
miss.

```bash
docker exec <container-name> python -m app manage backup
docker cp <container-name>:/app/data/backups/backup-<time>.tar.gz .
```

The file lands in `data/backups/`. It belongs to the container's user on a
Linux host, which is what `docker cp` is for.

**`secret.key` is not in a backup unless you pass `--include-key`**, and nothing
can read a backup without it, so keep a copy of the key apart from your
backups. Each backup's manifest records the key's fingerprint, and the command
prints it. Say this explicitly when setting backups up; it is the kind of thing
discovered at the worst possible time. The activity log is left out too: it can
be far larger than everything else, and losing it loses history, not data.

Put one back with:

```bash
docker/restore.sh path/to/backup.tar.gz
docker/restore.sh path/to/backup.tar.gz config/secret.key.retired-<date>
```

It stops the container, restores in a throwaway container with the same mounts,
and starts it again. The backup is unpacked and every record decoded before
anything in place changes, so a wrong key or a damaged file is refused with the
data untouched. What it replaces stays in the data directory as
`app.db.before-restore-<time>` and `storage.before-restore-<time>`. Delete them
once you are happy. `config.yaml` is not replaced; the backup's copy is left
beside them as `config.yaml.from-backup-<time>`.

A backup made under another key comes back on the key in use now, so
`secret.key` itself never changes and everyone just signs in again.

Scheduled backups are off by default: set `backups.enabled: true`, from the
Settings screen or `config.yaml`, and the hourly job writes
`auto-<time>.tar.gz` every `backups.interval_hours`, keeping the last
`backups.keep`. Backups made by hand are never removed. They sit on the same
disk as the data, so copy them somewhere else as well.

`configuration.md` covers the equivalent outside Docker, where the key may
live inside the app directory instead.

## Tuning

Every knob is a key in `config.yaml` -- session and login-token lifetimes,
rate limits, body size, at-rest encryption, proxy trust, and the container's
own resource limits. `config/defaults.yaml` documents all of them with their
defaults.

Change a setting from the owner's Settings screen, or edit the project's
`config/config.yaml` on the host: `[live]` keys apply within seconds,
`[restart]` keys on `docker restart <name>`, and `docker:` keys on the next
`docker/run.sh`. The image stays the same across environments;
each deployment's `config.yaml` is what differs. See `configuration.md`.
