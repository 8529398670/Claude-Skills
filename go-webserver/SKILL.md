---
name: go-webserver
description: Scaffold a new security-hardened Go + Fiber v3 web server with bolt (bbolt) storage, link-based login (no open signup -- admins hand out one-time login links, or one multi-use invite link a whole group chat can join through), optional password login the owner switches on, optional two-factor sign-in, encrypted file uploads, always-fresh gzip'd static assets, all UI text in a language.yaml file, a hardened Alpine Dockerfile + docker/run.sh with no docker-compose, a scripts/build.sh that cross-compiles single-file portable binaries with the whole frontend embedded, scripts/run.sh and scripts/login.sh that report the real URL, paths and state, one config/config.yaml that configures the server, Docker, and the build (nothing reads environment variables), and one state directory convention (~/.config/APP/) holding the database and app storage. It copies every feature but switches on only the ones the app needs, working that out from what the app does, so the rest stay one setting away. Use this whenever the user wants to start a website, web app, internal tool, dashboard, or API from scratch in Go -- trigger on "build me a Go webserver", "new Fiber app", "golang backend with logins", "scaffold a Go web project with Docker", "I need a small internal tool in Go", or any Go web project that needs auth and static file serving rather than a bare HTTP handler. Also trigger when someone wants a Go web app shipped as one self-contained executable with no Docker and no runtime dependencies -- "single binary web app", "embed the frontend with go:embed", "cross-compile for linux and windows", "one file I can just hand someone", "where should the app keep its database and config". Also use it when asked to add a login system, an admin panel, invite links, API keys or programmatic/machine access, bolt storage, or Docker packaging to a Go web project that has none -- trigger on "a link I can drop in our group chat so everyone can join", "let a script call my Go API", "API tokens with admin and read-only levels", "how do I authenticate a cron job against my Fiber app", "service account for CI", "add an MCP server/endpoint", "let Claude (Code) use my app", "add password login", or "add 2FA / two-factor / TOTP". This is the Go counterpart to the `python-webserver` skill -- pick this one whenever Go, golang, or Fiber is mentioned, and that one for Python/Starlette.
---

# Go webserver scaffolder

This scaffolds one specific, opinionated kind of project: a Fiber v3 app
whose only real jobs are proving who is logged in, storing data in a bolt
file, and serving the HTML/JS/CSS that is the actual application.

The opinions below are already decided. Do not re-litigate them with the
user unless they raise one -- ask only about what is specific to *their*
project.

- **Links first, no signup form.** On first boot the server mints a
  random one-time login link and prints it to its own logs. Opening it
  shows a sign-in page; confirming there logs you in as admin and sets a
  secure cookie (opening alone spends nothing, so link previews cannot burn
  it). Admins mint more links for more people, and anyone signed in can
  mint a fresh one for their own account to sign in on another device.
  That, plus the invite links below, is how every account starts.
- **Passwords are an option, off by default.** With `auth.passwords` on,
  someone already signed in can set a username and password. The owner
  switches it on in Settings and can block one account. The browser hashes
  the password (PBKDF2) and the server bcrypts that, so the server never sees
  it -- but HTTPS is still required, because the hash works as a password on
  the wire.
- **Two-factor sign-in, for whoever wants it.** An authenticator-app code plus
  recovery codes, asked for on every browser sign-in: password, login link and
  invite link alike. API keys are exempt. `auth.two_factor` is the owner's
  switch, on by default.
- **File uploads, encrypted.** Everyone gets a Files screen with progress, resume
  and undoable delete; only the uploader and admins can open a file. Files arrive in
  pieces under the normal body limit, each sealed as it arrives with the file's own
  key, so rotating the secret key touches no file data. `files.enabled` switches it.
- **Invite links for groups.** An admin can also make one multi-use link
  (`/join/<id>.<secret>`) to post in a group chat: each person types their
  name and joins as a `user`, up to a set number of places, until it expires
  or is switched off. Typing a name close to one that already joined through
  the same link offers "is this you?", which is how that person signs in on
  their next device; a person can set a PIN so nobody else holding the link
  can pick their name. Links die with their maker's admin rights, and
  creating one needs a browser session -- an API key never can. `auth.invite_links`
  switches them and PINs.
- **Every account can mint API keys, at its own permission level.** A key
  carries the same admin/user role a person does -- one permission model, not
  two -- sent as `Authorization: Bearer <key>`. A key can never outrank the
  account that owns it, and it cannot create credentials -- no minting keys,
  creating accounts, or reissuing login links -- so revoking a leaked key
  ends it. That is what lets a cron job, a CI pipeline, or a sidecar call the
  app.
- **The same keys speak MCP at `/mcp`.** An agent such as Claude Code uses the
  app through a key, at that key's role: admin tools are invisible to a
  user-scoped key, a browser session is refused, and no tool creates a
  credential or deletes anything. Stateless JSON-RPC over POST, written by
  hand -- no SDK, no SSE, no sessions. `server.mcp: false` removes it.
- **Webhooks tell other systems what happened.** The owner adds endpoints on a
  Webhooks screen. Events go through a durable outbox, signed with HMAC-SHA256 and
  retried with backoff, and an endpoint can only be a public `https` address, checked
  again on the address actually dialled. `webhooks.enabled` switches it.
- **Failures look right.** A browser gets a styled error page, and scripts keep their
  JSON. Every list in the API is paged, and nothing here is meant for search engines:
  `robots.txt` and an `X-Robots-Tag` header on every response say so.
- **The first admin is the owner.** Any admin can permanently delete an
  ordinary user; only the owner can delete another admin, and nobody can
  delete, disable, or revoke the owner from the web. Ownership is a marker,
  not a third role, and moves only from a shell (`manage transfer-owner`).
  Deleting an account takes its sessions, links, and keys with it and needs a
  browser session, so a leaked admin key cannot erase anyone.
