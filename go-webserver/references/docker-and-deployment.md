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
then runs *its* binary (`config shell docker`) in a throwaway container with
no network, a read-only root, and no capabilities, and `eval`s the
single-quoted assignments it prints. So the deploy gets the server's own
defaults and strict validation, and needs nothing on the host but Docker. The
image name lives in `config.yaml` too, which is why it is applied with
`docker tag` after the build rather than passed to it. An extra tag is a
positional argument: `docker/build.sh v1.2.3`. The build says which
Dockerfile and which config file it used (or that there is none yet and the
image is named from the defaults), and stops early with a plain message when
`docker` is missing or its daemon is not answering.

Both stamp version metadata into the binary the same way `scripts/build.sh` does, so
`docker exec <name> /app/server version` is as informative as a portable
binary.

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

- **Multi-stage.** The Go toolchain, module cache, and source exist only in
  the builder stage. The shipped image contains a single static binary and
  nothing else -- the frontend is compiled into it by `go:embed`, and account
  administration is the `manage` subcommand rather than a second executable.
- **No `static/`, `language.yaml`, `config.yaml` or `secret.key` in the
  image.** `server.static_dir` (`./static` beside the config, so
  `/app/static`) does not exist, so the embedded frontend is used; a
  `language.yaml` in the data directory overrides the embedded wording.
  `config.yaml` and `secret.key` are kept out by `.dockerignore` -- an image
  layer is forever, and a key in one is everyone's -- and mounted at run time
  instead, so one image serves any deployment. Most settings apply while it
  runs; the rest need a restart, never a rebuild.
- **No `ENV`, no `EXPOSE`.** `-tags container` pins the image's fixed
  layout into the binary instead (config at `/app/config/config.yaml`, state in
  `/app/data`, listen on `0.0.0.0`; see `configuration.md`). The port is
  `server.port`, so a hardcoded `EXPOSE` would be stale the first time it
  changed.
- **`CGO_ENABLED=0`.** A statically linked binary with no libc dependency,
  which is what keeps the runtime image small and sidesteps musl-vs-glibc
  surprises entirely.
- **`-trimpath -ldflags="-s -w"`.** Keeps build-machine paths out of the
  binary and drops debug symbol tables -- smaller, and slightly less detail
  for anyone poking at a compromised container.
- **Dependency layer caching.** `go.mod`/`go.sum` are copied and downloaded
  before the source, so editing Go code does not re-download modules.
  `go.sum` ships with the template, so the first build is reproducible and
  does not silently resolve to newer versions.
- **Non-root user with a fixed uid/gid of 10001.** `app` is created with
  `/sbin/nologin`, and `USER app` drops root before the entrypoint runs. The
  number is fixed and high on purpose: `docker/run.sh` grants this uid the
  host's data directory and this gid read access to `config.yaml`, and a
  low system uid is likely to belong to a real account on a Linux host, which
  would get the same access.
- **Only `/app/data` is writable, and only by `app`.** Everything else stays
  root-owned and read-only to the process, so code execution inside the
  container still cannot rewrite the binary or the frontend.
- **Healthcheck is `/app/server health`,** which reads `server.port` from the
  mounted config and asks `/api/health`, an endpoint that touches nothing. A
  healthcheck that queried the database would let a slow query get a working
  container killed and restarted -- turning a small problem into an outage.
  Doing it in the binary also means the runtime image needs no HTTP client.
  `/api/ready` is the one that touches the database: point a load balancer at
  that, never the healthcheck.

The Go version in `go.mod` and the builder image tag must stay in step.
Dependencies here require Go 1.26; bumping one without the other produces a
confusing toolchain error at build time.

`go.mod` carries two version lines. `go 1.26.0` is the language version.
`toolchain go1.26.9` names the oldest compiler with the standard-library
security fixes the template was last checked against (`govulncheck ./...`
reported nothing reachable with it). A local `go build` on an older 1.26
fetches that toolchain on its own. The `golang` image sets `GOTOOLCHAIN=local`,
so there the image itself must be at least that patch: rebuild with a fresh
pull of `golang:1.26-alpine`, or pin `golang:1.26.9-alpine` if you prefer
explicit bumps. Standard-library fixes ship in Go patch releases, not in
`go.mod`'s `require` list, so re-run `govulncheck ./...` on both the toolchain
and the dependencies before a release -- and scan the built image too, which
`govulncheck` does not cover.

