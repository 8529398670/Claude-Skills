---
name: python-webserver
description: Scaffold a new security-hardened Python web server on Starlette + uvicorn with link-based login (no open signup, no passwords -- admins hand out one-time login links, or one multi-use invite link a whole group chat can join through), every database record encrypted at rest in a single SQLite file, API keys that reuse the admin/user roles, always-fresh gzip'd static assets, all UI text in a language.yaml file, a hardened Alpine Dockerfile + docker/run.sh with no docker-compose, scripts/run.sh and scripts/login.sh that report the real URL, paths and state, one config/config.yaml that configures the server and the Docker container (nothing reads environment variables), and one state directory convention (~/.config/APP/) holding the database and app storage. Use this whenever the user wants to start a website, web app, internal tool, dashboard, or API from scratch in Python -- trigger on "build me a webserver/website/web app", "new Starlette app", "Python backend with logins", "I need a simple internal tool with logins", "set up a Python backend with an admin panel", "scaffold a new app with Docker", or "I don't want a signup page, just let me create accounts myself". Also use it when asked to add a login system, an admin panel, invite links, API keys or programmatic/machine access, encrypted storage, or Docker packaging to a Python web project that has none -- trigger on "a link I can drop in our group chat so everyone can join", "let a script call my Python API", "API tokens with admin and read-only levels", "how do I authenticate a cron job against my app", "service account for CI", "add an MCP server/endpoint", or "let Claude (Code) use my app". Do NOT use it for a one-off script, a data pipeline, a CLI tool, or a project that already has its own auth/serving stack the user wants extended in its own style. This is the Python counterpart to the `go-webserver` skill -- pick that one whenever Go, golang, or Fiber is mentioned, or when the user needs one self-contained executable with no runtime at all.
---

# Python webserver scaffolder

This scaffolds one specific, opinionated kind of project: a Starlette app
whose only real jobs are proving who is logged in, storing data in one SQLite
file, and serving the HTML/JS/CSS that is the actual application.

The opinions below are already decided. Do not re-litigate them with the
user unless they raise one -- ask only about what is specific to *their*
project.

- **No passwords, no signup form.** On first boot the server mints a
  random one-time login link and prints it to its own logs. Opening it
  shows a sign-in page; confirming there logs you in as admin and sets a
  secure cookie (opening alone spends nothing, so link previews cannot burn
  it). Admins mint more links for more people, and anyone signed in can
  mint a fresh one for their own account to sign in on another device.
  That, plus the invite links below, is the entire account system.
- **Invite links for groups.** An admin can also make one multi-use link
  (`/join/<id>.<secret>`) to post in a group chat: each person types their
  name and joins as a `user`, up to a set number of places, until it expires
  or is switched off. Typing a name close to one that already joined through
  the same link offers "is this you?", which is how that person signs in on
  their next device; a person can set a PIN so nobody else holding the link
  can pick their name. Links die with their maker's admin rights, and
  creating one needs a browser session -- an API key never can.
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
  which. `run.bat` / `login.bat` are the same on Windows.
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
  rather than a silently ignored typo. The file holds the secret key, so it is
  mode 0600, gitignored, and never baked into an image.
- **One directory holds all state.** The database and app storage live in
  `~/.config/<slug>/` (move it with `server.app_dir`; the container always
  uses `/app/data`). Never write state relative to the working directory.

## When this template does not fit

Say so rather than scaffolding the wrong thing:

- A CLI tool, a data pipeline, a worker, or a library with no HTTP surface.
- A project that already has its own framework, router, auth, or
  deployment style the user wants extended -- extend that instead of
  introducing a second incompatible pattern.
- A need for real self-service signup, OAuth/SSO, or password login. This
  template's whole point is avoiding those. If the user wants one, say so
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

## Step 2: Scaffold

Run the bundled script rather than retyping files -- it does the copy and
placeholder substitution consistently and will not overwrite a non-empty
directory:

```bash
python3 <skill_dir>/scripts/scaffold.py <target-dir> "<project-name>"
```

This copies `assets/template/`, replacing `{{PROJECT_NAME}}` and
`{{PROJECT_SLUG}}`, and writes the project's `config/config.yaml` (the
documented defaults plus a freshly generated secret key, mode 0600) and
`config/config.example.yaml` (the same defaults with no key, for committing). It installs
nothing, starts nothing, and creates no users -- so nothing happens on the
network or on disk that the user did not ask for.

### What you get