- **The server stays thin.** Business logic and rendering belong in
  `static/js/`. The Go side is auth, storage, and file serving. Resist
  adding server-side templating.
- **Everything is modular.** One concern per file under `server/`. When
  something grows, add a package -- never let a file become the monolith
  the layout exists to prevent.
- **No build step, ever.** Plain browser JS and CSS. No npm, no bundler,
  no transpiler, no JSX. Third-party libraries are downloaded once into
  `static/vendor/` and served from there, never pulled from a CDN at
  runtime.
- **All UI text lives in `language.yaml`.** No user-facing string is
  hardcoded in HTML or JS.
- **Bootstrap 5 + Bootstrap Icons, light mode by default.** Vendored in
  `static/vendor/`; a sidebar of screens, one shown at a time. Use its
  components and utility classes before writing CSS.
- **Docker, never compose.** One hardened Alpine image, one `docker/run.sh`.
- **`scripts/run.sh` just makes it run** outside Docker, from the checkout:
  it stops the copy it started last and starts a fresh one in the foreground
  (rebuilding first when anything changed), and says what is really
  happening -- the URL, config file, app dir, whether the database is new,
  which process holds the port if it is taken -- then `Ready: <url>` once it
  answers. With arguments it is the admin CLI instead and leaves the running
  server alone (`scripts/run.sh manage list-users`). **`scripts/login.sh`**
  prints a clickable login link for the owner (user 1) from whichever
  deployment is live -- the local server or the docker container -- and says
  which. With neither running it starts the local server in the background
  (log in `.run/server.log`, `scripts/run.sh stop` stops it) and waits until
  it answers, so the link works the moment it is printed; `--no-start` writes
  it into the database offline instead. `run.bat` / `login.bat` are the same
  on Windows. Run inside the skill's own `assets/template/`, they stamp a demo project
  into `~/.cache/go-webserver-demo/` (via `scripts/demo.py`) and runs
  that, so the template can be tried without scaffolding anything.
- **Ships as a single file too.** The frontend is compiled in with `go:embed`,
  so `scripts/build.sh` cross-compiles portable binaries that need no Docker, no Go
  toolchain, and no files alongside them. The same binary is also the admin
  CLI (`myapp manage ...`).
- **One `config/config.yaml` configures everything** -- the server, the Docker
  container (image name, ports, mounts, resource limits), and the portable
  build (binary name, targets). Nothing reads environment variables: not the
  Go code, not `docker/run.sh`, not `scripts/build.sh`, and the container gets no `-e`
  flags. The scripts ask the app binary for their values
  (`config shell docker|build`), so there is one YAML parser and one set of
  validation rules, and an unknown key is an error rather than a silently
  ignored typo. It holds no secret: the key that encrypts the database is in
  `config/secret.key` beside it (0600, gitignored, never baked into an image,
  mounted read-only into the container).
- **Settings change while the server runs.** The owner gets a **Settings**
  screen: config.yaml as a form, one field per key, with the help text taken
  from the comments in `defaults.yaml` and a badge saying when a change takes
  effect -- **Live** (at once), **Restart**, or **Scripts** (read by
  `docker/run.sh` / `scripts/build.sh`). Saving rewrites only the changed
  values in place, so every comment in the file survives, and is refused if
  the file changed since it was loaded. The server also polls config.yaml
  every two seconds and applies a hand edit (or `myapp config set key value`);
  an edit that does not validate is reported and ignored, and the server keeps
  running on the last good version. Each key's apply mode is a `[live]` /
  `[restart]` / `[scripts]` tag in `defaults.yaml`'s comments.
- **Everything is logged, and only the owner reads it.** Every request
  (who, from where, what, status, time), every sign-in and admin action, and
  what people do on the page (screens, clicks, form submits by field name,
  undo/redo, JS errors, failed calls, device details at sign-in) go to an
  activity log under `<app dir>/logs/`, one file per UTC day, encrypted with
  the database's key. The owner gets a **Logs** screen: one collapsible block
  per day, a search box with `kind:` `user:` `status:5xx` filters, live
  tail, download, delete. Other admins and every API key get 404. Retention
  is `logs:` in config.yaml (days kept, total size with oldest-day
  recycling, part size, gzip after N days), applied hourly. Login links, key
  secrets, PINs, passwords, codes and anything typed are never written.
- **Background jobs** run from one registry: named, never overlapping, due
  times that survive a restart, and an owner **Jobs** screen showing how each
  last went, with a Run now button. Purge, log retention and scheduled backups
  are jobs; an app adds its own in one line.
- **People can see where they are signed in** on the Account screen (device,
  address, last active) and sign out one device or all the others remotely.
- **Monitoring.** `/api/health` stays trivial for the container; `/api/ready`
  checks the database for a load balancer. Every response carries an
  `X-Request-ID`, and a 5xx shows it to the person, so it finds the log line.
  `/metrics`, off by default, is Prometheus text for an admin API key.
- **Maintenance mode.** `server.maintenance` is `none`, `read_only` or `full`,
  applied live from Settings. Everyone but the owner's browser session gets a
  503 with `Retry-After`: a page in a browser, JSON for a script.
- **An OpenAPI spec.** `config/openapi.json` describes every route, and
  `GET /api/openapi.json` hands it to anyone signed in. A test fails until a
  new route is in it.
- **Live updates.** `server/stream` is a server-sent events helper that handles
  the write deadline, re-checks access on open streams and ends them on
  shutdown. The Logs screen's Live button runs on it, with no polling.
- **One directory holds all state.** The database and app storage live in
  `~/.config/<slug>/` (move it with `server.app_dir`; the container always
  uses `/app/data`). Never write state relative to the working directory -- a
  binary that gets moved would lose track of it.

## When this template does not fit

Say so rather than scaffolding the wrong thing:

