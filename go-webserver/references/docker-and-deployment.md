# Docker & deployment reference

## Two scripts

`dockerBuild.sh` builds the image; `dockerRun.sh` calls it and then runs the
container. They are split so building and running are separately runnable --
CI can build without starting anything, and a restart-only redeploy need not
rebuild. The common case is still one command: `./dockerRun.sh`.

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
- **No `static/` or `language.yaml` in the image.** `STATIC_DIR` and
  `LANGUAGE_FILE` keep their defaults (`/app/static`, `/app/language.yaml`),
  neither of which exists, so the embedded copies are used. Mounting either
  path overrides it -- which is the supported way to run the shipped UI with
  different wording:
  `-v ./language.yaml:/app/language.yaml:ro`.
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
- **Non-root user.** `app` is created with `/sbin/nologin`, and `USER app`
  drops root before the entrypoint runs.
- **Only `/app/data` is writable, and only by `app`.** Everything else stays
  root-owned and read-only to the process, so code execution inside the
  container still cannot rewrite the binary or the frontend.
- **Healthcheck hits `/api/health`,** which touches nothing. A healthcheck
  that queried the database would let a slow query get a working container
  killed and restarted -- turning a small problem into an outage.

The Go version in `go.mod` and the builder image tag must stay in step.
Dependencies here require Go 1.26; bumping one without the other produces a
confusing toolchain error at build time.

## dockerRun.sh choices

- **Publishes to `127.0.0.1` only.** This app does **not** terminate TLS. Put
  nginx, Caddy, or Traefik in front and let it hold the certificate.
- **`SECURE_COOKIES` stays `true`** whenever a proxy is serving HTTPS. Set it
  to `false` *only* for a plain-`http://localhost` run. Getting this wrong is
  the single most common cause of "login does nothing": a `Secure` cookie is
  silently dropped over plain http, so the redirect succeeds and the session
  never sticks.
- **`TRUST_PROXY=true`** makes Fiber believe `X-Forwarded-For`, so the rate
  limiter sees real client IPs rather than the proxy's. Only correct when a
  proxy you control really is in front -- otherwise clients forge their own IP
  and walk straight past the limiter.
- **`--read-only`** plus a `noexec,nosuid` tmpfs for `/tmp`: the container
  cannot write to its own filesystem. This is what turns the Dockerfile's
  ownership choices into an actual guarantee.
- **`--cap-drop ALL`** and **`--security-opt no-new-privileges:true`**: no
  Linux capabilities, and no process inside can gain more rights than it
  started with, even via a setuid binary.
- **`--pids-limit 128`, `--memory 256m`, `--cpus 1`**: blast-radius limits, so
  a runaway loop degrades one container instead of the host. Raise these
  deliberately for real load rather than discovering them under traffic.

## APP_DIR in containers

`APP_DIR=/app/data` is set explicitly in the Dockerfile and by `dockerRun.sh`,
overriding the `~/.config/<slug>` default that applies outside a container. A
container has no meaningful home directory, and the point of the mount is that
state lives on the host.

Everything the app persists is therefore in that one mounted directory --
database, `storage/`, and an optional `config.yaml`. The exception is
`SECRET_KEY`, which `dockerRun.sh` passes from `.secret_key` on the host so
the key stays *outside* the volume. See `configuration.md` for why that
distinction is worth keeping.

## The data directory and container ownership

`dockerRun.sh` bind-mounts `./data` so the database stays visible on the host
for backups. A bind mount ignores the image's ownership, though, so the script
makes sure the container's unprivileged user can actually write there before
starting anything.

It has to, because the two platforms disagree:

- **Linux**: the host directory's ownership is real. The container user (uid
  100 in this image) needs to be granted it, so the chown matters.
- **macOS** (Docker Desktop, colima): the file-sharing layer ignores guest
  ownership -- any uid can write, and the chown fails harmlessly.

