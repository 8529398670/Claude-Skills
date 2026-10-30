---
name: go-webserver
description: Scaffold a new security-hardened Go + Fiber v3 web server with bolt (bbolt) storage, link-based login (no open signup, no passwords -- admins hand out one-time login links, or one multi-use invite link a whole group chat can join through), always-fresh gzip'd static assets, all UI text in a language.yaml file, a hardened Alpine Dockerfile + docker/run.sh with no docker-compose, a scripts/build.sh that cross-compiles single-file portable binaries with the whole frontend embedded, scripts/run.sh and scripts/login.sh that report the real URL, paths and state, one config/config.yaml that configures the server, Docker, and the build (nothing reads environment variables), and one state directory convention (~/.config/APP/) holding the database and app storage. Use this whenever the user wants to start a website, web app, internal tool, dashboard, or API from scratch in Go -- trigger on "build me a Go webserver", "new Fiber app", "golang backend with logins", "scaffold a Go web project with Docker", "I need a small internal tool in Go", or any Go web project that needs auth and static file serving rather than a bare HTTP handler. Also trigger when someone wants a Go web app shipped as one self-contained executable with no Docker and no runtime dependencies -- "single binary web app", "embed the frontend with go:embed", "cross-compile for linux and windows", "one file I can just hand someone", "where should the app keep its database and config". Also use it when asked to add a login system, an admin panel, invite links, API keys or programmatic/machine access, bolt storage, or Docker packaging to a Go web project that has none -- trigger on "a link I can drop in our group chat so everyone can join", "let a script call my Go API", "API tokens with admin and read-only levels", "how do I authenticate a cron job against my Fiber app", "service account for CI", "add an MCP server/endpoint", or "let Claude (Code) use my app". This is the Go counterpart to the `python-webserver` skill -- pick this one whenever Go, golang, or Fiber is mentioned, and that one for Python/Starlette.
---

# Go webserver scaffolder

This scaffolds one specific, opinionated kind of project: a Fiber v3 app
whose only real jobs are proving who is logged in, storing data in a bolt
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
  deployment is live -- the local server, the docker container, or the
  database offline -- and says which. `run.bat` / `login.bat` are the same
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
  ignored typo. The file holds the secret key, so it is mode 0600,
  gitignored, and never baked into an image.
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
- A need for real self-service signup, OAuth/SSO, or password login. This
  template's whole point is avoiding those. If the user wants one, say so
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

## Step 2: Scaffold

Run the bundled script rather than retyping files -- it does the copy and
placeholder substitution consistently and will not overwrite a non-empty
directory:

```bash
python3 <skill_dir>/scripts/scaffold.py <target-dir> "<project-name>" [module-path]
```