- A CLI tool, a data pipeline, a worker, or a library with no HTTP surface.
- A project that already has its own framework, router, auth, or
  deployment style the user wants extended -- extend that instead of
  introducing a second incompatible pattern.
- A need for real self-service signup or OAuth/SSO. This template's whole
  point is avoiding those; passwords here are something a person adds after
  signing in with a link, not a way to register. If the user wants one, say so
  and adapt deliberately rather than quietly shipping magic links anyway.
  The same goes for machine access: the built-in API keys are bearer
  credentials tied to an account, not OAuth client credentials, JWTs, or
  per-endpoint scopes. If someone needs third-party apps to authorise against
  their app, that is a different system -- say so rather than stretching
  these.
- Something that needs relational queries or joins. bolt is a key/value
  store with no query language; a reporting-heavy app wants SQLite or
  Postgres instead.
- Something that needs several processes writing the database directly.
  bolt hands its file to exactly one process. The template's answer is the
  control socket (`server/control/`): the server owns the database and
  everything else asks it over a unix socket. That covers a CLI, a cron job,
  or a sidecar. It does not cover multiple *servers* sharing one database --
  that genuinely needs SQLite or Postgres.

## Step 1: Confirm the basics

You need three things before scaffolding:

1. **Project name** -- becomes the Docker image/container name, the page
   title, and the default `language.yaml` title.
2. **Target directory** -- the scaffold refuses to write into a non-empty
   directory rather than risk clobbering existing work.
3. **What the app actually does** -- enough to plan where features go
   (new route files? new buckets? new pages?), even if you build them in
   a later pass.

Optionally a **Go module path**. If the user does not offer one, derive it
from the project name; only a project meant to be imported elsewhere needs
a real `github.com/...` path.

## Step 1b: Choose the features

Every feature's code goes into every project. What you choose is which ones
start **switched on**: an unused one has no screen, its routes answer 404, and
it sits there until someone turns it on in Settings. So a wrong call is cheap,
and the point is a project that shows only what it needs.

Work it out from what the app does. Do not interview the user about this.
Pass exactly the features with a reason to be on:

| Feature | Switch it on when the app... |
|---|---|
| `files` | lets people upload, attach, or share documents, images, or data |
| `api_keys` | is called by a script, cron job, CI, or another service |
| `mcp` | should be usable by Claude or another agent (brings `api_keys`) |
| `webhooks` | has to tell another system when something happens |
| `invite_links` | is joined by a group: "a link for our chat", a class, a team, a club (includes PINs) |
| `passwords` | has people who want to come back without asking for a new link |
| `two_factor` | has real accounts: anyone besides the owner, or anything reachable from the internet |
| `metrics` | is watched by Prometheus, Grafana, or similar (brings `api_keys`) |
| `backups` | holds data worth keeping in a long-lived deployment |

A feature the description gives no reason for stays off. `two_factor` is the
one exception: it stays on unless the app is plainly a one-person tool on the
owner's own machine (that is `--features none`), because it costs people nothing
until they choose it. If it is unclear who will use the app, it stays on.

Sign-in links, sessions, users, the activity log, background jobs, Settings, and
maintenance mode are the core: always on, and not on this list.

Say what you chose in one line, with the reason for anything unusual, and go
straight on to scaffolding. For example: "Features: files and two_factor, since
it is a shared document store with real accounts; everything else stays off, one
switch away in Settings." The user can correct you at any point, and doing so
needs no rescaffold. Only if they asked for "everything" do you leave `--features`
out, which keeps the template's own defaults.

## Step 2: Scaffold

Run the bundled script rather than retyping files -- it does the copy and
placeholder substitution consistently and will not overwrite a non-empty
directory:

```bash
python3 <skill_dir>/scripts/scaffold.py <target-dir> "<project-name>" [module-path] --features files,two_factor
```

This copies `assets/template/`, replacing `{{PROJECT_NAME}}` and
`{{MODULE_PATH}}`, and writes the project's `config/config.yaml` (the
documented defaults, mode 0600), `config/secret.key` (a freshly generated key,
mode 0600), and `config/config.example.yaml` (the same defaults, for
committing). It installs
nothing, starts nothing, and creates no users -- so nothing happens on the
network or on disk that the user did not ask for.

`scaffold.py` also writes `.skill-origin.yaml`: the skill commit, name, port and
features this project started from. Commit it. The `webserver-upgrade` skill uses
it to bring later template changes into the project.

When you change `assets/template/`, add an entry to `UPGRADING.md` in this skill.

`--features` is the exact list from Step 1b, comma-separated, or `none`. It sets
those switches in `config/defaults.yaml`, so `config.yaml`, the example, and a fresh
clone all agree. A feature that needs another brings it along and says so, an unknown
name stops before anything is written, and the summary prints what is on and off.

Each project also gets **its own port and its own session cookie name**:
`scaffold.py` picks a random free port in 20000-39999 for `server.port` and
`docker.host_port` (it prints it -- use that wherever this file says
`<port>`), and `server.cookie_name` is `<slug>_session`. Both are needed:
browsers share cookies across every port on `localhost`, so two projects
that both used `session` would keep signing each other out no matter which
ports they ran on. Do not reset either to a shared value.

### What you get

