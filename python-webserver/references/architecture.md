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
- [Sessions: seeing and ending your own](#sessions-seeing-and-ending-your-own)
- [Monitoring: request IDs, readiness, metrics](#monitoring-request-ids-readiness-metrics)
- [Live updates](#live-updates)
- [Paging lists](#paging-lists)
- [Error pages](#error-pages)
- [Maintenance mode](#maintenance-mode)
- [The API reference](#the-api-reference)
- [Webhooks](#webhooks)
- [Stopping cleanly](#stopping-cleanly)
- [File uploads](#file-uploads)
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
- [Changing a stored record: migrations](#changing-a-stored-record-migrations)
- [Live configuration](#live-configuration)
- [Where assets come from: configured or bundled](#where-assets-come-from-configured-or-bundled)
- [One entry point](#one-entry-point)
- [Known limits](#known-limits)

## Auth model: links first

Links are the way in. A password is optional, off by default, and only ever
set by someone who already signed in with a link (see
[Passwords](#passwords-an-option-the-owner-switches-on)). With passwords off,
identity is proven exactly two ways:

1. **Redeeming a login token** (`app/models/login_tokens.py`). An
   admin -- or the first-run bootstrap -- mints a high-entropy credential for
   a specific user. Only a bcrypt hash of its secret half is stored. By
   default it works once and expires after `auth.login_token_ttl_seconds`;
   whoever makes it can instead let it work up to N times (1-1000) or any
   number of times, always for that one account, and pick its expiry
   (30 minutes to 90 days in the UI, up to 365 days over the API, or never).
   See [Reusable login links](#reusable-login-links). An account has at most
   one live link: issuing a new one deletes the old. Opening `/login/<credential>` only serves the page; the
   page POSTs the credential to `/api/login` when the person confirms, so a
   chat app's link preview or a mail scanner cannot spend it, and someone
   already signed in is told they are about to switch accounts. The bcrypt
   comparison runs first, outside any transaction; then one write
   transaction re-checks the token still has a use left, checks the account
   is enabled, counts the use, and stores the session. Two requests racing
   for a link's last use cannot both succeed, and a link redeemed while the account is being
   disabled either lands first (and is swept up by the disable) or is
   refused.
2. **Presenting a valid session cookie** (`app/models/sessions.py`). The
   cookie holds an opaque reference, never the user id or role. Revoking a
   session is deleting a record -- no denylist, no token to outlive its
   welcome.

An **invite link** is the other way to get a session: one link a whole group
can join through, each person becoming a new `user`. It has its own section
below, because it is the one public way to create an account.

A third way exists for callers that are not people: an **API key** in an
`Authorization: Bearer` header, carrying a role from the same set users have.
It is a separate section below, because the interesting parts are the ceiling
(a key never outranks its owner) and the two rules that follow from a
credential not being ambient.

"Forgot password" is not a flow of its own. It is replaced by "an admin
mints a new link" (or `python -m app manage reissue-login` from a shell). That
is a real trade: less code, no reset-flow attack surface,
at the cost of needing an admin or shell access when someone loses their
session. A project needing genuine self-service recovery -- email round trip,
say -- has to add it; it is not part of the base model.

Anyone already signed in can also mint a fresh link for their own account
from the account screen ("Sign in on another device",
`POST /api/account/login-link`), which is how a second device gets in
without an admin. It is a reissue pointed at yourself, so it behaves exactly
like one. An API key may call it too (and the `create_my_login_link` tool), but
only a key that carries the account's full role: the link becomes a session at
the account's role, so an admin's user-scoped key would otherwise climb. See
[keys and the ways in they hand out](#keys-and-the-ways-in-they-hand-out).

Four operations look alike and are not:

| | live link | sessions | API keys | account |
|---|---|---|---|---|
| **Reissue login** (`New link`, `manage reissue-login`, or your own `Make a login link`) | replaced | kept | kept | kept |
| **Cancel link** (`POST /api/admin/users/{id}/cancel-login-link`, or your own) | deleted | kept | kept | kept |
| **Disable** (`manage set-disabled`) | deleted | deleted | refused while disabled, restored on re-enable | kept, disabled |
| **Revoke access** (`manage revoke-access`, `POST /api/admin/users/{id}/revoke-access`) | deleted | deleted | revoked | kept, enabled |
| **Delete** (`manage delete-user`, `POST /api/admin/users/{id}/delete`) | deleted, used ones too | deleted | records deleted | deleted, for good |

"Live link" means one that would still let someone in: unused, or reusable
with uses left, and unexpired. A reusable link that has been used is still
live, so all of these end it. Used-up and expired links are left for the
purge, so a late click says "already used" rather than "no such link".

None of these touch invite links. A link an admin made stops working
when that admin is disabled, deleted, or demoted, and the accounts a link
made are ordinary accounts that all four apply to.

Reissuing is for a lost cookie. When someone else may hold an account's
credentials, revoke access first, then reissue -- reissuing alone leaves a
stolen session signed in. Disable is for someone who may come back; delete is
for an account that should not exist at all. Disable, revoke, and delete each
run as one transaction with the account change (`app/models/credentials.py`),
so nothing issued concurrently survives them.

The first-run bootstrap prints the initial admin link to the server's own
logs, so a fresh deploy is usable without a separate setup step. Until that
link is used, anyone who can read those logs can claim the first admin
account -- and log access is often wider than deployment access (a log
shipper, a shared dashboard, a support engineer). Use the link promptly, or
keep the logs private until you have; it also expires after
`auth.login_token_ttl_seconds`. It only fires when no enabled admin exists, so
restarts do not keep minting admins.

## Reusable login links

A login link works once unless whoever makes it says otherwise. The create-user
form, the per-row **New link** dialog, **Sign in on another device**, `python -m
app manage create-login` / `reissue-login` (`--uses 5|unlimited --expires
30m|12h|7d|never`) and `scripts/login.sh` (`--uses`, `--expires`) all take the
same two choices:

- **Works:** once, up to N times (1-1000), or any number of times. Every use
  signs in the same account; this is not an invite link, which makes new ones.
- **Expires:** a preset, or never. Left at the default, a once link lasts
  `auth.login_token_ttl_seconds` (30 minutes) and any other
  `auth.multi_use_login_token_ttl_seconds` (7 days), both `[live]`.

Over the API the same choices are `max_uses` (1-1000, 0 for unlimited, left out
for once) and `expires_in_minutes` (0 for never, left out for the default) on
`POST /api/admin/users`, `/api/admin/users/{id}/reissue-login` and
`/api/account/login-link`; the answer adds `max_uses`, `unlimited` and
`expires_at` (null for never). `login_tokens.resolve_link` fills the defaults
and checks the limits for every door, the CLI included.

The account's live link shows on its row in Users (`login_link`: uses so far,
limit, expiry, last use) and on its own Account screen (`/api/me`), each with a
**Cancel link** button. An API key may cancel and make links too; a link a
key made dies with the key.

A reusable link is a standing credential, the way a password is: anyone who
holds it is that person until it runs out, expires or is cancelled. Prefer a
use count or an expiry over "any number of times, never", and cancel or
reissue as soon as it may have gone somewhere it should not. bcrypt is still
paid once per sign-in, never per request, and `/api/login` is on the sign-in
rate budget, so reuse does not open a cheap way to burn CPU.

Stored records gained `max_uses`, `unlimited` and `uses`; `used_at` became the
last use and a null `expires_at` means never. A record from before has none of
these and still reads as "once" (`used_at` set counts as one use), so no
migration is needed. The purge drops expired records, and a used-up record that
never expires 30 days after its last use.

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
credentials anyway. All three ways end in one function, `start_sign_in_tx` (`app/models/sign_in.py`), inside the
transaction that spent the first factor. For an account with 2FA it stores a
five-minute one-time challenge instead of a session and the page asks for a
code, so a way of signing in cannot forget to. The first factor is already
spent: a login link used here has lost that use even if the code never comes
(a one-time link is gone; a reusable one counts it).

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
`python -m app manage reset-2fa --user-id N` is the way back for anyone, the owner included. `revoke-access`
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
| **Anything else, as an admin** | yes | no | no | see below |
| **Anything else, as the owner** | yes | yes | -- | see below |
| **CLI** (the operator) | delete, disable, revoke | delete, disable, revoke | revoke only | -- |

- **One rule for acting on someone else's account,** in `users.may_act_on_tx` and checked
  inside each change's own transaction: nobody acts on the owner, and only the
  owner acts on another admin. "Anything else" is disabling and re-enabling,
  revoking access, reissuing or cancelling a login link, ending a session, and
  minting, revoking, deleting, restoring or changing the expiry of an API key
  that is not your own. A login link is a way in, so before this rule reached
  reissue, any admin could sign in as the owner. Acting on yourself goes by
  each route: you cannot disable, revoke or delete yourself, and your own
  login link and keys are yours. An app route that changes another account
  should call the same helper.
- **Renaming and roles follow the same table,** from the Users screen, an
  admin API key or the MCP tools. Only the owner renames an admin or takes an admin's rights
  away; nobody changes their own role, and the owner is always an admin. The one
  difference: any admin may make another, as any admin can create one.
- **Becoming an admin ends the account's sessions and live login link.** Whoever
  holds an invite link can sign in as an account that joined through it, so a
  session already open may not be the person promoted. Losing admin rights ends
  nothing: the role is read on every request. Admin keys narrow to the new role,
  and invite links the account made stop working. Both come back if the account
  is made an admin again.
- **A marker, not a role.** The owner is one id in the `meta` bucket
  (`owner_user_id`), not a third role beside admin and user. Roles are also
  what API keys carry and what `narrow_role` compares, so a third one would
  spread into key scoping, role validation, and every role check -- for
  something only a few account routes care about. Everywhere else the owner
  is an ordinary admin. `/api/me` reports `is_owner`; the admin user list
  marks the owner's row.
- **The invariant:** the owner is always an existing, enabled admin. Nothing
  deletes or disables the owner, not even the CLI, and ownership only moves
  to another enabled admin with `manage transfer-owner --user-id N`. There is
  no web route for that on purpose: a compromised admin session must not be
  able to promote itself past the other admins. Because an enabled admin
  always exists, the first-run bootstrap never fires again either.
- **The CLI may revoke the owner's access.** When it is the owner's own
  credentials that leaked, someone with shell access has to be able to clean
  up. The web route refuses.
- **Older databases** get an owner on the next boot (or the next `manage`
  command): the lowest-id enabled admin, which is the bootstrap admin on any
  database that started from first run. See `users.ensure_owner`.
- **An admin key may delete.** `POST /api/admin/users/{id}/delete` takes keys
  like disable and revoke access, under the same owner rules. It cannot be
  undone, so its MCP tool (`admin_delete_user`) is marked destructive and a
  client asks before calling it; disable is the reversible version.
- **Ids are never reused.** They come from the `sequences` table, so a
  deleted account's id cannot come back as a different person, and any stale
  reference to it stays a dangling id rather than a wrong one.
- **Records your app adds are not touched.** `credentials.delete_user`
  removes sessions, login links, API keys, and the user record. When a
  feature stores a `user_id` -- an assignee, an author -- decide in
  `delete_user` whether those records go with the account (delete them in the
  same transaction) or outlive it (render a missing user as `#<id>`).

The rules live in `app/models/credentials.py` (`delete_user`,
`set_user_disabled`, `revoke_all_credentials`), checked inside each one's own
transaction, so ownership cannot move between a check and the change it
guards.

## Invite links: one link for a group

A login link is for one named person. An **invite link** is for a group: an
admin makes one (`POST /api/admin/invite-links`, or `python -m app manage create-invite`), posts it in a group chat, and each
person who opens it joins on their own. The code is `app/models/invite_links.py`
(the record and the transactions), `app/models/pins.py` (PINs and name matching),
`app/routes/join.py` (the public join page's calls), `app/routes/invite_links.py` (the admin
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
- **Setting a PIN needs a browser session; managing links does not.** A PIN
  is a credential of your own, like a password. An admin key may create,
  rename, grow, switch on, delete and restore links; a link a key made, grew,
  switched on or restored is that key's, and revoking or deleting the key
  switches it off (see
  [keys and the ways in they hand out](#keys-and-the-ways-in-they-hand-out)).

The secret half is sha256-hashed, not bcrypt: it is 24 random bytes, not
something a person chose, so there is nothing to slow down.

What a leaked link costs: anyone holding it can join, up to the places left,
and can sign in as any account it created that has no PIN. That is the trade
for "one link in a group chat". Keep the places and the lifetime close to the
group's real size, switch a link off once everyone is in, and tell people to
set a PIN if the chat is large or public.

**A link can be edited after it is made** (`rename`, `limits` and `switch-on` under
`/api/admin/invite-links/:id`), and here it differs from an API key on purpose. An
expired link can be extended and a switched-off one switched back on, because a
link owns the accounts that joined through it and is how they sign back in on a
new device, so a replacement would lock them out. Switching back on asks first,
since the same link works again for anyone holding it. Places cannot drop below
the people already in. An admin key may make all three edits, and the link then
counts as that key's: revoking the key switches it off again. Neither edit helps a link whose maker has lost their admin rights.

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

### Keys and the ways in they hand out

`RequireSession` guards what a key must not reach even with a correctly scoped
admin key: creating, revoking and deleting keys, your own password, PIN and
two-factor, other people's files, and the owner's screens.

Accounts and sign-in are not on that list. An admin key may create and delete
accounts, change roles, reset two-factor, hand out login links and invite links
and sign devices out; any key may list and sign out its own account's devices
and make that account a login link. That last one needs a key carrying the
account's full role (`actor.role == actor.user.role`): the link becomes a
session, and a session acts with the account's role, so an admin's user-scoped
key would otherwise turn itself into an admin.

What keeps revoking a leaked key meaningful is provenance. Every login link and
invite link records the key that made it (`minted_by_key`, from
`actor.key_id`), and an invite link a key grows, switches on or restores
becomes that key's too. Revoking or deleting the key runs
`login_tokens.void_links_minted_by_key` in the same transaction: its login
links that could still sign someone in are deleted and its invite links are
switched off. Restoring a deleted key does not bring them back.

What that does not undo is a session already signed in through such a link,
or what that session did since -- a key it minted, say. Those show in the
account's devices list and the key list, and revoke access ends all of them at
once. That is the trade for letting agents hand out ways in; keys themselves
stay browser-only, so a key can never directly leave a successor behind.

### The rest of the model

- **Same `<id>.<secret>` shape and the same sha256 hash as a session**, for
  the same reasons -- see the next two sections. This runs on every request
  against a 256-bit random secret, so bcrypt would be a latency tax buying
  nothing.
- **Shown once.** Only the hash is stored. A lost key is revoked or deleted
  and replaced, never recovered.
- **Revoke or delete.** Both stop the key on its next request. Revoking keeps
  the record, marked dead, so the listing still answers "what existed last
  week"; the purge drops it after 30 days. Deleting removes it now. A key's
  owner can do either to their own keys, and an admin to anyone's
  (`/api/keys/{id}/delete`, `/api/admin/keys/{id}/delete`,
  `manage delete-key`). Deleting an account deletes all of its keys.
- **Expiry is bounded by default** (`auth.api_key_ttl_seconds`, 90 days) so a
  forgotten key stops being a live credential. A key with no expiry is
  available but has to be asked for: `expires_in_days: 0`, or
  `manage create-key --days 0`.
- **A key can be renamed or given a new expiry afterwards**
  (`/api/keys/:id/rename`, `/api/keys/:id/expiry`, and the same under
  `/api/admin/keys`). A name is only a label, so any admin can change one, on a
  dead key too. An expiry is stricter. It only moves on a live key: an edit must
  not bring back an expired or revoked one, so a new key is the way back. And
  changing someone else's follows the minting ladder: nobody touches the owner's
  keys, and only the owner touches another admin's. Both need a browser session,
  so a leaked key cannot extend itself.
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
  can mint keys, so the bucket needs a bound; revoking or deleting one frees a
  slot. The count and the insert share one transaction -- counted in one and
  inserted in another, sixteen concurrent requests at 24 keys all pass the
  check. Dead keys are kept 30 days for the listing, but at most 100 rows per
  account (`API_KEY_MAX_ROWS_PER_USER`): past that, minting a key drops the
  oldest dead ones, so mint-revoke churn cannot grow the bucket (or the scans
  over it) without bound.
- **`auth.api_keys: false` closes the whole surface.** The header stops being
  believed, existing keys stop authenticating, and the management endpoints
  404. The records survive, so turning it back on restores them.

### A key made for someone else

An admin can make a key that acts as another account, for setting up someone's
integration for them: `POST /api/admin/keys` with a `user_id`, or the form on
the All keys screen. A key made this way *is* that person to the server, so it
follows the ladder deleting an account does:

- any admin can make one for an ordinary user;
- only the owner can make one for another admin;
- nobody can make one for the owner over HTTP. A shell still can, with
  `manage create-key`.

The ordinary ceiling applies too: it cannot outrank the account. The route needs
a browser session, so a leaked admin key cannot mint itself a way in as somebody
else. The key records `created_by`, the person it acts as sees "Created by ..."
in their own list, and the audit entry and the `api_key.created` webhook both
carry it. A key from before this field, or one a person made themselves, has none.

### Where the code is

| Concern | File |
|---|---|
| The record, the ceiling, expiry, the per-user cap | `app/models/api_keys.py` |
| Header parsing, the `Actor`, `RequireSession`, CSRF exemption | `app/core/security.py` |
| Endpoints for minting, listing, revoking, deleting | `app/routes/api_keys.py` |
| Admin CLI (`list-keys`, `create-key`, `revoke-key`, `delete-key`) | `app/manage.py` |
| The screen | `static/js/keys.js` |

## Soft delete and restore

Deleting an API key or an invite link in the web UI is undoable: the
frontend's toast says "API key deleted · Undo", and Ctrl+Z does the same
(`references/frontend.md`, "Undo and redo"). The server makes that true
rather than the screen faking it:

- **Delete only marks the row.** `deleted_at` and `deleted_by` are set
  (`mark_deleted` in `app/models/__init__.py`). From that moment the row is
  dead -- `load_api_key` and `load_invite_link` refuse it, `live` is false --
  and it is gone from every listing and from the live-key count. Deleting
  it again is "no such key".
- **Restore undoes it, for `RESTORE_WINDOW` (10 minutes).**
  `POST /api/keys/{id}/restore`, `/api/admin/keys/{id}/restore` and
  `/api/admin/invite-links/{id}/restore` clear the mark, and the key or link
  works again exactly as before: same secret, same expiry, same uses. Past
  the window it is 409; a row that is not deleted is 404.
- **Who may restore:** whoever deleted it, or an admin. A key an admin
  deleted is the admin's to bring back, not its owner's (403).
- **Restoring a key keeps the live-key cap**, checked in the same
  transaction as the restore, like `issue_api_key` (409 when full).
- **The purge drops deletions past the window** on its hourly pass. Restore
  checks the window itself, so a row the purge has not reached yet is still
  refused.
- **Revoke has no restore, on purpose.** Revoking is the switch for a key
  that may have leaked; an undo would quietly trust it again. Delete is
  tidying up, and tidying up the wrong row is the mistake an undo is for.
- **The CLI hard-deletes** (`manage delete-key` calls `purge_api_key`): an
  operator at a shell has no Undo button, so a soft delete there would only
  leave the row lying around.

A new deletable record follows the same pattern: `deleted_at`/`deleted_by`
on the record, `mark_deleted` / `unmark_deleted` in update_value, a filter
in its listing and loader, a `/restore` route answering through
`restore_refused`, and `deleted_long_ago` in its purge. Tests:
`app/tests/test_soft_delete.py`.

Server-backed undo of an *edit* uses compare-and-set instead: the request
carries the value it expects to replace (`expect`), checked in the same
transaction, and a mismatch is 409 with the current value, so undoing a
rename never overwrites one made in another tab since (`users.rename_user`).

## MCP: the API keys, for agents

`POST /mcp` speaks the Model Context Protocol, so an agent -- Claude Code, or
any MCP client that can send a header -- can use the app with an API key:

```bash
claude mcp add --scope user --transport http myapp https://myapp.example/mcp \
  --header "Authorization: Bearer <key>"
```

The MCP card on Account (`static/js/mcp.js`) writes that command, or the
JSON Claude Desktop and other clients want, with a new key filled in.

It is the API keys in another shape, not a new way in:

- **Keys only.** `RequireAPIKey` is `RequireSession` turned around: a browser
  session gets 403, no credential gets 401 with `WWW-Authenticate: Bearer`.
  With no ambient cookie accepted there is nothing for another page to ride
  on, so there is no CSRF token to ask for -- and a cross-origin `Origin`
  header is refused anyway, which is the spec's DNS rebinding rule.
- **The key's role, per tool.** Each tool is plain or `admin_only`, read
  against the actor's *effective* role. `tools/list` leaves admin tools out
  for a user-scoped key, and `tools/call` answers "unknown tool" for one, so
  the key is never even told they exist.
- **Every key route, and nothing more.** Whatever a person can do on a
  screen, a key can do over HTTP and an agent through `/mcp` -- except managing
  API keys, your own password, PIN and two-factor, other people's files and the
  owner's screens. Those need a browser session on the REST side (see
  [Keys and the ways in they hand out](#keys-and-the-ways-in-they-hand-out)),
  and a tool is a key-authenticated route, so the same rule keeps them out.
  Accounts, login links, devices and invite links have tools
  (`app/routes/mcp_tools_accounts.py`).
  Everything else a key may call has a tool naming its route, or an
  `mcp_http` reason in its `doc()` for staying HTTP only (logout,
  the references themselves, `/metrics`, the browser's own activity reports,
  and file bytes). `app/tests/test_mcp_parity.py` holds the two lists together, and calls every route
  with an admin key to check the docs' `security` against the guards.
- **File bytes travel over HTTP.** An MCP message is JSON, and clients cap its
  size, so `files_start_upload` reserves the file and answers `chunk_url`, the
  piece size and a `curl` recipe; the agent POSTs each piece to
  `/api/files/<id>/chunks/<n>` with the same `Authorization` header, then calls
  `files_finish_upload`. `files_download_url` answers the address to GET. The
  same pattern fits any other binary an app adds.
- **One set of rules.** A tool calls the helper its HTTP handler calls
  (`me_row`, `user_rows`, `set_disabled_checked`, `create_file_checked`, ...), so "you cannot disable yourself", the owner protection and the
  quota hold through both doors and refuse in the same words.

The transport is the simplest legal form of Streamable HTTP: stateless, one
JSON-RPC message per POST, a JSON reply. No SSE stream, no `Mcp-Session-Id`,
no batches; `GET`/`DELETE /mcp` answer 405. A tool call is then an ordinary
request -- authenticate, check, act, answer -- with no per-client state to
hold or leak between keys. It is written by hand (`app/mcp/`, about 300
lines) rather than with the `mcp` SDK, which would bring pydantic, httpx and
an SSE library -- more native code to trust, and against the pinned
Alpine-wheels rule in app/requirements.txt -- for four methods.

What it is not: an OAuth authorization server. claude.ai's remote connectors
and other clients that can only sign in through OAuth cannot use it; anything
that can set a header can. That follows from the template's position on
OAuth -- see "When this template does not fit" in SKILL.md.

`server.mcp: false`, or `auth.api_keys: false`, means the route is never
registered.

### config/mcp.json, the MCP reference

`config/mcp.json` is what `tools/list` says for every tool and any key, plus the
route each one stands for and the `http_only` routes with their reasons. It is
generated, not written: `scripts/api_docs.sh` rebuilds it from the registered
tools, and a test fails while the committed copy is stale, so it can be linked
from GitHub (the MCP card's "MCP reference" link goes to `<app.repo>/blob/HEAD/config/mcp.json`,
or to `GET /api/mcp.json` when `app.repo` is empty) and still be right. It names
no features: scaffolding cuts a pruned feature's tools out by their route's
`x-feature` in `openapi.json`. See [The API reference](#the-api-reference). The Go and Python templates write
it byte for byte the same.

### Where the MCP code is

| Concern | File |
|---|---|
| JSON-RPC, version negotiation, the transport rules | `app/mcp/protocol.py` |
| `Tool`, `Call.bind`, `Refusal`, role filtering | `app/mcp/registry.py` |
| The built-in tools | `app/routes/mcp_tools.py` |
| config/mcp.json | `app/mcp/manifest.py`, `app/apidocs/generate.py`, `scripts/api_docs.sh` |
| The parity tests | `app/tests/test_mcp_parity.py` |
| The guard | `RequireAPIKey` in `app/core/security.py` |

## The activity log

Everything that happens is written down, and only the owner can read it
(`app/core/logs.py`, `app/core/request_log.py`, `app/routes/logs.py`,
`static/js/track.js`, `static/js/logs.js`). The day files, line format,
search grammar and JSON API are the same as the Go template's, so the
frontend is shared.

**What goes in.** Each line is one entry with a `kind`:

| kind | written by | what it says |
|---|---|---|
| `request` | `RequestLog` middleware, every request | method, path, status, ms, bytes, referer |
| `error` | the same, for a 5xx or an exception | the exception's repr, or the cause a handler gave `server_error` (the client only ever sees "internal error" and the reference) |
| `auth` | `audit`/`audit_as` in `app/routes/logs.py` | signed in, link refused, joined through an invite, wrong PIN, password refused, wrong code, 2FA on or off, signed out, PIN or password changed, another session ended |
| `admin` | `audit` | account, key and invite-link changes, log days deleted, prunes, jobs started by hand |
| `ui` | `POST /api/logs/events` from `track.js` | screens, clicks, form submits, undo/redo, JS errors, failed calls, device details |
| `system` | the server | started, stopped, pruned, entries dropped, a job failing |

Every entry also carries the actor (account id, name, role, and whether it
came in by session, API key -- by its public id -- or anonymously), the
client IP (`middleware.client_ip`, the same proxy-trust rules as the rate
limiter), and the user agent. `request`, `error`, `auth` and `admin` entries
carry the `request_id` too (see [Monitoring](#monitoring-request-ids-readiness-metrics)).
A `ui` entry's time is the server's; the
browser's own clock is kept beside it as `ui.at`, so a page cannot backdate
what it reports.

**What never goes in.** Request bodies, cookies, `Authorization`, the secret
half of anything, and anything typed. `logs.redact` turns `/login/<cred>`
and `/join/<cred>` into `/login/…` and masks `token`, `key`, `secret`,
`code`, `credential`, `csrf_token`, `pin`, `password`, `password_digest`,
`challenge` and `recovery_code` query values; the
referer goes through the same. `track.js` sends a form's field *names*, a
control's `data-i18n` key or id, and only `#hash` hrefs. A part of the page
marked `data-no-track` (the Logs screen itself) is skipped.

**Where it lives.** Files, not SQLite: `<app dir>/logs/YYYY-MM-DD.jsonl`,
with `.1.jsonl`, `.2.jsonl` ... once a day passes `logs.max_file_mb`, and
`.jsonl.gz` once compressed. A log is append-only, grows all day and is read
a day at a time; as encrypted records it would make retention a decrypt of
every row (`delete_where` cannot filter in SQL) and double `app.db` for
something never joined against app data. With `server.encrypt_at_rest` each
line is the entry's JSON sealed with the server key
(`encryption.xchacha_encrypt_bytes`) and base64'd, so the logs directory is
as unreadable as `app.db` without `secret.key`. The reader takes either form
line by line, so flipping the setting never strands old days.

**How it is written.** `Logger.log` puts the entry on a 10,000-slot queue and
returns -- safe on the event loop, no `await run` needed. One daemon thread
writes, flushing every second or 200 lines. A full queue drops the entry and
counts it, and the writer logs the count as a `system` warning once it
catches up. A slow disk can lose log lines; it can never slow a request. The
writer, the readers and the pruner share one lock, so a file is never
compressed or deleted mid-write. The lifespan starts the thread and, on
shutdown, drains and closes it. After a flush the writer calls `on_flushed`,
at most once a second, which is what the live tail listens to (see
[Live updates](#live-updates)).

**Retention**, from `logs:` in config.yaml, applied by the hourly `prune-logs`
job and on "Prune now": finished days older than `compress_after_days` are
gzipped (to a temporary name, then renamed, so a crash leaves the original);
days older than `retention_days` are deleted; then, while the total is over
`max_total_mb`, the oldest day goes. Today is never touched, and
`POST /api/owner/logs/days/<today>/delete` is a 409.

**Who reads it.** `RequireOwner`: the owner, from a browser session. Another
admin, any API key and the signed out all get the 404 an unknown route gets
-- the log holds everyone's addresses and activity, including the other
admins', which is the reason they cannot read it, and a key is a credential
that lives in files. (The Go template answers the signed out with 401, since
its `/api` group checks login first; the screen behaves the same.)
`manage logs` reads the same files from a shell; they are append-only, so
that is safe with the server running.

**Search** is `logs.Query`: free words match the entry's JSON in any case;
`kind:` `level:` `user:` `status:` (`5xx` for a class) `method:` `path:`
(prefix) `ip:` (prefix, or part of what the owner calls it) `action:` `screen:` narrow; a comma is OR, a leading
`-` is NOT, quotes make a phrase. It is a linear pass over the day's lines,
decrypting each, in a worker thread -- see Known limits.

**Names for addresses.** The owner can name an address or a range, which the Logs and
Sessions screens show, and `ip:` and free text search. They are stored in the database
and added as the log is read, so the files never hold them and renaming one renames its
past. The most specific wins. Downloads carry `ip_name` beside `ip`. Only the owner sees
them: they are notes about people.

## Background jobs

Work the server does with nobody asking: `app/core/jobs.py` is the registry,
`app/core/housekeeping.py` the template's own jobs, `app/models/jobs.py` what is
remembered about them, and the owner's Jobs screen (`GET /api/owner/jobs`,
`POST /api/owner/jobs/<name>/run`) shows them and starts one by hand.

```python
registry.register("purge", 3600, lambda: purge(store))   # fn() -> (note, error)
```

| job | what it does | `last_note` |
|---|---|---|
| `purge` | drops expired sessions, login tokens and challenges, and long-dead keys and invite links | what it removed |
| `prune-logs` | applies `logs:` retention; registered only with `logs.enabled` on | days deleted and compressed |
| `backup` | runs `backup.run_scheduled` | `off`, `not due`, or `wrote <file>` |

**When a job runs comes from the database.** A tick every 30 seconds calls
`run_due(now)`, and a job is due when it has never run or `last_started +
interval` has passed. `last_started` is written before the job runs. So a
restart a moment after a run does not repeat it, a process that dies mid-run
has still used its turn, and -- the reason this exists -- a server restarted
more often than a job's interval still runs it. The loop this replaced slept an
hour first, and a server restarted hourly never purged anything. A job never
run, or overdue, starts at the first tick after boot, so housekeeping now
happens shortly after a start.

**What a job can rely on.** One run of a job at a time. It runs in a worker
thread, so it uses the store directly. An exception is recorded as that job's
failure, printed, and written to the activity log, and the tick goes on. A
purge step that fails does not stop the next one; the errors are joined.

**`running` is memory, not a stored flag,** because a stored one would stay
true after a crash. A start with no finish after it, and nothing running, is
what the screen calls interrupted.

**Stopping** waits up to five seconds for a run under way, then leaves it. Not
cancelled: a thread cannot be. That is also why jobs run through
`graceful.run_abandonable` rather than `run`. `asyncio.run` cancels every task
left at exit and waits for them, and a plain `run_in_threadpool` task cannot
finish being cancelled until its thread returns, so one long backup would hold
the process open however bounded everything before it was. A backup cut off by
that exit leaves only its temporary file, which `backup.prune` clears later.

The `jobs` bucket is an ordinary bucket: it goes into backups, so a restored
database brings back when each job last ran. A backup from before the bucket
existed has no table for it, and `db.check_file` and `db.rekey` skip what the
file does not have; the next start creates it.

## Sessions: seeing and ending your own

The Account screen lists an account's own sessions and can end the others
(`app/routes/sessions.py`, `static/js/sessions.js`). A key may do it too
(`list_my_sessions`, `revoke_my_session`, `revoke_my_other_sessions`). A key has
no device of its own, so nothing in its list is current and revoke-others
signs every browser out.

A session record gains three optional fields, read with `.get` so older
records need no migration: `ip`, `user_agent` (cut to 256 characters) and
`last_seen_at`. Every way of signing in ends in `auth.sign_in_response`, so that
is the one place they are captured, 2FA included, and failing to write them
never fails the sign-in. `last_seen_at` is touched at most every 15 minutes from
`security._resolve_session`, the same trade as an API key's `last_used_at`. It
is deliberately not touched from `load_session`, which is what re-checks an open
stream: a tab left on the Logs screen is not someone using the app.

`Session.id` exists only in memory: `load_session` sets it, `to_record` leaves
it out.

- **A session in use extends itself.** The same write that records last seen
  moves the end to `auth.session_ttl_seconds` from now, so only an idle session
  ends at that age. Someone without a password would otherwise have to ask an
  admin for a new link every two weeks. `auth.session_max_age_seconds` is the
  limit, counted from signing in, so a stolen cookie cannot be kept alive forever.
  The cookie has its own expiry, so the response to a request that extended the
  session sends it again; a route that sets the cookie itself, logging out or
  switching account, wins. Set the two equal for a fixed lifetime.
  `LoadActor` adds the header on the way out, and leaves it off when the route set
  the cookie itself, since two headers for one cookie would leave the answer to
  their order.

- `GET /api/account/sessions` builds each row field by field, so `secret_hash`
  and `csrf_token` cannot leak through a record gaining a field.
- `POST /api/account/sessions/<id>/revoke` looks and deletes in one
  transaction. Someone else's id and an id that does not exist get the same
  404, so the route cannot be used to find out which ids are real. The session
  in use is refused with a 400: signing out is what ends that one, and it also
  clears the cookie.
- `POST /api/account/sessions/revoke-others` ends the rest.
- `GET /api/admin/users/<id>/sessions` and `POST .../<session_id>/revoke` are an
  admin's view of someone else's, from the **Sessions** button on the Users screen,
  for ending one device where `revoke-access` would end everything. A browser
  session is required, as for your own. Only the owner reaches the owner's, and
  only the owner sees addresses, as in the activity log, which keeps them from
  other admins too.

These are revokes, so they ask first and offer no undo. The IPs and user agents
are personal data, and unlike the activity log the sessions bucket is in every
backup.

## Monitoring: request IDs, readiness, metrics

**Request IDs** (`app/core/request_id.py`). Every request gets 16 hex
characters. It is always made here and an inbound `X-Request-ID` is ignored,
since one a caller could choose is one that could point a search at the wrong
line. It comes back in the `X-Request-ID` header, sits at the top level of log
entries, and is in the body of a 5xx: `{"error":"internal error",
"request_id":"..."}`. The page shows it after the error message, and pasting it
into the Logs search finds the line.

A reference is only worth having if the line it finds says something, so every
500 has a cause. An unhandled exception's is its repr. `helpers.server_error`
takes one: `server_error(request, cause)`. Both go through
`middleware.internal_error`, which prints `[error] <id> METHOD path: cause` to
stderr and leaves the cause on the request for `RequestLog`.

`RequestID` is its own middleware, outermost, because `RequestLog` does nothing
with the log off. It still cannot cover a crash: `server_error`'s response is
sent by Starlette's `ServerErrorMiddleware`, outside every middleware in the
stack, so `internal_error` sets the header on the response itself.

**`GET /api/ready`** (`meta.get_ready`) is public and says 200 `{"ok":true}` or
503 `{"ok":false}`, nothing more. It checks the schema version is the one this
code expects and reads the owner marker, which proves the file opens and the
key decrypts it. `/api/health` stays trivial: it decides whether a container is
restarted, and a database problem restarting a container helps nobody, while
`/api/ready` decides whether traffic should come here. The check is bounded at 2
seconds with `asyncio.wait`, not `wait_for`: a timeout must not cancel a thread
that cannot be cancelled, or it would wait for exactly the thing that is slow. A
probe in progress is shared by anyone else who asks, so a stuck disk costs one
thread however often it is polled.

**`GET /metrics`** (`app/core/metrics.py`, `app/routes/metrics.py`) is Prometheus
text, written by hand. `metrics.enabled` is off by default. It answers 404 while
that or `auth.api_keys` is off, then 401 anonymous, 403 for a browser session
or a user-scoped key, and 200 for an admin key.

| series | type | |
|---|---|---|
| `app_http_requests_total{method,status_class}` | counter | methods from a fixed list, else `OTHER` |
| `app_http_request_duration_seconds` | histogram | no labels |
| `app_http_requests_in_flight` | gauge | |
| `app_uptime_seconds` | gauge | |
| `app_logs_dropped_total` | counter | |
| `app_job_last_success_timestamp_seconds{job}` | gauge | left out for a job with none |
| `app_job_last_failure_timestamp_seconds{job}` | gauge | left out for a job with none |

The prefix is a fixed `app_`, not the project's name, and the names match the Go
template's, so one dashboard fits every app. There are no path labels and the
method list is closed, because a label a request can invent is a series count
anyone can grow. A live stream leaves the in-flight gauge as soon as it turns
out to be one and never reaches the histogram, since how long someone watches
says nothing about the server. It is counted when it ends.

`MeasureRequests` is its own middleware, outside the rate limiter, so a refused
flood shows up in the counters though it never reaches the log.

## Live updates

`app/core/sse.py` is server-sent events by hand: the format is a few lines of
text. `sse.event` turns a line break in the data into the next `data:` line, so
data can never start a field of its own.

A `Hub` fans a message out to its clients. Each has a queue of 16, and a message
that does not fit is dropped rather than waited for, so a stalled browser costs
itself an update and costs the sender nothing. It takes eight clients, past
which the route answers 503.

`EventStream` is the response for one client. Three things about it are not
obvious:

- **The slot is freed in a `finally` around `__call__`,** not in the generator.
  When a client goes away Starlette cancels the send, and a generator suspended
  there can be left unfinished.
- **Every `send` has a 30 second timeout,** because uvicorn has no write timeout.
  A client that stops reading otherwise holds its slot, and holds up a stop, for
  as long as the socket lasts.
- **Every 15 seconds it sends `: ping` and asks again** whether the request is
  still allowed. A stream is a request that was authorised once, possibly hours
  ago. For the log tail that is `security.still_owner`, which calls
  `load_session` directly, so it does not count as activity and cannot be
  answered by an API key, and the stream ends within 15 seconds of the session
  being revoked, or of ownership moving. An error there ends it too.

The one stream in the template is `GET /api/owner/logs/stream`, the Logs
screen's live tail. It sends a `changed` nudge, not entries, and `logs.js` runs
its ordinary refresh: sending the entries would need a second filter, decryption
and paging path over the same files. The nudge comes from the log writer's
`on_flushed`, gated to one a second because a quiet server flushes after every
entry. An idle server sends nothing but pings. The route is under
`/api/owner/logs`, which `RequestLog` skips, so a stream cannot feed itself.

## Stopping cleanly

A stop has to finish in order, and each step has a bound:

| step | where | bound |
|---|---|---|
| end the live streams | `graceful.Server.shutdown` calling `on_stop` | at once |
| requests finish | uvicorn, `timeout_graceful_shutdown` | 10 seconds |
| a job in progress | `Registry.stop`, from the lifespan | 5 seconds |
| the log is flushed and closed | the lifespan | normally milliseconds |

The first row is why `app/core/graceful.py` exists. uvicorn.run offers nothing
between "a stop was asked for" and "wait for every connection", and the lifespan
runs after that wait, so an open stream would cost the full 10 seconds on every
stop. `graceful.run` is `uvicorn.run` for this one case, with its
`KeyboardInterrupt` catch and its exit status when the server never started.

That adds up to more than `docker stop`'s default 10 seconds, hence
`--stop-timeout 20` in `docker/run.sh`, and 20 seconds before `scripts/lib.sh`
sends SIGKILL.

## Paging lists

Every list route takes `?limit=` (default 200, at most 1000) and `?offset=`, and
answers `{"users": [...], "next": 200, "total": 431}`. The rows sit under a name
of their own, `next` is the offset of the following page or `null` on the last
one, and `total` counts every row. A bad value is a 400. The MCP list tools take
the same two arguments and answer the same shape. Use `send_page( request , "users" , rows )` and `page_body` in
`app/routes/helpers.py`, and `_page` in `app/routes/mcp_tools.py`, for a new list, so
it pages the same way.

**It is an offset, not a cursor.** Most lists sort on a field that is encrypted
at rest, so each page reads the whole list anyway and a cursor would save
nothing. The natural cursor for a key or a session is also its record id, which
has no business in a URL. The cost is that a row deleted between two pages shifts
the next one by a row. The screens reload from the first page after any change,
so they never see that, but a script walking a list while it changes can skip a
row.

**Paging bounds the response, not the read.** See [Known limits](#known-limits).
Jobs, log days and invite-link name matches are not paged, because each is
bounded already; a day of the log has its own `before` paging. `manage`, the
internal callers and the model functions still see every row.

## Error pages

A person who hits a bad address sees a page, and a script sees what it always
did. `app/core/error_page.py` decides by asking whether the request is a page load: GET or HEAD,
outside `/api/`, `/mcp` and `/metrics`, with `text/html` in `Accept`. Anything
else keeps its plain answer, so `api.js` always gets JSON and a missing
stylesheet is a plain-text 404 that reads clearly in the console.

The server renders it, rather than the browser app, for two reasons. The app
needs `/api/language` before it can show a word, and that call is rate limited,
so it cannot be relied on while answering a 429. And it has nothing to bootstrap,
so a broken app can still show it. Its text is the `error_page` section of
`language.yaml`, HTML-escaped. The CSP allows no inline styles or scripts, so it
links the stylesheets and `theme.js` the app already serves, which the rate
limiter exempts: it is light unless this browser saved a dark choice. A 5xx shows
the request id as a reference, and never the cause. It is never cached, since the
same address may work next time.

**There is no `index.html` fallback.** The app navigates by hash, and the only
paths it reads are `/login/...` and `/join/...`, which have routes of their own.
Anything else is a real 404. An app that adds a page with a path of its own must
give it a route before the catch-all, as `serve_login_page` does.

## Maintenance mode

`server.maintenance` takes three values, and is `[live]`, so Settings, `config
set` and a hand edit all apply on the next request. The normal value is `none`
rather than `off` because YAML 1.1 readers take a bare `off` as false.

| | `read_only` | `full` |
|---|---|---|
| Reads (GET, HEAD) | allowed | 503 |
| Writes | 503 | 503 |
| Joining through an invite link, `/mcp` | 503 | 503 |
| The owner's browser session | everything | everything |

Open in both modes: files the page loads, `/api/health`, `/api/ready`,
`/metrics`, `/api/language`, `/api/me`, signing in and out, and `/login/*`.
That list is what lets the owner get back to the switch, so an owner who has
signed out comes in through a login link (`scripts/login.sh`). A refusal is a
503 with `Retry-After`: the maintenance page for a browser, and
`{"error": ..., "maintenance": "<mode>"}` for everything else. `/api/me` reports
the mode, and the app shows a banner, so the owner cannot leave it on by
accident. The owner's API keys are refused like anyone's, as on every owner route.

`/mcp` is shut in both modes because one POST there can be a read or a write.

State these to the user:

- **`read_only` trusts GET routes not to write.** A route that changes something
  on a GET will keep doing so. Bookkeeping such as a session's last-seen time
  does carry on.
- **A one-time login link opened during `full` is spent.** A person who is not
  the owner lands on the maintenance page, and needs a new link afterwards.
- **Background jobs and webhooks keep running**, and `manage` is not affected.

The rules are in `app/core/maintenance.py`, and the check is in `app/core/maintenance.py`.

## The API reference

The API reference is three files, all written from the code and committed:
`config/openapi.json` (OpenAPI 3.1), `config/mcp.json` (the MCP tools, see
[above](#configmcpjson-the-mcp-reference)) and `config/api.md`, the same
again as tables a person reads. Nobody edits them by hand. `GET /api/openapi.json`
serves the spec to any signed-in caller, session or key: it maps the admin
surface, and nobody who cannot sign in has a use for it.

**Where the docs live.** Every route's handler declares its docs once, in
`app/routes/<area>_api.py` next to the handler's own file: `doc(account.create_my_login_link, summary=..., tag=..., ...)` with a summary, a tag, who may
call it, and TypedDicts for its body and reply. A key is required unless `NotRequired[...]`; `Annotated[T, Field(...)]` carries the details: `doc`, `enum`, `format`, `minimum`/`maximum`, `min_length`/`max_length`, `default`, `example` and `nullable`. `T | None` may be null; `@schema(name=...)` makes a named schema, `@schema(doc=...)` a description, `one_of(name, A, B)` a union, and inheriting a named TypedDict makes allOf. `app/apidocs/schema.py` has the full list.

**How it is built.** `apidocs.build` (`app/apidocs`) walks the route table the app really
registers and pairs each route with its handler's docs, so paths and methods
come from the code that serves them. The pages at `/login/*` and `/join/*` and
the catch-alls are the only routes left out. `operationId`, path parameters and
the shared error replies are worked out; everything else comes from the docs.
The Go and Python templates write all three files byte for byte the same.

**Who may call.** `access` (the default is anyone signed in; `PUBLIC`, `SESSION_ONLY` or `KEY_ONLY` otherwise) and `role` (`ADMIN`, `OWNER`) become `security` and `x-required-role`. A test
(`test_mcp_parity.py`) calls every route with an admin key to check that the guards agree.
A route a key may call either has an MCP tool or a `mcp_http` reason, one line
on why it stays HTTP only (see [MCP](#mcp-the-api-keys-for-agents)). `feature`
becomes `x-feature` on the path, which scaffolding reads to cut a part out.

**What keeps it honest.** `app/tests/test_api_docs.py`: `test_every_route_is_documented` fails while a route has no doc, a doc has no route, or a handler is a lambda; `test_the_api_docs_are_current` fails while a committed file differs from what the code says (`scripts/api_docs.sh`, which is `python -m app.apidocs`, rewrites them, needing nothing but the app's requirements). And `RoutesTestCase.check_reply` holds each 2xx JSON reply a route test gets against the documented schema, so a field the docs promise but a handler leaves out, or one it sends that the docs never mention, fails whichever test saw it.

**Scaffolding.** The template ships the three files for a project with every
part. A project scaffolded without some parts runs `scripts/api_docs.sh` once,
and its tests say so until it has. A maintainer who changes the template's docs
runs `scripts/refresh_api_docs.py` in the skill, which scaffolds a full project
and copies its files back; `check_features.py` fails while they are stale.

## Webhooks

The owner's Webhooks screen lists endpoints that want to hear about things. The
code is `app/core/webhooks.py`, `app/models/webhooks.py` and the Webhooks routes. Owner only, from a browser
session, and no MCP tools: an endpoint sends what happens here to somewhere else,
so a leaked key must not be able to add one.

**It is an outbox.** `await emit( request , event , data )` from `app/routes/helpers.py` writes one delivery row per endpoint that wants the
event and returns. The `webhooks` background job sends them, so a slow receiver
never holds up a request and a restart loses nothing. An emit asks for a run
straight away; when one is already going, a later run picks the row up, within
about a minute. Retries wait 1 minute, 5 minutes, 30 minutes, 2 hours and 12
hours, and the sixth failure gives up. Finished deliveries stay listed for 7 days.
Each run sends at most 50. An MCP tool, already in a worker thread, calls `call.request.app.state.webhooks.emit(...)`.

**Delivery is at least once.** A crash between the send and recording it sends it
again, so a receiver dedupes on `X-Webhook-Delivery`. A retry sends the same bytes
with the same id.

**Events are a fixed list** in `app/models/webhooks.py`, because their names are a promise to
whoever receives them, which the activity log's free-text messages are not:
`user.created`, `user.disabled`, `user.deleted`, `api_key.created`,
`api_key.revoked`, `invite_link.created`, `invite_link.revoked` and `job.failed`,
plus `webhook.test` from the screen's test button. A change made through MCP fires
too, and switching the log off changes nothing. Sign-ins are left out on purpose:
they are frequent, and nobody needs them yet. To add one, append it to the list
and emit it where the thing happens. An event not on the list still reaches `*`
subscribers. Payloads carry ids and names only, the same rule as a log entry's
detail.

**What a receiver gets** is a POST with this JSON body, and these headers:

```
{"id":"<delivery id>","event":"user.created","created_at":"...","data":{...}}

X-Webhook-Event:     user.created
X-Webhook-Delivery:  <delivery id>
X-Webhook-Signature: t=<unix seconds>,v1=<hex>
```

`v1` is HMAC-SHA256 of `<t>.<raw body>` with the endpoint's secret. Check it
against the raw bytes before parsing, compare in constant time, and refuse an old
`t`, so a captured request cannot be replayed. `webhook_cases.json` has worked
examples. Any 2xx counts as delivered. A redirect is a failure and is not
followed.

**The secret has to come back**, unlike a key's, because the server signs with it.
So it is sealed with the secret key on its own, the way a TOTP secret is, and
`sealed.rekey_record` carries it across a rotation and a restore. It is shown once, on creation.

**This is the only code that calls an address someone typed**, so it is where a
server-side request forgery would start. An address must be `https` with no
username or password, and is checked when it is saved and before each send. The
check that counts is on the address actually dialled, after DNS, so a name that
resolves to something public today and `169.254.169.254` tomorrow gets nowhere.
That is a replacement for `http.client`'s `_create_connection`, which runs
`getaddrinfo` itself and checks each address before connecting. TLS is wrapped
around that socket afterwards, so certificate and hostname checks still use the
name in the URL. `http.client` follows no redirects and reads no proxy settings.
The attribute is private, so `test_webhooks.py` fails if a Python release moves
it. There is no new dependency.

The blocked ranges cover this machine, private networks, link-local (where cloud
metadata lives) and everything else that is not the public internet, and the same
list is in both templates. `webhooks.allow_private_targets` lifts all of this and
allows plain `http`, for a receiver on the same network.

**Editing an endpoint.** The name, address and events can change, and a new
address gets the same check as a new endpoint's. Deliveries still waiting go to
the new address, because the sender reads the endpoint at each attempt: that is
what fixing a mistyped one needs. **New secret** replaces the signing secret at
once, with no overlap and no way back. Each attempt is signed as it is sent, so a
receiver that refuses deliveries until it has the new secret gets them on a retry.

A delivery's error text never holds the URL, which may carry a token. The audit
log records an endpoint's name and id only, for the same reason.

## File uploads

Everyone gets a Files screen and `/api/files`, on by default, switched with
`files.enabled`. The code is `app/models/files.py` (storage) and `app/routes/files.py` (who may
do what).

### A file arrives in pieces

```
POST /api/files                       {name, size, type} -> {id, chunks, chunk_bytes}
POST /api/files/<id>/chunks/<index>   the raw bytes of one piece, in order from 0
POST /api/files/<id>/finish
GET  /api/files/<id>                  received: the next piece to send
```

A piece is 768 KiB, under the 1 MiB `server.body_limit_bytes`, so **no route needs
a bigger limit than any other.** That is the reason for the shape. One big
request would mean raising the limit for every route, public sign-in included,
or streaming bodies, which takes the limit away from every route in Go; and a
multipart body spills to the temp directory, which is 16 MiB in the container and
would hold the file in the clear. Both templates speak this same protocol.

`index` has to be the next piece. Anything else is a 409 carrying `received`,
which is also what a retry of a piece that did arrive gets, so a client resumes
by doing what the server says. The server keeps the count; `files.js` never
trusts its own. A second request writing the same upload is a 409 too.

A piece's body is the file, so its CSRF token comes in an `X-CSRF-Token` header.
API keys are exempt, as everywhere. Pieces have their own rate limit,
`server.rate_limit_upload_max`, because on the general budget an upload would
both crawl and use up what the page needs.

uvicorn has no timeout on reading a body, so a slow client can hold a connection
for as long as one piece takes. A piece is small, which bounds how long that is
for any honest client; `Starlette`'s multipart parser is not used, so nothing
spills to the temp directory.

### Encryption: one key per file

Each file has a random key, and only that key is sealed with the secret key, in
the record. So **a rotation or a restore onto a new key rewrites one field and no
file data**: the field is in `SEALED_FIELDS` in `app/models/sealed.py`, and neither
`rotate-key` nor restore has any code for files. Sealing files with the secret
key directly would have meant rewriting all of `storage/` on every rotation, at
twice the disk.

Each piece is sealed with XChaCha20-Poly1305 on its own, with `<id>|<index>|<total
pieces>` as additional data (`xchacha_encrypt_bound`). A piece moved, swapped with another
file's or cut off the end fails to open. Files are always sealed, with
`encrypt_at_rest` on or off, like TOTP secrets. The name and type are in the
record, so they follow `encrypt_at_rest`.

Every piece but the last is the same size, so offsets are arithmetic and the file
on disk is nothing but sealed pieces.

### Who can reach what

| Route | Who |
|---|---|
| `/api/files...` | The uploader, and their API keys. Anyone else gets 404. |
| `GET /api/admin/files` | Admins, with an admin key too. Names, sizes, owners. |
| `/api/admin/files/<id>/download`, `/delete`, `/restore` | Admins, from a browser session. |

An admin key can list but not open: a leaked key must not be a way to carry off
every account's files. An admin opening someone else's file is written to the
activity log.

A download is always `Content-Disposition: attachment`, always
`application/octet-stream` and `Cache-Control: private, no-store`, with `nosniff`
from the usual headers. The type an uploader declares is shown and never served,
so an uploaded page cannot run as this site. Don't add an inline preview without
serving it from another origin. Pieces are decrypted one at a time while
streaming, so memory stays flat, and a piece that fails to open ends the connection
short rather than sending anything unverified.

### On disk and cleaning up

Pieces go to `<app dir>/uploads/<id>`, outside `storage/`, so backups skip them, and
`finish` renames the file to `storage/files/<id>`. Finished files never change,
which suits a backup copying them while the server runs. Client names never reach
a path: paths use the id.

`files.quota_bytes_per_user` is checked in the transaction that creates the record
and counts uploads still arriving, so parallel uploads cannot pass it. A deleted
file stops counting; restoring one checks again.

Delete is the usual [soft delete](#soft-delete-and-restore). The `files` job, hourly,
removes files past the undo window, uploads idle for
`files.abandoned_upload_hours`, files whose account has gone, and data on disk with
no record once it is a day old. It removes the record first, so a crash leaves a
stray for the next run rather than a record pointing at nothing. That is also why
deleting an account needs no code for files.

`file.uploaded` and `file.deleted` go to webhooks. There are no MCP tools.

### Sending a file from a script

```bash
base=https://app.example.com key=<id>.<secret> file=report.pdf
size=$(wc -c < "$file")
started=$(curl -sf -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d "{\"name\":\"$file\",\"size\":$size}" "$base/api/files")
id=$(jq -r .id <<<"$started"); chunk=$(jq -r .chunk_bytes <<<"$started"); chunks=$(jq -r .chunks <<<"$started")
for ((index = 0; index < chunks; index++)); do
  dd if="$file" bs="$chunk" skip="$index" count=1 2>/dev/null |
    curl -sf -o /dev/null -H "Authorization: Bearer $key" -H 'Content-Type: application/octet-stream' \
      --data-binary @- "$base/api/files/$id/chunks/$index"
done
curl -sf -H "Authorization: Bearer $key" -H 'Content-Type: application/json' -d '{}' "$base/api/files/$id/finish"
```

## Why credentials are shaped `<id>.<secret>`

Login tokens, sessions, API keys, and invite links all use `<id>.<secret>`,
and the reason is worth understanding before changing it.

Storing only a hash of a credential is standard -- it means a stolen database
cannot be replayed. But a *salted* hash like bcrypt cannot be looked up: you
would have to fetch every candidate record and bcrypt-compare each one, which
hands anyone a trivial way to pin the CPU with garbage tokens. (And with every
record encrypted at rest, there is no column to search by anyway.)

Splitting the credential fixes that. The id is the record key, so lookup is
one fetch, and exactly one hash comparison follows.

The halves then use different hashes on purpose:

- **Login tokens use bcrypt.** They are checked once per sign-in through a
  link (a reusable link is still one check per sign-in, never per request,
  and sign-ins have their own rate budget), so bcrypt's deliberate slowness
  costs a single request and buys real resistance. The comparison runs
  *before* the write transaction that counts the use, so SQLite's single
  write lock is never held for the slow part; the transaction re-checks that
  the token still has a use left before spending it.
- **Sessions, API keys, and invite links use sha256.** Sessions and keys
  are checked on every authenticated request, and an invite link's secret is
  24 random bytes, not a person's choice. Against a long random secret there
  is nothing to brute force, so a fast hash is correct -- bcrypt here would
  just be a self-inflicted latency tax.

Comparisons go through `encryption.constant_time_equal` so the check does not
leak, via timing, how many leading characters matched.

## Which crypto is used where, and why

Everything lives in `app/core/encryption.py`. Nothing else in the project
imports `secrets`, `hashlib`, `hmac`, `bcrypt`, or `nacl` -- that is what makes
"how does this app handle secrets" a one-file question.

| Primitive | Library | Used for |
|---|---|---|
| OS CSPRNG | `secrets` | Every credential, key, and nonce |
| bcrypt | `bcrypt` | Hashing one-time login secrets, PINs, and password digests |
| HMAC-SHA1 | stdlib | TOTP codes (RFC 6238) |
| XChaCha20-Poly1305 | PyNaCl (libsodium) | Session cookie payloads, records at rest, uploaded files |
| sha256 + `hmac.compare_digest` | stdlib | Session, API key, and invite link secret lookup, constant-time compare |
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
XChaCha-encrypted with the key in `secret.key` when `server.encrypt_at_rest` is on
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

The key lives in `secret.key`, beside `config.yaml` and never in it -- which
is what lets `config.yaml` be edited while the server runs, and be writable
inside the container, without either path touching the key. Where
`secret.key` sits relative to the database depends on how the app is run --
`configuration.md` covers this in full, but the short version:

- **Container**: `config/secret.key` stays in the project directory and is
  mounted read-only, separately from the data volume, so a leaked `./data`
  backup is not enough to read it.
- **Run outside Docker with no `./config/config.yaml`**: the first run writes
  `config.yaml` and `secret.key` into `~/.config/<slug>/` beside `app.db`. That
  is what lets it just run -- at the cost that at-rest encryption then protects
  a database file that leaks *on its own*, not someone who takes the whole
  directory.

`config.load` refuses to start with a malformed key, or a missing one once a
database exists, rather than continuing or generating a new one, because the alternative failure mode only
shows up later, when every record has already become unreadable. A wrong key
fails loudly on the first read with a message that names the cause. An older
`config.yaml` with `server.secret_key` in it has the key moved into
`secret.key` (written and read back first, the line removed second), and two
different keys stop it cold.

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
  `manage` can read `secret.key`; a socket would have guarded nothing extra.

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
One thing is owner-only: reading the activity log (see above).
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
first states the intent rather than leaving it to the path checks below it.

A missing path is a 404, whether or not it has an extension: see
[Error pages](#error-pages). A browser loading a page gets the error page, and a
missing `.js` or `.css` gets plain text, since serving HTML for one would leave
the browser reporting a confusing MIME error instead of the real problem.

`static/favicon.ico` is a neutral placeholder: replace the file to brand an app.

**Search engines are told to stay out, twice.** These are private apps, so
`static/robots.txt` disallows everything, which also stops polite crawlers
fetching a login link that ends up somewhere public, and every response carries
`X-Robots-Tag: noindex, nofollow`. Neither is a lock. A crawler that obeys
`robots.txt` never fetches a page, so it never sees the header, and it can still
list a bare address that someone linked publicly. Keeping a page private is the
sign-in's job.

If this ever costs too much CPU (very large JS, very high traffic), the fix is
smaller static output, not caching the text assets -- that would reintroduce
exactly the staleness this avoids.

## Where state lives

Everything the app persists -- database, uploaded files and the activity log
(`logs/`) -- lives in one
directory, `~/.config/<slug>/` by default and `/app/data` in a container.
Everything it can be told -- server and Docker settings -- lives in one
`config.yaml`, with the key that encrypts the database in `secret.key` beside
it, and nothing reads environment variables.

That convention has its own reference: see `configuration.md`. Read it before
adding a setting or writing a file anywhere, because the two rules it exists
to protect (one backup unit, one file that holds every setting) are easy to
break by accident.

## Changing a stored record: migrations

Records are JSON, so most changes need nothing. A new optional field is simply
absent from older records (read it with `record.get`), and a new bucket is
created at startup. A migration is for the rest: a field renamed or split, a
value given a new meaning, records moved between buckets.

The database holds a schema version (`PRAGMA user_version`): how many of the
steps in `MIGRATIONS` (`app/core/db.py`) have run on it. `open_store` runs
whatever is missing, in order, before anything else sees the data.

```python
def split_name(tx: Transaction) -> None:
    for key, record in tx.all(BUCKET_USERS):
        first, _, last = record.pop("name").partition(" ")
        record.update(first=first, last=last)
        tx.put(BUCKET_USERS, key, record)


MIGRATIONS: list[Migration] = [
    Migration("users: split name into first and last", split_name),
]
```

A step works on plain dictionaries, because it has to keep working after a
model has moved on: never use a model's dataclass in one. `tx` is the same
record API as `Store.transaction`.

The rules:

- **Add to the end.** Never edit, reorder or remove a step that has shipped: a
  database's version is a position in this list.
- **A new database runs none of them.** It is stamped with the latest version,
  so the models must already describe the newest shape.
- **An existing database is copied first**, to
  `app.db.before-migration-v<N>-<time>`, to undo a migration that turns out
  wrong. Delete it once you are happy.
- **Each step is one transaction with its version bump.** One that fails leaves
  the database as the last one left it, and the server refuses to start and
  names it. Fix the step and start again.
- **The server does the upgrading.** SQLite lets `manage` open the file beside
  a live server, so `manage` refuses to migrate while one holds `server.lock`:
  the server would carry on with the old shape in memory. Restart the server
  instead. With it stopped, `manage` migrates like anything else.
- **Keep steps quick.** Nothing else runs until they finish, so a long
  migration is a long startup.
- **A database newer than the code is refused**, rather than read by code that
  would not know what the new shape means. So is a backup from one. To go back
  to older code, restore the `before-migration` copy.

## Live configuration

`config.yaml` changes under a running server -- the owner's Settings screen,
a hand edit, `config set` -- and every one of those goes through one place,
the `Holder` in `app/core/config_live.py`: parse the whole file over the
defaults, validate it, then swap in a new `Config` or change nothing. The
watcher that notices hand edits (`Holder.poll`, every two seconds from an
asyncio task, run in the threadpool) applies a change only once two polls
agree on the content, and an invalid file is reported and ignored, never half
applied.

The rule that makes a change *live* is about readers, not writers: anything
that reads a setting per request reads `request.app.state.settings.get()`
(`helpers.get_config` does it for routes) once per request and uses that copy
throughout, never a `Config` captured at startup. That covers the guard
(cookie lifetime and the Secure flag, whether API keys are believed), the
routes, the rate limiter and its trusted proxies, the security headers,
`/mcp` (always routed, answering 404 while switched off), and the activity
log's settings. What is built once -- the listener, the database, Starlette's
body-size limit -- reads `Holder.boot()`, and its keys are tagged `[restart]`
in `defaults.yaml`. `live_config` copies the boot `Config` and overwrites only
the `[live]` fields; a test fails if a tag and `live_config` disagree.

The Settings screen is the owner's alone, like the activity log, and for the
same reason: the file decides who can sign in how (`auth.api_keys`, cookie
security, proxy trust), so it is the most privileged thing in the app. The
secret key is never part of it -- not in the file, not in the schema the
screen is built from. Inside the container, `docker:` is read-only there: it
decides what `docker/run.sh` mounts and hands to the container's user on the
host, so a compromised container must not choose it. Every save is in the
activity log with each key's old and new value.

## Where assets come from: configured or bundled

`app/core/assets.resolve` decides, at startup, which `static/` directory and
which `language.yaml` to serve, and logs the answer:

```
[assets] static=/srv/app/static (server.static_dir) language=/home/you/.config/myapp/language.yaml
```

That one line answers "why isn't my edit showing up" faster than any amount of
debugging, which is why it is printed on every start.

The rule: `server.static_dir` (default `./static` in the project directory -- the folder holding `config/`) wins
when it exists; otherwise the `static/` that ships beside the `app/` package
is used. In a source checkout those are the same directory, so editing a file
and refreshing just works -- the whole point of the no-store policy. The
language file is decided independently: `server.language_file` if set, else a
`language.yaml` in the app directory, else one beside `config.yaml`
(`config/language.yaml`), else the bundled copy.

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
  is in `secret.key`, which `manage backup` leaves out on purpose: keep a copy
  somewhere safe, apart from your backups.
- **An API key is a bearer credential.** Anything holding it is that account,
  at that key's role, until the key is revoked or expires -- there is no
  second factor and no origin binding. That is what a key is for, but it means
  the admin key listing is a real answer to "what could get in right now", and
  worth looking at rather than assuming.
- **Rate limiting has three budgets, and static files have none.** Each address
  gets `server.rate_limit_max` for most requests, a separate
  `server.rate_limit_signin_max` for signing in and for the invite-link calls,
  and `server.rate_limit_upload_max` for the pieces of an upload.
  Files the page loads are not counted, because `index.html` alone pulls in
  over twenty, all `no-store`. That leaves them unlimited here: a flood of them
  costs a read and a gzip each, with no database involved, so anything exposed
  to real hostility should have a limit on the proxy in front too. The sign-in
  budget is a backstop, and the per-account and per-address attempt counters
  are what actually stop guessing. Everyone behind one office network shares an
  address, so a group joining together can hit it: raise
  `rate_limit_signin_max` for that rather than the general budget.
- **Rotating the key re-wraps file keys, not file data.** Someone holding the old
  secret key and an old copy of the database can still open files from that time.
  If both have leaked, rotating is not enough: the files have to be uploaded again.
- **A backup's database and files are not from one instant.** A record without
  its file answers 404, and a file without a record is removed by the `files` job
  a day later.
- **Backups hold every file, in full.** Scheduled backups are full copies on the
  same disk, so `backups.keep: 7` means about seven times the storage. Lower it, or
  copy backups elsewhere, before uploads get large.
- **Nothing stops an upload on a nearly full disk.** The quota is per account, so a
  full disk shows up as a failed piece. An admin sees what is free under the
  everyone's files switch, from `GET /api/admin/storage`, but nothing acts on it.
  Set `files.quota_bytes_per_user` with the disk in mind.
- **An abandoned upload holds its quota.** The Files screen gives up its own failed
  uploads, but one left by a closed tab counts until the `files` job removes it.
- **A download cannot resume.** There is no Range support, so an interrupted
  download starts again.
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
- **Paging bounds the response, not the read.** Every record is encrypted, so
  each page of a list decodes all of it and then cuts out the rows. Fine at this
  template's scale; past that, the list needs an index of its own.
- **A webhook can arrive twice, or late.** Delivery is at least once and in no
  guaranteed order, and a new event can wait up to a minute behind a run already
  going. Six failures over about fifteen hours and it is given up on, until the
  owner presses **Resend** on the Webhooks screen. That queues a new delivery with
  the same body, so its `id` still names one event for a receiver that keeps track,
  while `X-Webhook-Delivery` is new. The job works through deliveries one at a time, so a
  receiver that hangs costs `webhooks.timeout_seconds` each, at most 50 a run.
- **`manage` does not fire webhooks.** Only the HTTP routes and MCP tools do, and
  there are no `manage` commands for endpoints. To change a signing secret, delete
  the endpoint and add it again.
- **Webhook rows are scanned.** Each run reads the whole outbox, which the 7-day
  retention keeps small unless an app emits a great deal.
- **Log search is a scan.** Listing days reads every line once per file and
  caches the counts; a search, or a page of a day, reads the day's lines
  again, decrypting each. Fine for the thousands-to-low-millions of lines a
  small app makes; past that, ship the downloaded JSON lines to a real log
  store instead of growing this one.
- **The log can drop lines under load** rather than slow requests down (the
  count shows on the Logs screen and in `app_logs_dropped_total`), and a 429
  from the rate limiter is not logged at all -- the limiter runs first, so a
  flood stays cheap.
- **A reference does not always find a line.** A 5xx on a path `RequestLog`
  skips (the log's own routes, `/metrics`), or from something outside it
  (`LoadActor`, the rate limiter), has its `request_id` on stdout only.
- **Sessions in backups.** Backups hold each session's IP address and user
  agent, for as long as the backups are kept.
- **The scraper holds an admin key.** There is no read-only scope, so what can
  read `/metrics` can also do what an admin key can.
- **Jobs run in the one server process,** and only there: `manage` commands do
  not start them, and a stopped server runs nothing.
- **A job past the stop limit is cut off with the process.** SQLite is safe from
  that and so is a backup, which only appears when complete, but a job an app
  adds has to be just as happy to be stopped half way.
- **Live streams are capped per process,** eight on the log hub. A hub is memory,
  so it does not span processes.
- **The log records personal data** -- IP addresses, user agents, what each
  person clicked. Tell the people using the app, and keep `retention_days`
  no longer than needed.
- **One process writes the logs.** Like the rate limiter, the writer is per
  process; run more than one server process against one app directory and
  their lines interleave in the same files with nothing coordinating parts.
- **`language.yaml` is public.** It is served to anyone at `/api/language`,
  signed in or not, because signed-out pages need text too. Never put
  anything sensitive in it.
- **Outside Docker there is no sandbox.** `python -m app` from a checkout is a
  plain process with the user's own permissions -- none of Docker's read-only
  filesystem, dropped capabilities, or pid limits apply. Fine for development
  or an internal box; not what should face the internet.
