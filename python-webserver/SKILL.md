---
name: python-webserver
description: Scaffold a new security-hardened Python web server on Starlette + uvicorn with link-based login (no open signup -- admins hand out login links -- one-time by default, or reusable a set number of times or until they expire -- or one multi-use invite link a whole group chat can join through), optional password login the owner switches on, optional two-factor sign-in, encrypted file uploads, every database record encrypted at rest in a single SQLite file, API keys that reuse the admin/user roles, always-fresh gzip'd static assets, all UI text in a language.yaml file, a hardened Alpine Dockerfile + docker/run.sh with no docker-compose, scripts/run.sh and scripts/login.sh that report the real URL, paths and state, one config/config.yaml that configures the server and the Docker container (nothing reads environment variables), and one state directory convention (~/.config/APP/) holding the database and app storage. It copies only the features the app needs, working that out from what the app does; the rest can be added later with the webserver-upgrade skill. Use this whenever the user wants to start a website, web app, internal tool, dashboard, or API from scratch in Python -- trigger on "build me a webserver/website/web app", "new Starlette app", "Python backend with logins", "I need a simple internal tool with logins", "set up a Python backend with an admin panel", "scaffold a new app with Docker", or "I don't want a signup page, just let me create accounts myself". Also use it when asked to add a login system, an admin panel, invite links, API keys or programmatic/machine access, encrypted storage, or Docker packaging to a Python web project that has none -- trigger on "a link I can drop in our group chat so everyone can join", "let a script call my Python API", "API tokens with admin and read-only levels", "how do I authenticate a cron job against my app", "service account for CI", "add an MCP server/endpoint", "let Claude (Code) use my app", "add password login", or "add 2FA / two-factor / TOTP". Do NOT use it for a one-off script, a data pipeline, a CLI tool, or a project that already has its own auth/serving stack the user wants extended in its own style. This is the Python counterpart to the `go-webserver` skill -- pick that one whenever Go, golang, or Fiber is mentioned, or when the user needs one self-contained executable with no runtime at all.
---

# Python webserver scaffolder

This scaffolds one specific, opinionated kind of project: a Starlette app
whose only real jobs are proving who is logged in, storing data in one SQLite
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
  A link works once by default; whoever makes it can let it work up to N
  times or any number of times, until a chosen expiry or never, and cancel
  it later. That, plus the invite links below, is how every account starts.
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
- **Every record is encrypted at rest.** SQLite is the file, but models store
  JSON records through one layer (`app/core/db.py`) that XChaCha-encrypts
  each one with the secret key. No model has to remember to; none can forget.
- **The server stays thin.** Business logic and rendering belong in
  `static/js/`. The Python side is auth, storage, and file serving. Resist
  adding server-side templating.
- **Everything is modular.** One concern per file under `app/`. When
  something grows, add a module -- never let a file become the monolith the
  layout exists to prevent.
- **No build step, ever.** Plain browser JS and CSS. No npm, no bundler, no
  transpiler, no JSX. Third-party libraries are downloaded once into
  `static/vendor/` and served from there, never pulled from a CDN at runtime.
- **All UI text lives in `config/language.yaml`.** No user-facing string is
  hardcoded in HTML or JS.
- **Bootstrap 5 + Bootstrap Icons, light mode by default.** Vendored in
  `static/vendor/`; a sidebar of screens, one shown at a time. Use its
  components and utility classes before writing CSS.
- **Docker, never compose.** One hardened Alpine image, one `docker/run.sh`.
- **`scripts/run.sh` just makes it run** outside Docker, from the
  checkout: it stops the copy it started last and starts a fresh one in the
  foreground (creating or updating `.venv` first), and says what is really
  happening -- the URL, config file, app dir, whether the database is new,
  which process holds the port if it is taken -- then `Ready: <url>` once it
  answers. With arguments it is the admin CLI instead and leaves the running
  server alone (`scripts/run.sh manage list-users`). **`scripts/login.sh`**
  prints a clickable login link for the owner (user 1) from whichever
  deployment is live -- the local server or the docker container -- and says
  which, then exits. With neither running it prints the link and starts the
  local server right there in the foreground, so Ctrl-C stops it; on a first
  start the server prints the owner's link itself. `--no-start` only writes
  the link into the database, and `--background` starts the server detached
  (log in `.run/server.log`, `scripts/run.sh stop` stops it) and exits, which
  is the one to use from a non-interactive shell, where the foreground start
  would never return. `run.bat` / `login.bat` are the same
  on Windows.
  Run inside the skill's own `assets/template/`, they stamp a demo project
  into `~/.cache/python-webserver-demo/` (via `scripts/demo.py`) and runs
  that, so the template can be tried without scaffolding anything.
