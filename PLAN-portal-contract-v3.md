# Plan: portal contract v3 -- ProjectPortal works through its own account, built in

Written 2026-11-22, after a week of patching apps one at a time so ProjectPortal
can run them (see `~/WORKSPACE/PROJECTS/ProjectPortal/CROSS_PROJECT.md`). The
goal is that an app scaffolded from either template, or upgraded to this, works
with the portal on day one, with no per-app fixes.

## How the portal is meant to connect (the model this plan builds in)

The user does not hand the portal the owner's key. Each app gets a dedicated
**ProjectPortal admin account**, and the portal holds one admin key of that
account. That keeps a layer above the portal: the owner's account is never
automated, the app's logs say "ProjectPortal did this", and revoking that one key
cuts the portal off and takes everything it made with it.

Today that model hits three walls:

1. **Apps narrow account routes to the owner.** Rook gated every
   `/api/admin/users*` route with an owner-only guard, so the portal's admin key
   got a bare `403 forbidden` (fixed in Rook on 2026-11-22 with
   `RequireAccountAdmin`; see Rook's `server/routes/admin_games.go` and
   `server/routes/account_admin_test.go`).
2. **Key minting is only for the owner's own keys.** `SetAPIKeyMinting` refuses
   any key not owned by the owner, so the portal's account can never mint. The
   Manage project screen's API keys tab then answers `key_minting_off` in every
   app (TodoBoard today).
3. **The portal can't tell any of this in advance.** `/api/me` says what the app
   supports (`capabilities`), not what *this key* may do. Problems show up only
   when someone clicks, as prose errors the portal has to pattern-match.

## Scope and rules

- Phase 1 changes only `~/.claude/skills`, on `master`. No branches or
  worktrees. Commits and pushes are left to `gitPushUpdate.sh`.
- Both templates in the same pass, with the same names, fields and error codes.
  Go is in `go-webserver/assets/template`, Python in
  `python-webserver/assets/template`.
- Every item gets tests in each template's own suite: Go
  `server/routes/contract_test.go` and the files beside it; Python
  `app/tests/test_portal_contract.py`.
- Regenerate the API docs in both (`scripts/api_docs.sh`).
- Update `SKILL.md` and `references/architecture.md` in both.
- Add `UPGRADING.md` entries in the documented entry format. Items 1, 3 and 4
  are fixes. Items 2, 5 and 6 are upgrades (`Upgrade` blocks with `Marks`).
- The wire format is binding. `ProjectPortal/server/remote/` will be written
  against it, so field names and codes below are exact.
- `contract` goes from 2 to 3. Everything v2 keeps working, and a v2 portal
  ignores the new fields.

---

## 1. Account routes are any admin's (fix, invariant)

The templates already have the right rule (`mayActOnTx` in Go,
`models/owner.go`; the Python equivalent):

- nobody acts on the owner's account;
- only the owner acts on another admin's account;
- any admin runs ordinary and viewer accounts.

Make it something an app can't quietly break:

- Every portal-contract route takes **role=ADMIN plus the ladder**, never an
  owner-only guard. That covers `/api/admin/users` list and create, and
  `{id}/reissue-login`, `role`, `expiry`, `disabled`, `revoke-access` and
  `delete`.
- Add a "**Don't narrow account routes to the owner**" rule to `SKILL.md`,
  under the section on adding routes. App-specific owner-only things (settings,
  everyone's keys, logs) use the owner guard. A route about one account uses the
  ladder. Name Rook as the example.
- Conformance test (see item 7): every contract route works with a **non-owner
  admin key** on an ordinary account, and answers `owner_protected` /
  `owner_required` on the owner and on another admin.

## 2. A service account for ProjectPortal (upgrade `portal-account`)

A first-class account the owner creates for the portal, so the user doesn't
build it by hand in each app.

**Model.** Add `kind` to `User`: `"person"` (default, every existing account)
or `"service"`.

- A service account is an `admin` and is never the owner. `transfer-owner`
  refuses it.
- It can't be signed into. Create-user refuses `kind: service`. Reissue-login,
  invite links, passwords, 2FA and PINs all refuse it, with code
  `service_account`. It has keys only.
- The ladder treats it as an admin, so only the owner acts on it. In
  particular, another admin can't revoke the portal's access.
- In the users list, `GET /api/admin/users` rows add `"kind"`, and the screen
  shows a **Service** badge.

**Minting rule change.** `may_mint_keys` may be switched on, by the owner from a
browser session, on:

- one of the owner's own admin keys (today's rule), **or**
- an admin key of a service account (new).