So the chown is attempted, its failure is tolerated, and a write is then
actually tested. If the test fails the script stops with the exact `chown`
command to run, rather than starting a container that crash-loops on
"permission denied" -- which looks like a bug in the app and is not.

## SECRET_KEY

`dockerRun.sh` generates `.secret_key` once (mode 600) on the host and reuses
it on every run. It must stay stable:

- It encrypts session cookie payloads -- rotating it logs everyone out.
- With `ENCRYPT_AT_REST=true` (default) it encrypts every value in the
  database -- **rotating or losing it makes the data permanently unreadable.**

So `.secret_key` is as important as `data/` for backups. Back up both together
or neither is useful. If the user is setting up backups, say this explicitly;
it is the kind of thing discovered at the worst possible time.

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

**bolt allows one writer process at a time**, and the running server holds it.
So `docker exec ... manage` against a live container does not work -- it
blocks and times out after 5s. This is not a bug to work around; it is what
keeps two processes from corrupting the file.

Day-to-day account management therefore belongs in the admin panel, which goes
through the running server and needs no downtime.

The CLI is the recovery path, for when nobody can get in. Stop the container
and run a one-off against the same data directory:

```bash
docker stop <name>
docker run --rm -v "$(pwd)/data:/app/data" -e "SECRET_KEY=$(cat .secret_key)" \
  <image> manage list-users
docker run --rm -v "$(pwd)/data:/app/data" -e "SECRET_KEY=$(cat .secret_key)" \
  <image> manage reissue-login -user-id 1
docker start <name>
```

`SECRET_KEY` has to be passed and has to match, or the stored records cannot
be decrypted.

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

`build.sh` also stamps version, commit, and build date via `-ldflags -X`, so
`./myapp version` can identify a binary once a few copies are in circulation.
It honours `SOURCE_DATE_EPOCH` and passes `-trimpath`, so builds are
reproducible.

**When to use which.** Docker is where the sandboxing lives -- read-only
filesystem, dropped capabilities, pid and memory limits, restart policy. A
portable binary has none of that; it is a normal process with the invoking
user's permissions. Use binaries for an internal box with no container
runtime, a Pi, a demo, or handing someone a copy. Use Docker for anything
long-lived or internet-facing. Do not let a bare binary end up on a public
host on the assumption that it is equally protected.

Two things to pass on with any binary you hand over:

- **`SECRET_KEY` must be the same on every run.** It encrypts session cookies
  and, with `ENCRYPT_AT_REST` on, the database. There is no `dockerRun.sh`
  generating and persisting it here, so whoever runs the binary owns that.
- **`SECURE_COOKIES` must be true anywhere real.** Only `false` for plain
  `http://localhost`.

Distributing an executable is a trust decision for whoever receives it, which
is why `build.sh` writes `SHA256SUMS` -- give them a way to check what they
got is what you built.

## Backups

The entire state is the `data/` directory plus `.secret_key`, and you need
both -- `app.db` without the key is unreadable. bolt is a single file, so a
copy taken while the server is stopped is complete and consistent; copying
from under a running server can catch a write in progress. Use `docker stop`,
copy, `docker start`.

`configuration.md` covers the equivalent for portable binaries, where the key
lives inside the app directory instead.

## Tuning

Every knob is resolved in `server/config/config.go` as environment variable,
then `config.yaml`, then default: `SESSION_TTL_SECONDS`,
`LOGIN_TOKEN_TTL_SECONDS`, `RATE_LIMIT_MAX`, `RATE_LIMIT_WINDOW_SECONDS`,
`BODY_LIMIT_BYTES`, `ENCRYPT_AT_REST`, `TRUST_PROXY`, and the rest --
`config.example.yaml` lists all of them.

In a container, prefer `-e` flags in `dockerRun.sh` over editing defaults into
`config.go`, so the same image stays reusable across environments. A
`config.yaml` dropped into the mounted app directory also works and survives
image rebuilds, which suits settings that belong to a particular deployment
rather than to a particular run. See `configuration.md`.
