# Configuration and state

Two rules underpin this: **one `config.yaml` configures everything**, and
**everything the app persists lives in a single directory**. Read this before
adding a setting or writing a file anywhere.

## Contents

- [One file, no environment variables](#one-file-no-environment-variables)
- [Where config.yaml is found](#where-configyaml-is-found)
- [Defaults and strict validation](#defaults-and-strict-validation)
- [How the scripts read it](#how-the-scripts-read-it)
- [The secret key](#the-secret-key)
- [The app directory](#the-app-directory)
- [Why one directory, and why ~/.config](#why-one-directory-and-why-config)
- [Reaching the database from another process](#reaching-the-database-from-another-process)
- [Storage for application files](#storage-for-application-files)
- [In containers](#in-containers)
- [Backups](#backups)
- [Adding a setting](#adding-a-setting)

## One file, no environment variables

```yaml
app:     name
server:  host, port, app_dir, static_dir, language_file, secret_key,
         encrypt_at_rest, secure_cookies, cookie_name, trust_proxy,
         trusted_proxies, proxy_header, body_limit_bytes, rate_limit_max,
         rate_limit_window_seconds, control_socket
auth:    session_ttl_seconds, login_token_ttl_seconds, api_keys,
         api_key_ttl_seconds
docker:  image, container, bind_address, host_port, data_dir, restart,
         memory, cpus, pids_limit, tmpfs_size
build:   binary_name, output_dir, targets
```

The server, `manage`, `run.sh`, `dockerRun.sh`, `dockerBuild.sh`, and
`build.sh` all read this one file. Nothing reads an environment variable for configuration:
not `server/config`, not the scripts, and the container is started with no
`-e` flags and an image with no `ENV`.

That is a deliberate trade. Environment overrides are convenient for a
one-off run, but a setting that can come from the environment, a file, or a
default has to be hunted for in three places -- and a deploy script that sets
`-e TRUST_PROXY=true` quietly disagrees with a `config.yaml` that says
`false`. With one source there is nothing to reconcile: what the file says is
what runs.

The only environment the tooling touches is the Go toolchain's own
(`CGO_ENABLED`, `GOOS`, `GOARCH` in `build.sh` and the Dockerfile), which is
how `go build` is told what to build, not app configuration.

## Where config.yaml is found

First match wins (`server/config.Locate`):

1. `/app/config.yaml` -- only in the container image, which pins it (see
   [In containers](#in-containers))
2. `./config.yaml` -- a source checkout; also what `run.sh`, `dockerRun.sh`,
   and `build.sh` read
3. `~/.config/<slug>/config.yaml` -- a portable binary run from anywhere else

If none exists, the server (and `manage`) **create the third one** from the
defaults with a fresh secret key at mode 0600, and say so in the startup log.
That is what lets a portable binary just run with nothing beside it.

`scaffold.py` writes `./config.yaml` for a new project. A fresh clone has none,
because it is gitignored: `go run . config init` writes one, and the first
`./dockerRun.sh` does the same and then stops so the file can be reviewed.

### config.example.yaml

`config.yaml` holds the secret key, so it is never committed. What is
committed is `config.example.yaml`: every default, the comments that document
them, and an empty `secret_key`. Someone on a fresh clone can read it to see
every setting, or copy it into place:

```bash
cp config.example.yaml config.yaml && chmod 600 config.yaml
openssl rand -hex 32        # paste into server.secret_key
```

The empty key is deliberate. A key in a committed file is everyone's key,
and a copy that never gets one refuses to start with the line to run, rather
than running on a shared secret. `config init` is still the one-step way --
it writes the same file with a key already in it.

The example is generated, never edited by hand: it is
`server/config/example_header.yaml` (the copy-me note) followed by
`defaults.yaml`. `scaffold.py` writes it on every scaffold, and
`go run . config example` rewrites it after a default changes, so it cannot
drift from what the server actually uses.

Relative paths in `server:` resolve against the directory holding
`config.yaml`, so the same file means the same thing whatever directory the
process was started from. Relative paths in `docker:` and `build:` resolve
against the project directory (the scripts' directory, which is also where
`config.yaml` lives).

Ask a binary which file it uses, without starting it:

```bash
./myapp config check        # validates it and prints the path
./myapp manage paths        # the config file, app dir, database, socket
```

## Defaults and strict validation

Defaults are not written in Go. They live in `server/config/defaults.yaml`,
which is embedded in the binary, documents every key, and is exactly what
`config init` writes out (with the key filled in). A `config.yaml` is decoded
*on top of* it, so a key you leave out takes the value shown there, and a
`config.yaml` holding only the keys you changed is valid.

Decoding is strict (`yaml.v3` with `KnownFields`). A typo is an error that
names the line and the key:

```
config.yaml: yaml: unmarshal errors:
  line 52: field secure_cookie not found in type config.ServerSection
```

When the file is the only source of configuration, a misspelled key that was
silently ignored would be a setting that silently is not applied -- exactly
the failure this design exists to remove. Values are validated too: ports in
range, Docker names, sizes and restart policies in the forms Docker accepts,
build targets shaped like `linux/amd64`.

## How the scripts read it

The scripts do not parse YAML. They ask the app:

```bash
./myapp config shell docker     # docker_image='myapp'  docker_publish=...
./myapp config shell build      # build_binary_name=... build_targets=...
```

`config shell` validates the *whole* file, then prints one section as
single-quoted shell assignments that the script `eval`s. It never prints the
secret key. So the scripts get the same defaults, the same strict decoding,
and the same validation as the server, and a typo in the `docker:` section
fails the deploy before anything starts.

- `build.sh` already needs Go, so it uses `go run . config shell build`.
- The Docker scripts need only Docker on the host, so they build the image
  first and run *its* binary in a throwaway container (no network, read-only,
  no capabilities) with `config.yaml` mounted. That ordering is why the image
  name -- which is in `config.yaml` like everything else -- is applied as a
  tag after the build.

## The secret key

`server.secret_key` encrypts session cookie payloads and, with
`server.encrypt_at_rest` on (the default), every value written to the
database. It is 64 hex characters, generated whenever a `config.yaml` is
created (`scaffold.py`, `config init`, a binary's first run, a fresh
`dockerRun.sh`).

That makes **`config.yaml` itself a secret**: it is written at mode 0600, it
is in `.gitignore` and `.dockerignore`, and the server logs a warning on every
start if it is readable by every user. Never commit it and never bake it into
an image.

An empty `secret_key` in an existing file is refused, not filled in. A file
that exists was created with a key, so an empty one means it was removed --
and quietly generating a replacement would make every existing record
permanently unreadable. The error says how to generate one for a genuinely
fresh install (`openssl rand -hex 32`).

**Be clear about the trade this makes for a portable binary.** Its
`config.yaml` lives in `~/.config/<slug>/` beside `app.db`, so at-rest
encryption protects a database that leaks *on its own* -- a stray copy of
`app.db`, a backup that captured only the database file -- but not someone who
takes the whole directory. Docker keeps them apart: `config.yaml` stays in the
project directory and is mounted separately from the data directory, so a
leaked backup of `./data` is not enough to read it. A binary gets the same
separation by setting `server.app_dir` to somewhere other than the directory
holding its `config.yaml`.

Losing the key loses the data. There is no recovery, and no in-place
re-encryption in the template -- rotating means exporting, changing the key,
and re-importing.

## The app directory

```
~/.config/<app-slug>/
  app.db           the bolt database
  control.sock     unix socket, mode 0600, only while the server is running
  storage/         application files (uploads, exports, generated artifacts)
  language.yaml    optional; overrides the embedded UI text
  config.yaml      when the binary created it here (see above)
```

`server.app_dir` moves it; empty means `~/.config/<slug>/`. With no home
directory at all (some minimal environments) it falls back to
`./<slug>-data` rather than failing to start. Inside the container it is
always `/app/data`.

The running server prints where everything landed, which is the fastest
answer to "where is my data":

```
[config] /home/you/.config/myapp/config.yaml
[state]  /home/you/.config/myapp
```

## Why one directory, and why ~/.config

**Why not `./data` next to the binary?** Because the single-file build is
meant to be moved around. A binary run from `~/Downloads` once and `/opt`
later would leave two half-populated databases and look like it had lost the
data. Anchoring state to the user rather than the working directory means the
app finds the same state wherever it is launched from.

**Why one directory instead of XDG's split?** Strict XDG would put config in
`~/.config`, the database in `~/.local/share`, and caches elsewhere. That
split earns its keep for large applications; here the entire state is a bolt
file and some uploads. One directory to back up -- and one to delete when
uninstalling -- is worth more than the taxonomy. `XDG_CONFIG_HOME` is not
consulted: it is an environment variable, and the location of the one config
file should not depend on one.

## Reaching the database from another process

bolt allows exactly one process to open its file, so a second process cannot
read or write the database directly while the server holds it. The running
server listens on `control.sock` in the app directory, and `manage` uses it
automatically:

```bash
./myapp manage reissue-login -user-id 1   # works with the server running
```

With the server stopped, the same command opens the database itself. Either
way it prints which route it took on stderr. `server.control_socket: false`
disables the socket; `manage` then requires the server to be stopped.

Anything else that needs the data -- a cron job, an importer, a sidecar --
should go through `server/control` rather than opening the bolt file, so that
one process keeps holding the secret key. See `architecture.md`.

A unix socket path is capped by the kernel (104 bytes on macOS, 108 on Linux).
A deep `server.app_dir` will exceed it; the server checks this at startup and
says so plainly rather than passing the kernel's bare "invalid argument" along.

## Storage for application files

`cfg.StorageDir` (`<app dir>/storage/`) is created at startup and is where
features added later should write files -- uploads, exports, generated
artifacts. Nothing in the base template writes there.

It exists so that the first feature needing to save a file has an obvious
place to put it, rather than inventing a path that then has to be discovered
separately at backup time. Use `filepath.Join( cfg.StorageDir , ... )` and
never a path relative to the working directory, which changes depending on how
the process was started.

## In containers

The Dockerfile builds with `-tags container`, which compiles in
`server/config/layout_container.go`. That pins three things regardless of
what `config.yaml` says:

| Pinned | Why |
|---|---|
| config file `/app/config.yaml` | where `dockerRun.sh` mounts `./config.yaml`, read-only |
| app dir `/app/data` | where `dockerRun.sh` mounts `docker.data_dir`, the only writable mount |
| listen host `0.0.0.0` | inside the container's own network namespace; exposure is `docker.bind_address` |

These are not settings because they are not choices -- they are the paths the
image is mounted into. Pinning them in the binary rather than in `ENV` is what
lets `docker exec <name> /app/server manage ...` find the same config and
database as the server with nothing extra to type, and stops a host-oriented
`server.app_dir` in the shared file from pointing the container at a path
that is not mounted. Everything else -- `server.port`, cookies, proxy trust,
TTLs -- comes from the mounted `config.yaml` as usual.

To reword the UI in a container without rebuilding, put a `language.yaml` in
the data directory (`/app/data/language.yaml`).

## Backups

Back up **`config.yaml` and the app directory together**:

- Portable binary: the whole `~/.config/<slug>/` directory (`config.yaml` is
  inside it unless `server.app_dir` moved the state elsewhere).
- Docker: `./config.yaml` **and** `docker.data_dir` (`./data` by default) --
  deliberately separate.

Either half alone is useless: `app.db` without the key is unreadable, and the
key without the database is nothing. bolt is a single file, so a copy taken
while the server is stopped is complete and consistent. Copying from under a
running server can catch a write in progress -- stop it first.

## Adding a setting

1. Add the key to `server/config/defaults.yaml`, with its default and a
   one-line comment on what it does and, where it matters, what goes wrong if
   it is set incorrectly. This file is the documentation.
2. Add the field to the matching section struct (`ServerSection`,
   `AuthSection`, ...) in `server/config/config.go`. Strict decoding rejects a
   key the struct does not have, so a key in `defaults.yaml` without a field
   fails at startup rather than drifting.
3. If the value needs checking, add it to `File.Validate`.
4. If the server reads it, copy it onto `Config` in `Load`. If a script reads
   it, add it to `config shell` in `server/config/command.go`.
5. Regenerate the committed example: `go run . config example`. It is
   `server/config/defaults.yaml` plus a header, and is never edited by hand.

Do not read `os.Getenv` anywhere, and do not make a script read an
environment variable instead of adding the key. Keeping every knob in one file
is what makes "what can this be told to do" answerable by reading one file.