```
<target-dir>/
  main.go                     # wiring only + the manage/config/version subcommands,
                              #   and the go:embed of static/, config/*.yaml and
                              #   config/openapi.json
  go.mod / go.sum             # pinned deps, so the first build is reproducible
  config/
    config.yaml               # THE config: server, docker, build -- no secret
                              #   (written by scaffold.py; gitignored, 0600;
                              #   editable live from the Settings screen)
    secret.key                # the key that encrypts the database (written by
                              #   scaffold.py; gitignored, 0600) -- back it up
    openapi.json              # the API, by hand; a test keeps it in step
    config.example.yaml       # every default -- commit this; a fresh clone
                              #   copies it to config/config.yaml
                              #   (`go run . config example` regenerates it)
    defaults.yaml             # every key, documented and tagged [live] /
                              #   [restart] / [scripts], embedded -- the defaults
    example_header.yaml       # the note config.example.yaml starts with
    language.yaml             # every user-facing string in the app
  server/
    config/                   # config.go: find, load, validate, resolve paths/URL
                              # secret.go: secret.key, and moving an old key out
                              # live.go: the running config, Settings saves, and
                              #   the 2-second watcher -- one validate-then-apply
                              # yamledit.go: rewrite one value, keep the comments
                              # schema.go: keys, types, help, [live] tags
                              # command.go: `config init|example|secret|migrate|
                              #   check|set|shell`
                              # layout_container.go: the image's fixed paths
    db/db.go                  # bolt open + buckets + encrypt-on-write helpers
    db/migrate.go             # the schema version and the list of migrations
    backup/backup.go          # `manage backup` / `restore`, and the schedule
    rotate/rotate.go          # `manage rotate-key`: re-encrypt onto a new key
    logs/                     # the activity log: entry.go (the line + redaction),
                              #   writer.go (async, rotating), store.go (read,
                              #   search, prune), query.go (search grammar)
    control/                  # unix socket so other processes reach the db
    disk/                     # room left on a disk, per operating system
    encryption/encryption.go  # the ONLY place that touches crypto primitives
    models/                   # user.go, session.go, login_token.go, api_key.go,
                              #   invite_link.go, pin.go, password.go,
                              #   two_factor.go, sign_in.go, attempts.go (+ tests)
    security/security.go      # sessions + API keys resolved to one Actor,
                              #   RequireLogin/RequireAdmin/RequireSession/
                              #   RequireAPIKey/RequireOwner, CSRF
    mcp/                      # mcp.go: the /mcp JSON-RPC protocol
                              # registry.go: Tool, argument binding, Refuse
    static/static.go          # gzip-always-fresh text, long-cached images
    language/language.go      # loads language.yaml, serves it as JSON
    middleware/middleware.go  # CSP + security headers, rate limit (static files
                              #   exempt, sign-in and upload pieces have their
                              #   own budgets), recover
    middleware/request_log.go # one activity-log entry per request
    middleware/request_id.go  # the X-Request-ID reference on every response
    middleware/maintenance.go # what each maintenance mode leaves open
    middleware/errors.go      # the ErrorHandler: generic text out, cause logged
    middleware/metrics.go     # counts every request for /metrics
    metrics/metrics.go        # the numbers, printed as Prometheus text
    jobs/                     # jobs.go: named background jobs on a schedule
                              # housekeeping.go: purge, prune-logs, backup
                              # files.go: clears out deleted and abandoned files
    stream/stream.go          # server-sent events: Hub, Serve, heartbeats
    webhooks/webhooks.go      # the outbox's sender, signing, and the address guard
    errorpage/errorpage.go    # the page a browser gets when a page load fails
    routes/                   # routes.go (the URL map) + one file per group,
                              #   including api_keys.go, files.go, mcp_tools.go
                              #   (the MCP tools, + mcp_test.go) and logs.go
                              #   (the owner's log API, page events, audit())
    bootstrap/bootstrap.go    # first-run admin + printing links as full URLs
    assets/assets.go          # picks disk assets when present, embedded otherwise
    healthcheck/              # `health` subcommand, the container HEALTHCHECK
    manage/manage.go          # the admin CLI commands
  static/
    index.html                # placeholder account + admin UI, fully data-i18n'd;
                              #   Bootstrap navbar + sidebar, one screen per view, toasts
    css/style.css             # overrides on top of Bootstrap: colour tokens, shell
    js/theme.js               # light/dark/preset themes, applied before first paint
    js/nav.js                 # sidebar: shows the screen the URL hash names
    js/undo.js                # Ctrl+Z / Ctrl+Y history every screen records into
    js/track.js               # sends what people do on the page to the log
    js/logs.js                # the owner's Logs screen
    js/jobs.js                # the owner's Jobs screen
    js/sessions.js            # "where you're signed in" on the Account screen
    js/fresh.js               # asks the server again on coming back to a tab
    js/qr.js                  # QR codes for 2FA and for login and invite links
    js/dom.js, i18n.js, api.js, app.js, keys.js, files.js,
      join.js, pin.js, invites.js   # invite links: join page, PIN, admin panel
      password.js, two_factor.js    # password form and panel; 2FA step and panel
    vendor/                   # self-hosted libs: Bootstrap 5, Bootstrap Icons (see its README)
  scripts/
    run.sh, run.bat           # local run: (re)starts the server; with args, the CLI
    login.sh, login.bat       # the owner's login link, from whatever is live
                              # (starts the server in the background if nothing is)
    build.sh                  # cross-compiles single-file portable binaries
    lib.sh                    # shared by run.sh and login.sh (sourced)
  docker/
    Dockerfile                # built with the project root as context
    build.sh, run.sh          # build the image; build + run the container
    restore.sh, rotate-key.sh # put a backup back; move to a new secret key
    lib.sh                    # shared by both (sourced)
  misc/vendor.sh              # downloads a library into static/vendor/
  .gitignore, .gitattributes, .dockerignore   # .dockerignore: what the build context leaves out
```

## Step 3: Build the actual app on top

This is the part specific to what the user asked for, so use judgment --
but stay consistent with the patterns already there:

- **A feature that was left off and turns out to be needed** is already in the
  project. Switch it on (Settings, or `scripts/run.sh config set files.enabled
  true`). Never rebuild something the template has, and never scaffold again to
  get it. Switching one off again keeps its data.
- **New screens** are a `<section class="card view" data-view="name">` in
  `index.html` plus a sidebar link to `#name` (see `references/frontend.md`).
  Give each its own file in `static/js/` with its own `init()`, included
  from the HTML. Do not keep appending to `app.js`.