- **One entry point.** `python -m app` is the server, and the same command is
  the admin CLI (`python -m app manage ...`), the config tool, the
  healthcheck, and `version`.
- **One `config/config.yaml` configures everything** -- the server and the Docker
  container (image name, ports, mounts, resource limits). Nothing reads
  environment variables: not the Python code, not the scripts, and the
  container gets no `-e` flags and the image no `ENV`. The scripts ask the
  app for their values (`python -m app config shell server|docker`), so there is one
  YAML parser and one set of validation rules, and an unknown key is an error
  rather than a silently ignored typo. It holds no secret: the key that
  encrypts the database is in `config/secret.key` beside it (0600, gitignored,
  never baked into an image, mounted read-only into the container).
- **Settings change while the server runs.** The owner gets a **Settings**
  screen: config.yaml as a form, one field per key, with the help text taken
  from the comments in `defaults.yaml` and a badge saying when a change takes
  effect -- **Live** (at once), **Restart**, or **Scripts** (read by
  `docker/run.sh`). Saving rewrites only the changed values in place, so every
  comment in the file survives, and is refused if the file changed since it
  was loaded. The server also polls config.yaml every two seconds and applies
  a hand edit (or `python -m app config set key value`); an edit that does not
  validate is reported and ignored, and the server keeps running on the last
  good version. Each key's apply mode is a `[live]` / `[restart]` /
  `[scripts]` tag in `defaults.yaml`'s comments.
- **Everything is logged, and only the owner reads it.** Every request
  (who, from where, what, status, time), every sign-in and admin action, and
  what people do on the page (screens, clicks, form submits by field name,
  undo/redo, JS errors, failed calls, device details at sign-in) go to an
  activity log under `<app dir>/logs/`, one file per UTC day, encrypted with
  the database's key. The owner gets a **Logs** screen: one collapsible block
  per day, a search box with `kind:` `user:` `status:5xx` filters, live
  tail, download, delete. Other admins, API keys and the signed out get 404.
  Retention is `logs:` in config.yaml (days kept, total size with oldest-day
  recycling, part size, gzip after N days), applied hourly. Login links, key
  secrets, PINs, passwords, codes and anything typed are never written.
- **Background work is a job.** `purge`, `prune-logs` and `backup` are registered
  in one place, due times live in the database so a restart neither repeats nor
  skips them, and the owner's **Jobs** screen shows how each last went and can
  start one. An exception is that job's failure, never the server's.
- **People can see and end their own sessions.** The Account screen lists each
  device with where it signed in from, and ends any of them but this one.
- **Every request has a reference.** 16 hex characters in `X-Request-ID`, on
  the log line, and in a 5xx body, so a quoted reference finds the line that
  says why. `/api/ready` answers "can this instance serve" for a load balancer,
  and `/metrics` is Prometheus text for an admin API key, off by default.
- **Maintenance mode.** `server.maintenance` is `none`, `read_only` or `full`,
  applied live from Settings. Everyone but the owner's browser session gets a
  503 with `Retry-After`: a page in a browser, JSON for a script.
- **An OpenAPI spec.** `config/openapi.json` describes every route, and
  `GET /api/openapi.json` hands it to anyone signed in. A test fails until a
  new route is in it.
- **Live updates are server-sent events, by hand.** A hub with a client cap, a
  small dropping queue per client, a heartbeat and an auth re-check every 15
  seconds. The Logs screen's live tail runs on it, and a stop ends open streams
  first so they never delay it.
- **One directory holds all state.** The database and app storage live in
  `~/.config/<slug>/` (move it with `server.app_dir`; the container always
  uses `/app/data`). Never write state relative to the working directory.

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
- Something that needs SQL queries or joins over its data. Encrypted records
  cannot be queried; models look records up by key and scan the rest, which
  is fine for an internal tool and wrong for a reporting app. Either turn off
  `server.encrypt_at_rest` and give that data real tables (deliberately --
  see `references/architecture.md`), or use Postgres.
- Several servers on several hosts sharing one database. SQLite is a local
  file: one host, any number of local processes. Past that, Postgres.
