# Architecture reference

Read this when you need to explain *why* the template is built this way, or
when you are changing the template itself rather than building features on it.

## Contents

- [Auth model: links instead of passwords](#auth-model-links-instead-of-passwords)
- [API keys: the same roles, for callers that are not browsers](#api-keys-the-same-roles-for-callers-that-are-not-browsers)
- [Why credentials are shaped `<id>.<secret>`](#why-credentials-are-shaped-idsecret)
- [Which crypto is used where, and why](#which-crypto-is-used-where-and-why)
- [Encrypted records in SQLite](#encrypted-records-in-sqlite)
- [At-rest encryption and the secret key tradeoff](#at-rest-encryption-and-the-secret-key-tradeoff)
- [Why there is no control socket](#why-there-is-no-control-socket)
- [The threadpool rule](#the-threadpool-rule)
- [Default permission level for new features](#default-permission-level-for-new-features)
- [Why auth is attached where routes are registered](#why-auth-is-attached-where-routes-are-registered)
- [CSRF](#csrf)
- [Why the server holds so little app logic](#why-the-server-holds-so-little-app-logic)
- [Static asset caching policy](#static-asset-caching-policy)
- [Where state lives](#where-state-lives)
- [Where assets come from: configured or bundled](#where-assets-come-from-configured-or-bundled)
- [One entry point](#one-entry-point)
- [Known limits](#known-limits)

## Auth model: links instead of passwords

There is no password field anywhere in the schema, and adding one would undo
the design. Identity is proven exactly two ways:

1. **Redeeming a one-time login token** (`app/models/login_tokens.py`). An
   admin -- or the first-run bootstrap -- mints a high-entropy credential for
   a specific user. Only a bcrypt hash of its secret half is stored. It
   expires, and an account has at most one pending link: issuing a new one
   deletes the old. Opening `/login/<credential>` only serves the page; the
   page POSTs the credential to `/api/login` when the person confirms, so a
   chat app's link preview or a mail scanner cannot spend it, and someone
   already signed in is told they are about to switch accounts. The bcrypt
   comparison runs first, outside any transaction; then one write
   transaction re-checks the token, checks the account is enabled, marks the
   token used, and stores the session. Two requests racing the same link
   cannot both succeed, and a link redeemed while the account is being
   disabled either lands first (and is swept up by the disable) or is
   refused.
2. **Presenting a valid session cookie** (`app/models/sessions.py`). The
   cookie holds an opaque reference, never the user id or role. Revoking a
   session is deleting a record -- no denylist, no token to outlive its
   welcome.

A third way exists for callers that are not people: an **API key** in an
`Authorization: Bearer` header, carrying a role from the same set users have.
It is a separate section below, because the interesting parts are the ceiling
(a key never outranks its owner) and the two rules that follow from a
credential not being ambient.

"Forgot password" does not exist as a concept. It is replaced by "an admin
mints a new link" (or `python -m app manage reissue-login` from a shell). That
is a real trade: less code, no password storage, no reset-flow attack surface,
at the cost of needing an admin or shell access when someone loses their
session. A project needing genuine self-service recovery -- email round trip,
say -- has to add it; it is not part of the base model.

Three operations look alike and are not:

| | pending link | sessions | API keys |
|---|---|---|---|
| **Reissue login** (`New link`, `manage reissue-login`) | replaced | kept | kept |
| **Disable** (`manage set-disabled`) | deleted | deleted | refused while disabled, restored on re-enable |
| **Revoke access** (`manage revoke-access`, `POST /api/admin/users/{id}/revoke-access`) | deleted | deleted | revoked |

Reissuing is for a lost cookie. When someone else may hold an account's
credentials, revoke access first, then reissue -- reissuing alone leaves a
stolen session signed in. Disable and revoke both run as one transaction with
the account change (`app/models/credentials.py`), so nothing issued
concurrently survives them.

The first-run bootstrap prints the initial admin link to the server's own
logs, so a fresh deploy is usable without a separate setup step. Until that
link is used, anyone who can read those logs can claim the first admin
account -- and log access is often wider than deployment access (a log
shipper, a shared dashboard, a support engineer). Use the link promptly, or
keep the logs private until you have; it also expires after
`auth.login_token_ttl_seconds`. It only fires when no enabled admin exists, so
restarts do not keep minting admins.

## API keys: the same roles, for callers that are not browsers

Sessions cover people with browsers. Everything else -- a cron job, a CI
pipeline, a monitoring script, a sidecar service -- authenticates with an API
key sent as a header:

```
Authorization: Bearer <id>.<secret>
```

The design rule is that **there is one permission model, not two**. A key
carries a role from the same set users do, so an admin-scoped key can do
exactly what an admin can do in the UI, and a user-scoped key exactly what a
non-admin can. There is no separate scope language to invent, document, and
keep in step with the role checks as the app grows.

`app/core/security.py` resolves both doors into one `Actor`, so a route
handler never knows which was used. It reads `actor.role` and gets an answer
that is already correct.

### A key can never outrank its owner

The role on a key is a *request*, and it is bounded twice:

- **When it is minted.** `api_keys.issue_api_key` loads the owner and refuses
  to create an admin-scoped key for a non-admin account. Every path goes
  through it -- the HTTP API and the CLI -- so neither can forget the rule.
- **On every request.** `api_keys.narrow_role` recomputes the effective role
  as the lower of the account's and the key's.

The second check is not redundant. It is what makes demotion work: if an admin
mints an admin key and later becomes an ordinary user, the stored record still
says admin and every request that key makes is treated as a user. `/api/me`
reports the effective role as `role` and the account's as `account_role`, for
the same reason -- a client should not believe it has permission the server
will refuse to act on.

### Why a key request skips the CSRF check

CSRF exists because a cookie is *ambient*: the browser attaches it to requests
some other site caused. Nothing attaches an `Authorization` header on its own,
so a forged cross-site request carries no key and there is nothing to forge
with. Requiring a token would mean every script had to fetch one first, for no
gain.

The cookie is resolved before the header for exactly this reason. Either order
is safe, but this one keeps the rule unambiguous: a request that arrives with
ambient browser credentials is always held to the token check, and cannot opt
out of it by also carrying a key.

### Why a key cannot create credentials

`RequireSession` guards every route that creates a credential: creating and
revoking keys, creating an account, and reissuing a login link. Those need a
browser session even with a correctly scoped admin key.

This is containment, not privilege. A key is a bearer credential that lives in
a config file or a CI secret. If a leaked key could create another credential,
revoking it would not end the compromise -- what it created while nobody was
looking survives. Guarding only `/api/keys` is not enough: a login link
becomes a session, and a session can mint keys, so an admin key that could
reissue a link could turn itself into a fresh session and a fresh key.
Requiring a session for all of them means every credential traces back to a
person signing in (or an operator at a shell), and revoking a key actually
revokes it.

What a leaked admin key can still do is everything else an admin can: list
and disable accounts, revoke access. That is why admin keys must be asked for.

Drop `RequireSession` from a route if an app genuinely needs to provision
accounts or keys programmatically. Do it knowingly -- such a key can then mint
its own successors -- and write it down in that app's docs.

### The rest of the model

- **Same `<id>.<secret>` shape and the same sha256 hash as a session**, for
  the same reasons -- see the next two sections. This runs on every request
  against a 256-bit random secret, so bcrypt would be a latency tax buying
  nothing.
- **Shown once.** Only the hash is stored. A lost key is revoked and replaced,
  never recovered.
- **Expiry is bounded by default** (`auth.api_key_ttl_seconds`, 90 days) so a
  forgotten key stops being a live credential. A key with no expiry is
  available but has to be asked for: `expires_in_days: 0`, or
  `manage create-key --days 0`.
- **`last_used_at` is written at most once every 15 minutes.** "Is anything
  still using this" is the question you ask before revoking, so it is worth
  recording -- but a write per authenticated request would put every handler
  that needs the write lock behind it.
- **A disabled account's keys stop working** immediately, because the owner is
  re-checked on every request. Unlike sessions they are not destroyed:
  re-enabling the account restores them. A session is a browser artifact and
  costs nothing to recreate; tearing down every integration's credential
  because someone was disabled for an afternoon is a footgun with no security
  gain, given the check already refuses the request.
- **25 live keys per account** (`API_KEY_MAX_PER_USER`). Any signed-in user
  can mint keys, so the bucket needs a bound; revoking one frees a slot. The
  count and the insert share one transaction -- counted in one and inserted in
  another, sixteen concurrent requests at 24 keys all pass the check. Dead
  keys are kept 30 days for the listing, but at most 100 rows per account
  (`API_KEY_MAX_ROWS_PER_USER`): past that, minting a key drops the oldest
  dead ones, so mint-revoke churn cannot grow the bucket (or the scans over
  it) without bound.
- **`auth.api_keys: false` closes the whole surface.** The header stops being
  believed, existing keys stop authenticating, and the management endpoints
  404. The records survive, so turning it back on restores them.

### Where the code is

| Concern | File |
|---|---|
| The record, the ceiling, expiry, the per-user cap | `app/models/api_keys.py` |
| Header parsing, the `Actor`, `RequireSession`, CSRF exemption | `app/core/security.py` |
| Endpoints for minting, listing, revoking | `app/routes/api_keys.py` |
| Admin CLI (`list-keys`, `create-key`, `revoke-key`) | `app/manage.py` |
| The screen | `static/js/keys.js` |

## Why credentials are shaped `<id>.<secret>`

Login tokens, sessions, and API keys all use `<id>.<secret>`, and the reason
is worth understanding before changing it.

Storing only a hash of a credential is standard -- it means a stolen database
cannot be replayed. But a *salted* hash like bcrypt cannot be looked up: you
would have to fetch every candidate record and bcrypt-compare each one, which
hands anyone a trivial way to pin the CPU with garbage tokens. (And with every
record encrypted at rest, there is no column to search by anyway.)

Splitting the credential fixes that. The id is the record key, so lookup is
one fetch, and exactly one hash comparison follows.

The halves then use different hashes on purpose:

- **Login tokens use bcrypt.** They are redeemed at most once, so bcrypt's
  deliberate slowness costs a single request and buys real resistance. The
  comparison runs *before* the write transaction that marks the token used,
  so SQLite's single write lock is never held for the slow part; the
  transaction re-checks that the token is still unused before consuming it.
- **Sessions and API keys use sha256.** These run on every authenticated
  request. Against a 256-bit random secret there is nothing to brute force,
  so a fast hash is correct -- bcrypt here would just be a self-inflicted
  latency tax.

Comparisons go through `encryption.constant_time_equal` so the check does not
leak, via timing, how many leading characters matched.

## Which crypto is used where, and why

Everything lives in `app/core/encryption.py`. Nothing else in the project
imports `secrets`, `hashlib`, `hmac`, `bcrypt`, or `nacl` -- that is what makes
"how does this app handle secrets" a one-file question.

| Primitive | Library | Used for |
|---|---|---|
| OS CSPRNG | `secrets` | Every credential, key, and nonce |
| bcrypt | `bcrypt` | Hashing one-time login secrets |
| XChaCha20-Poly1305 | PyNaCl (libsodium) | Session cookie payloads, records at rest |
| sha256 + `hmac.compare_digest` | stdlib | Session and API key secret lookup, constant-time compare |
| secretbox (XSalsa20-Poly1305) | PyNaCl | Available for authenticated blobs; unused by the base template |
| X25519 | PyNaCl | Available for key exchange; unused by the base template |

The last two ship ready but unwired, and that is deliberate. The login flow
needs no key exchange -- a session cookie is an opaque database reference.
They are there for features apps built on this template do grow: handing a
browser or a peer service a public key, or end-to-end encrypting message
payloads.

The Go counterpart also ships Kyber (post-quantum KEM) for a hybrid with
X25519. There is no well-maintained Python implementation with Alpine wheels
to pair with it, so it is left out rather than vendored. Add it when a feature
actually needs it.

Two rules hold throughout:

- **Random bytes come straight from the OS CSPRNG.** A hand-rolled generator
  layered over a CSPRNG cannot be stronger than the CSPRNG, only harder to
  review.
- **Anything on an auth path raises rather than swallowing a failure.** A
  decrypt that quietly returns `""` turns a tampered cookie into an
  empty-but-successful value, which is how a subtle bug becomes an auth
  bypass. Every way a cookie can be wrong -- too short, not base64, wrong
  key, tampered -- ends in `CipherTextError`, which the caller treats as "not
  signed in".

Cookie encryption is not what makes a session unforgeable -- the database
lookup is. It means the cookie is opaque to anyone reading a browser profile
or a proxy log, and that a tampered cookie fails to decrypt before it ever
reaches the lookup. It does **not** stop replay: a copied cookie is still a
working bearer credential until the session ends. What ends it is logging
out, the session expiring, the account being disabled, or revoke access --
each deletes the server-side record the cookie points at.

All three libraries ship prebuilt musllinux wheels, so the Alpine image still
needs no compiler; the Dockerfile enforces that with `--only-binary=:all:`.

## Encrypted records in SQLite

SQLite is the file. What goes in it is records: `app/core/db.py` gives every
"bucket" a table of `(key, value)` rows, and each value is a JSON document,
XChaCha-encrypted with `server.secret_key` when `server.encrypt_at_rest` is on
(the default). Models never see SQL or ciphertext; they call
`put`/`get`/`all`/`update_value`/`delete_where` with plain dicts.

**Why records rather than tables.** Encrypting individual columns would make
encryption something every model has to remember, column by column, and a
forgotten one is a plaintext leak nobody notices. Encrypting whole records in
one place makes it automatic: a new model is encrypted by construction. It
mirrors the Go template's bolt store closely enough that features move between
the two without changing shape.

**Why SQLite rather than a key/value library.** It is in the standard
library, it is one file, and it gives real transactions and multi-process
access (WAL) for free -- which is what lets `manage` run against a live server
with no control socket (below).

**What it costs.** Encrypted values cannot be queried. Models look records up
by key and scan a bucket for anything else (`any_admin_exists`, listing a
user's keys). At internal-tool scale -- hundreds or thousands of records -- a
scan is microseconds and the simplicity is worth it. It is the wrong shape for
reporting, search, or anything with joins.

**When a feature genuinely needs SQL**, it can create its own table through
`store.connect()` -- but those columns are then *not* encrypted at rest. That
is a decision to make deliberately and write down, not a default to drift
into. For an app that needs that broadly, turn `server.encrypt_at_rest` off
and use tables throughout, or move to Postgres.

**Atomicity.** Anything that reads and then writes takes the write lock first
(`BEGIN IMMEDIATE`): `update_value`, `delete_where`, `next_sequence`, and the
login-token transaction. Get-change-put in two steps has a lost update hiding
in the gap -- two concurrent callers both read the old record and the second
write silently discards the first one's change -- so anything that edits an
existing record goes through `update_value`.

## At-rest encryption and the secret key tradeoff

**The cost is real and worth stating plainly to the user: lose the key and the
database is unreadable.** There is no recovery.

The key always lives in `config.yaml`, but where that file sits relative to
the database depends on how the app is run -- `configuration.md` covers this
in full, but the short version:

- **Container**: `config.yaml` stays in the project directory and is mounted
  read-only, separately from the data volume, so a leaked `./data` backup is
  not enough to read it.
- **Run outside Docker with no `./config.yaml`**: the first run writes
  `config.yaml`, key included, into `~/.config/<slug>/` beside `app.db`. That
  is what lets it just run -- at the cost that at-rest encryption then protects
  a database file that leaks *on its own*, not someone who takes the whole
  directory.

`config.load` refuses to start with a missing or malformed key rather than
continuing or generating a new one, because the alternative failure mode only
shows up later, when every record has already become unreadable. A wrong key
fails loudly on the first read with a message that names the cause.

Turn it off (`server.encrypt_at_rest: false`) when operational simplicity beats
at-rest confidentiality -- the data is not sensitive, or disk encryption
already covers it. Decide before there is data: switching an existing database
either way makes its records unreadable until switched back.

## Why there is no control socket

The Go template runs a unix control socket so `manage` can reach the database
while the server holds it, because bolt locks its file to a single process.
SQLite does not. In WAL mode any number of local processes can open the file:
readers never block, and a writer waits (up to the 5-second busy timeout) for
another writer rather than failing.

So `manage` -- and anything else that needs the data -- opens the database
directly, whether or not the server is running. The properties the socket
existed to protect still hold without it:

- **One implementation of the rules.** `manage` calls the same model
  functions the routes do, in the same codebase, so disabling an account
  revokes its sessions and pending links and the key ceiling applies
  whichever door a change comes through.
- **The key is guarded by the filesystem either way.** Anything that can run
  `manage` can read `config.yaml`; a socket would have guarded nothing extra.

What this means for code you add: a cron job, importer, or sidecar on the same
host should import the models and go through them -- never raw SQL against the
buckets, which would bypass both the rules and the encryption.

## The threadpool rule

Starlette handlers are async; SQLite and bcrypt are synchronous. An async
handler that calls them directly blocks the event loop, and every other
in-flight request waits -- for a bcrypt redemption, a noticeable fraction of a
second.

So every database or bcrypt call from async code goes through
`await run(fn, ...)` (`app/routes/helpers.py`, Starlette's
`run_in_threadpool`). The actor middleware does the same for the per-request
lookup, and the static server reads and gzips files the same way. A new
handler that forgets this still works -- it just quietly serialises the whole
server behind itself, which is why it is a rule rather than a suggestion.

## Default permission level for new features

Only two things are admin-gated: managing accounts and minting login links.
(Seeing and revoking *everyone's* API keys is the admin view of the same
question -- who can get in.)

When adding features -- projects, documents, comments, whatever the app is
for -- **default to "any signed-in user."** Add an admin check only if the
user's request specifically calls for it. Most small internal tools built on
this are for one team where everyone is a peer once they are let in at all.
Admin here means "can decide who gets in," not "can use more of the app."

If a feature needs to reference other users, use `GET /api/team`. It is
available to any signed-in user and returns only id and display name --
deliberately omitting the role and disabled status that the admin endpoint
exposes.

API keys inherit this for free, which is the point of giving them the same
roles rather than their own scopes: a feature added under "any signed-in
user" is reachable by any key, and one gated on admin is reachable only by an
admin-scoped key held by an admin. There is nothing extra to declare per
feature.

## Why auth is attached where routes are registered

`RequireLogin`, `RequireAdmin`, and `RequireSession` are ASGI middleware
classes. `app/routes/__init__.py` builds every protected route through a
`group(...)` builder that attaches them (`signed_in = group(RequireLogin)`),
rather than each handler calling a helper at its top.

A handler that forgets a helper call is silently public, and nothing about
reading that handler tells you it is wrong. With builders, protection comes
from how a route is registered -- visible in one place, in the URL map itself.
`LoadActor` runs on every request and resolves the caller at most once, so a
handler and a guard both asking "who is this" do not cost two lookups.

The guards attach to each route (Starlette's `Route(middleware=...)`), not to
a path prefix, so a group can never leak onto routes outside it, and route
order only matters for the static catch-all, which must stay last. Any
`/api/...` path nothing claims answers a JSON 404 rather than falling through
to `index.html`.

## CSRF

`SameSite=Strict` already blocks the common cross-site POST, but it is a
promise the browser makes: older browsers, odd cross-scheme cases, and
non-browser clients do not all honour it. So state-changing endpoints also
check a per-session token that `/api/me` hands out and the client echoes in
the JSON body. It costs nothing and does not depend on the browser behaving.

`Api.post` in `static/js/api.js` attaches it automatically, so a new endpoint
cannot forget it from the client side.

## Why the server holds so little app logic

The stated goal is that the server does auth, storage, and file serving --
not rendering pages or owning UI state:

- `app/core/static_files.py` serves whatever is in `static/`, unmodified
  except for gzip.
- `app/routes` exposes the minimum JSON API to make the auth model usable
  from a browser. How the admin panel looks lives in `static/js/`.

When adding features, put new server logic in new modules under `app/routes/`
(one concern per file) and keep rendering in `static/`. The layout exists to
prevent a monolith; adding to the wrong file is how you get one anyway.

## Static asset caching policy

`app/core/static_files.py` implements one rule:

- **Text assets** (`.html`, `.css`, `.js`, `.json`, `.svg`, ...) are read off
  disk and gzipped on every request with `Cache-Control: no-store`. A deploy
  is live on the next refresh -- no content hashing, no `?v=123`, no
  cache-busting build step to keep in sync. Gzip is what makes that
  affordable: the bytes on the wire are a fraction of the file.
- **Images and binaries** get `max-age` of a week and no gzip. Gzip does
  nothing for already-compressed formats, and nobody wants a photo
  re-downloaded on every view.

Path traversal is stopped two ways: any request containing a `..` segment is
rejected outright, and the resolved path (after symlinks) is then confirmed to
still be under the root. The second check is the real guarantee -- it verifies
where the path landed rather than trying to blocklist how it got there. The
first exists so probes answer 404 instead of quietly hitting the index
fallback, which is clearer in an access log.

A missing path *with* an extension is a 404. A missing path *without* one
falls back to `index.html` so client-side routing works. Serving HTML for a
missing `.js` would leave the browser reporting a confusing MIME error instead
of the real problem.

If this ever costs too much CPU (very large JS, very high traffic), the fix is
smaller static output, not caching the text assets -- that would reintroduce
exactly the staleness this avoids.

## Where state lives

Everything the app persists -- database and uploaded files -- lives in one
directory, `~/.config/<slug>/` by default and `/app/data` in a container.
Everything it can be told -- server and Docker settings, plus the secret key
-- lives in one `config.yaml`, and nothing reads environment variables.

That convention has its own reference: see `configuration.md`. Read it before
adding a setting or writing a file anywhere, because the two rules it exists
to protect (one backup unit, one file that holds every setting) are easy to
break by accident.

## Where assets come from: configured or bundled

`app/core/assets.resolve` decides, at startup, which `static/` directory and
which `language.yaml` to serve, and logs the answer:

```
[assets] static=/srv/app/static (server.static_dir) language=/home/you/.config/myapp/language.yaml
```

That one line answers "why isn't my edit showing up" faster than any amount of
debugging, which is why it is printed on every start.

The rule: `server.static_dir` (default `./static` beside `config.yaml`) wins
when it exists; otherwise the `static/` that ships beside the `app/` package
is used. In a source checkout those are the same directory, so editing a file
and refreshing just works -- the whole point of the no-store policy. The
language file is decided independently: `server.language_file` if set, else a
`language.yaml` in the app directory, else one beside `config.yaml`, else the
bundled copy.

Treating the two independently gives a useful third case: dropping a
`language.yaml` into the app directory rewords the UI without touching the
code, while the frontend stays the shipped one. That is the supported way to
translate or soften a deployment's copy, including inside the container
(`/app/data/language.yaml`).

## One entry point

`python -m app` is the server; `python -m app manage ...`, `config ...`,
`health`, and `version` are the tools around it. One entry point means the
server and every tool read the same `config.yaml` the same way, and there is
no second place (a uvicorn command line, an env file) where a setting could
quietly differ. Each subcommand imports only what it needs, so the
container's 30-second `health` probe does not pay for loading the web stack.

## Known limits

State these to the user rather than letting them be discovered in production:

- **One host per database.** SQLite is a local file. Many processes on one
  host are fine (WAL); several servers on several hosts sharing one database
  is the point at which this template's storage has been outgrown -- move to
  Postgres.
- **Encrypted records cannot be queried.** Lookups are by key and everything
  else is a scan. See [Encrypted records in SQLite](#encrypted-records-in-sqlite).
- **HTTPS is not this server's job, and nothing works safely without it.**
  Login links, session cookies, and API keys are all bearer credentials;
  `secure_cookies` only tells the browser not to send the cookie over plain
  http, it encrypts nothing. Keep the backend's plain-http port reachable only
  by the TLS proxy (`server.host: 127.0.0.1`, `docker.bind_address:
  127.0.0.1`).
- **One server process.** The rate limiter's counters live in memory, which is
  correct for the single uvicorn process `python -m app` starts. Add workers or
  replicas and each holds its own counters, so the effective limit multiplies
  -- move the limiter to a shared store at that point rather than assuming it
  still holds.
- **Losing the secret key loses the data** when at-rest encryption is on. It
  is in `config.yaml`; back that file up with the database, never instead of
  it.
- **An API key is a bearer credential.** Anything holding it is that account,
  at that key's role, until the key is revoked or expires -- there is no
  second factor and no origin binding. That is what a key is for, but it means
  the admin key listing is a real answer to "what could get in right now", and
  worth looking at rather than assuming.
- **Rate limiting counts by IP, not by key.** A key called from many hosts is
  not throttled as one caller, and several keys behind one NAT share a budget.
  The limiter runs before the request is authenticated, which is what keeps an
  unauthenticated flood cheap to refuse; keying it on the actor would mean
  doing the database lookup first.
- **No TLS.** The app expects a reverse proxy in front. See
  `docker-and-deployment.md`.
- **`language.yaml` is public.** It is served to anyone at `/api/language`,
  signed in or not, because signed-out pages need text too. Never put
  anything sensitive in it.
- **Outside Docker there is no sandbox.** `python -m app` from a checkout is a
  plain process with the user's own permissions -- none of Docker's read-only
  filesystem, dropped capabilities, or pid limits apply. Fine for development
  or an internal box; not what should face the internet.
