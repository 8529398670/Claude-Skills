---
name: go-webserver
description: Scaffold a new security-hardened Go + Fiber v3 web server with bolt (bbolt) storage, link-based login (no signup, no passwords), always-fresh gzip'd static assets, all UI text in a language.yaml file, a hardened Alpine Dockerfile + dockerRun.sh with no docker-compose, a build.sh that cross-compiles single-file portable binaries with the whole frontend embedded, and one state directory convention (~/.config/APP/) holding the database, config, and app storage. Use this whenever the user wants to start a website, web app, internal tool, dashboard, or API from scratch in Go -- trigger on "build me a Go webserver", "new Fiber app", "golang backend with logins", "scaffold a Go web project with Docker", "I need a small internal tool in Go", or any Go web project that needs auth and static file serving rather than a bare HTTP handler. Also trigger when someone wants a Go web app shipped as one self-contained executable with no Docker and no runtime dependencies -- "single binary web app", "embed the frontend with go:embed", "cross-compile for linux and windows", "one file I can just hand someone", "where should the app keep its database and config". Also use it when asked to add a login system, an admin panel, bolt storage, or Docker packaging to a Go web project that has none. This is the Go counterpart to the Python `webserver` skill -- pick this one whenever Go, golang, or Fiber is mentioned, and that one for Python/Starlette.
---

# Go webserver scaffolder

This scaffolds one specific, opinionated kind of project: a Fiber v3 app
whose only real jobs are proving who is logged in, storing data in a bolt
file, and serving the HTML/JS/CSS that is the actual application.

The opinions below are already decided. Do not re-litigate them with the
user unless they raise one -- ask only about what is specific to *their*
project.

- **No passwords, no signup form.** On first boot the server mints a
  random one-time login link and prints it to its own logs. Visiting it
  logs you in as admin and sets a secure cookie. Admins mint more links
  for more people. That is the entire account system.
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
- **Mobile-first and light-mode by default.**
- **Docker, never compose.** One hardened Alpine image, one `dockerRun.sh`.
- **Ships as a single file too.** The frontend is compiled in with `go:embed`,
  so `build.sh` cross-compiles portable binaries that need no Docker, no Go
  toolchain, and no files alongside them. The same binary is also the admin
  CLI (`myapp manage ...`).
- **One directory holds all state.** The database, an auto-generated secret
  key, app storage, and an optional `config.yaml` live in
  `~/.config/<slug>/` (override with `APP_DIR`; containers use `/app/data`).
  Never write state relative to the working directory -- a binary that gets
  moved would lose track of it.
- **Settings resolve env > config.yaml > default,** everywhere, with no
  exceptions to remember.

## When this template does not fit

Say so rather than scaffolding the wrong thing:

- A CLI tool, a data pipeline, a worker, or a library with no HTTP surface.
- A project that already has its own framework, router, auth, or
  deployment style the user wants extended -- extend that instead of
  introducing a second incompatible pattern.
- A need for real self-service signup, OAuth/SSO, or password login. This
  template's whole point is avoiding those. If the user wants one, say so
  and adapt deliberately rather than quietly shipping magic links anyway.
- Something that needs relational queries, joins, or concurrent writers
  across processes. bolt is a single-writer embedded key/value store; a
  reporting-heavy app wants SQLite or Postgres instead.

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
`{{MODULE_PATH}}`. It installs nothing, starts nothing, and creates no
users -- so nothing happens on the network or on disk that the user did
not ask for.

### What you get

```
<target-dir>/
  main.go                     # wiring only + the manage/version subcommands
  embed.go                    # go:embed of static/ + language.yaml
  build.sh                    # cross-compiles single-file portable binaries
  config.example.yaml         # every setting + its env equivalent, documented
  go.mod / go.sum             # pinned deps, so the first build is reproducible
  language.yaml               # every user-facing string in the app
  server/
    config/config.go          # every env-tunable setting, in one place
    db/db.go                  # bolt open + buckets + encrypt-on-write helpers
    encryption/encryption.go  # the ONLY place that touches crypto primitives
    models/                   # user.go, session.go, login_token.go -- no ORM
    security/security.go      # cookies, current user, RequireLogin/RequireAdmin, CSRF
    static/static.go          # gzip-always-fresh text, long-cached images
    language/language.go      # loads language.yaml, serves it as JSON
    middleware/middleware.go  # CSP + security headers, rate limit, recover
    routes/                   # routes.go (the URL map) + one file per group
    bootstrap/bootstrap.go    # first-run admin + login link printing
    assets/assets.go          # picks disk assets when present, embedded otherwise
    config/config.go          # app dir resolution + env > config.yaml > default
    manage/manage.go          # the admin CLI commands, shared by both entry points
  cmd/manage/main.go          # thin wrapper -- builds manage as a standalone tool
  static/
    index.html                # placeholder account + admin UI, fully data-i18n'd
    css/style.css             # mobile-first, light default
    js/dom.js, i18n.js, api.js, app.js
    vendor/                   # self-hosted third-party libs (see its README)
  scripts/vendor.sh           # downloads a library into static/vendor/
  Dockerfile, dockerBuild.sh, dockerRun.sh, .dockerignore, .gitignore
```

## Step 3: Build the actual app on top

This is the part specific to what the user asked for, so use judgment --
but stay consistent with the patterns already there:

- **New pages** are static files under `static/`. Give each screen its own
  file in `static/js/` with its own `init()`, included from that page's
  HTML. Do not keep appending to `app.js`.
- **New text** goes in `language.yaml` and is referenced with
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
- **New data** gets a bucket constant in `server/db/db.go` (added to
  `bucketNames` so it is created at startup) and a new file in
  `server/models/` with plain functions, matching `user.go`. Go through
  `store.Put`/`Get`/`ForEach` -- that is what keeps at-rest encryption
  automatic instead of something each model must remember.