- One self-contained executable that runs with no Python installed. Python
  cannot produce that portably; `go-webserver` exists for it.

## Step 1: Confirm the basics

You need three things before scaffolding:

1. **Project name** -- becomes the page title, the default `language.yaml`
   title, and (slugified) the Docker image/container name and the state
   directory `~/.config/<slug>/`.
2. **Target directory** -- the scaffold refuses to write into a non-empty
   directory rather than risk clobbering existing work.
3. **What the app actually does** -- enough to plan where features go
   (new route modules? new buckets? new pages?), even if you build them in
   a later pass.

## Step 1b: Choose the features

A project gets only the features it uses. One left off is **cut out**: no code,
no screen, no routes, no settings. Adding it later is one `add-feature` run of the
`webserver-upgrade` skill, which brings its code in and switches it on. So a wrong
call is cheap, and the point is a project that holds only what it needs.

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
| `backups` | holds data worth keeping in a long-lived deployment and should back itself up on a schedule |

A feature the description gives no reason for stays off. `two_factor` is the
one exception: it stays on unless the app is plainly a one-person tool on the
owner's own machine (that is `--features none`), because it costs people nothing
until they choose it. If it is unclear who will use the app, it stays on.

Sign-in links, session cookies, users, and `manage backup` and `manage restore` are
the core, in every project: `backups` is only the schedule, because a project should
never be without a way to back up by hand. The
activity log (`logs`), the devices list (`sessions`), the Jobs screen (`jobs`),
`maintenance`, the Settings screen (`settings`), `appearance`, `shortcuts` and the
API spec (`openapi`) come with every project too, unless the user asks for a smaller
app or the app plainly has no use for one. Then name them with `--without`.

Say what you chose in one line, with the reason for anything unusual, and go
straight on to scaffolding. For example: "Features: files and two_factor, since
it is a shared document store with real accounts; the rest are left out, and one
`add-feature` run brings any of them in." The user can correct you at any point, and
doing so needs no rescaffold. If they asked for "everything", list all nine.

## Step 2: Scaffold

Run the bundled script rather than retyping files -- it does the copy and
placeholder substitution consistently and will not overwrite a non-empty
directory:

```bash
python3 <skill_dir>/scripts/scaffold.py <target-dir> "<project-name>" --features files,two_factor
```

This copies `assets/template/`, replacing `{{PROJECT_NAME}}` and
`{{PROJECT_SLUG}}`, and writes the project's `config/config.yaml` (the
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
clone all agree, and cuts the rest out with `scripts/prune.py`. `--without` cuts other
parts too. A feature that needs another brings it along and says so, an unknown name
stops before anything is written, and the summary prints what is on and what was left
out. What was cut is recorded under `pruned:` in `.skill-origin.yaml`.