## docker/run.sh choices

- **Stops on first run for review.** A fresh clone has no `config/config.yaml` (it
  is gitignored), so the first `docker/run.sh` writes one from the image's
  defaults, and `config/secret.key` with a new key, and exits, naming the settings that depend on what
  sits in front of this. Starting on unreviewed defaults would mean guessing
  at `secure_cookies` and `trust_proxy`.
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
- **Names whatever holds the host port.** With its own old container
  removed, anything still listening on `docker.host_port` is something else,
  and `docker run` would fail with a bind error after creating the
  container. The script names the process (and the container, if it is one)
  and stops instead.
- **Waits until it is really up.** After `docker run -d` it polls the image's
  own healthcheck (`/app/server health`) for up to 30 seconds. If the
  container exits or never answers, it prints the status, the exit code and
  the last 40 log lines, and exits non-zero; only a healthy container gets
  `Ready: <url>`. On a fresh database it then prints the owner's first-run
  login link straight from the log, and points at `scripts/login.sh --docker`
  for a new one later.
- **`--stop-timeout 20`.** A shutdown can take 15 seconds: ten for requests to
  finish, five for a background job. Docker's default of ten would kill the
  process before the activity log and the database close.
- **Publishes to `docker.bind_address`, `127.0.0.1` by default.** This app
  does **not** terminate TLS. Put nginx, Caddy, or Traefik in front and let it
  hold the certificate.
- **`server.secure_cookies` stays `true`** whenever a proxy is serving HTTPS.
  Set it to `false` *only* for a plain-`http://localhost` run. Getting this
  wrong is the single most common cause of "login does nothing": a `Secure`
  cookie is silently dropped over plain http, so the sign-in appears to
  succeed and the session never sticks.
