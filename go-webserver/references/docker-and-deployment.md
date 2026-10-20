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
then runs *its* binary (`config shell docker`) in a throwaway container with
no network, a read-only root, and no capabilities, and `eval`s the
single-quoted assignments it prints. So the deploy gets the server's own
defaults and strict validation, and needs nothing on the host but Docker. The
image name lives in `config.yaml` too, which is why it is applied with
`docker tag` after the build rather than passed to it. An extra tag is a
positional argument: `./dockerBuild.sh v1.2.3`.

Both stamp version metadata into the binary the same way `build.sh` does, so
`docker exec <name> /app/server version` is as informative as a portable
binary.

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
- **No `static/`, `language.yaml`, or `config.yaml` in the image.**
  `server.static_dir` (`./static` beside the config, so `/app/static`) does
  not exist, so the embedded frontend is used; a `language.yaml` in the data
  directory overrides the embedded wording. `config.yaml` is kept out by
  `.dockerignore` because it holds the secret key and an image layer is
  forever -- it is mounted at run time instead, so one image serves any
  deployment and a config change needs a restart, not a rebuild.
- **No `ENV`, no `EXPOSE`.** `-tags container` pins the image's fixed
  layout into the binary instead (config at `/app/config.yaml`, state in
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
  number is fixed and high on purpose: `dockerRun.sh` grants this uid the
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
  `dockerRun.sh` reminds you while it is off.
- **`config.yaml` is mounted read-only, separately from the data
  directory,** so the secret key is never inside `docker.data_dir`. The
  script makes the file readable by the container's group (0640, you stay the
  owner) and tests that it worked, the same way it handles the data
  directory below.
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

The binary in the image is built with `-tags container`, which pins the app
directory to `/app/data` and the config file to `/app/config.yaml`,
overriding `server.app_dir` and the `~/.config/<slug>` default that apply
outside a container. A container has no meaningful home directory, and the
point of the mounts is that state lives on the host.

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

- **Linux**: the host's ownership is real. The container user (uid 10001 in
  this image) needs to be granted it, so the chown/chgrp the script attempts
  matters.
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
- With `server.encrypt_at_rest: true` (default) it encrypts every value in
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
docker exec -it <container-name> /app/server manage list-users
docker exec -it <container-name> /app/server manage reissue-login -user-id 1
```

## Operating the container

**bolt allows one process to open the database**, and the running server holds
it. `docker exec ... manage` still works, because the server listens on
`control.sock` inside the data directory and the CLI goes through it rather
than opening the file:

```bash
docker exec -it <name> /app/server manage list-users
docker exec -it <name> /app/server manage reissue-login -user-id 1
```

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
assuming the answer is nothing. Set `auth.api_keys: false` in `config.yaml`
and restart to close that surface entirely.

If the container is stopped, or it was started with
`server.control_socket: false`, run a one-off with the same two mounts:

```bash
docker stop <name>
docker run --rm -v "$(pwd)/data:/app/data" \
  -v "$(pwd)/config.yaml:/app/config.yaml:ro" \
  <image> manage reissue-login -user-id 1
docker start <name>
```

The `config.yaml` mount is what carries the secret key; without it the
stored records cannot be decrypted.

The socket lives inside the mounted data directory, so it is covered by the
volume's permissions and never crosses the container boundary. Nothing about
it is published to the network, and it needs no port.

## Portable binaries (build.sh)

`./build.sh` cross-compiles a self-contained executable per platform into
`dist/`, plus `SHA256SUMS`. Each one carries the whole frontend, so it runs
with no Docker, no Go toolchain, and no files beside it.

```bash
./build.sh                  # every target
./build.sh linux/amd64      # just one
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

- **Its `config.yaml` is the key.** On first run the binary writes
  `~/.config/<slug>/config.yaml` with a fresh `server.secret_key`, which
  encrypts session cookies and, with `server.encrypt_at_rest` on, the
  database. Whoever runs the binary owns backing that file up with the data.
- **`server.secure_cookies` must be true anywhere real.** Only `false` for
  plain `http://localhost`.

Distributing an executable is a trust decision for whoever receives it, which
is why `build.sh` writes `SHA256SUMS` -- give them a way to check what they
got is what you built.

## Backups

The entire state is the `data/` directory plus `config.yaml`, and you need
both -- `app.db` without the key in `config.yaml` is unreadable. bolt is a single file, so a
copy taken while the server is stopped is complete and consistent; copying
from under a running server can catch a write in progress. Use `docker stop`,
copy, `docker start`.

`configuration.md` covers the equivalent for portable binaries, where the key
lives inside the app directory instead.

## Tuning

Every knob is a key in `config.yaml` -- session and login-token lifetimes,
rate limits, body size, at-rest encryption, proxy trust, and the container's
own resource limits. `server/config/defaults.yaml` documents all of them with
their defaults.

Edit the project's `config.yaml` and re-run `./dockerRun.sh` (or just
`docker restart <name>` for a `server:` or `auth:` change, since the file is
mounted rather than baked in). The image stays the same across environments;
each deployment's `config.yaml` is what differs. See `configuration.md`.
