# Configuration and state

One rule underpins this: **everything the app persists lives in a single
directory**, and everything tunable resolves the same way. Read this before
adding a setting or writing a file anywhere.

## Contents

- [The app directory](#the-app-directory)
- [Why one directory, and why ~/.config](#why-one-directory-and-why-config)
- [Settings precedence](#settings-precedence)
- [The secret key](#the-secret-key)
- [Storage for application files](#storage-for-application-files)
- [In containers](#in-containers)
- [Backups](#backups)
- [Adding a setting](#adding-a-setting)

## The app directory

```
~/.config/<app-slug>/
  app.db           the bolt database
  secret.key       generated on first run, mode 0600
  control.sock     unix socket, mode 0600, only while the server is running
  storage/         application files (uploads, exports, generated artifacts)
  config.yaml      optional; see config.example.yaml in the project root
  language.yaml    optional; overrides the embedded UI text
```

Resolved in this order (`server/config.DefaultAppDir`):

1. `APP_DIR` -- explicit, wins over everything
2. `$XDG_CONFIG_HOME/<slug>` -- if that variable is set
3. `~/.config/<slug>` -- the normal case
4. `./<slug>-data` -- last resort, when there is no home directory at all
   (some minimal containers), so the server starts rather than failing

The running server prints where it landed, which is the fastest answer to
"where is my data":

```
[state]  /home/you/.config/myapp
[secret] /home/you/.config/myapp/secret.key
```

Or ask a binary directly, without starting it -- this works even while the
server is running, because it opens no database:

```bash
./myapp manage paths
```

That also reports whether a server is currently running, which decides how
every other `manage` command reaches the data:

```
control        /home/you/.config/myapp/control.sock  (server running)
```

## Reaching the database from another process

bolt allows exactly one process to open its file, so a second process cannot
read or write the database directly while the server holds it. The running
server listens on `control.sock` in the app directory, and `manage` uses it
automatically:

```bash
./myapp manage reissue-login -user-id 1   # works with the server running
```

With the server stopped, the same command opens the database itself. Either
way it prints which route it took on stderr. `CONTROL_SOCKET=false` disables
the socket, which returns the pre-socket behaviour: `manage` then requires the
server to be stopped.

Anything else that needs the data -- a cron job, an importer, a sidecar --
should go through `server/control` rather than opening the bolt file, so that
one process keeps holding the secret key. See `architecture.md`.

A unix socket path is capped by the kernel (104 bytes on macOS, 108 on Linux).
A deep `APP_DIR` will exceed it; the server checks this at startup and says so
plainly rather than passing the kernel's bare "invalid argument" along.

## Why one directory, and why ~/.config

**Why not `./data` next to the binary?** Because the single-file build is
meant to be moved around. A binary run from `~/Downloads` once and `/opt`
later would leave two half-populated databases and look like it had lost the
data. Anchoring state to the user rather than the working directory means the
app finds the same state wherever it is launched from.

**Why one directory instead of XDG's split?** Strict XDG would put config in
`~/.config`, the database in `~/.local/share`, and caches elsewhere. That
split earns its keep for large applications; here the entire state is a bolt
file, a key, and some uploads. One directory to back up -- and one to delete
when uninstalling -- is worth more than the taxonomy. `XDG_CONFIG_HOME` is
still honoured, because respecting it costs nothing for anyone who has set it.

## Settings precedence

Environment variable, then `config.yaml`, then the built-in default. Always,
for every setting.

Environment wins because that is what a container, a systemd unit, or a
one-off debugging run sets, and those should not require editing a file inside
a volume. The file beats defaults because that is what a person edits.

```yaml
# ~/.config/myapp/config.yaml
port: 9000
secure_cookies: true
rate_limit_max: 240
```

```bash
PORT=9999 ./myapp        # 9999 wins over the file's 9000
```

`config.yaml` is optional and read only from the app directory -- not from the
project root. `config.example.yaml` ships in the project root as documentation
and lists every key with its environment equivalent; copy it into the app
directory to use it.

## The secret key

`SECRET_KEY` encrypts session cookie payloads and, with `ENCRYPT_AT_REST` on
(the default), every value written to the database. It resolves as:

1. `SECRET_KEY` environment variable -- nothing is written to disk
2. `secret_key` in `config.yaml`
3. `secret.key` in the app directory -- read if present, **generated at mode
   0600 if not**

That third step is what lets a portable binary just run: there is nobody to
set an environment variable for it, so it makes a key once and keeps working
across restarts.

**Be clear about the trade this makes.** With the key sitting beside the
database, at-rest encryption protects a database that leaks *on its own* -- a
stray copy of `app.db`, a backup that captured only the database file. It does
not protect against someone who takes the whole directory, because they have
both halves.

If that distinction matters for a deployment, pass `SECRET_KEY` through the
environment and no key file is ever written. That is exactly what
`dockerRun.sh` does: it keeps `.secret_key` on the host, outside the mounted
volume, so a leaked `./data` backup is not enough to read it.

Losing the key loses the data. There is no recovery, and no in-place
re-encryption in the template -- rotating means exporting, changing the key,
and re-importing.

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

`APP_DIR=/app/data` is set explicitly in the Dockerfile and by `dockerRun.sh`.
A container has no meaningful home directory, and the point of the mount is
that state lives on the host, so the `~/.config` default is simply wrong
there.

`dockerRun.sh` bind-mounts `./data` to `/app/data` and passes `SECRET_KEY`
from `.secret_key` on the host -- so the key stays out of the volume, as
described above.

To reword the UI in a container without rebuilding, mount a language file into
the app directory:

```bash
-v ./language.yaml:/app/data/language.yaml:ro
```

## Backups

Back up the **app directory and the key together**:

- Portable binary: the whole `~/.config/<slug>/` directory (the key is inside).
- Docker: `./data/` **and** `./.secret_key` (the key is deliberately outside).

Either half alone is useless: `app.db` without the key is unreadable, and the
key without the database is nothing. bolt is a single file, so a copy taken
while the server is stopped is complete and consistent. Copying from under a
running server can catch a write in progress -- stop it first.

## Adding a setting

1. Add the field to `Config` in `server/config/config.go`.
2. Resolve it in `Load` with `r.str` / `r.integer` / `r.boolean` / `r.seconds`,
   passing the environment name, the `config.yaml` key, and the default. Those
   helpers implement the precedence, so a new setting gets it for free.
3. Add the key to `config.example.yaml` with a one-line comment on what it
   does and, where it matters, what goes wrong if it is set incorrectly.

Do not read `os.Getenv` anywhere else. Keeping every knob in one file is what
makes "what can this be told to do" answerable by reading one file, and stops
the Dockerfile's `-e` flags from drifting away from what the code looks at.
