---
name: webserver
description: Scaffold a new security-hardened Python web server project with link-based (no signup/password) admin+user login, gzip'd static asset serving, a modular file layout, and a hardened Docker deployment. Use this whenever the user wants to start a new website, web app, internal tool, or dashboard from scratch in Python and needs auth, not just a script -- trigger on phrases like "build me a webserver/website/web app", "I need a simple internal tool with logins", "set up a Python backend with an admin panel", "scaffold a new app with Docker", or "I don't want a signup page, just let me create accounts myself". Also use it when asked to add Docker support, a login system, or an admin panel to a Python project that doesn't already have one. Do NOT use this for a task that just needs a one-off script, a data pipeline, a CLI tool, or a project that already has its own established auth/serving stack the user wants extended in its own style -- see "When this template does not fit" below.
---

# Webserver scaffolder

This skill scaffolds a specific, opinionated kind of project: a small
Starlette (ASGI) app whose only real jobs are (1) proving who's logged in
via one-time links instead of a signup/password system, and (2) storing +
serving the static HTML/JS/CSS that makes up the actual app. Read
`references/architecture.md` for the full reasoning; the short version:

- **No passwords, no signup form.** An admin (or the CLI) mints a random,
  single-use, expiring login link for a person. Visiting it logs them in
  and sets a secure session cookie. That's the entire "auth system."
- **The server stays thin.** Business logic and page rendering belong in
  `static/js/`, not in new server-side templating. The server's routes are
  just enough JSON API to make login/account/admin management work from a
  browser.
- **Everything is modular.** Config, db access, session/token logic,
  static serving, and each route group live in their own small file under
  `app/`. Don't grow one file into a monolith -- add a new module instead.
- **Docker, no compose.** A hardened multi-stage Alpine `Dockerfile` plus
  a single `dockerRun.sh` (no docker-compose, ever) is the deployment
  story.

## When this template does not fit

This is a strong, specific opinion about auth and structure -- it's not
the right tool for everything Python-and-web:

- A quick script, scraper, or data pipeline with no need for logins at
  all -- just write that directly.
- A project that already has an established framework, auth system, or
  deployment setup the user wants to extend in its existing style --
  extend that instead of introducing a second, incompatible pattern.
- A need for true self-service signup, OAuth/SSO, or password-based login
  -- this template's whole point is avoiding that; if the user explicitly
  wants one of those, say so and adjust rather than silently building
  magic links anyway.

If in doubt, ask rather than assume -- scaffolding the wrong auth model
into a real project is expensive to unwind later.

## Step 1: Confirm the basics

Before scaffolding, make sure you know:

1. **Project/app name** -- used as the Docker image/container name and
   the page title. Ask if not given.
2. **Target directory** -- where the project should be created. Refuse to
   scaffold into a non-empty directory (the scaffold script already
   enforces this) rather than overwriting something that might be
   existing work.
3. **What the app actually does** -- the template ships one placeholder
   page (account settings + admin panel). Get enough of a sense of the
   real feature set that you can plan where it goes (new route modules?
   new static pages?) even if you build it out in a later pass.

You do not need to interrogate the user about the auth model, Docker
setup, or file layout -- those are exactly what this skill already
decided. Only ask about things specific to *this* project.

## Step 2: Scaffold the project

Run the bundled script rather than hand-copying or retyping the
boilerplate -- it does the copy + placeholder substitution consistently
and refuses to clobber a non-empty directory:

```bash
python3 <skill_dir>/scripts/scaffold.py <target-dir> "<project-name>"
```

This copies `assets/template/` (the full app -- see its layout below)
into `<target-dir>` and replaces every `{{PROJECT_NAME}}` placeholder with
the given name. It does not install dependencies, start anything, or
create any users -- that's deliberate, so nothing happens on disk or over
the network that the user didn't ask for yet.

### What you get