The template carries markers saying which code belongs to which part: the top of
`scripts/prune.py` explains them. New code that belongs to one part needs one, and
`webserver-upgrade/scripts/check_features.py` proves the template still builds and
passes its tests with every part alone and every part removed.

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
  config/
    config.yaml               # THE config: server, docker -- no secret
                              #   (written by scaffold.py; gitignored, 0600;
                              #   editable live from the Settings screen)
    secret.key                # the key that encrypts the database (written by
                              #   scaffold.py; gitignored, 0600) -- back it up
    openapi.json              # the API, by hand; a test keeps it in step
    config.example.yaml       # every default -- commit this; a fresh clone
                              #   copies it to config/config.yaml
                              #   (`python -m app config example` regenerates it)
    defaults.yaml             # every config key, documented and tagged
                              #   [live] / [restart] / [scripts] -- the defaults
    example_header.yaml       # the note config.example.yaml starts with
    language.yaml             # every user-facing string in the app
  app/
    requirements.txt          # pinned; every dependency ships Alpine wheels
    __main__.py               # `python -m app` dispatch: serve | manage |
                              #   config | health | version
    main.py                   # wiring only: config, assets, db, routes, uvicorn
    manage.py                 # the admin CLI (opens the database directly)
    core/
      config.py               # locate, strict-parse, validate, resolve paths/URL
      config_secret.py        # secret.key, and moving an old key out
      config_live.py          # the running config, Settings saves, and the
                              #   2-second watcher -- one validate-then-apply
      yaml_edit.py            # rewrite one value, keep the comments
      config_schema.py        # keys, types, help, [live] tags
      config_command.py       # `config init|example|secret|migrate|check|set|
                              #   shell server|docker`
      layout.py               # the container image's pinned paths
      encryption.py           # the ONLY module that touches crypto
      db.py                   # SQLite record store, encrypt-on-write, and the
                              #   schema version and the list of migrations
      backup.py               # `manage backup` / `restore`, and the schedule
      rotate.py               # `manage rotate-key`: re-encrypt onto a new key
      server_lock.py          # how restore and rotate know a server is running
      security.py             # sessions + API keys -> one Actor, guards
                              #   (incl. RequireAPIKey for /mcp, RequireOwner
                              #   for the logs), CSRF
      middleware.py           # CSP + security headers, rate limit (static files
                              #   exempt, sign-in has its own budget), JSON errors
      logs.py                 # the activity log: entries, redaction, the
                              #   rotating writer thread, read/search/prune
      request_log.py          # one activity-log entry per request
      request_id.py           # X-Request-ID, made here for every request
      metrics.py              # /metrics: the counters and their middleware
      jobs.py                 # background jobs: the registry and the tick
      housekeeping.py         # the template's jobs: purge, prune-logs, backup
      sse.py                  # server-sent events: format, hub, EventStream
      webhooks.py             # the outbox's sender, signing, and the address guard
      error_page.py           # the page a browser gets when a page load fails
      graceful.py             # starting uvicorn, and stopping in the right order
      static_files.py         # gzip-always-fresh text, long-cached images
      assets.py               # configured static dir/language file, or bundled
      language.py             # loads language.yaml, serves it as JSON
      bootstrap.py            # first-run admin + printing links as full URLs
      healthcheck.py          # `health`, the container HEALTHCHECK
      maintenance.py          # server.maintenance: what each mode leaves open
    models/                   # users.py, sessions.py, login_tokens.py, api_keys.py,
                              #   invite_links.py, pins.py, passwords.py,
                              #   two_factor.py, sign_in.py, attempts.py, jobs.py,
                              #   webhooks.py, files.py
    mcp/                      # protocol.py: the /mcp JSON-RPC protocol
                              # registry.py: Tool, Call.bind, Refusal
    routes/                   # __init__.py (the URL map) + one module per group,
                              #   including mcp_tools.py (the MCP tools),
                              #   logs.py (the owner's log API, page events, audit()),
                              #   sessions.py, jobs.py, webhooks.py and metrics.py
    tests/                    # stdlib unittest: python -m unittest discover app/tests
  static/
    index.html                # placeholder account + admin UI, fully data-i18n'd;
                              #   Bootstrap navbar + sidebar, one screen per view, toasts
    css/style.css             # overrides on top of Bootstrap: colour tokens, shell
    js/theme.js               # themes and their worked-out colours, applied before first paint
    js/themes_editor.js       # Appearance screen: own themes, admin defaults, colour pickers
    js/nav.js                 # sidebar: shows the screen the URL hash names
    js/undo.js                # Ctrl+Z / Ctrl+Y history every screen records into
    js/shortcuts.js           # every shortcut; the per-user Shortcuts screen
    js/track.js               # sends what people do on the page to the log
    js/logs.js                # the owner's Logs screen
    js/jobs.js                # the owner's Jobs screen
    js/sessions.js            # the Account screen's list of sessions
    js/fresh.js               # asks the server again on coming back to a tab
    js/qr.js                  # QR codes for 2FA and for login and invite links
    js/login_links.js         # how often and how long a login link works
    js/dom.js, i18n.js, api.js, app.js, keys.js, files.js,
      join.js, pin.js, invites.js   # invite links: join page, PIN, admin panel
      password.js, two_factor.js    # password form and card; 2FA step and card
    vendor/                   # self-hosted libs: Bootstrap 5, Bootstrap Icons (see its README)
  scripts/
    run.sh, run.bat           # local run: (re)starts the server; with args, the CLI
    login.sh, login.bat       # the owner's login link, from whatever is live
                              # (starts the server right there if nothing is)
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
- **New screens** are a `<section class="view" data-view="name">` in
  `index.html` holding a heading and one card per job, plus a sidebar link to
  `#name` (see `references/frontend.md`). A form and the list it fills are two
  cards. Never divide a card with `<hr>`. Settings for your own account are a
  new card on Account, not a new screen.
  Give each its own file in `static/js/` with its own `init()`, included
  from the HTML. Do not keep appending to `app.js` -- `keys.js` is the
  worked example.
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
- **New endpoints** get a new module in `app/routes/`, registered in
  `app/routes/__init__.py` through one of the `group(...)` builders there
  (`signed_in`, `admins`, ...). Protection comes from which builder a route is
  registered with, so it cannot be silently forgotten. Keep the static
  catch-all last; it matches everything after it.