```
<target-dir>/
  config/
    config.yaml               # THE config: server, docker + secret key
                              #   (written by scaffold.py; gitignored, 0600)
    config.example.yaml       # every default, no secret key -- commit this; a
                              #   fresh clone copies it to config/config.yaml
                              #   (`python -m app config example` regenerates it)
    defaults.yaml             # every config key, documented -- the defaults
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
      config_command.py       # `config init|example|check|shell server|docker`
      layout.py               # the container image's pinned paths
      encryption.py           # the ONLY module that touches crypto
      db.py                   # SQLite record store, encrypt-on-write
      security.py             # sessions + API keys -> one Actor, guards
                              #   (incl. RequireAPIKey for /mcp), CSRF
      middleware.py           # CSP + security headers, rate limit, JSON errors
      static_files.py         # gzip-always-fresh text, long-cached images
      assets.py               # configured static dir/language file, or bundled
      language.py             # loads language.yaml, serves it as JSON
      bootstrap.py            # first-run admin + printing links as full URLs
      healthcheck.py          # `health`, the container HEALTHCHECK
    models/                   # users.py, sessions.py, login_tokens.py, api_keys.py,
                              #   invite_links.py, pins.py
    mcp/                      # protocol.py: the /mcp JSON-RPC protocol
                              # registry.py: Tool, Call.bind, Refusal
    routes/                   # __init__.py (the URL map) + one module per group,
                              #   including mcp_tools.py (the MCP tools)
  static/
    index.html                # placeholder account + admin UI, fully data-i18n'd;
                              #   Bootstrap navbar + sidebar, one screen per view, toasts
    css/style.css             # overrides on top of Bootstrap: colour tokens, shell
    js/theme.js               # light/dark/preset themes, applied before first paint
    js/nav.js                 # sidebar: shows the screen the URL hash names
    js/dom.js, i18n.js, api.js, app.js, keys.js,
      join.js, pin.js, invites.js   # invite links: join page, PIN, admin panel
    vendor/                   # self-hosted libs: Bootstrap 5, Bootstrap Icons (see its README)
  scripts/
    run.sh, run.bat           # local run: (re)starts the server; with args, the CLI
    login.sh, login.bat       # the owner's login link, from whatever is live
    lib.sh                    # shared by run.sh and login.sh (sourced)
  docker/
    Dockerfile                # built with the project root as context
    Dockerfile.dockerignore   # what the build context leaves out
    build.sh, run.sh          # build the image; build + run the container
    lib.sh                    # shared by both (sourced)
  misc/vendor.sh              # downloads a library into static/vendor/
  .gitignore, .gitattributes
```

## Step 3: Build the actual app on top

This is the part specific to what the user asked for, so use judgment --
but stay consistent with the patterns already there:

- **New screens** are a `<section class="card view" data-view="name">` in
  `index.html` plus a sidebar link to `#name` (see `references/frontend.md`).
  Give each its own file in `static/js/` with its own `init()`, included
  from the HTML. Do not keep appending to `app.js` -- `keys.js` is the
  worked example.
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
- **New data** gets a bucket constant in `app/core/db.py` (added to
  `BUCKETS` so its table is created at startup) and a new module in
  `app/models/` with a dataclass and plain functions, matching `users.py`. Go
  through `store.put`/`get`/`all`/`update_value`/`delete_where` -- that is
  what keeps at-rest encryption automatic instead of something each model
  must remember. Anything that edits an existing record goes through
  `update_value`, which is atomic.
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
- **Adding a library?** Run `misc/vendor.sh <url>` and reference
  `/vendor/<file>`. A `<script src>` pointing at a CDN is blocked by the
  Content-Security-Policy -- that is the policy working. Vendor the file
  rather than loosening the CSP.
- **New admin CLI commands** go in `app/manage.py`, calling the same model
  functions the routes use, so a rule (disabling kills sessions and pending
  links, the key ceiling) holds whichever door a change comes through. An
  operation that touches several buckets at once goes in one
  `store.transaction()`, the way `app/models/credentials.py` does.
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

`scaffold.py` already wrote `config/config.yaml` with a generated secret key.
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
- a POST without a `csrf_token` is rejected with 403 -- logout included.

Then exercise the API keys, because the interesting property is the ceiling
and it is the kind of thing that silently stops holding:

```bash
# in the UI: create one "user" key and one "admin" key, then
curl -H "Authorization: Bearer <admin key>" localhost:8000/api/admin/users   # 200
curl -H "Authorization: Bearer <user key>"  localhost:8000/api/admin/users   # 403
curl -H "Authorization: Bearer <user key>"  localhost:8000/api/me            # role: user
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
curl -s localhost:8000/mcp -H "$MCP" -H "Authorization: Bearer <user key>" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'                  # no admin_* tools
curl -s localhost:8000/mcp -H "$MCP" -H "Authorization: Bearer <admin key>" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"admin_list_users","arguments":{}}}'
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8000/mcp -d '{}'   # 401
```

