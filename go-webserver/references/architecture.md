# Architecture reference

Read this when you need to explain *why* the template is built this way, or
when you are changing the template itself rather than building features on it.

## Contents

- [Auth model: links first](#auth-model-links-first)
- [Passwords: an option the owner switches on](#passwords-an-option-the-owner-switches-on)
- [Two-factor sign-in](#two-factor-sign-in)
- [The owner, and deleting accounts](#the-owner-and-deleting-accounts)
- [Invite links: one link for a group](#invite-links-one-link-for-a-group)
- [API keys: the same roles, for callers that are not browsers](#api-keys-the-same-roles-for-callers-that-are-not-browsers)
- [Soft delete and restore](#soft-delete-and-restore)
- [MCP: the API keys, for agents](#mcp-the-api-keys-for-agents)
- [The activity log](#the-activity-log)
- [Background jobs](#background-jobs)
- [Sessions and devices](#sessions-and-devices)
- [Monitoring](#monitoring)
- [Live updates](#live-updates)
- [Why credentials are shaped `<id>.<secret>`](#why-credentials-are-shaped-idsecret)
- [Which crypto is used where, and why](#which-crypto-is-used-where-and-why)
- [At-rest encryption and the secret key tradeoff](#at-rest-encryption-and-the-secret-key-tradeoff)
- [Default permission level for new features](#default-permission-level-for-new-features)
- [Why auth is middleware, not a helper call](#why-auth-is-middleware-not-a-helper-call)
- [CSRF](#csrf)
- [Why the server holds so little app logic](#why-the-server-holds-so-little-app-logic)
- [Static asset caching policy](#static-asset-caching-policy)
- [Where state lives](#where-state-lives)
- [Changing a stored record: migrations](#changing-a-stored-record-migrations)
- [Live configuration](#live-configuration)
- [Where assets come from: disk or embedded](#where-assets-come-from-disk-or-embedded)
- [Known limits](#known-limits)

## Auth model: links first

Links are the way in. A password is optional, off by default, and only ever
set by someone who already signed in with a link (see
[Passwords](#passwords-an-option-the-owner-switches-on)). With passwords off,
identity is proven exactly two ways:

1. **Redeeming a one-time login token** (`server/models/login_token.go`). An
   admin -- or the first-run bootstrap -- mints a high-entropy credential for
   a specific user. Only a bcrypt hash of its secret half is stored. It
   expires, and an account has at most one pending link: issuing a new one
   deletes the old. Opening `/login/<credential>` only serves the page; the
   page POSTs the credential to `/api/login` when the person confirms, so a
   chat app's link preview or a mail scanner cannot spend it, and someone
   already signed in is told they are about to switch accounts. The bcrypt
   comparison runs first, outside any transaction; then one bolt write
   transaction re-checks the token, checks the account is enabled, marks the
   token used, and stores the session. Two requests racing the same link
   cannot both succeed, and a link redeemed while the account is being
   disabled either lands first (and is swept up by the disable) or is
   refused.
2. **Presenting a valid session cookie** (`server/models/session.go`). The
   cookie holds an opaque reference, never the user id or role. Revoking a
   session is deleting a row -- no denylist, no token to outlive its welcome.

An **invite link** is the other way to get a session: one link a whole group
can join through, each person becoming a new `user`. It has its own section
below, because it is the one public way to create an account.

A third way exists for callers that are not people: an **API key** in an
`Authorization: Bearer` header, carrying a role from the same set users have.
It is a separate section below, because the interesting parts are the ceiling
(a key never outranks its owner) and the two rules that follow from a
credential not being ambient.

"Forgot password" is not a flow of its own. It is replaced by "an admin
mints a new link" (or `manage reissue-login` from a shell). That is a real
trade: less code, no reset-flow attack surface, at the
cost of needing an admin or shell access when someone loses their session. A
project needing genuine self-service recovery -- email round trip, say -- has
to add it; it is not part of the base model.

Anyone already signed in can also mint a fresh link for their own account
from the account screen ("Sign in on another device",
`POST /api/account/login-link`), which is how a second device gets in
without an admin. It is a reissue pointed at yourself, so it behaves exactly
like one, and like every route that hands out a login link it needs a
browser session: an API key cannot call it.

Four operations look alike and are not:

| | pending link | sessions | API keys | account |
|---|---|---|---|---|
| **Reissue login** (`New link`, `manage reissue-login`, or your own `Make a login link`) | replaced | kept | kept | kept |
| **Disable** (`manage set-disabled`) | deleted | deleted | refused while disabled, restored on re-enable | kept, disabled |
| **Revoke access** (`manage revoke-access`, `POST /api/admin/users/:id/revoke-access`) | deleted | deleted | revoked | kept, enabled |
| **Delete** (`manage delete-user`, `POST /api/admin/users/:id/delete`) | deleted, used ones too | deleted | rows deleted | deleted, for good |

None of the four touch invite links. A link an admin made stops working
when that admin is disabled, deleted, or demoted, and the accounts a link
made are ordinary accounts that all four apply to.

Reissuing is for a lost cookie. When someone else may hold an account's
credentials, revoke access first, then reissue -- reissuing alone leaves a
stolen session signed in. Disable is for someone who may come back; delete is
for an account that should not exist at all. Disable, revoke, and delete each
run as one transaction with the account change, so nothing issued
concurrently survives them.

The first-run bootstrap prints the initial admin link to the server's own
logs, so a fresh deploy is usable without a separate setup step. Until that
link is used, anyone who can read those logs can claim the first admin
account -- and log access is often wider than deployment access (a log
shipper, a shared dashboard, a support engineer). Use the link promptly, or
keep the logs private until you have; it also expires after
`auth.login_token_ttl_seconds`. It only fires when no enabled admin exists, so
restarts do not keep minting admins.

## Passwords: an option the owner switches on

Links stay the way in. `auth.passwords` (off by default) lets a person who has
already signed in with a link set a username and password on their own screen,
so there is still no signup, and a forgotten password is still "get a new login
link". Turning it off stops every password working and hides the form; the
hashes stay, so turning it back on restores them. The owner can also block one
account from the Users screen (`POST /api/owner/users/<id>/password-access`),
which keeps the hash and leaves its sessions alone.

**The server never sees a password.** `static/js/password.js` derives a digest
in the browser -- PBKDF2-HMAC-SHA256, 600,000 iterations, 32 bytes, base64url --
and that digest is what travels and what bcrypt hashes. The salt is
`<install salt>:<lowercased username>`. The install salt is random, not secret,
and comes from the public `GET /api/login/options`; it is the same for everyone
because a per-user salt would need a lookup that says which usernames exist.
Understand what that does and does not buy:

- A leaked database cannot be replayed: it holds a bcrypt of the digest, not
  the digest. Hashing only in the browser would not give this -- whatever the
  browser sends is the password as far as the server is concerned, so the
  server has to hash it again.
- A compromised server, or a stray log, cannot expose a password someone reuses
  elsewhere. This is the whole benefit over the usual approach of sending the
  password over TLS and hashing on the server.
- **It is no substitute for HTTPS.** The digest works as a password on the
  wire, so anyone reading the traffic could replay it, or change the page that
  computes it. WebCrypto only exists on HTTPS and localhost, so on plain http
  the form refuses (`insecure_context`) rather than send anything.
- **Password rules live in the browser**, because the server cannot see the
  password. Skipping the 12-character minimum only weakens your own account;
  the server checks only that a digest has the right shape.
- **`ITERATIONS` and the salt format are part of every stored password.**
  Changing either makes every password stop working.

Usernames exist because display names are not unique. They are lowercased, 3-32
of `a-z0-9._-`, and checked by scanning users in the write transaction. Changing
one means entering the password again, because the digest is salted with it, so
one form sends both.

`POST /api/login/password` gives one answer for an unknown name, a wrong
password, and a blocked or disabled account, after one bcrypt each -- a dummy
hash stands in when there is no account. Wrong guesses are limited per name
typed (not per account, so a lockout does not say which names are real) and per
address. Links keep working through a lockout, so nobody can lock the owner out
by guessing.

Setting a password does not ask for the old one, because that is also how a
forgotten one is replaced after signing in with a link, and a session can
already mint itself a link. An account with 2FA has to give a code. Setting one
ends the account's other sessions.

**An invite link must not walk around a password.** Anyone holding the link can
pick any name on it, so a claim on an account with a usable password and no PIN
answers `use_password`. An account with a PIN keeps that path.

`revoke-access` removes the password along with everything else, because
"nothing issued before this moment can act as the account" has to stay true.

## Two-factor sign-in

Optional per person, offered while `auth.two_factor` is on (the default): a
code from an authenticator app (TOTP: SHA-1, six digits, thirty seconds, one
step either side), with ten single-use recovery codes. Written with the standard
library's HMAC, so it adds no dependency. The QR code is drawn by the vendored
`qrcode.js`; the secret is shown as text as well.

**It covers every browser sign-in: login links, invite links, and passwords.**
Otherwise any login link, including one an admin mints, would get around it.
API keys are exempt: they are not interactive, and they cannot create
credentials anyway. All three ways end in one function, `startSignInTx` (`server/models/sign_in.go`), inside the
transaction that spent the first factor. For an account with 2FA it stores a
five-minute one-time challenge instead of a session and the page asks for a
code, so a way of signing in cannot forget to. The first factor is already
spent: a login link used here is gone even if the code never comes.

The secret has to be readable, so unlike every other credential it cannot be
hashed. It is encrypted on its own with the secret key, on top of whatever the
record gets, because `server.encrypt_at_rest` can be off. Recovery codes are 80
random bits, so they are sha256'd like the other random secrets. Each TOTP step
works once: the last step used is stored, and anything at or before it is
refused. Five wrong codes lock that account's codes for 15 minutes.

Turning it off, or changing a password, needs a valid code, so a stolen session
cannot remove it. Someone with neither phone nor recovery codes gets a reset:
`POST /api/admin/users/<id>/reset-two-factor` follows the delete rules (only the
owner resets an admin, nobody resets themselves or the owner), and
`manage reset-2fa -user-id N` is the way back for anyone, the owner included. `revoke-access`
leaves 2FA alone, since it is not a way in; reset it as a separate step if it is
what went wrong. With `auth.two_factor` off, nobody is asked and the secrets stay.

## The owner, and deleting accounts

One admin is the **owner**: the first admin ever created, whether by the
first-run bootstrap or `manage bootstrap-admin`. The owner exists for a single
decision the admin role cannot settle by itself -- who may permanently delete
an admin. Without it, any admin can delete any other, and whoever acts first
wins.

| | an ordinary user | another admin | the owner | yourself |
|---|---|---|---|---|
| **Delete, as an admin** | yes | no | no | no |
| **Delete, as the owner** | yes | yes | -- | no |
| **Disable / revoke access** (any admin) | yes | yes | no | no |
| **CLI / control socket** (the operator) | delete, disable, revoke | delete, disable, revoke | revoke only | -- |

- **A marker, not a role.** The owner is one id in the `meta` bucket
  (`owner_user_id`), not a third role beside admin and user. Roles are also
  what API keys carry and what `NarrowRole` compares, so a third one would
  spread into key scoping, role validation, and every role check -- for
  something only a few account routes care about. Everywhere else the owner
  is an ordinary admin. `/api/me` reports `is_owner`; the admin user list
  marks the owner's row.
- **The invariant:** the owner is always an existing, enabled admin. Nothing
  deletes or disables the owner, not even the CLI, and ownership only moves
  to another enabled admin with `manage transfer-owner -user-id N`. There is
  no web route for that on purpose: a compromised admin session must not be
  able to promote itself past the other admins. Because an enabled admin
  always exists, the first-run bootstrap never fires again either.
- **The CLI may revoke the owner's access.** When it is the owner's own
  credentials that leaked, someone with shell access has to be able to clean
  up. The web route refuses.
- **Older databases** get an owner on the next boot (or the next `manage`
  command): the lowest-id enabled admin, which is the bootstrap admin on any
  database that started from first run. See `models.EnsureOwner`.
- **Deleting needs a browser session.** `POST /api/admin/users/:id/delete`
  sits behind `RequireSession`, unlike disable and revoke access. Those can
  be undone, so a leaked admin-scoped key may use them; deleting cannot, so a
  key may not.
- **Ids are never reused.** They come from the bucket's sequence, so a deleted
  account's id cannot come back as a different person, and any stale
  reference to it stays a dangling id rather than a wrong one.
- **Records your app adds are not touched.** `models.DeleteUser` removes
  sessions, login links, API keys, and the user row. When a feature stores a
  `user_id` -- an assignee, an author -- decide in `DeleteUser` whether those
  rows go with the account (delete them in the same transaction) or outlive
  it (render a missing user as `#<id>`).

The rules live in `models.DeleteUser`, `models.SetUserDisabled`, and
`models.RevokeAllCredentials`, checked inside each one's own transaction, so
ownership cannot move between a check and the change it guards.

## Invite links: one link for a group

A login link is for one named person. An **invite link** is for a group: an
admin makes one (`POST /api/admin/invite-links`, or `manage create-invite`), posts it in a group chat, and each
person who opens it joins on their own. The code is `server/models/invite_link.go`
(the record and the transactions), `server/models/pin.go` (PINs and name matching),
`server/routes/join.go` (the public join page's calls), `server/routes/invite_links.go` (the admin
panel's), and `static/js/join.js`, `pin.js`, `invites.js`.

How it works:

- **Opening `/join/<id>.<secret>` only serves the page**, with
  `Referrer-Policy: no-referrer`, exactly like a login link -- a link preview
  creates nothing. The page asks for a name and POSTs it with the credential
  in a JSON body, under the same `same-origin JSON` rule as `/api/login`.
- **A new name becomes a new account, always role `user`,** up to the link's
  places (1-1000) and until it expires (1-365 days) or is switched off.
  Checking the link, taking a place, creating the account, and storing its
  session are one write transaction, so two people cannot both take the last
  place.
- **A name close to one that already joined through this link is offered
  back** ("is one of these you?"). Picking it signs in as that account -- this
  is how the same person gets in on their next device, with no admin
  involved. The page shows this step even with no matches, because a typo
  would otherwise use up a place. Only accounts the link itself created can be
  offered or picked: never an admin, never a disabled account, never someone
  who joined another way.
- **A PIN closes the obvious hole in that.** Anyone holding the link can pick
  anyone who joined through it, so a person can set a 4-8 digit PIN
  (`POST /api/account/pin`) and it is then asked for. It is bcrypt-hashed, and
  5 wrong guesses lock that account's PIN for 15 minutes. It is not a
  password: nothing but the invite-link claim checks it.
- **A link dies with its maker's admin rights.** Disabling, deleting, or
  demoting the admin who made it stops it working at once, without touching
  the link's row. Switching it off (`revoke`) keeps the row for the list;
  deleting removes it. Neither touches the accounts it made or their sessions
  -- disable or revoke those per account as usual.
- **Creating a link, deleting one, and setting a PIN need a browser session.**
  A link mints accounts and sessions, and a PIN lets its holder sign in through
  a link, so an API key that could do either could turn itself into a session
  -- the same rule as "a key never creates credentials". Listing links and
  switching one off are fine for an admin key.

The secret half is sha256-hashed, not bcrypt: it is 24 random bytes, not
something a person chose, so there is nothing to slow down.

What a leaked link costs: anyone holding it can join, up to the places left,
and can sign in as any account it created that has no PIN. That is the trade
for "one link in a group chat". Keep the places and the lifetime close to the
group's real size, switch a link off once everyone is in, and tell people to
set a PIN if the chat is large or public.

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

`server/security` resolves both doors into one `Actor`, so a route handler
never knows which was used. It reads `actor.Role` and gets an answer that is
already correct.

### A key can never outrank its owner

The role on a key is a *request*, and it is bounded twice:

- **When it is minted.** `models.IssueAPIKey` loads the owner and refuses to
  create an admin-scoped key for a non-admin account. Every path goes through
  it -- the HTTP API, the control socket, the CLI -- so none of them can
  forget the rule.
- **On every request.** `models.NarrowRole` recomputes the effective role as
  the lower of the account's and the key's.

The second check is not redundant. It is what makes demotion work: if an admin
mints an admin key and later becomes an ordinary user, the stored row still
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

`Guard.RequireSession` gates every endpoint that creates a credential:
creating, revoking, and deleting keys, creating an account, and reissuing a
login link. Those need a browser session even with a correctly scoped admin
key. Deleting an account needs one too, for a different reason -- it cannot
be undone (see [the owner](#the-owner-and-deleting-accounts)).

This is containment, not privilege. A key is a bearer credential that lives in
a config file or a CI secret. If a leaked key could create another credential,
revoking it would not end the compromise -- what it created while nobody was
looking survives. Gating only `/api/keys` is not enough: a login link becomes
a session, and a session can mint keys, so an admin key that could reissue a
link could turn itself into a fresh session and a fresh key. Requiring a
session for all of them means every credential traces back to a person
signing in, and revoking a key actually revokes it.

What a leaked admin key can still do is everything else an admin can: list
and disable accounts, revoke access -- but not delete accounts. That is why
admin keys must be asked for.

Drop `RequireSession` from a route if an app genuinely needs to provision
accounts or keys programmatically. Do it knowingly -- such a key can then mint
its own successors -- and write it down in that app's docs.

### The rest of the model

- **Same `<id>.<secret>` shape and the same sha256 hash as a session**, for
  the same reasons -- see the next two sections. This runs on every request
  against a 256-bit random secret, so bcrypt would be a latency tax buying
  nothing.
- **Shown once.** Only the hash is stored. A lost key is revoked or deleted
  and replaced, never recovered.
- **Revoke or delete.** Both stop the key on its next request. Revoking keeps
  the row, marked dead, so the listing still answers "what existed last
  week"; the `purge` job drops it after 30 days. Deleting removes the row now.
  A key's owner can do either to their own keys, and an admin to anyone's
  (`/api/keys/:id/delete`, `/api/admin/keys/:id/delete`,
  `manage delete-key`). Deleting an account deletes all of its keys.
- **Expiry is bounded by default** (`API_KEY_TTL_SECONDS`, 90 days) so a
  forgotten key stops being a live credential. A key with no expiry is
  available but has to be asked for: `expires_in_days: 0`, or
  `manage create-key -days 0`.
- **`last_used_at` is written at most once every 15 minutes.** "Is anything
  still using this" is the question you ask before revoking, so it is worth
  recording -- but bolt has a single writer, and a write per authenticated
  request would put every other handler behind it.
- **A disabled account's keys stop working** immediately, because the owner is
  re-checked on every request. Unlike sessions they are not destroyed:
  re-enabling the account restores them. A session is a browser artifact and
  costs nothing to recreate; tearing down every integration's credential
  because someone was disabled for an afternoon is a footgun with no security
  gain, given the check already refuses the request.
- **25 live keys per account** (`models.APIKeyMaxPerUser`). Any signed-in user
  can mint keys, so the bucket needs a bound; revoking or deleting one frees a
  slot. The count and the insert share one transaction -- counted in one and
  inserted in another, sixteen concurrent requests at 24 keys all pass the
  check. Dead keys are kept 30 days for the listing, but at most 100 rows per
  account: past that, minting a key drops the oldest dead ones, so mint-revoke
  churn cannot grow the bucket (or the per-request scans over it) without
  bound.
- **`auth.api_keys: false` closes the whole surface.** The header stops being
  believed, existing keys stop authenticating, and the management endpoints
  404. The rows survive, so turning it back on restores them.

### Where the code is

| Concern | File |
|---|---|
| The record, the ceiling, expiry, the per-user cap | `server/models/api_key.go` |
| Header parsing, the `Actor`, `RequireSession`, CSRF exemption | `server/security/security.go` |
| Endpoints for minting, listing, revoking, deleting | `server/routes/api_keys.go` |
| Admin CLI (`list-keys`, `create-key`, `revoke-key`, `delete-key`) | `server/manage/manage.go` |
| The same operations for other processes | `server/control/control.go` |
| The screen | `static/js/keys.js` |

## Soft delete and restore

Deleting an API key or an invite link in the web UI is undoable: the
frontend's toast says "API key deleted · Undo", and Ctrl+Z does the same
(`references/frontend.md`, "Undo and redo"). The server makes that true
rather than the screen faking it:

- **Delete only marks the row.** `APIKey` and `InviteLink` embed
  `models.SoftDelete` (`server/models/soft_delete.go`): `DeletedAt` and
  `DeletedBy`. From that moment the row is dead -- `LoadAPIKey` and
  `LoadInviteLink` refuse it, `Live()` is false -- and it is gone from every
  listing and from the live-key count. Deleting it again is
  `db.ErrNotFound`.
- **Restore undoes it, for `RestoreWindow` (10 minutes).**
  `POST /api/keys/:key_id/restore`, `/api/admin/keys/:key_id/restore` and
  `/api/admin/invite-links/:link_id/restore` clear the mark, and the key or
  link works again exactly as before: same secret, same expiry, same uses.
  Past the window it is 409; a row that is not deleted is 404.
- **Who may restore:** whoever deleted it, or an admin. A key an admin
  deleted is the admin's to bring back, not its owner's (403).
- **Restoring a key keeps the live-key cap**, checked in the same
  transaction as the restore, like `IssueAPIKey` (409 when full).
- **The purge drops deletions past the window** on its regular pass.
  Restore checks the window itself, so a row the purge has not reached yet
  is still refused.
- **Revoke has no restore, on purpose.** Revoking is the switch for a key
  that may have leaked; an undo would quietly trust it again. Delete is
  tidying up, and tidying up the wrong row is the mistake an undo is for.
- **The CLI hard-deletes.** `manage delete-key`, directly or through the
  control socket, calls `PurgeAPIKey`: an operator at a shell has no Undo
  button, so a soft delete there would only leave the row lying around.

A new deletable record follows the same pattern: embed `SoftDelete`, delete
with `markDeleted` and restore with `unmarkDeleted` inside `UpdateValue`,
filter `Deleted()` out of its listing and loader, add a `/restore` route
answering through `restoreRefused`, and check `deletedLongAgo()` in its
purge. Tests: `server/models/soft_delete_test.go`.

Server-backed undo of an *edit* uses compare-and-set instead: the request
carries the value it expects to replace (`expect`), checked in the same
transaction, and a mismatch is 409 with the current value, so undoing a
rename never overwrites one made in another tab since (`models.RenameUser`).

## MCP: the API keys, for agents

`POST /mcp` speaks the Model Context Protocol, so an agent -- Claude Code, or
any MCP client that can send a header -- can use the app with an API key:

```bash
claude mcp add --transport http myapp https://myapp.example/mcp \
  --header "Authorization: Bearer <key>"
```

It is the API keys in another shape, not a new way in:

- **Keys only.** `Guard.RequireAPIKey` is `RequireSession` turned around: a
  browser session gets 403, no credential gets 401 with
  `WWW-Authenticate: Bearer`. With no ambient cookie accepted there is nothing
  for another page to ride on, so there is no CSRF token to ask for -- and a
  cross-origin `Origin` header is refused anyway, which is the spec's DNS
  rebinding rule.
- **The key's role, per tool.** Each tool is plain or `AdminOnly`, read
  against the actor's *effective* role. `tools/list` leaves admin tools out
  for a user-scoped key, and `tools/call` answers "unknown tool" for one, so
  the key is never even told they exist.
- **No credentials, no deleting.** The built-in tools are the safe half of the
  REST API: `whoami`, `list_team`, and for admins listing users and invite
  links, disabling/enabling an account, revoking an account's access, and
  switching off an invite link. Creating accounts, login links, invite links,
  PINs or keys, and every delete, need a browser session on the REST side for
  the reasons in [Why a key cannot create credentials](#why-a-key-cannot-create-credentials);
  a tool is a key-authenticated route, so the same rule keeps them out.
- **One set of rules.** A tool calls the helper its HTTP handler calls
  (`meRow`, `userRows`, `setUserDisabled`, `revokeAccess`, ...), so "you
  cannot disable yourself" and the owner protection hold through both doors
  and refuse in the same words.

The transport is the simplest legal form of Streamable HTTP: stateless, one
JSON-RPC message per POST, a JSON reply. No SSE stream, no `Mcp-Session-Id`,
no batches; `GET`/`DELETE /mcp` answer 405. A tool call is then an ordinary
request -- authenticate, check, act, answer -- with no per-client state to
hold or leak between keys. It is written by hand (`server/mcp/`, about 300
lines) rather than with an SDK, because four methods do not justify a
dependency and its net/http adaptor.

What it is not: an OAuth authorization server. claude.ai's remote connectors
and other clients that can only sign in through OAuth cannot use it; anything
that can set a header can. That follows from the template's position on
OAuth -- see "When this template does not fit" in SKILL.md.

`server.mcp: false`, or `auth.api_keys: false`, means the route is never
registered.

| Concern | File |
|---|---|
| JSON-RPC, version negotiation, the transport rules | `server/mcp/mcp.go` |
| `Tool`, argument binding, `Refuse`, role filtering | `server/mcp/registry.go` |
| The built-in tools | `server/routes/mcp_tools.go` |
| The guard | `Guard.RequireAPIKey` in `server/security/security.go` |
| End-to-end tests | `server/routes/mcp_test.go` |

## The activity log

Everything that happens is written down, and only the owner can read it
(`server/logs`, `middleware/request_log.go`, `routes/logs.go`,
`static/js/track.js`, `static/js/logs.js`).

**What goes in.** Each line is one entry with a `kind`:

| kind | written by | what it says |
|---|---|---|
| `request` | `middleware.RequestLog`, every request | method, path, status, ms, bytes, referer |
| `error` | the same, for a 5xx | the handler's error text (the client only ever sees "internal error") |
| `auth` | `handlers.audit`/`auditAs` | signed in, link refused, joined through an invite, wrong PIN, password refused, wrong code, 2FA on or off, signed out, PIN or password changed |
| `admin` | `handlers.audit` | account, key and invite-link changes, log days deleted, prunes |
| `ui` | `POST /api/logs/events` from `track.js` | screens, clicks, form submits, undo/redo, JS errors, failed calls, device details |
| `system` | the server | started, stopped, pruned, entries dropped |

Every entry also carries the actor (account id, name, role, and whether it
came in by session, API key -- by its public id -- or anonymously), the
client IP (through the same `TrustProxy` rules as the rate limiter), and the
user agent. A `ui` entry's time is the server's; the browser's own clock is
kept beside it as `ui.at`, so a page cannot backdate what it reports.

**What never goes in.** Request bodies, cookies, `Authorization`, the secret
half of anything, and anything typed. `logs.Redact` turns `/login/<cred>` and
`/join/<cred>` into `/login/…` and masks `token`, `key`, `secret`, `code`,
`credential`, `csrf_token`, `pin`, `password`, `password_digest`, `challenge`
and `recovery_code` query values; the referer
goes through the same. `track.js` sends a form's field *names*, a control's
`data-i18n` key or id, and only `#hash` hrefs. A part of the page marked
`data-no-track` (the Logs screen itself) is skipped.

**Where it lives.** Files, not bolt: `<app dir>/logs/YYYY-MM-DD.jsonl`, with
`.1.jsonl`, `.2.jsonl` ... once a day passes `logs.max_file_mb`, and
`.jsonl.gz` once compressed. A log is append-only, grows all day and is read
a day at a time; as bolt records it would make retention a scan of every
record and double the database file for something that is never joined
against app data. With `server.encrypt_at_rest` each line is the entry's
JSON sealed with the server key (XChaCha20-Poly1305, `encryption.ChaCha*`)
and base64'd, so the logs directory is as unreadable as `app.db` without
`config.yaml`. The reader takes either form line by line, so flipping the
setting never strands old days.

**How it is written.** `Logger.Log` puts the entry on a 10,000-slot channel
and returns; one goroutine writes, flushing every second or 200 lines. A
full channel drops the entry and counts it, and the writer logs the count as
a `system` warning once it catches up. A slow disk can lose log lines; it
can never slow a request. The writer, the readers and the pruner share one
mutex, so a file is never compressed or deleted mid-write.

**Retention**, from `logs:` in config.yaml, applied hourly by the `prune-logs`
job and on "Prune now": finished days older than `compress_after_days` are
gzipped (to a temporary name, then renamed, so a crash leaves the original);
days older than `retention_days` are deleted; then, while the total is over
`max_total_mb`, the oldest day goes. Today is never touched, and
`POST /api/owner/logs/days/<today>/delete` is a 409.

**Who reads it.** `Guard.RequireOwner`: the owner, from a browser session.
Another admin or any API key gets the 404 an unknown route gets -- the log
holds everyone's addresses and activity, including the other admins', which
is the reason they cannot read it, and a key is a credential that lives in
files. Signed out, the `/api` group's `RequireLogin` answers first with 401,
as it does for every `/api` route. `manage logs` reads the same files from a
shell; they are append-only, so that is safe with the server running.

**Search** is `logs.ParseQuery`: free words match the entry's JSON in any
case; `kind:` `level:` `user:` `status:` (`5xx` for a class) `method:`
`path:` (prefix) `ip:` (prefix) `action:` `screen:` narrow; a comma is OR,
a leading `-` is NOT, quotes make a phrase. It is a linear pass over the
day's lines, decrypting each -- see Known limits.

## Background jobs

Anything that has to happen on a schedule is a job in `server/jobs`, not a
goroutine of its own. A job is a name, an interval and a function:

```go
registry.Register( "send-digest" , 24 * time.Hour , func() ( note string , err error ) {
	sent , err := digest.Send( store )
	note = fmt.Sprintf( "sent %d" , sent )
	return
} )
```

Register it in `main.go`, beside `jobs.Housekeeping`, before `registry.Start()`.
The registry then gives it everything below, and a row on the owner's Jobs
screen (`GET /api/owner/jobs`), with no other change. The screen's "Run now"
is `POST /api/owner/jobs/:name/run`; it answers 202 as the run starts, because a
backup can outlast a request, and 409 if the job is already running.

- **Due times come from the last start, not from when the process began.** The
  record is in the `jobs` bucket. A job never run, or overdue, runs right after
  boot, so a server restarted more often than a job's interval still runs it,
  and one that ran a minute before a restart does not run twice.
- **The start is written before the job runs.** A job that kills the process
  has used its turn, so it cannot crash the server on every boot. It shows as
  Interrupted.
- **`note` says what a run did.** "not due" and "wrote a backup" both end with
  no error, and the owner needs to tell them apart. Return a note and an error
  together for a run that did part of its work.
- **Runs never overlap**, whichever of the schedule and the owner asked first.
  **A panic** becomes that run's error. A failure is printed and logged as a
  `system` entry.
- **A job has to be safe to lose at any moment.** Nothing is cancelled: shutdown
  waits up to five seconds for a run, then leaves it, because the process can be
  killed at any time anyway.

`Housekeeping` registers the three every app has: `purge` (expired sessions,
login links and challenges, dead keys and invite links), `prune-logs` (only
while the activity log is on) and `backup` (checks hourly;
`backups.interval_hours` decides whether a check makes one). Each purge step
runs whatever happens to the one before.

Jobs live in this process, so they stop with it, and there is no queue: a run
missed while the server was down happens once, on the next boot.

## Sessions and devices

The Account screen lists where the account is signed in, from
`GET /api/account/sessions`, and can end any session but this one
(`POST /api/account/sessions/:session_id/revoke`) or every other
(`POST /api/account/sessions/revoke-others`). All three need a browser session,
so a leaked key cannot lock its owner out of the screen that revokes it.

- **A session records its sign-in address and browser**, written by
  `signInResponse` just after the session is made, since every way of signing in
  ends there and the transaction that makes it has no request to read. They are
  best effort, and optional, so a session from before they existed still loads.
  The address is the one from sign-in; it is not updated.
- **Last seen** is written at most every 15 minutes, by `Guard.resolveSession`,
  for the same reason as an API key's last use. It is not in `LoadSession`: a
  stream re-checking its session is not the person being there.
- **Only your own.** Someone else's id and an id that never existed get the same
  404, so the answer does not say which exist. An admin's way to end another
  account's sessions is `revoke-access`.
- **This device is refused** with a 400, because ending it here would leave the
  browser holding a dead cookie. Log out does that properly.
- **Never undoable.** It is the button for a lost phone, so it asks first and
  stays done, like revoking a key.

The list names only fields it means to show (`sessionRow`), so the secret's hash
and the CSRF token cannot reach a response through a change to the model.

## Monitoring

**`/api/health` and `/api/ready` answer different questions.** Health is what
the container's healthcheck reads, and a failure gets the container restarted,
so it touches nothing: a slow query must not turn into an outage. `/api/ready`
is for a load balancer or a person deciding whether to send traffic. It checks
that the database answers, is at this build's schema version, and can be read
with this key, which reading the owner record proves. It answers 200 or 503,
says nothing more because it is public, and gives up after two seconds. Callers
share one probe, so a slow database does not get a pile of reads on top. It uses
the general rate budget and is not logged.

**Request IDs.** Every request gets 16 hex characters from
`middleware.RequestID`, the outermost middleware, sent back as `X-Request-ID`
and written as `request_id` on its log entries. One sent by the caller is
ignored: it would let anyone choose what appears in the log. A 5xx puts it in
the body as well, and the page shows it, so what someone reads out goes straight
into the Logs screen's search. With logs off, the same id is on the `[error]`
line on stdout.

That only helps if the line says what went wrong, so `serverError( c , cause )`
returns the cause instead of writing a response. `ErrorHandler`
(`server/middleware/errors.go`) answers with the generic message and the id,
and `RequestLog` records the cause. Pass the real error at every call: the cause
goes to the log and never to the client.

**`/metrics`** is Prometheus's text format, printed by `server/metrics` with no
client library. It is off until `metrics.enabled` is set, and `auth.api_keys`
has to be on; otherwise it answers 404. It takes an admin API key as a bearer
token: a browser session and a user-scoped key get 403. That means the scraper
holds an admin-level credential, so keep it where you would keep any other, and
give it its own key so it can be revoked alone.

| metric | type | labels |
|---|---|---|
| `app_http_requests_total` | counter | `method`, `status_class` |
| `app_http_request_duration_seconds` | histogram | none |
| `app_http_requests_in_flight` | gauge | none |
| `app_uptime_seconds` | gauge | none |
| `app_logs_dropped_total` | counter | none |
| `app_job_last_success_timestamp_seconds` | gauge | `job` |
| `app_job_last_failure_timestamp_seconds` | gauge | `job` |

The prefix is the same for every app, so one dashboard fits them all; tell them
apart by the scrape's job label. Nothing is labelled by path, because a path has
as many values as anyone cares to request, and a scraper keeps a series for each
forever. Keep that rule for any metric you add. The counters live in memory and
restart from zero with the process, which Prometheus expects.

## Live updates

`server/stream` pushes server-sent events to a page: an `EventSource` there, a
`stream.Hub` here. The Logs screen's Live button is the worked example.

```go
// main.go: one hub per topic, closed on shutdown beside log_stream.
orders := stream.NewHub( 32 )

// A route, behind whatever guards it needs.
func ( handlers *Handlers ) StreamOrders( c fiber.Ctx ) ( err error ) {
	credential , cookie_err := handlers.Guard.DecryptCookie( c.Cookies( handlers.cfg().CookieName ) )
	if cookie_err != nil {
		err = notFound( c , "not found" )
		return
	}
	// Asked on every heartbeat. It runs after the request has ended, so it
	// uses what was copied above and never c.
	err = stream.Serve( c , handlers.Orders , func() bool {
		_ , user := models.LoadSession( handlers.Store , credential )
		return user != nil
	} )
	return
}

// Wherever something happens. Never blocks.
handlers.Orders.Publish( "changed" , "1" )
```

A stream is unlike a normal request in four ways, all handled in `Serve`:

- **The server's `WriteTimeout` is one deadline for the whole response**, so each
  write sets its own instead, keeping the protection against a client that has
  stopped reading. The 15-second heartbeat has to stay under `WriteTimeout`,
  which still applies until the first heartbeat replaces it. Writes pass through
  a small buffer, so a stalled client can hold its slot for about a hundred
  seconds rather than thirty; the hub's limit is what bounds that.
- **Middleware runs once, at connect.** The check you pass runs on every
  heartbeat and ends the stream when it says no, so a revoked session stops
  within 15 seconds. It runs after the request has ended: copy what it needs
  first, and never use `c` in it. `Guard.StillOwner` is the Logs screen's.
- **The writer has its own goroutine**, where a panic would end the process, so
  it recovers.
- **A stream never finishes by itself**, so `Hub.Close` has to end it on
  shutdown. Forget that and every open page costs the whole drain timeout.

**Send a nudge, not the data, if you can.** The Logs stream says only "look
again", and the page reads through the same route as always, which is what keeps
search, paging and decryption in one place, and means a dropped event loses
nothing. Each client's queue holds 16 events and drops the rest, so an event that
must arrive needs another way to be caught up.

The first event is `changed`, which covers anything missed between the page's
last read and connecting. A hub lives in this process, so it reaches nobody
connected to another. Over its limit a client gets 503. `EventSource` cannot send
an `Authorization` header, so a stream is for browser sessions, and on HTTP/1.1 a
browser allows about six connections per server, so close a stream when its
screen is not showing, as `logs.js` does.

## Why credentials are shaped `<id>.<secret>`

Login tokens, sessions, API keys, and invite links all use `<id>.<secret>`,
and the reason is worth understanding before changing it.

Storing only a hash of a credential is standard -- it means a stolen database
cannot be replayed. But a *salted* hash like bcrypt cannot be looked up:
you would have to fetch every candidate row and bcrypt-compare each one,
which hands anyone a trivial way to pin the CPU with garbage tokens.

Splitting the credential fixes that. The id is the bolt key, so lookup is one
O(1) fetch, and exactly one hash comparison follows.

The two halves then use different hashes on purpose:

- **Login tokens use bcrypt.** They are redeemed at most once, so bcrypt's
  deliberate slowness costs a single request and buys real resistance.
- **Sessions, API keys, and invite links use sha256.** Sessions and keys
  are checked on every authenticated request, and an invite link's secret is
  24 random bytes, not a person's choice. Against a long random secret there
  is nothing to brute force, so a fast hash is correct -- bcrypt here would
  just be a self-inflicted latency tax.

Comparisons go through `encryption.ConstantTimeEqual` so the check does not
leak, via timing, how many leading characters matched.

## Which crypto is used where, and why

Everything lives in `server/encryption/encryption.go`. Nothing else in the
project imports a crypto primitive directly -- that is what makes "how does
this app handle secrets" a one-file question.

| Primitive | Used for |
|---|---|
| `crypto/rand` | Every credential, key, and nonce |
| `bcrypt` | Hashing one-time login secrets, PINs, and password digests |
| `crypto/hmac` + `crypto/sha1` | TOTP codes (RFC 6238) |
| `chacha20poly1305` (XChaCha) | Session cookie payloads, values at rest |
| `sha256` + `crypto/subtle` | Session, API key, and invite link secret lookup, constant-time compare |
| `nacl/secretbox` | Available for authenticated blobs; unused by the base template |
| `curve25519` | Available for key exchange; unused by the base template |
| `kyber-k2so` | Available for post-quantum KEM; unused by the base template |

The last three ship ready but unwired, and that is deliberate rather than an
oversight. The login flow needs no key exchange at all -- a session cookie is
an opaque database reference. They are there for the features apps built on
this template do grow: handing a browser or a peer service a public key, or
end-to-end encrypting message payloads. Pairing Kyber with X25519 gives the
usual hybrid, where an attacker recording traffic now against a future quantum
computer still has to break both.

Two deliberate differences from the reference implementation the API mirrors:

- **Random bytes come straight from `crypto/rand`.** A hand-rolled PRNG layered
  over a CSPRNG cannot be stronger than the CSPRNG, only harder to review.
- **Anything on an auth path returns an error rather than swallowing it.** A
  decrypt that quietly returns `""` turns a tampered cookie into an
  empty-but-successful value, which is how a subtle bug becomes an auth bypass.

Cookie encryption is not what makes a session unforgeable -- the database
lookup is. It means the cookie is opaque to anyone reading a browser profile
or a proxy log, and that a tampered cookie fails to decrypt before it ever
reaches the lookup. It does **not** stop replay: a copied cookie is still a
working bearer credential until the session ends. What ends it is logging
out, the session expiring, the account being disabled, or revoke access --
each deletes the server-side row the cookie points at.

## At-rest encryption and the secret key tradeoff

With `server.encrypt_at_rest: true` (the default), every value written to
bolt is XChaCha-encrypted with the key in `secret.key`, beside `config.yaml`.
`server/db/db.go` does this in `encode`/`decode`, so models never think about
it and no future model can forget to.

**The cost is real and worth stating plainly to the user: lose the key and the
database is unreadable.** There is no recovery.

The key lives in `secret.key`, never in `config.yaml` -- which is what lets
`config.yaml` be edited while the server runs, and be writable inside the
container, without either path touching the key. Where `secret.key` sits
relative to the database depends on how the app is run, and the two cases
trade differently -- `configuration.md` covers this in full, but the short
version:

- **Container**: `config/secret.key` stays in the project directory and is
  mounted read-only, separately from the data volume, so a leaked `./data`
  backup is not enough to read it.
- **Portable binary**: on first run it writes `config.yaml` and `secret.key`
  into `~/.config/<slug>/` beside `app.db`. That is what lets a single file
  just run -- at the cost that at-rest encryption then protects a database
  file that leaks *on its own*, not someone who takes the whole directory.

`config.Load` refuses to start with a malformed key, or a missing one once a
database exists, rather than continuing or generating a new one, because the
alternative failure mode only shows up on the next restart, when every record
has already become unreadable. An older `config.yaml` with `server.secret_key`
in it has the key moved into `secret.key` (written and read back first, the
line removed second), and two different keys stop it cold.

Turn it off (`server.encrypt_at_rest: false`) when operational simplicity beats
at-rest confidentiality -- the data is not sensitive, or disk encryption
already covers it.

## Default permission level for new features

Only two things are admin-gated: managing accounts and minting login links.
One thing is owner-only: reading the activity log (see above).

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

## Why auth is middleware, not a helper call

`RequireLogin` and `RequireAdmin` are Fiber middleware, applied to route
groups in `server/routes/routes.go`, rather than helpers called at the top of
each handler.

A handler that forgets a helper call is silently public, and nothing about
reading that handler tells you it is wrong. With groups, protection comes from
where a route is registered -- visible in one place, in the URL map itself.
`LoadUser` runs on everything and resolves the cookie at most once, so a
handler and a middleware both asking "who is this" do not cost two reads.

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

- `server/static` serves whatever is in `static/`, unmodified except for gzip.
- `server/routes` exposes the minimum JSON API to make the auth model usable
  from a browser. How the admin panel looks lives in `static/js/`.

When adding features, put new server logic in new files under `server/routes/`
(one concern per file) and keep rendering in `static/`. The layout exists to
prevent a monolith; adding to the wrong file is how you get one anyway.

## Static asset caching policy

`server/static/static.go` implements one rule:

- **Text assets** (`.html`, `.css`, `.js`, `.json`, `.svg`, ...) are read off
  disk and gzipped on every request with `Cache-Control: no-store`. A deploy
  is live on the next refresh -- no content hashing, no `?v=123`, no
  cache-busting build step to keep in sync. Gzip is what makes that
  affordable: the bytes on the wire are a fraction of the file.
- **Images and binaries** get `max-age` of a week and no gzip. Gzip does
  nothing for already-compressed formats, and nobody wants a photo
  re-downloaded on every view.

Path traversal is stopped two ways: any request containing a `..` segment is
rejected outright, and the resolved path is then confirmed to still be under
the root. The second check is the real guarantee -- it verifies where the path
landed rather than trying to blocklist how it got there. The first exists so
probes answer 404 instead of quietly hitting the index fallback, which is
clearer in an access log.

A missing path *with* an extension is a 404. A missing path *without* one
falls back to `index.html` so client-side routing works. Serving HTML for a
missing `.js` would leave the browser reporting a confusing MIME error instead
of the real problem.

If this ever costs too much CPU (very large JS, very high traffic), the fix is
smaller static output, not caching the text assets -- that would reintroduce
exactly the staleness this avoids.

## Where state lives

Everything the app persists -- database, uploaded files and the activity
log (`logs/`) -- lives in one directory, `~/.config/<slug>/` by default and `/app/data` in a container.
Everything it can be told -- server, Docker, and build settings -- lives in
one `config.yaml`, with the key that encrypts the database in `secret.key`
beside it, and nothing reads environment variables.

That convention has its own reference: see `configuration.md`. Read it before
adding a setting or writing a file anywhere, because the two rules it exists
to protect (one backup unit, one file that holds every setting) are easy to
break by accident.

## Changing a stored record: migrations

Records are JSON, so most changes need nothing. A new optional field reads as
its zero value from older records, and a new bucket is created at startup. A
migration is for the rest: a field renamed or split, a value given a new
meaning, records moved between buckets.

The database holds a schema version: how many of the steps in `migrations`
(`server/db/migrate.go`) have run on it. It lives in its own `schema` bucket,
unencrypted, so a backup can report it without the key. `db.Open` runs whatever
is missing, in order, before anything else sees the data. That covers the
server starting and `manage` opening the file with the server stopped, so bolt's
lock means nothing runs under a live server.

```go
var migrations = []Migration{
	{
		Name: "users: split name into first and last",
		Up: func( tx *Tx ) ( err error ) {
			err = tx.EachRecord( BucketUsers , func( key []byte , record map[string]any ) ( changed bool , each_err error ) {
				first , last , _ := strings.Cut( record[ "name" ].( string ) , " " )
				delete( record , "name" )
				record[ "first" ] , record[ "last" ] = first , last
				changed = true
				return
			} )
			return
		},
	},
}
```

`EachRecord` gives each record as generic JSON, because a migration has to keep
working after the Go struct has moved on: never use a model's struct in one.
`tx` also has the usual `Get`, `Put`, `Delete` and `ForEach`.

The rules:

- **Add to the end.** Never edit, reorder or remove a step that has shipped: a
  database's version is a position in this list.
- **A new database runs none of them.** It is stamped with the latest version,
  so the structs must already describe the newest shape.
- **An existing database is copied first**, to
  `app.db.before-migration-v<N>-<time>`, to undo a migration that turns out
  wrong. Delete it once you are happy.
- **Each step is one transaction with its version bump.** One that fails leaves
  the database as the last one left it, and the server refuses to start and
  names it. Fix the step and start again.
- **Keep steps quick.** bolt has one writer, and nothing else runs until they
  finish, so a long migration is a long startup.
- **A database newer than the build is refused**, rather than read by code that
  would not know what the new shape means. So is a backup from one. To go back
  to an older build, restore the `before-migration` copy.

## Live configuration

`config.yaml` changes under a running server -- the owner's Settings screen,
a hand edit, `config set` -- and every one of those goes through one function,
`config.Holder.Apply`: parse the whole file over the defaults, validate it,
then swap in a new `Config` or change nothing. The watcher that notices hand
edits (`Holder.Poll`, every two seconds) applies a change only once two polls
agree on the content, and an invalid file is reported and ignored, never half
applied.

The rule that makes a change *live* is about readers, not writers: anything
that reads a setting per request reads `Holder.Get()` once per request and
uses that copy throughout, never a `Config` captured at startup. That covers
the guard (cookie lifetime and the Secure flag, whether API keys are
believed), the handlers, the rate limiter, the security headers, `/mcp`
(always routed, answering 404 while switched off), the control socket's
operations, and the activity log's settings. What is built once -- the
listener, the database, the control socket, fiber's proxy and body-limit
config -- reads `Holder.Boot()`, and its keys are tagged `[restart]` in
`defaults.yaml`. `liveConfig` copies the boot `Config` and overwrites only the
`[live]` fields; a test fails if a tag and `liveConfig` disagree.

The Settings screen is the owner's alone, like the activity log, and for the
same reason: the file decides who can sign in how (`auth.api_keys`, cookie
security, proxy trust), so it is the most privileged thing in the app. The
secret key is never part of it -- not in the file, not in the schema the
screen is built from. Inside the container, `docker:` and `build:` are
read-only there: they decide what `docker/run.sh` mounts and hands to the
container's user on the host, so a compromised container must not choose
them. Every save is in the activity log with each key's old and new value.

## Where assets come from: disk or embedded

`server/assets.Resolve` decides, at startup, whether the frontend is read from
disk or from the copy compiled into the binary by `main.go`'s `go:embed` lines. Disk wins when
it is present; embedded is the fallback. It logs which it chose:

```
[assets] static=disk /srv/app/static language=embedded
```

That one line answers "why isn't my edit showing up" faster than any amount of
debugging, which is why it is printed on every start.

The rule is *disk wins* rather than a flag or a build tag, because the two
situations it distinguishes are never ambiguous in practice:

- A source checkout has `./static` next to the binary, and someone is editing
  it. Reading from disk is what makes the no-store serving policy useful --
  edit, refresh, done.
- A binary copied somewhere on its own has no `./static`, so it uses what is
  inside it. This is what `build.sh` produces.

A useful third case falls out of treating the static tree and the language
file independently: dropping a `language.yaml` next to a shipped binary
rewords the UI without rebuilding anything, while the frontend stays
embedded. That is the supported way to translate or soften a deployment's copy
without a Go toolchain.

The disk case opens `./static` with `os.OpenRoot` and serves `root.FS()`
rather than raw file paths, so both sources are the same `fs.FS` interface and
`server/static` has exactly one code path. `fs.ValidPath` refuses `..`
elements for free, independently of the explicit check in the handler.

It is `os.Root` and not `os.DirFS` on purpose. `os.DirFS` follows symlinks
wherever they point -- its documentation says so -- so a link in `static/` to
`../config/secret.key` would serve the secret key to any visitor. An `os.Root`
refuses to resolve anything, symlinks included, that lands outside the
directory. Two consequences: a symlink that stays inside `static/` still
works, and the root holds the directory open for the life of the process, so
replacing `static/` wholesale (as opposed to the files in it) is only seen
after a restart. Static content should still be writable only by whoever
deploys the app -- the confinement is a backstop, not a licence to let
untrusted users write there.

### The single binary is also the CLI

`main.go` dispatches `manage`, `config`, `health` and `version` subcommands
before starting the server. The commands themselves live in `server/manage`,
and the container image runs the same binary (`docker exec <name> /app/server
manage ...`) -- one implementation, one entry point, no drift.

This matters more than it looks: a portable binary that could not mint a login
link would strand anyone who lost their session on a machine with no Go
toolchain and no container runtime. `myapp manage reissue-login` is what makes
"one file that just runs" actually self-sufficient.

## The control socket

bolt hands its file to exactly one process. That is a real constraint, and the
template's answer is to lean on it rather than work around it: the server owns
the database, and every other process asks the server to act on its behalf
over a unix socket.

    ~/.config/myapp/control.sock     mode 0600, created by the running server
    /tmp/control/control.sock        the same, inside the container image

`manage` takes whichever route is available -- the socket when a server is
running, the database file when one is not -- so the same command works on a
live deployment and on a fresh install with nothing started yet. It prints
which route it used on stderr. Accounts and API keys both go through it, which
is what lets a deploy script mint a key for a CI system before anyone has
signed in.

### Why there is no token on the socket

There is deliberately no password, token, or handshake. Outside the container
the socket lives in the app directory, which is mode 0700, next to `app.db`,
and only the user that owns `config.yaml` (mode 0600, holding the key) can reach it. Any
process that can open the socket can already read the key and the database
directly, so a credential would guard nothing while adding one more secret to
store and rotate. The filesystem permission *is* the authentication.

This is also the reason the socket must never be moved somewhere
world-readable and is never bound to a TCP address. The argument depends
entirely on the socket being as hard to reach as the database file. A network listener would need a real credential, a second
copy of the auth checks, and an audit story -- which is the cost the socket
exists to avoid.

### Why the container's socket is on tmpfs

The container's data directory is normally shared from the host, and such a
mount may be unable to hold a socket: colima's refuses the bind with "operation
not permitted". The image therefore pins the socket to `/tmp/control` on the
container's tmpfs (`server/config/layout_container.go`). `docker exec` shares
that tmpfs, so `manage` finds the socket with nothing extra to type.

`/tmp` is open to every user, so the socket never sits in it directly.
`control.Listen` makes `/tmp/control` mode 0700 and refuses to start unless it
is a real directory, owned by this user, that nobody else can enter. That leaves
the container's own user and root, who can read the key and the database
already, so the no-token argument holds. It is also tighter than a socket in a
shared folder, because a tmpfs is private to its container and nothing outside
can reach it short of `docker exec`.

The cost is that a separate `docker run` gets a tmpfs of its own and cannot see
a running server. It falls back to opening the database directly, which is
right when the server is stopped. With the server up, bolt's lock refuses it
after 5 seconds, and `manage` says to use `docker exec` or stop the server.

### What this buys, in security terms

One process holds the secret key. One process applies the model layer's rules.
Disabling an account revokes its sessions and pending login links the same way
whether the change came from the admin UI, the CLI, or a cron job, because all
three run the same code in the same process. The alternative -- a second process opening the database
directly -- means a second copy of the key in memory, a second implementation
to keep in step, and no single place to audit.

### Adding an operation

Add the endpoint in `server/control/control.go` and the matching method in
`client.go`. If a CLI command needs it, add it to the `backend` interface in
`server/manage/manage.go`; the compiler then requires both the socket
implementation and the direct-database one, which is what stops the two routes
from drifting apart.

### Limits worth knowing

A unix socket path is capped by the kernel at 104 bytes on macOS and the BSDs,
108 on Linux. A deep `server.app_dir` overflows it, and the kernel's own error is a
bare "invalid argument", so `control.Listen` checks the length first and fails
with a message naming the limit and the fix. On Windows the socket works
(AF_UNIX is supported) but the 0600 chmod is a no-op and the directory ACL is
what protects it.

## Known limits

State these to the user rather than letting them be discovered in production:

- **One process per bolt file.** bolt takes an exclusive lock, so a second
  process cannot open the database while the server holds it. The control
  socket (below) is how everything else reaches the data; without it, or with
  `server.control_socket: false`, `manage` needs the server stopped. This is why
  `Open` sets a 5s timeout rather than hanging forever.
- **The control socket is a local door, not a remote one.** It is a unix
  socket, so it does not cross machines. Two servers on two hosts sharing one
  database is still not possible and is the point at which this template's
  storage choice has been outgrown.
- **HTTPS is not this server's job, and nothing works safely without it.**
  Login links, session cookies, and API keys are all bearer credentials;
  `secure_cookies` only tells the browser not to send the cookie over plain
  http, it encrypts nothing. Keep the backend's plain-http port reachable only
  by the TLS proxy (`server.host: 127.0.0.1`, `docker.bind_address:
  127.0.0.1`).
- **Rate limiting is per-process, in memory.** Correct for the single
  container `docker/run.sh` builds. With multiple replicas each holds its own
  counters, so the effective limit multiplies -- move to a shared
  `limiter.Config.Storage` at that point rather than assuming it still holds.
- **Live streams and metrics are per-process too.** A `stream.Hub` reaches only
  the clients of this process, and `/metrics` counts only its requests, so with
  replicas each needs its own scrape and an event needs a shared broker.
- **Losing the secret key loses the data** when at-rest encryption is on. It
  is in `secret.key`, which `manage backup` leaves out on purpose: keep a copy
  somewhere safe, apart from your backups.
- **An API key is a bearer credential.** Anything holding it is that account,
  at that key's role, until the key is revoked or expires -- there is no
  second factor and no origin binding. That is what a key is for, but it means
  the admin key listing is a real answer to "what could get in right now", and
  worth looking at rather than assuming.
- **Rate limiting has two budgets, and static files have none.** Each address
  gets `server.rate_limit_max` for most requests, and a separate
  `server.rate_limit_signin_max` for signing in and for the invite-link calls.
  Files the page loads are not counted, because `index.html` alone pulls in
  over twenty, all `no-store`. That leaves them unlimited here: a flood of them
  costs a read and a gzip each, with no database involved, so anything exposed
  to real hostility should have a limit on the proxy in front too. The sign-in
  budget is a backstop, and the per-account and per-address attempt counters
  are what actually stop guessing. Everyone behind one office network shares an
  address, so a group joining together can hit it: raise
  `rate_limit_signin_max` for that rather than the general budget.
- **Rate limiting counts by IP, not by key.** A key called from many hosts is
  not throttled as one caller, and several keys behind one NAT share a budget.
  The limiter runs before the request is authenticated, which is what keeps an
  unauthenticated flood cheap to refuse; keying it on the actor would mean
  doing the database lookup first.
- **No TLS.** The app expects a reverse proxy in front. See
  `docker-and-deployment.md`.
- **An invite link is a bearer credential for a group.** Anyone holding it
  can join until the places run out, and can sign in as any account it made
  that has no PIN. See [Invite links](#invite-links-one-link-for-a-group).
- **A password digest is a bearer credential on the wire.** HTTPS is what
  protects it. See [Passwords](#passwords-an-option-the-owner-switches-on).
- **Password sign-in scans users.** Every record is encrypted, so finding a
  username reads them all, once per attempt. Fine at this template's scale.
- **The attempt limits are in memory**, like the PIN lockout: a restart clears
  them, and with several processes each keeps its own count.
- **The PIN lockout is in memory**, like the rate limiter: a restart clears
  it, and with several processes each keeps its own count.
- **Log search is a scan.** Listing days reads every line once per file and
  caches the counts; a search, or a page of a day, reads the day's lines
  again, decrypting each. Fine for the thousands-to-low-millions of lines a
  small app makes; past that, ship the downloaded JSON lines to a real log
  store instead of growing this one.
- **The log can drop lines under load** rather than slow requests down (the
  count shows on the Logs screen), and a 429 from the rate limiter is not
  logged at all -- the limiter runs first, so a flood stays cheap.
- **The log records personal data** -- IP addresses, user agents, what each
  person clicked. Tell the people using the app, and keep `retention_days`
  no longer than needed.
- **`language.yaml` is public.** It is served to anyone at `/api/language`,
  signed in or not, because signed-out pages need text too. Never put
  anything sensitive in it.
- **A portable binary has no sandbox.** `build.sh` output is a plain process
  with the user's own permissions -- none of Docker's read-only filesystem,
  dropped capabilities, or pid limits apply. That is fine for an internal box
  or a demo, and not what should face the internet. Say this rather than
  letting someone assume the binary is as protected as the container.