- **Never call the database or bcrypt directly from an async handler.** They
  are synchronous; call them as `await run(fn, ...)` (from
  `app/routes/helpers.py`), which runs them in a worker thread. A direct call
  stalls every other request while it waits.
- **Default new features to "any signed-in user."** Admin status in this
  template means "can decide who gets in," not "can use more features."
  Only gate something on admin if the user specifically asks. If a feature
  needs to list other people, use the existing `GET /api/team` rather than
  the admin-only user list.
- **API keys need no per-feature work.** A key authenticates as its owner at
  its own role, so a route behind `RequireLogin` is reachable by any key and
  one behind `RequireAdmin` only by an admin-scoped key. Read permission
  through `actor_from(request)` (`actor.role`, `actor.is_admin`) rather than
  `actor.user.role`, which is the account's role and ignores a key that
  deliberately carries less. Add `RequireSession` to a route only when it
  must be browser-only, the way key management, account creation and
  login-link reissue are -- and to any new route that creates a credential.
- **New MCP tools** go in `app/routes/mcp_tools_<feature>.py`, registered
  from `build_registry` in `mcp_tools.py`. Pull the HTTP handler's body into
  a synchronous helper both call (the way `me_row` and `set_disabled_checked`
  are), so the two doors share one set of rules. Default to any signed-in
  key; set `admin_only` only when the matching route is admin-only. Read
  arguments with `call.bind({...})` (strict), return a dict, and say no with
  `raise Refusal("...")`. Never write a tool that creates a credential or
  deletes something -- `/mcp` takes only keys, and those routes need a
  session. Set `read_only`/`destructive` honestly; clients use them to decide
  when to ask the person first.
- **Log what matters, never what is secret.** Requests and page activity are
  logged for free. A new action worth an audit line (something changed,
  someone was let in or refused) calls
  `audit(request, KIND_ADMIN, "thing deleted", {"thing_id": thing_id})` (from
  `app.routes.logs`) where it succeeds -- ids and names in the detail, never a
  credential, a body or a typed value. It never blocks, so no `await run`.
  Part of the page that should not be tracked (it shows secrets, or reading
  it would fill the log) gets `data-no-track`.
- **New data** gets a bucket constant in `app/core/db.py` (added to
  `BUCKETS` so its table is created at startup) and a new module in
  `app/models/` with a dataclass and plain functions, matching `users.py`. Go
  through `store.put`/`get`/`all`/`update_value`/`delete_where` -- that is
  what keeps at-rest encryption automatic instead of something each model
  must remember. Anything that edits an existing record goes through
  `update_value`, which is atomic.
- **A change to a stored record's shape** (a field renamed, split, or given a
  new meaning) gets a step at the end of `MIGRATIONS` in `app/core/db.py`, once
  the app has data anyone cares about. A new optional field or a new bucket
  needs nothing. `architecture.md` has the rules and an example.
- **User uploads** already have a home: build on `app/models/files.py` rather than writing
  a route that takes a body. Add a field to the record for what the file belongs to,
  and check it in `app/routes/files.py`.
- **New files on disk** go under `cfg.storage_dir` (`<app dir>/storage/`),
  never a path relative to the working directory -- that changes depending on
  how the process was started, and would land outside what users back up.
- **New settings** get a key in `config/defaults.yaml` (with its default
  and a comment), a field on the matching section dataclass in
  `app/core/config.py`, a check in `validate()` if the value can be wrong, and
  -- if the server reads it -- a field on `Config` filled in by `load()`.
  Strict decoding means a key missing from the dataclass is rejected, so the
  two cannot drift silently. Never read `os.environ`, and never make
  a script read an environment variable instead -- add the value to
  `config shell` in `app/core/config_command.py`. See
  `references/configuration.md`.
- **Never import a crypto module outside `app/core/encryption.py`.** Add a
  function there instead. One file to audit is the entire point.
- **Never build HTML from strings containing user data.** Use `Dom.el`
  with `text:`, which is why that helper exists.
- **Nothing moves under the reader.** After a press, the page stays where it
  was. Build the new content first and put it in with `Dom.swap`; never clear a
  container and fill it after an `await`. Update the pressed control in place
  rather than rebuilding it. Scroll only through `Dom.reveal`. See "Nothing moves
  under the reader" in `references/frontend.md`.