Every other rule on minted keys stays the same:

- the role is capped at the account's role;
- a minted key never gets `may_mint_keys` or `may_sign_in`;
- revoking a key revokes everything it minted, down the chain.

**The owner's "ProjectPortal" card** goes on the owner screen, next to API keys.
It needs `api_keys`.

- **Connect**: a name (default `ProjectPortal`) and a key lifetime in days
  (default 365, 1-365, required).
  - It makes the service account if there is none by that name, plus one admin
    key with `may_mint_keys` on and `may_sign_in` off.
  - It shows the credential once, with Copy, and the line: "Paste it into
    ProjectPortal: Catalog → this app → Admin key".
- **Status**: the account's live keys, each with its expiry. It warns when a key
  has under 30 days left.
- **Disconnect**: revokes every key of the service account. Minted keys and
  links die through the existing cascade. Asks to confirm.
- CLI for headless installs: `manage portal-connect [-name ProjectPortal]
  [-days 365]` prints the credential once. Also over the control socket, like
  `create-key`.
- No MCP tool: keys are never managed over MCP.

**Answer to "should key minting be on by default everywhere?"** No, and this
item makes that unnecessary. Ordinary keys stay unable to mint: a leaked
cron-job key must not be able to make more credentials. The portal's key, made
through Connect, gets minting from the start. So the portal works with no
manual switch, and no other key gains anything.

Capability: `service_accounts`.

## 3. `/api/me` reports what this key may do (fix)

When the caller is an API key, `/api/me` adds:

```json
"key": { "id": "...", "name": "...", "role": "admin",
         "may_mint_keys": true, "may_sign_in": false,
         "expires_at": "<RFC 3339>" },
"kind": "service",
"owner": false
```

- `owner` and `kind` appear for sessions too.
- `key` is absent for a session.

Capability: `key_grants`. The portal's Check now can then say "this key can't
mint keys", "this key expires in 12 days" or "this is the owner's key" before
anyone clicks.

## 4. Refusals carry a machine-readable code (fix)

Every 4xx answer becomes `{"error": "<human sentence>", "code": "<code>"}`. The
portal shows `error` and branches on `code`. Codes:

