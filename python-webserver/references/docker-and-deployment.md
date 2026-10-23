# Docker & deployment reference

## Two scripts, one config file

`dockerBuild.sh` builds the image; `dockerRun.sh` builds through the same
function (`scripts/docker.sh`) and then runs the container. They are split so
building and running are separately runnable -- CI can build without
starting anything. The common case is still one command: `./dockerRun.sh`.

Every value either script uses comes from `config.yaml`: the `docker:`
section (image and container names, `bind_address`, `host_port`, `data_dir`,
`restart`, `memory`, `cpus`, `pids_limit`, `tmpfs_size`) and `server.port` for
the container side of the published port. Neither reads an environment
variable, and the container is started with no `-e` flags -- it reads the
same `config.yaml`, mounted read-only.

The scripts do not parse YAML themselves. `build_image` builds the image,
then runs *its* app (`config shell docker`) in a throwaway container with no
network, a read-only root, and no capabilities, and `eval`s the single-quoted
assignments it prints. So the deploy gets the server's own defaults and
strict validation, and needs nothing on the host but Docker -- not even
Python. The image name lives in `config.yaml` too, which is why it is applied
with `docker tag` after the build rather than passed to it. An extra tag is a
positional argument: `./dockerBuild.sh v1.2.3`.

Both stamp version metadata into the image (`git describe`, the commit, the
build time), so `docker exec <name> python -m app version` says which build is
running.

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
- **No `config.yaml` in the image.** `.dockerignore` keeps it out because it
  holds the secret key and an image layer is forever -- it is mounted at run
  time instead, so one image serves any deployment and a config change needs a
  restart, not a rebuild.
- **No `ENV`, no `EXPOSE`.** The image's fixed layout is pinned by a file the
  Dockerfile writes (`app/core/layout_pinned.py`: config at
  `/app/config.yaml`, state in `/app/data`, listen on `0.0.0.0`; see
  `configuration.md`). The port is `server.port`, so a hardcoded `EXPOSE`
  would be stale the first time it changed. Even the interpreter flags that
  are usually set with `ENV` are on the entrypoint instead: `python -u` for
  unbuffered output, so the first-run login link reaches `docker logs` at
  once.
- **Bytecode compiled at build time.** `compileall` runs in the image because
  `--read-only` means the process can never write `.pyc` files later.
- **Non-root user with a fixed uid/gid of 10001.** `app` is created with
  `/sbin/nologin`, and `USER app` drops root before the entrypoint runs. The
  number is fixed and high on purpose: `dockerRun.sh` grants this uid the
  host's data directory and this gid read access to `config.yaml`, and a
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
what you develop on; `requirements.txt` is pinned so the first build is
reproducible and does not silently resolve to newer versions.

## dockerRun.sh choices

- **Stops on first run for review.** A fresh clone has no `config.yaml` (it
  is gitignored), so the first `./dockerRun.sh` writes one from the image's
  defaults with a new key and exits, naming the settings that depend on what
  sits in front of this. Starting on unreviewed defaults would mean guessing
  at `secure_cookies` and `trust_proxy`.
- **Checks that Docker can see the project at all.** Docker Desktop and
  colima run the daemon in a VM that shares only some host paths (colima
  shares your home directory). Mount anything outside those and Docker
  silently mounts an empty directory from inside the VM: `config.yaml` arrives
  as a directory, and the database would live in the VM where no host backup
  ever finds it. The script checks that `config.yaml` arrives as a file and
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
  ranges and make sure it overwrites what the client sent. `dockerRun.sh`
  reminds you while it is off.
- **`config.yaml` is mounted read-only, separately from the data
  directory,** so the secret key is never inside `docker.data_dir`. The
  script makes the file readable by the container's group (0640, you stay the
  owner) and tests that it worked, the same way it handles the data directory
  below.
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
`/app/config.yaml`, overriding `server.app_dir` and the `~/.config/<slug>`
default that apply outside a container. A container has no meaningful home
directory, and the point of the mounts is that state lives on the host.