- **Adding a library?** Run `misc/vendor.sh <url>` and reference
  `/vendor/<file>`. A `<script src>` pointing at a CDN is blocked by the
  Content-Security-Policy -- that is the policy working. Vendor the file
  rather than loosening the CSP.
- **New admin CLI commands** go in `app/manage.py`, calling the same model
  functions the routes use, so a rule (disabling kills sessions and pending
  links, the key ceiling) holds whichever door a change comes through. An
  operation that touches several buckets at once goes in one
  `store.transaction()`, the way `app/models/credentials.py` does.
- **New work on a schedule** is a job: `registry.register("name", 3600, fn)`
  next to the others in `housekeeping.register`. `fn` is an ordinary synchronous
  function, runs in a worker thread, and returns `(note, error)`: a few words
  for the Jobs screen and why it failed, empty if it did not. Raising counts as
  failing. Never start your own `asyncio` loop or thread for this: a job gets
  no overlaps, a due time that survives restarts, a place on the Jobs screen
  and a series in `/metrics` for nothing. It can be cut off when the server
  stops, so make it safe to run again from the start. `app/core/jobs.py`.
- **A new event** goes in the list in `app/models/webhooks.py`, and is sent with
  `await emit( request , name , data )` where the thing happens. Ids and names only in
  `data`, since it leaves the server, and never rename one once it has shipped.
- **A new route** goes in `config/openapi.json` too: `app/tests/test_openapi.py` fails
  until it is. A route that changes something must not be a GET, or `read_only`
  maintenance will not stop it.
- **A new list route** answers with `send_page( request , "rows" , rows )`, and its
  screen uses `Pager.load`: a plain `forEach` would stop at the first page.
- **A new page with a path of its own** needs a route before the catch-all, as
  `serve_login_page` has. There is no `index.html` fallback, so an unknown path is a 404.
- **New live data** is a `sse.Hub` on `app.state`, created in `build_app` and
  closed from the `on_stop` that `serve` passes to `graceful.run`, and a route that does what
  `logs.stream_log_changes` does: `hub.join()` (503 when it says `None`), then
  `sse.EventStream(hub, client, allowed, ...)`. `allowed` is the check to repeat
  every 15 seconds, and it is required, because a stream outlives the guard that
  let it in. Publish with `hub.publish(sse.event("name", data))`, or
  `sse.publish_from_thread` from a worker thread. Send "this changed" and let the
  page fetch, rather than a second copy of a read path. See "Live updates" in
  `references/architecture.md`, and `docker-and-deployment.md` for proxies.
- **A 500 you decide on** is `server_error(request, "what went wrong")`, never a
  bare status: the cause is what the log line shows when someone quotes the
  reference.
- **New work for another process** -- a cron job, an importer, a sidecar --
  can run `python -m app manage ...`-style code against the same database:
  SQLite's WAL mode allows it alongside the live server. Go through the model
  functions, never raw SQL, so the rules and the encryption still apply.

## Step 4: Run it and verify the login flow

Do not scaffold and hand it over unverified. Actually exercise the flow:

```bash
cd <target-dir>
# in config/config.yaml: server.secure_cookies: false   (plain http on localhost only)
scripts/run.sh
scripts/login.sh   # from another terminal: a fresh owner login link
```

(`scripts/login.sh` on its own also works: with no server running it prints
a link and starts one in that terminal, where Ctrl-C stops it. Add
`--background` to detach it instead.)

`scaffold.py` already wrote `config/config.yaml` and `config/secret.key`.
`scripts/run.sh` creates `.venv` and installs `app/requirements.txt` when needed (again
whenever that file changes), stops the server it started last
(`.run/server.pid`), and runs `python -m app` in the foreground, reading that
file. Run it again after a Python change. Before starting it prints the
resolved URL, config file, app dir, database (new or existing), and whether
static/ and language come from disk; it refuses to start -- naming the
process -- when something else holds the port, warns when `secure_cookies` is
on for a plain `http://` URL, and prints `Ready: <url>` once the server
answers. `scripts/run.sh config shell server` prints those same resolved
values, `scripts/run.sh manage paths` reports them too, and
`scripts/run.sh config check` validates the file. Any arguments make `run.sh`
a one-off: it runs `python -m app` with them and leaves the running server
alone. On Windows, `scripts\run.bat` does the same.