| code | when |
|---|---|
| `not_authenticated` | 401, no or bad credential |
| `not_admin` | the route needs admin |
| `view_only` | a viewer writing (today's `error` value, kept) |
| `session_only` | a key on a browser-only route |
| `owner_required` | the ladder: only the owner does this to an admin |
| `owner_protected` | the ladder: nobody does this to the owner |
| `key_minting_off` | the calling key may not mint (today's `error` value, kept) |
| `service_account` | a sign-in action on a service account |
| `not_found` | no such account, key or link |
| `invalid` | a bad field; `field` names it when it's one field |
| `conflict` | a 409 |
| `rate_limited` | a 429 |

- Keep `error` byte-identical where an existing client matches on it:
  `view_only`, `key_minting_off`, and the 403 text ProjectPortal words today.
- Add it in the one place refusals are built: Go `refuse(...)` /
  `forbidden(...)` helpers, Python `helpers.py`. Don't edit call sites one by
  one.
- A test walks every route in the route table and checks that each declared
  error status answers with a `code` from this list.

Capability: `error_codes`.

## 5. Keys rotate themselves (upgrade `key-rotation`)

`POST /api/keys/self/rotate`, called by any key (no body), answers
`{"credential": "..."}`, shown once.

- The old secret stops at once.
- The key's id, name, role, grants, lineage and `expires_at` are unchanged, so
  keys and links it minted keep working.
- Rotation never extends the expiry. Only the owner, from a session, makes a new
  key. That way a leaked key that rotates itself still dies on schedule.
- A viewer key may call it (add it to the viewer allow-list).
- Session form for the owner's own keys: `POST /api/keys/{key_id}/rotate`.

The portal can then rotate its key on a schedule with no human involved.
Capability: `key_rotation`.

## 6. Every admin action says who did it (upgrade `actor-audit`, needs `logs`)

- Each audit entry from an admin route records `actor_user_id`, `actor_name`,
  and when it was a key, `actor_key_id` and `actor_key_name`.
- The Logs screen shows "ProjectPortal · key *name*".
- Webhook payloads (where `webhooks` is on) gain an `actor` object with the same
  fields.
- This is the attribution the service account exists for.

## 7. Contract conformance, in every app and against live ones (fix)

- **In-repo**: one table-driven test per template listing every contract route
  with its method, a valid body and the expected status. Run it four times:
  - with a service-account key (once item 2 is in; until then a non-owner admin
    key);
  - with a user key: expect `not_admin`;
  - with a viewer key: expect `view_only` on writes;
  - against the owner and another admin: expect `owner_protected` /
    `owner_required`.

  The test lives in a file an app keeps verbatim, so an app-specific change that
  breaks the contract fails that app's own `go test` / `pytest`.
- **Live**: add `scripts/contract_check.sh <base_url>`, which reads the key from
  stdin, never argv. It does read-only calls: `/api/health`, `/api/me`,
  `GET /api/admin/users?limit=1`, and with `key_grants` and minting on,
  `GET /api/admin/users/{first non-owner}/keys`. It prints a pass/fail line for
  each part of the contract, naming the code on a failure.
- `webserver-upgrade`'s `status` should show each app's `contract` from that
  app's code (it already reads `portalContract`).

## 8. `/api/me` says which template it came from (fix)

- Add `"skill": "go-webserver"` and `"skill_commit": "<sha>"`, taken from
  `.skill-origin.yaml` at build time.
  - Go: embed it.
  - Python: read it at start.
  - Omit both when the file is missing.
- With these, the portal's Catalog can show "behind the template" and the user
  doesn't have to track it in `TODO.md`.

## 9. Bump and document

- `portalContract` / `PORTAL_CONTRACT` becomes 3.
- `capabilities` adds `service_accounts`, `key_grants`, `error_codes` and
  `key_rotation`, each listed only while the feature is on.
- Update the "Portal contract" section of `SKILL.md` in both skills. It's the
  table and the wire format that `CROSS_PROJECT.md` now carries. Make the skills
  the source of truth, with `CROSS_PROJECT.md` pointing at them.

---

## Phase 2: rollout (after phase 1 is committed)

App by app with `webserver-upgrade`, one at a time, in the order of
`ProjectPortal/TODO.md`. Back up each app's database before applying. For each
app:

1. Take the fixes plus `portal-account`, and `key-rotation` and `actor-audit`
   where the user wants them.
2. Run its tests.
3. Open the owner screen and use Connect.
4. Paste the key into the portal and run `scripts/contract_check.sh`.

Rook already has its own `RequireAccountAdmin`. Its owner is its "superadmin"
(`models/superadmin.go`), so the merge has to line the template's owner helpers
up with that instead of adding a second owner.

## Phase 3: ProjectPortal (its own session, in `~/WORKSPACE/PROJECTS/ProjectPortal`)

Not part of this plan, but listed so the wire format above serves it:

- Check now reads `key`, `owner` and `kind`. It records the key's expiry and
  grants, and flags "owner's key" (works, but tell the user) and "can't mint".
- Branch on `code` in `server/remote/remote.go` `refusalText`, instead of
  matching words.
- Gate the Manage tabs on grants as well as capabilities.
- Add a rotation job for `key_rotation` apps, and an expiry warning on the
  Catalog row.
- Add a "behind the template" badge from `skill_commit`.

## Phase 3b: real people across apps (decided 2026-11-22, after phase 2)

Goal: the portal shows **person → accounts → sessions** across every app, so
duplicate accounts, renames and the same person in several apps all read as one.
At most about 100 people.

- **Portal side (ProjectPortal only).** A `person` table (uuid, display name,
  notes) and a link table (app, account id) → person. The portal suggests links
  (same email or username, shared IP, same User-Agent, sign-ins close in time).
  The owner confirms, splits and merges in the portal. Nothing is linked
  without confirmation. Apps never learn about people.
- **App side (both templates, then a small rollout).** Today the only session
  route is `GET /api/account/sessions`, which lists *your own* devices. The
  portal needs an admin read of any account's devices: `GET
  /api/admin/users/{id}/sessions` (id, created, last seen, IP, User-Agent,
  label), under the same `mayActOnTx` rules and readable by the portal's key.
  Since phase 2 has already shipped, this needs a second, small upgrade to
  each app.
- Account ids must stay stable across renames. They already do. Links use the
  id, never the name.
- A shared device-id cookie works only if the apps share a parent domain.
  Otherwise, cross-app links come from IP, User-Agent and the owner's
  confirmation.

## Not in scope (ideas only, decide later)

- A cheap "accounts changed since" signal (an ETag on `GET /api/admin/users`),
  so the portal's lists refresh only when something changed.
- Invite links for the apps scaffolded without them (ZoteroWebDAV, VocabTrainer,
  Rook, PaperReference): that's a per-app choice, already covered by the
  `invite_links` upgrade.
