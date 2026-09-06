# Architecture reference

Read this when you need to explain *why* the template is built the way it
is, or when you're extending it and want to stay consistent with its
existing patterns.

## Auth model: links instead of passwords

There is no password anywhere in the schema. Identity is proven exactly
two ways:

1. **Redeeming a one-time login token** (`app/models/login_tokens.py`) --
   a high-entropy random string an admin minted for a specific user. It is
   hashed (sha256) before it ever touches the database, expires after
   `LOGIN_TOKEN_TTL_SECONDS`, and is deleted-in-effect (marked `used_at`)
   the instant it's redeemed, atomically, so two concurrent requests for
   the same link can't both succeed.
2. **Presenting a valid session cookie** (`app/models/sessions.py`) -- the
   cookie holds a random opaque value; only its hash lives server-side.
   This is what lets a session be revoked instantly (delete the row)
   without needing any cryptographic trick like JWT denylists.

This means "forgot password" doesn't exist as a concept -- it's replaced
by "ask an admin (or yourself, via the CLI) to mint a new link." That's a
deliberate trade: less code, no password storage/hashing/reset-flow attack
surface, at the cost of needing an admin (or shell access) in the loop
when someone loses their session. If a project built from this template
needs true self-service account recovery (e.g. via email), that has to be
added -- it isn't part of the base model.

## Default permission level for new features

Only two things are admin-gated in the base template: managing user
accounts (`app/routes/admin.py`) and minting/reissuing login links. When
you add a new feature on top of this template -- projects, documents,
comments, whatever the app is actually for -- default to **any logged-in
user can use it**, and only add an admin check if the user's request
specifically calls for admin-only control over that feature. Most small
internal tools built on this template are for a single team where
everyone is a peer once they're let in at all; admin status here means
"can manage who's let in," not "can use more features."

If a feature needs to reference *other* users (an owner picker, an
assignee field, an @mention list), use `GET /api/team`
(`app/routes/team.py`) rather than `admin.list_users` or a new endpoint --
it's already available to any logged-in user and returns only
`id`/`display_name`, deliberately leaving out role/disabled status that a
non-admin has no business seeing.

## Why routes call `require_role`/`current_user` instead of a decorator

Starlette route functions are plain `async def(request) -> Response`.
`app/core/security.py` exposes small functions rather than decorators so
that a route can decide *what to return* on failure (redirect vs. 403 vs.
404) without the decorator needing to guess. Every protected route follows
the same two-line pattern:

```python
if (forbidden := require_role(request, "admin")) is not None:
    return forbidden
```

Keep new protected routes consistent with this rather than introducing a
second auth-checking style.

## CSRF

Since the cookie is the only bearer of identity and `SameSite=Strict` is
set, CSRF risk is already reduced, but state-changing POST endpoints still
check a per-session CSRF token (`app/core/security.check_csrf`) that the
frontend gets from `/api/me` and must echo back in the JSON body. This
guards against the case where `SameSite=Strict` doesn't apply (some older
browsers, some cross-scheme edge cases) and costs almost nothing to keep.

## Why the server holds so little app logic

The design goal stated for this template is: the server's job is auth +
storage + serving files, not rendering pages or owning UI state. Concretely:

- `app/routes/pages.py` + `app/core/static_files.py` serve whatever is in
  `static/` -- html/css/js files, unmodified except for gzip.
- `app/routes/account.py` / `app/routes/admin.py` expose the *minimum*
  JSON API needed to make the auth model work from a browser (who am I,
  rename myself, mint/revoke logins). Everything else -- how the admin
  panel looks, how the dashboard is laid out -- lives in `static/js/`.

When you add real application features on top of this template, prefer
putting new business logic in new route modules under `app/routes/` (one
concern per file, like `auth.py`/`admin.py`/`account.py` already are) and
keep page rendering/interaction in `static/`, rather than growing any one
file into a monolith.

## Static asset caching policy

See the docstring at the top of `app/core/static_files.py` -- the short
version: `.html`/`.js`/`.css`/`.json`/`.svg` are read fresh off disk and
gzip-compressed on every request with `Cache-Control: no-store`, so a
deploy shows up on the next refresh with zero cache-busting machinery.
Images and other binaries get normal long-lived caching instead, since
gzip does nothing for most image formats and users expect repeat image
loads to be instant. If a project outgrows in-memory-per-request gzip
(very high traffic, very large JS bundles), the fix is to add a
build/bundle step that produces smaller static output -- not to start
caching the text assets, which would reintroduce the staleness problem
this design avoids.

## Rate limiting and multi-process deployments

`app/core/middleware.py`'s `RateLimitMiddleware` keeps its counters in
per-process memory. That's correct for the single-container deployment
`dockerRun.sh` produces. If you ever run more than one replica/worker
process behind a load balancer, this limiter's counts won't be shared
across them -- move it to a shared store (Redis, etc.) at that point
rather than trusting the in-memory version to still be enforcing the limit
you think it is.