- **New user actions are undoable.** Anything that changes what the page
  shows or stores goes through `Undo.run( { label , do , undo } )` (or
  `Undo.record` once it has happened), so Ctrl+Z / Ctrl+Y cover it. Deletes
  and other changes not obvious on screen add `toast: true` (a toast with an
  Undo button); a form calls `Undo.saved( form )` after saving. Deletes are
  soft and restorable on the server, so they need no confirm dialog; revokes,
  logout and issuing a link or key stay out. See "Undo and redo" in
  `references/frontend.md`.
- **New text** goes in `config/language.yaml` and is referenced with
  `data-i18n="section.key"` (or `data-i18n-attr="placeholder:section.key"`).
  Never hardcode a user-facing string in HTML or JS. A missing or empty
  key hides its element -- that is the documented way to switch a label or
  a section off.
- **New endpoints** get a new file in `server/routes/`, registered in
  `routes.go`. Put them behind `handlers.Guard.RequireLogin` (or
  `RequireAdmin`) by adding them to the right group -- protection comes
  from where a route is registered, so it cannot be silently forgotten.
  Keep the catch-all static route last; it matches everything after it.
- **Default new features to "any signed-in user."** Admin status in this
  template means "can decide who gets in," not "can use more features."
  Only gate something on admin if the user specifically asks. If a feature
  needs to list other people, use the existing `GET /api/team` rather than
  the admin-only user list.
- **API keys need no per-feature work.** A key authenticates as its owner at
  its own role, so a route registered behind `RequireLogin` is reachable by
  any key and one behind `RequireAdmin` only by an admin-scoped key. Read
  permission through `security.ActorFrom( c )` (`actor.Role`,
  `actor.IsAdmin()`) rather than `user.Role`, which is the account's role and
  ignores a key that deliberately carries less. Add
  `Guard.RequireSession` to a route only when it must be browser-only, the
  way key management, account creation and login-link reissue are -- and to
  any new route that creates a credential.
- **New MCP tools** go in `server/routes/mcp_tools_<feature>.go`, registered
  from `registerMCPTools` in `mcp_tools.go`. Pull the HTTP handler's body into
  a helper both call (the way `meRow` and `setUserDisabled` are), so the two
  doors share one set of rules. Default to any signed-in key; set
  `AdminOnly` only when the matching route is admin-only. Decode arguments
  with `call.Bind` (strict), return an object, and say no with
  `mcp.Refuse( "..." )`. Never write a tool that creates a credential or
  deletes something -- `/mcp` takes only keys, and those routes need a
  session. Set `ReadOnly`/`Destructive` honestly; clients use them to decide
  when to ask the person first.
- **Group middleware is mounted on a path prefix,** not on the routes written
  under it: `app.Group( "/api" , mw )` puts `mw` in front of every later route
  starting with `/api`, including `/api/admin/...`. Give a group that should
  not do that its own prefix -- that is why the key routes live under
  `/api/keys` and `/api/admin/keys`.
- **Log what matters, never what is secret.** Requests and page activity are
  logged for free. A new action worth an audit line (something changed,
  someone was let in or refused) calls
  `handlers.audit( c , logs.KindAdmin , "thing deleted" , fiber.Map{ "thing_id": id } )`
  where it succeeds -- ids and names in the detail, never a credential, a
  body or a typed value. Part of the page that should not be tracked (it
  shows secrets, or reading it would fill the log) gets `data-no-track`.
- **New data** gets a bucket constant in `server/db/db.go` (added to
  `bucketNames` so it is created at startup) and a new file in
  `server/models/` with plain functions, matching `user.go`. Go through
  `store.Put`/`Get`/`ForEach` -- that is what keeps at-rest encryption
  automatic instead of something each model must remember.
- **A change to a stored record's shape** (a field renamed, split, or given a
  new meaning) gets a step at the end of `migrations` in
  `server/db/migrate.go`, once the app has data anyone cares about. A new
  optional field or a new bucket needs nothing. `architecture.md` has the rules
  and an example.
- **User uploads** already have a home: build on `server/models/file.go` rather than writing
  a route that takes a body. Add a field to the record for what the file belongs to,
  and check it in `server/routes/files.go`.
- **New files on disk** go under `cfg.StorageDir` (`<app dir>/storage/`),
  never a path relative to the working directory -- that changes depending on
  how the process was started, and would land outside what users back up.
- **New settings** get a key in `config/defaults.yaml` (with its
  default and a comment), a field on the matching section struct in
  `server/config/config.go`, and -- if the server reads it -- a line in
  `Load` copying it onto `Config`. Strict decoding means a key missing from
  the struct is rejected, so the two cannot drift silently. A setting a shell
  script needs goes in `config shell` (`command.go`). Never call `os.Getenv`,
  and never make a script read an environment variable instead. See
  `references/configuration.md`.
- **Never import a crypto package outside `server/encryption`.** Add a
  function there instead. One file to audit is the entire point.
- **Never build HTML from strings containing user data.** Use `Dom.el`
  with `text:`, which is why that helper exists.
- **A new event** goes in the list in `server/models/webhook.go`, and is sent with
  `handlers.Webhooks.Emit( name , data )` where the thing happens. Ids and names only
  in `data`, since it leaves the server, and never rename one once it has shipped.
- **A new route** goes in `config/openapi.json` too: `go test ./server/routes` fails
  until it is. A route that changes something must not be a GET, or `read_only`
  maintenance will not stop it.
- **A new list route** answers with `sendPage( c , "rows" , rows )`, and its screen
  uses `Pager.load`: a plain `forEach` would stop at the first page.