```
<target-dir>/
  app/
    main.py                 # wires routes + middleware together, nothing else
    core/
      config.py              # every env-configurable setting, in one place
      db.py                  # sqlite connection + schema (shared by all models)
      security.py            # cookie/session helpers, require_login/require_role, CSRF check
      static_files.py        # gzip-fresh serving for html/js/css, cached serving for images
      middleware.py           # security headers + basic rate limiting
    models/
      users.py, sessions.py, login_tokens.py   # one table each, no ORM
    routes/
      auth.py                # GET /login/{token} -- the entire login flow
      account.py              # /api/me, rename, logout -- any logged-in user
      team.py                  # /api/team -- id+display_name for any logged-in user (owner pickers etc.)
      admin.py                 # /api/admin/... -- create/disable users, mint login links
      pages.py                 # hooks up static_files.serve_static as the catch-all
  static/
    index.html, css/style.css, js/api.js, js/app.js   # placeholder account+admin UI
  scripts/
    manage.py                 # CLI: bootstrap-admin, create-login, reissue-login
  requirements.txt             # starlette + plain uvicorn (no C deps -- see docker reference)
  Dockerfile                   # multi-stage, alpine, non-root
  dockerRun.sh                 # build + hardened `docker run`, no compose
  .dockerignore, .gitignore
```

## Step 3: Build out the actual app on top

This is the part that's specific to what the user asked for, so use
judgment rather than a fixed recipe -- but stay consistent with the
patterns already in the template:

- **New pages** are static files under `static/`. Wire up any new
  client-side behavior in a new file under `static/js/` and include it
  from the relevant HTML page, rather than growing `app.js` indefinitely.
- **New server-side capabilities** (anything that needs to touch the
  database or enforce a permission) get a new route module under
  `app/routes/`, following the two-line auth-check pattern used in
  `admin.py`/`account.py` (see `references/architecture.md`). Register new
  routes in `app/main.py`'s `routes` list -- keep the catch-all static
  route last, since it matches everything. Default new features to "any
  logged-in user can use this" -- only account/login management is
  admin-only unless the user asks for stricter permissions on something
  specific. If a feature needs to list other users (an owner picker, an
  assignee field), use the existing `GET /api/team` rather than the
  admin-only user list or a new endpoint.
- **New data** gets a new table in `app/core/db.py`'s `SCHEMA` and a new
  module under `app/models/` with plain functions (no ORM) that open a
  connection via `get_conn()`, matching `users.py`/`sessions.py`.
- Reuse `app/core/security.py`'s `current_user`/`require_role`/
  `check_csrf` for anything auth-related instead of writing a parallel
  check -- that's the one place session/role logic should live.

## Step 4: Run it locally and verify

Don't just scaffold and hand it over -- actually run it and exercise the
login flow before calling the task done, the same way you would for any
other web app change:

```bash
cd <target-dir>
python3 -m venv .venv && ./.venv/bin/pip install -r requirements.txt
export SECURE_COOKIES=false   # plain http:// on localhost -- never in real deployments
export DATA_DIR="$(pwd)/data"
./.venv/bin/python -m scripts.manage bootstrap-admin --name "Your Name"
./.venv/bin/uvicorn app.main:app --host 127.0.0.1 --port 8000
```

Open the printed `/login/<token>` path in a browser (or curl it with `-c
cookies.txt`, then use `-b cookies.txt` for subsequent requests) to
confirm: the link logs you in, `/api/me` reflects the admin account, the
admin panel can create a new user and produce a working login link for
them, and a second visit to the same login link fails (it's single-use).

## Step 5: Docker

```bash
cd <target-dir>
./dockerRun.sh
```

Then bootstrap the first admin inside the container (see
`references/docker-and-deployment.md` for the exact command and for what
each hardening flag in the Dockerfile/dockerRun.sh is doing and why).
Report the printed login link back to the user -- they need it to actually
log in.

## Reference files

- `references/architecture.md` -- the auth model, why routes are
  structured the way they are, the static-caching policy, and rate-limit
  caveats. Read this before making a structural change to the template
  itself (as opposed to building app features on top of it).
- `references/docker-and-deployment.md` -- what each Dockerfile/
  dockerRun.sh choice buys, TLS/reverse-proxy expectations, and how to
  bootstrap the first admin in a running container.