- **New files on disk** go under `cfg.StorageDir` (`<app dir>/storage/`),
  never a path relative to the working directory -- that changes depending on
  how the process was started, and would land outside what users back up.
- **New settings** get a field on `Config` and one `r.str`/`r.integer`/
  `r.boolean`/`r.seconds` call in `server/config`, plus a line in
  `config.example.yaml`. Those helpers already implement env > file >
  default, so a new setting inherits it. Never call `os.Getenv` elsewhere.
  See `references/configuration.md`.
- **Never import a crypto package outside `server/encryption`.** Add a
  function there instead. One file to audit is the entire point.
- **Never build HTML from strings containing user data.** Use `Dom.el`
  with `text:`, which is why that helper exists.
- **Adding a library?** Run `./scripts/vendor.sh <url>` and reference
  `/vendor/<file>`. A `<script src>` pointing at a CDN is blocked by the
  Content-Security-Policy -- that is the policy working. Vendor the file
  rather than loosening the CSP.
- **Anything under `static/` is embedded automatically** by `embed.go`, so new
  pages and vendored libraries end up in the portable binary with no extra
  step. A new asset directory *outside* `static/` would need its own
  `go:embed` line -- putting it under `static/` is simpler.
- **New admin CLI commands** go in `server/manage/manage.go`, which both the
  `manage` subcommand and `cmd/manage` dispatch to, so they cannot drift.

## Step 4: Run it and verify the login flow

Do not scaffold and hand it over unverified. Actually exercise the flow:

```bash
cd <target-dir>
go build ./...
export SECURE_COOKIES=false          # plain http on localhost only, never deployed
go run .
```

No `SECRET_KEY` needed: on first run it generates one into
`~/.config/<slug>/secret.key` (mode 0600) and reuses it thereafter. The
startup log prints where state landed; `go run . manage paths` reports the
same without starting the server.

On a fresh database the server prints a one-time login link. Confirm, by
actually doing it:

- the printed `/login/<credential>` logs you in and sets a cookie,
- `/api/me` reports the admin account,
- the admin panel creates a user and produces a working link for them,
- visiting the first link a second time fails (single-use),
- a POST without a `csrf_token` is rejected with 403.

`SECURE_COOKIES=false` matters locally: a `Secure` cookie is silently
dropped over plain http, so login appears to succeed and then does
nothing. That symptom is almost always this setting.

Note bolt is single-writer -- stop the server before running `manage`
against the same data directory, or it will block and time out.

If the user wants the single-file build, verify that too rather than assuming
it works: `./build.sh` for their platform, then run the binary from an empty
directory and confirm the startup log says `[assets] static=embedded` and the
page still renders. That is the one check that proves the embed worked.

## Step 5: Ship it

Two ways out, and they are for genuinely different situations. Pick based on
what the user is doing rather than defaulting to one.

### Docker -- for a long-lived deployment

```bash
cd <target-dir>
./dockerRun.sh
docker logs <project-slug>     # the first-run login link is printed here
```

This is where the real hardening lives: read-only root filesystem, all
capabilities dropped, no-new-privileges, memory and pid limits, restart
policy. Use it for anything internet-facing or long-running.

The image and container are named with the lowercased slug of the project
name (Docker rejects uppercase repository names); `scaffold.py` prints it.

### build.sh -- for one file you can hand someone

```bash
cd <target-dir>
./build.sh                     # every target, into dist/
./build.sh linux/amd64         # or just one
```

Produces a self-contained executable per platform (darwin, linux, windows;
amd64 and arm64) with the entire frontend compiled in. No Docker, no Go
toolchain, no `static/` directory, no `language.yaml`, no libc. Copy the file
somewhere and run it; it creates `./data/app.db` beside itself and prints the
first-run login link.

Reach for this for an internal box with no container runtime, a Raspberry Pi,
a colleague who just wants to try it, or a USB stick. It does **not** replace
Docker's sandboxing -- say so rather than letting someone ship a bare binary
to the internet believing it is equally protected.

The same binary is the admin CLI, which is what keeps it genuinely
self-sufficient:

```bash
./myapp manage list-users
./myapp manage reissue-login -user-id 1
./myapp version
```

If `./static` or `./language.yaml` happen to sit next to the binary, those win
over the embedded copies -- that is what makes editing the frontend during
development work, and it doubles as the supported way for an operator to
reword the UI without rebuilding.

Either way, report the login link back to the user -- they need it to get in,
it is shown once, and it is not recoverable.
`references/docker-and-deployment.md` covers the hardening flags, the TLS
expectations, and `SECRET_KEY` handling for both paths.

## Reference files

Read these when the task goes past "scaffold and build features":

- `references/architecture.md` -- the auth model, why credentials are
  shaped `<id>.<secret>`, which crypto is used where and why, the static
  caching policy, the `language.yaml` contract, and the known limits
  (single-writer bolt, per-process rate limiting, at-rest key loss). Read
  before changing the template's structure rather than building on it.
- `references/docker-and-deployment.md` -- what each Dockerfile and
  `dockerRun.sh` choice is protecting against, TLS/reverse-proxy
  expectations, `SECRET_KEY` handling, backups, and operating the
  container.
- `references/configuration.md` -- the app directory convention, settings
  precedence, where the secret key comes from and what that trades away,
  storage for application files, and what to back up. Read before adding a
  setting or writing a file anywhere.
- `references/frontend.md` -- the language.yaml contract in detail, the
  no-build-step rule and how to vendor a library, the JS file conventions,
  and the mobile-first/light-mode CSS structure.