Everything the app persists is therefore in the mounted data directory --
database and `storage/`. The exception is the secret key, which lives in
`config.yaml` in the project directory and is mounted separately, so the key
stays *outside* the data volume. See `configuration.md` for why that
distinction is worth keeping.

## The data directory and container ownership

`dockerRun.sh` bind-mounts `docker.data_dir` (`./data`) so the database stays
visible on the host for backups. A bind mount ignores the image's ownership,
though, so the script makes sure the container's unprivileged user can
actually write there -- and read `config.yaml` -- before starting anything.

It has to, because platforms disagree:

- **Linux**: the host's ownership is real. The container user (uid 10001)
  needs to be granted it, so the chown/chgrp the script attempts matters.
- **Docker Desktop on macOS**: the file-sharing layer usually ignores guest
  ownership -- any uid can read and write -- and the chown fails harmlessly.
- **colima with sshfs mounts**: host file modes are enforced *and* root in the
  container cannot change them. A 0600 `config.yaml` is unreadable to uid
  10001, and the script stops with the exact `sudo chgrp`/`sudo chown` to run.

So the changes are attempted, their failure is tolerated, and the result is
then actually tested. If a test fails the script stops with the command to
run, rather than starting a container that crash-loops on "permission
denied" -- which looks like a bug in the app and is not.

## The secret key

`server.secret_key` is generated once, when `config.yaml` is created
(`scaffold.py`, `config init`, or the first `./dockerRun.sh` on a fresh
clone), and read from there on every run. It must stay stable:

- It encrypts session cookie payloads -- rotating it logs everyone out.
- With `server.encrypt_at_rest: true` (default) it encrypts every record in
  the database -- **rotating or losing it makes the data permanently
  unreadable.**

So `config.yaml` is as important as `data/` for backups. Back up both
together or neither is useful. If the user is setting up backups, say this
explicitly; it is the kind of thing discovered at the worst possible time.

To rotate deliberately, export the data first, change the key, re-import.
There is no in-place re-encryption in the template.

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
assuming the answer is nothing. Set `auth.api_keys: false` in `config.yaml`
and restart to close that surface entirely.

If the container is stopped, run a one-off with the same two mounts:

```bash
docker run --rm -v "$(pwd)/data:/app/data" \
  -v "$(pwd)/config.yaml:/app/config.yaml:ro" \
  <image> manage reissue-login --user-id 1
```

The `config.yaml` mount is what carries the secret key; without it the stored
records cannot be decrypted.

## Running without Docker

For development, an internal box with no container runtime, or a quick demo,
run from a checkout:

```bash
./run.sh                                   # (re)start the server
./run.sh manage reissue-login --user-id 1  # any arguments: the CLI, server untouched
```

`run.sh` creates `.venv` if it is missing and runs `pip install -r
requirements.txt` whenever that file is newer than `.venv/.requirements.stamp`
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
SQLite runs in WAL mode. On a fresh clone with no `config.yaml` it writes one
(`config init`) and stops, as `dockerRun.sh` does.

It runs from the project directory, so `./config.yaml` is the one found (and
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

The entire state is the `data/` directory plus `config.yaml`, and you need
both -- `app.db` without the key in `config.yaml` is unreadable. Stop the
container before copying: while the server runs, recent writes can still be in
`app.db-wal` rather than `app.db`. Use `docker stop`, copy, `docker start`.

`configuration.md` covers the equivalent outside Docker, where the key may
live inside the app directory instead.

## Tuning

Every knob is a key in `config.yaml` -- session and login-token lifetimes,
rate limits, body size, at-rest encryption, proxy trust, and the container's
own resource limits. `app/core/defaults.yaml` documents all of them with their
defaults.

Edit the project's `config.yaml` and re-run `./dockerRun.sh` (or just
`docker restart <name>` for a `server:` or `auth:` change, since the file is
mounted rather than baked in). The image stays the same across environments;
each deployment's `config.yaml` is what differs. See `configuration.md`.