A signed-in browser posting to `/mcp` gets 403, `GET /mcp` gets 405, and
`admin_set_user_disabled` on your own account or the owner comes back with
`isError: true`. To use it from Claude Code:
`claude mcp add --transport http <name> http://localhost:8000/mcp --header "Authorization: Bearer <key>"`.

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

`server.secure_cookies: false` matters locally: a `Secure` cookie is
silently dropped over plain http, so login appears to succeed and then does
nothing. That symptom is almost always this setting. Set it back to `true`
for anything served over https.

## Step 5: Ship it with Docker

```bash
cd <target-dir>
docker/run.sh                  # build, run, wait until healthy, print the URL
scripts/login.sh --docker      # a fresh owner login link from the container
```

Every flag on the `docker run` comes from the `docker:` section of
`config/config.yaml` (plus `server.port` for the container side of the
published port), and the container reads the same file mounted read-only at
`/app/config/config.yaml`. `docker/run.sh` waits for the healthcheck, prints
`Ready: <url>` (or the container's last log lines if it fails), and on a fresh
database prints the owner's first login link straight from the log. On a fresh
clone there is no `config/config.yaml` (it is gitignored), so the first
`docker/run.sh` writes one with a new key and
stops, asking for a review of `server.secure_cookies`, `server.trust_proxy`
(with `server.trusted_proxies`), and the published port before anything
starts.

This is where the real hardening lives: read-only root filesystem, all
capabilities dropped, no-new-privileges, non-root fixed uid, memory, CPU and
pid limits, restart policy. Use it for anything internet-facing or
long-running.

Before starting anything, `docker/run.sh` also proves the container can read
`config.yaml` and write the data directory, and that both are really the
host's files -- a project outside the paths the Docker VM shares (colima
shares only your home directory) is refused with a clear message instead of
silently keeping its database inside the VM. Where the platform enforces host
file modes (colima's sshfs mounts), it prints the exact `chgrp`/`chown` to run.

The image and container names are `docker.image` and `docker.container`,
which default to the slug of the project name (Docker rejects uppercase
repository names); `scaffold.py` prints it.

Day to day, account management belongs in the admin panel. The CLI is for
when nobody can get in, and works against the live container:

```bash
docker exec -it <project-slug> python -m app manage list-users
docker exec -it <project-slug> python -m app manage reissue-login --user-id 1
docker exec -it <project-slug> python -m app manage create-key --user-id 1 --name "CI" --role admin --days 90
docker exec -it <project-slug> python -m app manage delete-user --user-id 3 --yes
docker exec -it <project-slug> python -m app manage transfer-owner --user-id 2
docker exec -it <project-slug> python -m app manage create-invite --user-id 1 --label "Book club" --uses 20 --days 14
docker exec -it <project-slug> python -m app version
```

Report the login link back to the user -- they need it to get in, it is
shown once, and it is not recoverable.
`references/docker-and-deployment.md` covers the hardening flags, the TLS
expectations, running without Docker, and secret-key handling.

## Reference files

Read these when the task goes past "scaffold and build features":

- `references/architecture.md` -- the auth model, invite links (who they
  can create and sign in as, the PIN, and what a leaked one costs), how API
  keys mirror the admin/user roles and why a key can neither outrank its owner nor mint
  another key, the owner account and who may delete whom, why credentials are
  shaped `<id>.<secret>`, the `/mcp` endpoint and what it deliberately leaves
  out, which crypto is used where and why, encrypted
  records in SQLite and what they cost, why there is no control socket, the
  threadpool rule, the static caching policy, and the known limits. Read
  before changing the template's structure rather than building on it.
- `references/configuration.md` -- the single `config.yaml`: where it is
  found, how it is validated, the secret key and what keeping it there trades
  away, the app directory convention, storage for application files, the
  container's pinned layout, and what to back up. Read before adding a
  setting or writing a file anywhere.
- `references/docker-and-deployment.md` -- what each Dockerfile and
  `docker/run.sh` choice is protecting against, how the scripts read
  `config.yaml` through the image, TLS/reverse-proxy expectations, the secret
  key, backups, operating the container, and running without Docker.
- `references/frontend.md` -- the language.yaml contract in detail, the
  no-build-step rule and how to vendor a library, the JS file conventions,
  Bootstrap, the sidebar/screen shell, and the light-mode CSS structure.