This copies `assets/template/`, replacing `{{PROJECT_NAME}}` and
`{{MODULE_PATH}}`, and writes the project's `config/config.yaml` (the
documented defaults plus a freshly generated secret key, mode 0600) and
`config/config.example.yaml` (the same defaults with no key, for committing). It installs
nothing, starts nothing, and creates no users -- so nothing happens on the
network or on disk that the user did not ask for.

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
                              #   and the go:embed of static/ and config/*.yaml
  go.mod / go.sum             # pinned deps, so the first build is reproducible
  config/
    config.yaml               # THE config: server, docker, build + secret key
                              #   (written by scaffold.py; gitignored, 0600)
    config.example.yaml       # every default, no secret key -- commit this; a
                              #   fresh clone copies it to config/config.yaml
                              #   (`go run . config example` regenerates it)
    defaults.yaml             # every key, documented, embedded -- the defaults
    example_header.yaml       # the note config.example.yaml starts with
    language.yaml             # every user-facing string in the app
  server/
    config/                   # config.go: find, load, validate, resolve paths/URL
                              # command.go: `config init|example|check|shell`
                              # layout_container.go: the image's fixed paths
    db/db.go                  # bolt open + buckets + encrypt-on-write helpers
    control/                  # unix socket so other processes reach the db
    encryption/encryption.go  # the ONLY place that touches crypto primitives
    models/                   # user.go, session.go, login_token.go, api_key.go,
                              #   invite_link.go, pin.go (+ invite_link_test.go)
    security/security.go      # sessions + API keys resolved to one Actor,
                              #   RequireLogin/RequireAdmin/RequireSession/
                              #   RequireAPIKey, CSRF
    mcp/                      # mcp.go: the /mcp JSON-RPC protocol
                              # registry.go: Tool, argument binding, Refuse
    static/static.go          # gzip-always-fresh text, long-cached images
    language/language.go      # loads language.yaml, serves it as JSON
    middleware/middleware.go  # CSP + security headers, rate limit, recover
    routes/                   # routes.go (the URL map) + one file per group,
                              #   including api_keys.go and mcp_tools.go
                              #   (the MCP tools, + mcp_test.go)
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
    js/dom.js, i18n.js, api.js, app.js, keys.js,
      join.js, pin.js, invites.js   # invite links: join page, PIN, admin panel
    vendor/                   # self-hosted libs: Bootstrap 5, Bootstrap Icons (see its README)
  scripts/
    run.sh, run.bat           # local run: (re)starts the server; with args, the CLI
    login.sh, login.bat       # the owner's login link, from whatever is live
    build.sh                  # cross-compiles single-file portable binaries
    lib.sh                    # shared by run.sh and login.sh (sourced)
  docker/
    Dockerfile                # built with the project root as context
    build.sh, run.sh          # build the image; build + run the container
    lib.sh                    # shared by both (sourced)
  misc/vendor.sh              # downloads a library into static/vendor/
  .gitignore, .gitattributes, .dockerignore   # .dockerignore: what the build context leaves out
```

## Step 3: Build the actual app on top

This is the part specific to what the user asked for, so use judgment --
but stay consistent with the patterns already there:

- **New screens** are a `<section class="card view" data-view="name">` in
  `index.html` plus a sidebar link to `#name` (see `references/frontend.md`).
  Give each its own file in `static/js/` with its own `init()`, included
  from the HTML. Do not keep appending to `app.js`.
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
- **New data** gets a bucket constant in `server/db/db.go` (added to
  `bucketNames` so it is created at startup) and a new file in
  `server/models/` with plain functions, matching `user.go`. Go through
  `store.Put`/`Get`/`ForEach` -- that is what keeps at-rest encryption
  automatic instead of something each model must remember.
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
- **Adding a library?** Run `misc/vendor.sh <url>` and reference
  `/vendor/<file>`. A `<script src>` pointing at a CDN is blocked by the
  Content-Security-Policy -- that is the policy working. Vendor the file
  rather than loosening the CSP.
- **Anything under `static/` is embedded automatically** by `main.go`, so new
  pages and vendored libraries end up in the portable binary with no extra
  step. A new asset directory *outside* `static/` would need its own
  `go:embed` line -- putting it under `static/` is simpler.
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
go test ./...      # invite links: capacity, claiming, follows-creator, PINs
# in config/config.yaml: server.secure_cookies: false   (plain http on localhost only)
scripts/run.sh
scripts/login.sh   # from another terminal: a fresh owner login link
```

`scaffold.py` already wrote `config/config.yaml` with a generated secret key, and
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

`server.secure_cookies: false` matters locally: a `Secure` cookie is
silently dropped over plain http, so login appears to succeed and then does
nothing. That symptom is almost always this setting. Set it back to `true`
for anything served over https.

`manage` works whether or not the server is running: with the server up it
sends the command over the control socket, and with it stopped it opens the
database directly. It prints which route it took on stderr. If a command
fails with a lock timeout, the server was started with
`server.control_socket: false`.

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
somewhere and run it; it creates `~/.config/<slug>/` with a `config.yaml`
(fresh secret key) and `app.db`, and prints the first-run login link.

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

- `references/architecture.md` -- the auth model, invite links (who they
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
  found, how it is validated, the secret key and what keeping it there trades
  away, the app directory convention, storage for application files, and what
  to back up. Read before adding a setting or writing a file anywhere.
- `references/frontend.md` -- the language.yaml contract in detail, the
  no-build-step rule and how to vendor a library, the JS file conventions,
  Bootstrap, the sidebar/screen shell, and the light-mode CSS structure.