- **`server.trust_proxy: true` plus `server.trusted_proxies`** make Fiber
  believe `X-Forwarded-For`, so the rate limiter sees real client IPs rather
  than the proxy's. The header is believed only on connections from an
  address in `trusted_proxies`, read right to left skipping trusted hops, and
  only once it parses as an IP. Both are required: Fiber with `TrustProxy` on
  and no allowlist trusts no proxy at all (every visitor shares the proxy's
  budget, and one noisy client blocks the rest), and the config check refuses
  `trust_proxy: true` with an empty list. For the Docker setup the proxy
  reaches the container from the bridge gateway, so `[172.16.0.0/12]` covers
  the default bridge -- confirm with `docker network inspect bridge`. It
  defaults to off because it is only correct when a proxy you control really
  is in front and clients cannot reach the backend any other way.
  `docker/run.sh` reminds you while it is off.
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
  Settings screen (or `docker exec <name> /app/server config set ...`) copies
  the previous version to `<data_dir>/config.yaml.bak` and then writes the
  file from the start, truncating the tail last, so it is never empty. On the
  host, edit it with something that saves in place too (vi, nano, VS Code
  do). An editor that writes a new file and renames it over the old one
  leaves the container holding the old file, because a bind mount follows the
  file, not the name. The server notices (the file's link count drops to 0),
  the Settings screen says to restart the container, and saves are refused
  until then.
- **Guards `docker.data_dir`.** `config.yaml` is writable from inside the
  container now, and the script hands `data_dir` to the container's user
  (`chown -R`). So the Settings screen refuses to change `docker:` (and
  `build:`) from inside the container, and the script refuses a `data_dir`
  of `/` or a home directory, or one that is not empty and is not this app's
  (no `app.db`, no `.<slug>-data` marker, which it leaves in every data
  directory it sets up). The marker is written as root inside the container
  as well as from the host, because after the first chown the host user can
  no longer touch anything in the data directory on Linux. It also prints any
  `docker:` setting that changed since the last run (`.run/docker.last`).
- **`--read-only`** plus a `noexec,nosuid` tmpfs for `/tmp`: the container
  cannot write to its own filesystem. This is what turns the Dockerfile's
  ownership choices into an actual guarantee. The tmpfs also holds the control
  socket, so running the image `--read-only` without it stops the server at
  startup unless `server.control_socket` is `false`.
- **`--cap-drop ALL`** and **`--security-opt no-new-privileges:true`**: no
  Linux capabilities, and no process inside can gain more rights than it
  started with, even via a setuid binary.
- **`docker.pids_limit`, `docker.memory`, `docker.cpus`** (128, 256m, 1):
  blast-radius limits, so a runaway loop degrades one container instead of
  the host. Raise these deliberately for real load rather than discovering
  them under traffic.

**Outbound calls.** The container can reach the network, and webhooks are the
one thing in the template that uses it. The image carries `ca-certificates` for
that. Nothing else limits where it can go, so if yours should only reach a few
addresses, enforce that on the host or a firewall too. The app refuses private
addresses on its own: see [Webhooks](architecture.md#webhooks).

### Shell scripts and `set -e`

Every script here runs under `set -euo pipefail`, so a command that reports its
answer through its exit code ends the script, silently, whenever that answer is
"no": `diff` (1 = they differ), `grep` (1 = no match), `cmp`, `lsof` (1 = nothing
listening), and `test`/`[ ]` on a line of their own. Wherever a non-zero result
is expected, use the command as an `if` condition (`if grep -q …; then`) or end
it with `|| true`, which covers the whole pipeline. Being inside an `if` body
does not protect a command: only the condition itself is exempt from `set -e`.
The settings-changed notice (`diff … | sed … || true`) and the port check
(`lsof … | head -n 1 || true`) in `docker/run.sh` are the pattern to copy.

## Paths inside the container

The binary in the image is built with `-tags container`, which pins the app
directory to `/app/data` and the config file to `/app/config/config.yaml`
(with `secret.key` beside it),
overriding `server.app_dir` and the `~/.config/<slug>` default that apply
outside a container. A container has no meaningful home directory, and the
point of the mounts is that state lives on the host.

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

- **Linux**: the host's ownership is real. The container user (uid 10001 in
  this image) needs to be granted it, so the chown/chgrp the script attempts
  matters.
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
- With `server.encrypt_at_rest: true` (default) it encrypts every value in
  the database and every activity-log line -- **losing it, or swapping it by
  hand, makes the data permanently unreadable.**

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
docker exec -it <container-name> /app/server manage list-users
docker exec -it <container-name> /app/server manage reissue-login -user-id 1
```

`reissue-login` and `create-login` take `-uses 5|unlimited` and
`-expires 30m|12h|7d|never` for a link that works more than once or lasts
longer (see `architecture.md`, Reusable login links).

## Operating the container

**bolt allows one process to open the database**, and the running server holds
it. `docker exec ... manage` still works, because the server listens on
`/tmp/control/control.sock` inside the container and the CLI goes through it
rather than opening the file:

```bash
docker exec -it <name> /app/server manage list-users
docker exec -it <name> /app/server manage reissue-login -user-id 1
```

The socket is on the container's tmpfs, not in the data directory, because a
directory shared from the host may be unable to hold one (colima's refuses the
bind). See "Why the container's socket is on tmpfs" in `architecture.md`.

Day-to-day account management still belongs in the admin panel; the CLI is the
recovery path for when nobody can get in.

API keys go through the same door, which is what lets a deploy script hand a
CI system a credential without anyone signing in first:

```bash
docker exec -it <name> /app/server manage create-key -user-id 1 -name "CI" -days 90
docker exec -it <name> /app/server manage list-keys
docker exec -it <name> /app/server manage revoke-key -key-id <id>
```

The key is printed once and is not recoverable -- capture it where the script
needs it, or revoke and mint another. `list-keys` is the standing answer to
"what has programmatic access to this deployment", and worth reading before
assuming the answer is nothing. Set `auth.api_keys: false` (Settings screen,
or `config.yaml` on the host) to close that surface entirely -- it applies at
once.

For a login link, `scripts/login.sh --docker` does the `docker exec` for you
and says which container and address the link is for.

If the container is stopped, or it was started with
`server.control_socket: false`, run a one-off with the same two mounts:

```bash
docker stop <name>
docker run --rm -v "$(pwd)/data:/app/data" \
  -v "$(pwd)/config/config.yaml:/app/config/config.yaml:ro" \
  -v "$(pwd)/config/secret.key:/app/config/secret.key:ro" \
  <image> manage reissue-login -user-id 1
docker start <name>
```

The `secret.key` mount is what carries the key; without it the stored
records cannot be decrypted.

Stop the container first in every case. A one-off has its own `/tmp`, so it
cannot see a running server's socket: it tries the database file instead, waits
5 seconds on bolt's lock, and fails, saying to use `docker exec`.

The socket lives inside the mounted data directory, so it is covered by the
volume's permissions and never crosses the container boundary. Nothing about
it is published to the network, and it needs no port.

### Behind a proxy: uploads

Nothing special is needed: a file arrives as pieces of 768 KiB, under the default
limits of nginx (`client_max_body_size 1m`) and most other proxies. Raising
`files.max_file_bytes` does not change that. A proxy rate limit does need to allow
for the pieces, about 1.3 requests per megabyte.

### Behind a proxy: live updates

A stream is one long response, and two proxy defaults break it. nginx buffers
responses, which `X-Accel-Buffering: no` on the stream turns off; anything else
needs the same done in its own config. Proxies also close a response that goes
quiet: keep the read timeout above the 15-second heartbeat. In nginx:

```nginx
location /api/owner/logs/stream {
    proxy_pass http://127.0.0.1:PORT;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_read_timeout 60s;
}
```

### Scraping `/metrics`

Set `metrics.enabled: true`, make an admin API key just for this, and give it to
Prometheus as a bearer token:

```yaml
scrape_configs:
  - job_name: myapp
    authorization:
      credentials_file: /etc/prometheus/myapp.key
    static_configs:
      - targets: ["myapp.example.com"]
    scheme: https
```

The key is admin-level, so treat the file like any other credential.

## Running from a checkout (scripts/run.sh, scripts/login.sh)

For development and internal boxes, without Docker:

```bash
scripts/run.sh                       # (re)start the server
scripts/login.sh                     # the owner's login link, from whatever is live
scripts/run.sh manage list-users     # any arguments: the CLI, server untouched
```

`scripts/run.sh` always runs `go build -o ./<slug>`; Go's build cache makes
that a near-instant no-op when nothing changed and a real rebuild when any
`.go` file, `go.mod`, or the embedded `static/` / `config/` did. The build
happens before anything is stopped, so a compile error leaves the old server
up. With no arguments it then reads `config shell server` -- every path and
the URL, resolved by the binary exactly as the server will -- and:

- moves `server.secret_key` out of an older `config/config.yaml` into
  `config/secret.key` (`config migrate`; silent when there is nothing to move),
  and refuses to start when `secret.key` is missing but the database exists;
- on a fresh clone with no `config/config.yaml`, writes it and
  `config/secret.key` (`config init`) and stops, as `docker/run.sh` does, saying which URL it will serve and,
  when that is plain `http://`, the line to change so `secure_cookies` does
  not silently break sign-in;
- stops the server it started last: it records its own pid in
  `.run/server.pid` just before `exec`ing the binary, and only signals that
  pid if its command line is still `<checkout>/<slug>` (SIGTERM, then SIGKILL
  after 15 seconds), so a stale pid is never mistaken for it;
- refuses to start, naming the process (via `lsof`), when anything else is
  listening on `server.port` -- including this project's docker container;
- warns when `secure_cookies` is on but the URL is plain `http://`;
- prints the URL and where it came from, the config file and secret.key, the app dir, the
  database (existing, or new with the first login link to follow), and
  whether static/ and language come from disk or the embedded copies;
- runs the server in the foreground and prints `Ready: <url>` once
  `/api/health` answers (or says it is not answering after 20 seconds).

Running it again from another terminal is how you restart. With arguments it
runs the binary with them and leaves the server alone; `manage` reaches the
live server over its control socket. The shared helpers are in
`scripts/lib.sh`.

`scripts/login.sh [user-id] [--uses N|unlimited] [--expires 30m|12h|7d|never] [--docker|--local|--no-start|--background]`
prints a login link (user 1, the owner, by default) as a full URL. It works
once, for `auth.login_token_ttl_seconds`, unless `--uses` or `--expires` say
otherwise; `scripts/login.bat` takes the same. It decides where from: the
local server when this checkout's is running (through its control socket),
otherwise the docker container when it is running (`docker exec`), otherwise
it writes the link into the local database, prints it, and starts the server
in that terminal, where Ctrl-C stops it. `--no-start` stops after writing the
link; `--background` starts the server detached and exits.
The local and container deployments keep separate databases, so it always
says which one the link is for; when both run, it uses the local one and
says so. Errors are specific: no config yet, no database yet, no such user
(with the command that lists them), container not running (with its status).

On Windows, `scripts\run.bat` and `scripts\login.bat` take the same arguments
and do the same, reading `config shell -format bat` and using PowerShell
for the port owner and the readiness check. `run.bat` has three differences
Windows forces. A running `.exe` cannot be overwritten, so it builds to
`.run\<slug>.exe` and copies that over `<slug>.exe` only after the old server
exits (one-offs run the `.run\` copy and never touch it). It finds the old
server by image path -- any process running this checkout's `<slug>.exe` --
instead of a pid file. And with no SIGTERM to send another console's
process, it kills the old server instead of draining it; bolt is crash-safe
and `control.Listen` clears the stale socket on the next start. It needs only
`go`, plus PowerShell (built into Windows).

Run from the checkout, `./static` is served from disk, so frontend edits need
only a page reload. Like a portable binary, it has no sandbox: none of
Docker's read-only filesystem, dropped capabilities, or pid and memory limits
apply.

## Portable binaries (scripts/build.sh)

`scripts/build.sh` cross-compiles a self-contained executable per platform into
`dist/`, plus `SHA256SUMS`. Each one carries the whole frontend, so it runs
with no Docker, no Go toolchain, and no files beside it.

```bash
scripts/build.sh                  # every target
scripts/build.sh linux/amd64      # just one
```

Three choices make that work: `CGO_ENABLED=0` (static linking, so there is no
libc version to match), `go:embed` (the frontend is inside the binary), and
bolt (the database is one file the binary creates on first run).

The binary name, output directory, and default target list are the
`build:` section of `config.yaml`, read through `go run . config shell build`
(targets on the command line replace the list for one run).

`build.sh` also stamps version, commit, and build date via `-ldflags -X`, so
`./myapp version` can identify a binary once a few copies are in circulation.
The build date is the last commit's timestamp rather than "now", and
`-trimpath` keeps checkout paths out, so building the same commit twice gives
the same bytes.

**When to use which.** Docker is where the sandboxing lives -- read-only
filesystem, dropped capabilities, pid and memory limits, restart policy. A
portable binary has none of that; it is a normal process with the invoking
user's permissions. Use binaries for an internal box with no container
runtime, a Pi, a demo, or handing someone a copy. Use Docker for anything
long-lived or internet-facing. Do not let a bare binary end up on a public
host on the assumption that it is equally protected.

Two things to pass on with any binary you hand over:

- **Its `secret.key` is the key.** On first run the binary writes
  `~/.config/<slug>/config.yaml` and a `secret.key` with a fresh key, which
  encrypts session cookies and, with `server.encrypt_at_rest` on, the
  database. Whoever runs the binary owns backing that file up with the data.
- **`server.secure_cookies` must be true anywhere real.** Only `false` for
  plain `http://localhost`.

Distributing an executable is a trust decision for whoever receives it, which
is why `build.sh` writes `SHA256SUMS` -- give them a way to check what they
got is what you built.

## Backups

The state is `data/` plus `config/secret.key` and `config/config.yaml`.
`manage backup` writes one file holding the database, `config.yaml` and
`storage/`, **while the server runs**: bolt copies the database from a read
transaction, which sees one fixed moment and holds up no writer.

```bash
docker exec <container-name> /app/server manage backup
docker cp <container-name>:/app/data/backups/backup-<time>.tar.gz .
```

The file lands in `data/backups/`. It belongs to the container's user on a
Linux host, which is what `docker cp` is for.

**`secret.key` is not in a backup unless you pass `-include-key`**, and nothing
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
Settings screen or `config.yaml`, and the `backup` job writes
`auto-<time>.tar.gz` every `backups.interval_hours`, keeping the last
`backups.keep`. Backups made by hand are never removed. They sit on the same
disk as the data, so copy them somewhere else as well. The owner's Jobs screen
shows how the last one went.

A backup holds the database, and so each live session's sign-in address and
browser, while the activity log stays out. Treat backups as personal data.

`configuration.md` covers the equivalent for portable binaries, where the key
lives inside the app directory instead.

## Tuning

Every knob is a key in `config.yaml` -- session and login-token lifetimes,
rate limits, body size, at-rest encryption, proxy trust, and the container's
own resource limits. `config/defaults.yaml` documents all of them with
their defaults.

Edit the project's `config/config.yaml` and re-run `docker/run.sh` (or just
`docker restart <name>` for a `server:` or `auth:` change, since the file is
mounted rather than baked in). The image stays the same across environments;
each deployment's `config.yaml` is what differs. See `configuration.md`.