- **A new page with a path of its own** needs a route using `handlers.Static.ServeIndex`
  before the catch-all. There is no `index.html` fallback, so an unknown path is a 404.
- **Adding a library?** Run `misc/vendor.sh <url>` and reference
  `/vendor/<file>`. A `<script src>` pointing at a CDN is blocked by the
  Content-Security-Policy -- that is the policy working. Vendor the file
  rather than loosening the CSP.
- **Anything under `static/` is embedded automatically** by `main.go`, so new
  pages and vendored libraries end up in the portable binary with no extra
  step. A new asset directory *outside* `static/` would need its own
  `go:embed` line -- putting it under `static/` is simpler.
- **Anything on a schedule** is a job: `registry.Register( name , interval , fn )`
  in `main.go`, beside `jobs.Housekeeping`. Never a goroutine with its own
  ticker. The registry stops overlaps, catches panics, survives restarts and
  puts it on the owner's Jobs screen. See "Background jobs" in
  `references/architecture.md`.
- **Pushing to a page** uses `server/stream`: a `Hub`, `stream.Serve` in the
  route, `Publish` where it happens, and `Hub.Close` in `main.go`'s shutdown.
  Never a long-lived handler of your own: the write deadline, re-checking access
  and shutdown are all handled there. See "Live updates".
- **A 5xx** is `serverError( c , cause )`, with the real error: it reaches the
  log under the request's id and never the client.
- **New metrics** go in `server/metrics`. Never label one by path or by anything
  else a caller controls.
- **New admin CLI commands** go in `server/manage/manage.go`, which the
  `manage` subcommand dispatches to. Add the operation to the `backend` interface there and implement it twice --
  once on `controlBackend` (a call to `server/control`) and once on
  `directBackend` (a call into `server/models`). The compiler enforces that
  both exist, which is what stops the two routes from behaving differently.
- **New work for another process** -- a cron job, an importer, a sidecar --
  gets an endpoint in `server/control/control.go` and a method in
  `client.go`, never a second process opening the bolt file. One process
  holds the database and the secret key; that is the whole security argument
  for the socket, and it stops holding the moment something else opens the
  file directly.

## Step 4: Run it and verify the login flow

Do not scaffold and hand it over unverified. Actually exercise the flow:

```bash
cd <target-dir>
go test ./...      # invite links, soft delete, MCP, and the activity log
# in config/config.yaml: server.secure_cookies: false   (plain http on localhost only)
scripts/run.sh
scripts/login.sh   # from another terminal: a fresh owner login link
```

(`scripts/login.sh` on its own also works: with no server running it starts
one in the background and prints a link that is ready to click.)

`scaffold.py` already wrote `config/config.yaml` and `config/secret.key`, and
`scripts/run.sh` builds `./<slug>` (Go's build cache makes an unchanged tree a
no-op), stops the server it started last (`.run/server.pid`), and runs the new
one in the foreground, reading that file. Run it again after any change; a
build error leaves the old server running. Before starting it prints the
resolved URL, config file, app dir, database (new or existing), and whether
static/ and language come from disk; it refuses to start -- naming the
process -- when something else holds the port, warns when `secure_cookies` is
on for a plain `http://` URL, and prints `Ready: <url>` once the server
answers. `scripts/run.sh config shell server` prints those same resolved
values, `scripts/run.sh manage paths` reports them plus whether a server is
live, and `scripts/run.sh config check` validates the file. Any arguments make
`run.sh` a one-off: it builds, runs the binary with them, and leaves the
running server alone. On Windows, `scripts\run.bat` does the same.

On a fresh database the server prints a one-time login link. Confirm, by
actually doing it:

- opening the printed `/login/<credential>` shows a sign-in page, and
  pressing the button logs you in and sets a cookie,
- `/api/me` reports the admin account,
- the admin panel creates a user and produces a working link for them,
- using the first link a second time fails (single-use), and so does a link
  replaced by "New link",
- a POST without a `csrf_token` is rejected with 403 -- logout included.

Then exercise the API keys, because the interesting property is the ceiling
and it is the kind of thing that silently stops holding:

```bash
# in the UI: create one "user" key and one "admin" key, then
curl -H "Authorization: Bearer <admin key>" localhost:<port>/api/admin/users   # 200
curl -H "Authorization: Bearer <user key>"  localhost:<port>/api/admin/users   # 403
curl -H "Authorization: Bearer <user key>"  localhost:<port>/api/me            # role: user
```

The third line is the one worth reading: an admin who mints a user-scoped key
gets `"role":"user"` back, because the effective role is the lower of the two.
Also confirm a non-admin is refused when asking for an admin key, that a key
cannot mint a key (403, session required), that an admin key gets 403 from
`POST /api/admin/users` and `POST /api/admin/users/<id>/reissue-login` too,
and any key gets 403 from `POST /api/account/login-link`
(a login link would become a session that mints keys), and that revoking a
key kills it on the next request. `manage list-keys` works from a shell either way.

Then `/mcp` with the same two keys (`go test ./server/routes -run MCP` covers
all of this too, but see it once against the real server):

```bash
MCP='Content-Type: application/json'
curl -s localhost:<port>/mcp -H "$MCP" -H "Authorization: Bearer <user key>" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'                  # no admin_* tools
curl -s localhost:<port>/mcp -H "$MCP" -H "Authorization: Bearer <admin key>" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"admin_list_users","arguments":{}}}'
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:<port>/mcp -d '{}'   # 401
```

A signed-in browser posting to `/mcp` gets 403, `GET /mcp` gets 405, and
`admin_set_user_disabled` on your own account or the owner comes back with
`isError: true`. To use it from Claude Code:
`claude mcp add --transport http <name> http://localhost:<port>/mcp --header "Authorization: Bearer <key>"`.

