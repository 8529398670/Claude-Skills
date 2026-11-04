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
- [Changing settings while the server runs](#changing-settings-while-the-server-runs)
- [The app directory](#the-app-directory)
- [Why one directory, and why ~/.config](#why-one-directory-and-why-config)
- [Reaching the database from another process](#reaching-the-database-from-another-process)
- [Storage for application files](#storage-for-application-files)
- [The activity log's settings](#the-activity-logs-settings)
- [In containers](#in-containers)
- [Backups](#backups)
- [Adding a setting](#adding-a-setting)

## One file, no environment variables

```yaml
app:     name
server:  host, port, public_url, app_dir, static_dir, language_file,
         encrypt_at_rest, secure_cookies, cookie_name, trust_proxy,
         trusted_proxies, proxy_header, body_limit_bytes, rate_limit_max,
         rate_limit_window_seconds, control_socket, mcp
auth:    session_ttl_seconds, login_token_ttl_seconds, api_keys,
         api_key_ttl_seconds, passwords, two_factor
logs:    enabled, retention_days, max_total_mb, max_file_mb,
         compress_after_days, static_assets, ui_clicks, echo
docker:  image, container, bind_address, host_port, data_dir, restart,
         memory, cpus, pids_limit, tmpfs_size
build:   binary_name, output_dir, targets
```

Two defaults differ per project: `scaffold.py` writes a random free
`server.port` (and the same `docker.host_port`) into the project's
`config/defaults.yaml`, and `server.cookie_name` is `<slug>_session`. That lets
several projects run on one machine at once without signing each other out --
browsers scope cookies by host, not port, so the distinct cookie name matters as
much as the distinct port.

The server, `manage`, everything in `scripts/` (`run`, `login`, `build`) and
everything in `docker/` all read this one file. Nothing reads an environment variable for configuration:
not `server/config`, not the scripts, and the container is started with no
`-e` flags and an image with no `ENV`.

That is a deliberate trade. Environment overrides are convenient for a
one-off run, but a setting that can come from the environment, a file, or a
default has to be hunted for in three places -- and a deploy script that sets
`-e TRUST_PROXY=true` quietly disagrees with a `config.yaml` that says
`false`. With one source there is nothing to reconcile: what the file says is
what runs.

The only environment the tooling touches is the Go toolchain's own
(`CGO_ENABLED`, `GOOS`, `GOARCH` in `scripts/build.sh` and the Dockerfile), which is
how `go build` is told what to build, not app configuration.

## Where config.yaml is found

First match wins (`server/config.Locate`):

1. `/app/config/config.yaml` -- only in the container image, which pins it
   (see [In containers](#in-containers))
2. `./config/config.yaml` -- a source checkout; also what every script in
   `scripts/` and `docker/` reads
3. `~/.config/<slug>/config.yaml` -- a portable binary run from anywhere else

If none exists, the server (and `manage`) **create the third one** from the
defaults at mode 0600, with a `secret.key` holding a fresh key beside it, and
say so in the startup log. That is what lets a portable binary just run with
nothing beside it.

`scaffold.py` writes `config/config.yaml` for a new project. A fresh clone has
none, because it is gitignored: `go run . config init` writes one, and the
first `scripts/run.sh` or `docker/run.sh` does the same and then stops so the
file can be reviewed -- saying which URL it will serve and, when that URL is
plain `http://`, which line to change so `secure_cookies` does not silently
break sign-in.

### config.example.yaml

`config.yaml` is this deployment's settings, so it is not committed. What is
committed is `config.example.yaml`: every default and the comments that
document them. Someone on a fresh clone can read it to see every setting, or
copy it into place:

```bash
cp config/config.example.yaml config/config.yaml && chmod 600 config/config.yaml
```

There is no key in it to fill in: the server writes `config/secret.key` on its
first start, as long as there is no database yet (see
[The secret key](#the-secret-key)). `config init` is the one-step way -- it
writes both files.

The example is generated, never edited by hand: it is
`config/example_header.yaml` (the copy-me note) followed by
`config/defaults.yaml`. `scaffold.py` writes it on every scaffold, and
`go run . config example` rewrites it after a default changes, so it cannot
drift from what the server actually uses.

### Relative paths: the project directory

Every relative path in the file resolves against one place, the **project
directory** (`config.ProjectDirFor`):

- when `config.yaml` sits in a folder named `config/` -- a checkout, or
  `/app/config/` in the image -- it is the folder around that, so
  `static_dir: ./static` and `data_dir: ./data` mean the project's `static/`
  and `data/`;
- otherwise (`~/.config/<slug>/config.yaml`) it is the folder holding
  `config.yaml`.

So the same file means the same thing whatever directory a process was
started from. The scripts `cd` to the project root before anything else,
which is where `docker:` and `build:` paths land too.

### What the scripts are told

The scripts never parse YAML or work a path out themselves. They ask the
binary, which resolves everything exactly the way the server does:

```bash
scripts/run.sh config shell server   # config_file, project_dir, app_dir,
                                     # database(+_exists), static_dir(+_on_disk),
                                     # language_file, host, port, url,
                                     # url_source, secure_cookies, ...
scripts/run.sh config shell docker   # docker_* values + docker_url
scripts/run.sh config shell build    # build_* values
./myapp config shell -format bat server   # plain key=value, for the .bat files
```

`sh` output is single-quoted for `eval`; `bat` output is unquoted for
`for /f "tokens=1,* delims=="`. Neither ever includes the secret key. A
value the scripts need goes here first, so the shell and batch files and the
server cannot disagree about it.

Ask a binary which file it uses, without starting it:

```bash
./myapp config check        # validates it and prints the path
./myapp manage paths        # the config file, project dir, URL, app dir,
                            # database, static, language, socket
```

### server.public_url: the address in printed links

Login, invite and API-key links are printed as one full URL. The server
cannot see what sits in front of it, so it takes the origin from
`server.public_url` (e.g. `https://app.example.com`; validated as a bare
http(s) origin, no path). Empty, it falls back to the listen address --
`http://server.host:server.port`, a wildcard host shown as `127.0.0.1` -- or,
inside the container, to `http://docker.bind_address:docker.host_port`. Every
printed link says which of those it used, so a `127.0.0.1` link is not
mistaken for one other people can open.

## Defaults and strict validation

Defaults are not written in Go. They live in `config/defaults.yaml`,
which `main.go` embeds and hands to `server/config` (`config.UseEmbedded`), documents every key, and is exactly what
`config init` writes out. A `config.yaml` is decoded
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
secret key -- only where `secret.key` is. So the scripts get the same defaults, the same strict decoding,
and the same validation as the server, and a typo in the `docker:` section
fails the deploy before anything starts.

- `build.sh` already needs Go, so it uses `go run . config shell build`.
- The Docker scripts need only Docker on the host, so they build the image
  first and run *its* binary in a throwaway container (no network, read-only,
  no capabilities) with `config.yaml` mounted. That ordering is why the image
  name -- which is in `config.yaml` like everything else -- is applied as a
  tag after the build.

## The secret key

The key that encrypts session cookie payloads and, with
`server.encrypt_at_rest` on (the default), every value written to the
database and every activity-log line, is **not in `config.yaml`**. It is in
`secret.key`, beside it:

| config.yaml | secret.key |
|---|---|
| `./config/config.yaml` (a checkout) | `./config/secret.key` |
| `~/.config/<slug>/config.yaml` (a portable binary) | `~/.config/<slug>/secret.key` |
| `/app/config/config.yaml` (the container) | `/app/config/secret.key`, mounted read-only |

One rule -- the folder `config.yaml` is in -- so there is nothing to
configure (`config.SecretPathFor`). The file is 64 hex characters and a
newline, mode 0600, in `.gitignore` and `.dockerignore`. It is written once,
with `O_EXCL` so nothing ever replaces it, and read back before anything
trusts it: by `scaffold.py`, `config init`, `config secret`, a binary's first
run, and the first `scripts/run.sh` or `docker/run.sh`.

It is separate so that `config.yaml` can be changed while the server runs --
from the owner's Settings screen, by hand, by `config set` -- and mounted
writable into the container, without any of those paths being able to read
or replace the key. The Settings screen only ever says where the file is.

When it is missing:

- **no database yet** (`cp config.example.yaml config.yaml` on a fresh clone):
  the server writes a new one and says so;
- **a database exists**: the server refuses to start and says to restore
  `secret.key` from the backup. Quietly generating a replacement would make
  every existing record permanently unreadable;
- **inside the container**: it refuses, since `docker/run.sh` mounts it.

The server also warns on every start if `secret.key` is open to other users
(group read is allowed -- it is how the container's user reads it).

### Moving the key out of an older config.yaml

Projects scaffolded before `secret.key` existed have the key as
`server.secret_key` in `config.yaml`. The first `scripts/run.sh` or
`docker/run.sh` after updating, or the server or `manage` starting, moves it
(`config migrate`, `config.MigrateLegacySecret`):

1. `secret.key` is written with the key and read back;
2. only then is the `secret_key:` line -- and the comment above it that
   described it -- replaced in `config.yaml` by a two-line note saying where
   the key went. Nothing else in the file changes.

Interrupted between the two, the next run finds both holding the same key and
just removes the line. Finding them holding *different* keys, it stops: which
one encrypted the data is not something to guess. Inside the container nothing
is written -- `config/` belongs to the host -- so a legacy `config.yaml` there
is an error that says to run `docker/run.sh`, which does the move on the host
first. `server.secret_key` stays in `ServerSection` only for this; it is not
in `defaults.yaml`, the schema, or the Settings screen, and a hand edit that
puts a different key back is rejected.

**Be clear about the trade this makes for a portable binary.** Its
`secret.key` lives in `~/.config/<slug>/` beside `app.db`, so at-rest
encryption protects a database that leaks *on its own* -- a stray copy of
`app.db`, a backup that captured only the database file -- but not someone who
takes the whole directory. Docker keeps them apart: `secret.key` stays in the
project's `config/` and is mounted separately from the data directory, so a
leaked backup of `./data` is not enough to read it. A binary gets the same
separation by setting `server.app_dir` to somewhere other than the directory
holding its `config.yaml`.

Losing the key loses the data. There is no recovery, and no in-place
re-encryption in the template -- rotating means exporting, changing the key,
and re-importing.

## Changing settings while the server runs

`config.yaml` can change under a running server three ways, and all three end
in the same place (`config.Holder.Apply` in `server/config/live.go`): the whole
file is parsed over the defaults and validated, and then either the running
configuration is swapped for the new one or nothing changes at all.

- **The Settings screen** (owner only, from a browser session; `GET` and
  `POST /api/owner/settings`). One field per key, grouped by section, with the
  key's comment from `defaults.yaml` as its help text. Saving sends only the
  changed keys plus the `version` (a hash of the file) the form was loaded
  from; if the file changed since, the save is refused with a 409 rather than
  overwriting a hand edit. Each save is audited in the activity log
  (`settings changed`, with every key's old and new value) and can be undone.
- **A hand edit.** The server checks the file every two seconds (mtime, size,
  then a hash) and applies a change once two checks in a row see the same
  content, so an editor caught half-way through a save is never applied.
- **`myapp config set section.key value`** -- the same in-place edit from a
  shell (or `docker exec <name> /app/server config set ...`), which the
  running server then picks up as a hand edit. Lists are `[a, b]` or `a,b`.

An edit that does not validate -- a typo, `server.port: 0`, broken YAML -- is
reported on stdout, in the activity log (once per distinct content), and as a
banner on the Settings screen, and **ignored**: the server keeps running on
the last version that validated. Fixing the file clears it.

### When a change takes effect

Every key in `defaults.yaml` carries a tag in its comments:

| tag | meaning | keys |
|---|---|---|
| `[live]` | the next request sees it | `server.public_url`, `secure_cookies`, `rate_limit_max`, `rate_limit_window_seconds`, `mcp`; all of `auth:`; `logs:` except `enabled` |
| `[restart]` | saved now, read at startup | `app.name`, `server.host`, `port`, `app_dir`, `static_dir`, `language_file`, `encrypt_at_rest`, `cookie_name`, `trust_proxy`, `trusted_proxies`, `proxy_header`, `body_limit_bytes`, `control_socket`, `logs.enabled` |
| `[scripts]` | read by `docker/run.sh` or `scripts/build.sh` only | `docker:`, `build:` |

A tag at the end of a key's line wins; then one in the comment above the key
(or above the run of keys it belongs to); then one above its section. The
Settings screen shows the tag as a badge, and for a `[restart]` key whose
saved value differs from the running one, says what the server is still
running with. `[restart]` keys are restart-only because something is built
from them once: the listener, the database, the control socket, the cookie
name every browser holds, fiber's proxy and body-limit settings.

"Live" is literal: everything that reads a setting per request reads
`Holder.Get()`, never a `Config` it was handed at startup -- the guard, the
handlers, the rate limiter (`MaxFunc`/`ExpirationFunc`), the security headers
(HSTS follows `secure_cookies`), `/mcp` (always routed, 404 while switched
off), the control socket's operations, and the activity log's own settings.
`liveConfig` copies the startup `Config` and overwrites only the live fields,
so a `[restart]` key can never leak in half-applied.

### How the file is edited

Re-encoding YAML would lose every comment, and the comments are the
documentation. So `config.EditYAML` (`server/config/yamledit.go`) parses the
file only to find where each value is and replaces that text, leaving the rest
byte for byte:

- a scalar on its key's line is replaced in place, keeping its quoting style
  where that is still safe and any comment after it;
- an empty value (`public_url:`) gets the value written after the colon;
- a flow list (`[a, b]`) is rewritten as a flow list, a block list (`- a`
  lines) as block lines at the same indentation, and an emptied block list
  becomes `[]`;
- a key the file lacks is added at the end of its section with the comment
  `defaults.yaml` has above it; a missing section is added at the end;
- CRLF line endings and a UTF-8 BOM are kept.

It refuses -- "edit it by hand" -- anchors and aliases, explicit tags, `|` and
`>` block scalars, a value spread over several lines, and a section written as
a `{flow: mapping}`. None appear in a file that started from `defaults.yaml`.
After editing, the result is parsed back and every key compared: the changed
ones must read back as exactly what was asked, every other one as exactly what
it was. Anything else is a bug in the editor, and nothing is written.

The file is replaced by writing a temporary file beside it and renaming it
over the old one (keeping its mode and group), so a crash leaves one version
or the other. The container cannot rename over its bind-mounted file, so there
it is rewritten in place, after the previous version is copied to
`<app dir>/config.yaml.bak`.

## The app directory

```
~/.config/<app-slug>/
  app.db           the bolt database
  control.sock     unix socket, mode 0600, only while the server is running
  storage/         application files (uploads, exports, generated artifacts)
  logs/            the activity log, one file per UTC day (see below)
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

## The activity log's settings

The `logs:` section decides what is logged and how long it is kept. The log
itself is described in `architecture.md` ("The activity log").

| key | default | meaning |
|---|---|---|
| `enabled` | `true` | false logs nothing, removes the Logs screen and its routes, and stops `track.js` |
| `retention_days` | `30` | days older than this are deleted (1-3650) |
| `max_total_mb` | `500` | while the logs directory is bigger, the oldest day is deleted -- never today |
| `max_file_mb` | `50` | a day past this continues in a new part file; at most `max_total_mb` |
| `compress_after_days` | `1` | finished days at least this old are gzipped; `0` never compresses |
| `static_assets` | `false` | also log requests for `/js`, `/css`, `/vendor` and other files with an extension |
| `ui_clicks` | `true` | log every click; screen changes, submits, undo and errors are logged either way |
| `echo` | `true` | print `[req] GET /path 200 1.2ms ip name` for each request to stdout (and so `docker logs`) |

Retention runs from the hourly purge loop and from "Prune now" on the Logs
screen. Every key but `enabled` is `[live]`: a change from the Settings screen
or a hand edit applies to the next request and the next prune.
`logs.enabled` needs a restart, since the writer and the log's routes are set
up once at startup.

## In containers

The Dockerfile builds with `-tags container`, which compiles in
`server/config/layout_container.go`. That pins three things regardless of
what `config.yaml` says:

| Pinned | Why |
|---|---|
| config file `/app/config/config.yaml` | where `docker/run.sh` mounts `./config/config.yaml`, writable (the Settings screen saves it in place); its folder makes `/app` the project directory, so `./static` and `./data` resolve as in a checkout, and puts `secret.key` (mounted read-only) beside it |
| app dir `/app/data` | where `docker/run.sh` mounts `docker.data_dir`, the only writable mount |
| listen host `0.0.0.0` | inside the container's own network namespace; exposure is `docker.bind_address` |

These are not settings because they are not choices -- they are the paths the
image is mounted into. Pinning them in the binary rather than in `ENV` is what
lets `docker exec <name> /app/server manage ...` find the same config and
database as the server with nothing extra to type, and stops a host-oriented
`server.app_dir` in the shared file from pointing the container at a path
that is not mounted. Everything else -- `server.port`, cookies, proxy trust,
TTLs -- comes from the mounted `config.yaml` as usual.

Inside the container the Settings screen shows `docker:` and `build:` (and the
pinned `server.host` and `server.app_dir`) read-only. The `docker:` section
decides what `docker/run.sh` mounts and hands to the container's user on the
host, so a compromised container must not be able to choose it; edit those on
the host. `docker/run.sh` also refuses a `data_dir` that is `/`, a home
directory, or a non-empty directory that is not this app's.

To reword the UI in a container without rebuilding, put a `language.yaml` in
the data directory (`/app/data/language.yaml`).

## Backups

Back up **`secret.key`, `config.yaml` and the app directory together**:

- Portable binary: the whole `~/.config/<slug>/` directory (`config.yaml` and
  `secret.key` are inside it unless `server.app_dir` moved the state elsewhere).
- Docker: `./config/secret.key`, `./config/config.yaml` **and**
  `docker.data_dir` (`./data` by default) -- deliberately separate.

`app.db` without the key is unreadable, and the key without the database is
nothing; `config.yaml` is only settings, but they are this deployment's. bolt is a single file, so a copy taken
while the server is stopped is complete and consistent. Copying from under a
running server can catch a write in progress -- stop it first.

## Adding a setting

1. Add the key to `config/defaults.yaml`, with its default and a
   one-line comment on what it does and, where it matters, what goes wrong if
   it is set incorrectly. This file is the documentation -- and the Settings
   screen's help text. Tag it `[live]`, `[restart]` or `[scripts]`
   (`TestEveryKeyIsTagged` fails until you do).
2. Add the field to the matching section struct (`ServerSection`,
   `AuthSection`, ...) in `server/config/config.go`. Strict decoding rejects a
   key the struct does not have, so a key in `defaults.yaml` without a field
   fails at startup rather than drifting.
3. If the value needs checking, add it to `File.Validate`.
4. If the server reads it, copy it onto `Config` in `build`, and read it per
   request through `Holder.Get()`. If it is `[live]`, also copy it in
   `liveConfig` (`server/config/live.go`) -- `TestLiveTagsMatchLiveConfig`
   fails if the tag and `liveConfig` disagree. If a script reads it, add it to
   `config shell` in `server/config/command.go` (the `server` section for
   anything about running locally, `docker` or `build` otherwise).
5. Regenerate the committed example: `go run . config example`. It is
   `config/defaults.yaml` plus a header, and is never edited by hand.

Do not read `os.Getenv` anywhere, and do not make a script read an
environment variable instead of adding the key. Keeping every knob in one file
is what makes "what can this be told to do" answerable by reading one file.