On a fresh database the server prints a one-time login link. Confirm, by
actually doing it:

- opening the printed `/login/<credential>` shows a sign-in page, and
  pressing the button logs you in and sets a cookie,
- `/api/me` reports the admin account,
- the admin panel creates a user and produces a working link for them,
- using the first link a second time fails (single-use), and so does a link
  replaced by "New link",
- a link made with **Works: 3 times** signs in three times and refuses the
  fourth; the user's row shows "Link: 2 of 3 used" in between, and
  **Cancel link** stops one made with **Any number of times / Never**,
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
key kills it on the next request. `python -m app manage list-keys` works from a shell, with
the server running or not.

Then `/mcp` with the same two keys:

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
- `python -m app manage create-invite --user-id 1 --label "Test"` prints a
  working `/join/...` path.

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

Then the logs, because the point of them is that nobody but the owner sees
them and nothing secret is in them:

- signed in as the owner, **Logs** is in the sidebar; today is open and
  shows your sign-in (`Sign-in`), the requests behind it, and `Page` entries
  for the screens you opened and buttons you clicked,
- `status:4xx`, `kind:auth,admin`, `user:<name>` and free words narrow the
  list; a day folds and unfolds; clicking a row shows its JSON,
- signed in as a second admin there is no Logs entry, and
  `curl -b <their cookie> /api/owner/logs/days` is 404 -- as is any API key,
  the owner's admin key included, and a signed-out request,
- with `server.encrypt_at_rest: false`, `grep` the login link's secret in
  `<app dir>/logs/*.jsonl`: it is not there (the path is `/login/…`),
- `scripts/run.sh manage logs` lists the days and
  `scripts/run.sh manage logs --day <today> --q kind:auth` prints entries, and