Then the delete rules, because they are permission checks that only show up
when someone is refused. Create a second admin B and a user C, sign B in, and
confirm: B can delete C (`POST /api/admin/users/<id>/delete`), B gets 403
deleting another admin or the owner, the owner can delete an admin, nobody can
delete themselves (400), an admin key gets 403 from the delete route (session
required), and B gets 403 disabling or revoking the owner. A key's owner can
delete it (`POST /api/keys/<id>/delete`) and it gets 401 on the next request;
deleting an account kills its keys too. `manage list-users` marks the owner.

Then the invite links, because they are the one public way to create an
account. In the admin panel make a link with 2 places and open it in a
private window:

- the page asks for a name and nothing is created until you press "Join as
  ..."; you land signed in as a `user`,
- a second private window typing a close spelling ("alise" for "Alice") is
  offered Alice and can sign in as her -- set a PIN as Alice and that same
  step now asks for it, and 5 wrong PINs give 429 `pin_locked`,
- a third new name gets 409 `invite_link_full`, but a returning name still
  signs in,
- an admin is never offered, and a wrong secret, an expired link, or a
  switched-off one all answer 404 `invite_link_invalid`,
- an admin API key gets 403 from `POST /api/admin/invite-links` and
  `POST /api/account/pin` (both would let a key become a session), and
- `manage create-invite -user-id 1 -label "Test"` prints a working
  `/join/...` path, with the server up or down.

Then passwords and 2FA, because the point of them is what they refuse. Turn on
`auth.passwords` in **Settings**, and on a user's account set a password:

- the signed-out page now has a password form, and the network panel shows only
  `password_digest` in the request, never the password,
- a wrong password, an unknown name and a blocked account all answer 400
  `invalid_login`; five tries at one name give 429 `login_locked`,
- turn on **Two-factor** and sign in again by password, by a new login link,
  and by an invite link: each asks for the code before it signs anyone in,
- a used code or recovery code is refused the second time,
- an invite-link claim on an account with a password and no PIN answers 403
  `use_password`, and
- `auth.passwords: false` makes `/api/login/password` answer 404, and turning
  it back on restores the password.

Then the features an admin and the owner lean on:

- with `server.maintenance: full`, a browser that is not the owner gets the
  maintenance page and the owner's session still works,
- an admin can make a key for a user on All keys, and that user sees who made it,
- with two sessions signed in, ending one from the other and then clicking
  anything in the ended one lands on the signed-out page saying why, not on a
  banner reading "not authenticated",
- the navbar's button switches light and dark, signed in and signed out; a fresh
  browser opens light whatever the system is set to, and a choice survives a
  reload,
- double-clicking **Create login link** makes one account,
- **QR code** beside a login or invite link shows a code that scans to that link,
  and a new link hides the old code,
- dropping files on the Files screen uploads them, Cancel leaves nothing behind,
  and the bar under the upload field shows what is used of the quota,
- with `server.announcement` set, the banner shows within five minutes of coming
  back to an open tab; dismissing it sticks, and a changed message comes back,
- **Sessions** on a user's row lists their devices and can end one; another admin
  sees no addresses, and only the owner reaches the owner's,
- a failed webhook delivery has a **Resend** button that adds a new row and leaves
  the failed one listed,
- `/api/openapi.json` is 401 signed out and 200 signed in.

Then the logs, because the point of them is that nobody but the owner sees
them and nothing secret is in them:

- signed in as the owner, **Logs** is in the sidebar; today is open and
  shows your sign-in (`Sign-in`), the requests behind it, and `Page` entries
  for the screens you opened and buttons you clicked,
- `status:4xx`, `kind:auth,admin`, `user:<name>` and free words narrow the
  list; a day folds and unfolds; clicking a row shows its JSON,
- signed in as a second admin there is no Logs entry, and
  `curl -b <their cookie> /api/owner/logs/days` is 404 -- as is any API key,
  the owner's admin key included,
- with `server.encrypt_at_rest: false`, `grep` the login link's secret in
  `<app dir>/logs/*.jsonl`: it is not there (the path is `/login/…`),
- `manage logs` lists the days and `manage logs -day <today> -q kind:auth`
  prints entries, with the server up or down, and
- "Prune now" with `logs.retention_days: 1` removes an older day file
  (copy today's to an older date to have one) and never today.

Then the Settings screen, because it writes a file the server runs on:

- signed in as the owner, **Settings** is in the sidebar; another admin and
  any API key get 404 from `/api/owner/settings`, and the response never
  contains the key in `config/secret.key`,
- changing `server.rate_limit_max` and saving applies at once (a `[req]`
  line count, or a 429 sooner), and `git diff --no-index config/defaults.yaml
  config/config.yaml` shows only that value changed -- every comment kept,
- changing `server.port` shows "Saved. The server keeps running with ..."
  until `scripts/run.sh` restarts it on the new port,
- editing `config.yaml` by hand (or `scripts/run.sh config set
  logs.echo false`) is picked up within a few seconds (`[config] config.yaml
  reloaded: live logs.echo`), and a hand edit made while the form is open
  makes its Save answer "changed while you were editing" instead of
  overwriting it,
- `server.port: 0` written by hand is reported on the Settings screen and the
  log, and the server keeps running; Ctrl+Z after a save puts the old values
  back, and
- a `config.yaml` with `server.secret_key` in it (an older project) has the
  key moved into `secret.key` by the next `scripts/run.sh`, which says so, and
  the existing accounts still sign in.

`server.secure_cookies: false` matters locally: a `Secure` cookie is
silently dropped over plain http, so login appears to succeed and then does
nothing. That symptom is almost always this setting. Set it back to `true`
for anything served over https.

`manage` works whether or not the server is running: with the server up it
sends the command over the control socket, and with it stopped it opens the
database directly. It prints which route it took on stderr. If a command
fails with a lock timeout, the server was started with
`server.control_socket: false`, or the command ran in a separate `docker run`,
which cannot see the running container's socket (use `docker exec`).
`manage restore` and `manage rotate-key` are the
exceptions: they need the server stopped.

To check backups, run `manage backup` with the server up, stop it, and run
`manage restore -from <file> -yes`. Tell the user a backup leaves `secret.key`
out unless they pass `-include-key`, and that it cannot be read without that key.

If the user wants the single-file build, verify that too rather than assuming
it works: `scripts/build.sh` for their platform, then run the binary from an empty
directory and confirm the startup log says `[assets] static=embedded` and the
page still renders. That is the one check that proves the embed worked.

## Step 5: Ship it

Two ways out, and they are for genuinely different situations. Pick based on
what the user is doing rather than defaulting to one.

### Docker -- for a long-lived deployment

```bash
cd <target-dir>
docker/run.sh                  # build, run, wait until healthy, print the URL
docker/restore.sh <file> [key] # put a backup back
docker/rotate-key.sh --yes     # move to a new secret key
scripts/login.sh --docker      # a fresh owner login link from the container
```

Every flag on the `docker run` comes from the `docker:` section of
`config/config.yaml` (plus `server.port` for the container side of the
published port), and the container reads the same file, mounted writable at
`/app/config/config.yaml` so the Settings screen can save it (in place --
the previous version goes to `<data_dir>/config.yaml.bak`), with
`config/secret.key` mounted read-only beside it. Inside the container the
`docker:` and `build:` keys are read-only on the Settings screen -- they decide
what `docker/run.sh` mounts and chowns on the host, so they are edited there. `docker/run.sh` waits for the healthcheck, prints
`Ready: <url>` (or the container's last log lines if it fails), and on a fresh
database prints the owner's first login link straight from the log. On a fresh
clone there is no `config/config.yaml` (it is gitignored), so the first
`docker/run.sh` writes it and a new `secret.key`, and
stops, asking for a review of `server.secure_cookies`, `server.trust_proxy`
(with `server.trusted_proxies`), and the published port before anything
starts.

This is where the real hardening lives: read-only root filesystem, all
capabilities dropped, no-new-privileges, memory and pid limits, restart
policy. Use it for anything internet-facing or long-running.

The image and container names are `docker.image` and `docker.container`,
which default to the lowercased slug of the project name (Docker rejects
uppercase repository names); `scaffold.py` prints it.

### scripts/build.sh -- for one file you can hand someone

```bash
cd <target-dir>
scripts/build.sh               # every target, into dist/
scripts/build.sh linux/amd64   # or just one
```

Produces a self-contained executable per platform (darwin, linux, windows;
amd64 and arm64) with the entire frontend compiled in. No Docker, no Go
toolchain, no `static/` directory, no `language.yaml`, no libc. Copy the file
somewhere and run it; it creates `~/.config/<slug>/` with a `config.yaml`, a
`secret.key` (fresh key) and `app.db`, and prints the first-run login link.

Reach for this for an internal box with no container runtime, a Raspberry Pi,
a colleague who just wants to try it, or a USB stick. It does **not** replace
Docker's sandboxing -- say so rather than letting someone ship a bare binary
to the internet believing it is equally protected.

The same binary is the admin CLI, which is what keeps it genuinely
self-sufficient:

```bash
./myapp manage list-users
./myapp manage reissue-login -user-id 1
./myapp manage create-key -user-id 1 -name "CI" -role admin -days 90
./myapp manage list-keys
./myapp manage delete-key -key-id <id>
./myapp manage delete-user -user-id 3 -yes
./myapp manage transfer-owner -user-id 2
./myapp manage create-invite -user-id 1 -label "Book club" -uses 20 -days 14
./myapp manage backup                                   # safe while it runs
./myapp manage restore -from backup-<time>.tar.gz -yes  # with it stopped
./myapp manage rotate-key -yes                          # with it stopped
./myapp version
```

If a `./static` directory sits beside it, or a `language.yaml` in its app directory, those win
over the embedded copies -- that is what makes editing the frontend during
development work, and it doubles as the supported way for an operator to
reword the UI without rebuilding.

Either way, report the login link back to the user -- they need it to get in,
it is shown once, and it is not recoverable.
`references/docker-and-deployment.md` covers the hardening flags, the TLS
expectations, and secret-key handling for both paths.

## Reference files

Read these when the task goes past "scaffold and build features":

- `references/architecture.md` -- the auth model, passwords and what hashing
  them in the browser does and does not buy, two-factor sign-in, invite links (who they
  can create and sign in as, the PIN, and what a leaked one costs), how API
  keys mirror the admin/user roles and why a key can neither outrank its owner nor mint
  another key, the owner account and who may delete whom, why credentials are
  shaped `<id>.<secret>`, the `/mcp` endpoint and what it deliberately leaves
  out, which crypto is used where and why, the static
  caching policy, the `language.yaml` contract, and the known limits (one
  process per bolt file, per-process rate limiting, at-rest key loss). Read
  before changing the template's structure rather than building on it.
- `references/docker-and-deployment.md` -- what each Dockerfile and
  `docker/run.sh` choice is protecting against, how the scripts read
  `config.yaml` through the image, TLS/reverse-proxy expectations, the
  secret key, backups, and operating the container.
- `references/configuration.md` -- the single `config.yaml`: where it is
  found, how it is validated, `secret.key` and the migration out of older
  config files, changing settings while the server runs (the Settings screen,
  the watcher, apply-mode tags, the in-place editor and its limits), the app
  directory convention, storage for application files, and what to back up.
  Read before adding a setting or writing a file anywhere.
- `references/frontend.md` -- the language.yaml contract in detail, the
  no-build-step rule and how to vendor a library, the JS file conventions,
  Bootstrap, the sidebar/screen shell, and the light-mode CSS structure.