- "Prune now" with `logs.retention_days: 1` removes an older day file
  (copy today's to an older date to have one) and never today.

Then what runs without anyone asking. The route tests in `app/tests` cover the
refusals; these need a real server:

- **Jobs** shows `purge` and the others as having run moments after the first
  start, and restarting within a few seconds does not run them again,
- with two sessions signed in, the Account screen lists both, ending the other
  gets it 401 on its next request, and no undo is offered,
- with two sessions signed in, ending one from the other and then clicking
  anything in the ended one lands on the signed-out page saying why, not on a
  banner reading "not authenticated",
- the Appearance screen switches the theme (without `appearance`, the navbar's
  button switches light and dark, signed in and signed out, and with it there is
  no navbar button); a fresh browser opens light whatever the system is set to,
  and a choice survives a reload and follows the account to another browser;
  new, duplicate, rename and delete work and undo; a fine-tune colour set
  unreadable is flagged and Reset brings back the readable one; an admin's
  change on "Defaults for everyone" reaches a user who has not changed their
  own list, and not one who has,
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
- with `server.maintenance: full`, a browser that is not the owner gets the
  maintenance page and the owner's session still works, and an admin can make a
  key for a user on All keys that the user sees as made by them,
- `/api/openapi.json` is 401 signed out and 200 signed in,
- `/api/ready` is 200; with `metrics.enabled` on, `/metrics` is 401 anonymous,
  403 for a browser or a user key, and 200 for an admin key,
- a forced 500 shows a reference, and that reference pasted into the Logs search
  finds a line with a message,
- **Live** on the Logs screen makes no calls while the server is idle, shows a
  new request in about two seconds, and turns itself off within 15 seconds of
  that session being revoked, and
- SIGTERM with Live open exits in under two seconds.

Then the Settings screen, because it writes a file the server runs on
(`app/tests/test_settings_routes.py` covers the routes, and needs `httpx2`, which
Starlette's test client now uses; without it every route test file is skipped,
so install it before trusting a run):

- signed in as the owner, **Settings** is in the sidebar; another admin and
  any API key get 404 from `/api/owner/settings`, and the response never
  contains the key in `config/secret.key`,
- changing `server.rate_limit_max` and saving applies at once, and
  `git diff --no-index config/defaults.yaml config/config.yaml` shows only
  that value changed -- every comment kept,
- changing `server.port` shows "Saved. The server keeps running with ..."
  until `scripts/run.sh` restarts it on the new port,
- editing `config.yaml` by hand (or `scripts/run.sh config set logs.echo
  false`) is picked up within a few seconds, and a hand edit made while the
  form is open makes its Save answer "changed while you were editing" instead
  of overwriting it,
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

## Step 5: Ship it with Docker

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
`docker:` keys are read-only on the Settings screen -- they decide what
`docker/run.sh` mounts and chowns on the host, so they are edited there. `docker/run.sh` waits for the healthcheck, prints
`Ready: <url>` (or the container's last log lines if it fails), and on a fresh
database prints the owner's first login link straight from the log. On a fresh
clone there is no `config/config.yaml` (it is gitignored), so the first
`docker/run.sh` writes it and a new `secret.key`, and
stops, asking for a review of `server.secure_cookies`, `server.trust_proxy`
(with `server.trusted_proxies`), and the published port before anything
starts.

This is where the real hardening lives: read-only root filesystem, all
capabilities dropped, no-new-privileges, non-root fixed uid, memory, CPU and
pid limits, restart policy. Use it for anything internet-facing or
long-running.

Before starting anything, `docker/run.sh` also proves the container can read
`secret.key` and `config.yaml` and write the data directory, and that both are really the
host's files -- a project outside the paths the Docker VM shares (colima
shares only your home directory) is refused with a clear message instead of
silently keeping its database inside the VM. Where the platform enforces host
file modes (colima's sshfs mounts), it prints the exact `chgrp`/`chown` to run.

The image and container names are `docker.image` and `docker.container`,
which default to the slug of the project name (Docker rejects uppercase
repository names); `scaffold.py` prints it.

To check backups, run `manage backup` with the server up, stop it, and run
`manage restore --from <file> --yes`. Tell the user a backup leaves `secret.key`
out unless they pass `--include-key`, and that it cannot be read without that key.

Day to day, account management belongs in the admin panel. The CLI is for
when nobody can get in, and works against the live container:

```bash
docker exec -it <project-slug> python -m app manage list-users
docker exec -it <project-slug> python -m app manage reissue-login --user-id 1
docker exec -it <project-slug> python -m app manage create-key --user-id 1 --name "CI" --role admin --days 90
docker exec -it <project-slug> python -m app manage delete-user --user-id 3 --yes
docker exec -it <project-slug> python -m app manage transfer-owner --user-id 2
docker exec -it <project-slug> python -m app manage create-invite --user-id 1 --label "Book club" --uses 20 --days 14
docker exec -it <project-slug> python -m app manage backup    # safe while it runs
docker/restore.sh data/backups/backup-<time>.tar.gz           # stops it, restores, starts it
docker/rotate-key.sh --yes                                    # new secret key, same data
docker exec -it <project-slug> python -m app version
```

Report the login link back to the user -- they need it to get in, it is
shown once, and it is not recoverable.
`references/docker-and-deployment.md` covers the hardening flags, the TLS
expectations, running without Docker, and secret-key handling.

## Reference files

Read these when the task goes past "scaffold and build features":

- `references/architecture.md` -- the auth model, passwords and what hashing
  them in the browser does and does not buy, two-factor sign-in, invite links (who they
  can create and sign in as, the PIN, and what a leaked one costs), how API
  keys mirror the admin/user roles and why a key can neither outrank its owner nor mint
  another key, the owner account and who may delete whom, why credentials are
  shaped `<id>.<secret>`, the `/mcp` endpoint and what it deliberately leaves
  out, background jobs, sessions, request IDs and metrics, live updates, the
  order of a clean stop, which crypto is used where and why, encrypted
  records in SQLite and what they cost, why there is no control socket, the
  threadpool rule, the static caching policy, and the known limits. Read
  before changing the template's structure rather than building on it.
- `references/configuration.md` -- the single `config.yaml`: where it is
  found, how it is validated, `secret.key` and the migration out of older
  config files, changing settings while the server runs (the Settings screen,
  the watcher, apply-mode tags, the in-place editor and its limits), the app
  directory convention, storage for application files, the container's
  pinned layout, and what to back up. Read before adding a
  setting or writing a file anywhere.
- `references/docker-and-deployment.md` -- what each Dockerfile and
  `docker/run.sh` choice is protecting against, how the scripts read
  `config.yaml` through the image, TLS/reverse-proxy expectations, the secret
  key, backups, stopping, health vs readiness and a scrape setup, streams
  through a proxy, operating the container, and running without Docker.
- `references/frontend.md` -- the language.yaml contract in detail, the
  no-build-step rule and how to vendor a library, the JS file conventions,
  Bootstrap, the sidebar/screen shell, and the light-mode CSS structure.
