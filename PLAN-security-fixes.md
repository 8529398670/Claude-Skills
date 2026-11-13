# Plan: security fixes in the go-webserver and python-webserver templates

Written 2026-11-13, from a security review of E2EEDB (built from go-webserver
at `5c06754`, which is the current `HEAD` of this repo). Every finding below
is in template code, not E2EEDB's own code, so each app built from these
templates probably has it too. The E2EEDB-only fixes are a separate plan in
`~/WORKSPACE/PROJECTS/E2EEDB/PLAN-security-fixes.md`.

## Scope and rules

- Phase 1 changes only `~/.claude/skills`, on `master`. No branches or
  worktrees. Commits and pushes are left to `gitPushUpdate.sh`.
- Fix both templates in the same pass. Findings marked **Go: confirmed** were
  reproduced with a test against E2EEDB. Findings marked **Python: check
  first** were found by reading the code and need a failing test before the
  fix.
- Every fix gets a regression test in that template's own suite. The test must
  fail before the fix and pass after it.
- Add one entry to each `UPGRADING.md` covering all of these fixes, so that
  `webserver-upgrade` offers them as one security update.
- Phase 2, rolling the fix out to the apps, comes only after phase 1 is
  committed. See the end of this plan.

## S1 (high): an admin can sign in as the owner or another admin

**Go: confirmed. Python: same code shape, check first.**

`reissueLogin` returns a fresh login link for any account that exists. It
skips the rule every other "act on an account" route applies: nobody acts on
the owner, and only the owner acts on another admin. As a result, a non-owner
admin, or an admin API key, can get an owner session.

- Go: `server/routes/admin.go`, `reissueLogin` (it serves both the route and
  the MCP tool `admin_reissue_login`), and `cancelLoginLink`.
- Python: `app/routes/admin.py`, `reissue_login_checked` and
  `cancel_login_link`.

Fix:

1. Add one shared helper, `mayActOn(actor, target)` (Python:
   `may_act_on`). It refuses when the target is the owner, and it refuses
   when the target is an admin and the actor is not the owner. Make the
   existing checks (delete, role, 2FA reset, minting a key for someone else)
   call it, so the rule lives in one place.
2. Call the helper at the top of reissue and cancel. Answer 403, with the
   same wording the other routes use.
3. Tests:
   - A non-owner admin, signed in with a session, cannot reissue a link for the
     owner or for another admin.
   - The same holds for an admin API key.
   - The same holds through the MCP tool, in apps that have MCP.
   - The owner can still reissue links for admins.

## S2 (medium): Settings can make `static_dir` serve secrets to the public

**Go: confirmed. Python: `locked()` only applies inside a container, check first.**

The owner's Settings screen accepts any path for `server.static_dir`. After a
restart, the catch-all static handler serves every file in that directory
without login, dotfiles included. That breaks the promise in `secret.go` that
config.yaml can never read the key. Chained with S1, a leaked admin key is
enough to download `secret.key` and the databases.

Fix:

1. Make `server.static_dir` and `server.language_file` file-only: they are
   not editable from Settings, inside a container or out. Then audit every
   other path-typed key (storage, backups, logs, and so on) for the same
   pattern.
2. At startup, refuse a `static_dir` that is the config dir or the state dir,
   is inside either of them, or contains the secret key or a database. Exit
   with a clear message rather than serving it.
3. Make the static handler refuse dotfiles and anything outside the resolved
   root, as defense in depth.
4. Tests:
   - Settings refuses an edit to `static_dir`.
   - Startup refuses a `static_dir` set to the config dir.
   - The static handler never serves a dotfile.

## S3 (low): the owner rule is applied unevenly to other admin actions

Disabling a user, revoking all access, ending sessions, and revoking or
deleting any API key only protect the owner. A non-owner admin can therefore
lock out another admin, though not take the account over.
`RestoreAnyAPIKey` also lets any admin bring back a key the owner deleted.

Fix: route all of these through `mayActOn` from S1. Restoring a key must
apply the rule to the key's account. Add one test per route. Python: the same
routes in `admin.py`, `sessions.py` and `api_keys.py`.

## S4 (low): the 2FA guess lockout can be raced

Go `two_factor.go:61-73` checks `Codes.Locked` before the transaction and
records the failure after it, so guesses sent in parallel all pass the check.
Python has the same check-then-record shape.

Fix: reserve the attempt atomically before checking the code. Increment under
the limiter's mutex, refuse if the count is over the limit, and release the
reservation on success. Test: 20 concurrent wrong codes lead to at most 5
evaluated attempts.

## S5 (low): `.run/server.log` is readable by every account on the machine

`scripts/lib.sh` (`start_in_background`) creates the log as 0644 in a 0755
directory. On a first run that log holds the owner's one-time login link.

Fix: in both templates' `lib.sh`, set `umask 077` before creating `.run/`
and the log, and `chmod 700`/`chmod 600` any that already exist. Do the same
for `run.bat` where Windows ACLs allow it, or leave a note there if they
don't.

## S6 (info): log redaction misses mixed-case login paths (Go)

Fiber routes paths without regard to case, but `server/logs/redact.go` and
`redactReferer` only match `/login/` and `/join/` exactly. So a visit to
`/LOGIN/<credential>` writes the credential to the logs in clear text.

Fix: match the prefixes without regard to case. Test with `/LOGIN/x` and
`/Join/x`. Python: Starlette routes are case-sensitive, so this is likely not
an issue there. Confirm and note it.

## S7 (info): a session cookie wins over an `Authorization` header

`LoadUser` checks the cookie first. In a signed-in browser, a request that
carries a different credential, such as a scoped key, runs as the signed-in
account instead. That is confusing at best.

Fix: when an `Authorization: Bearer` header is present, use it and ignore
the cookie. Test: signed-in cookie plus a scoped key sends a request that acts
as the key.

## S8 (decide): revoking an API key leaves the sessions it made signed in

A full-role key can mint a login link, and redeeming that link gives a browser
session. Revoking the key already voids the link, but the session survives.

Options:

- Record the issuing key on each session and end those sessions when the key
  is revoked.
- Document the behavior in `references/` and change nothing.

Recommend the first.

## Order

S1, S2, S3 (it shares the S1 helper), S4, S7, S5, S6, S8. Run the full test
suite of both templates, then add the `UPGRADING.md` entry.

## Phase 2: roll out with `webserver-upgrade` (after phase 1 is committed)

| App | Template | Built from | Notes |
|---|---|---|---|
| E2EEDB | go | `5c06754` | No MCP. Do this rollout before its own plan's server work |
| TodoBoard | go | `2028ee4` | Has MCP: test the MCP path for S1 |
| IOSWebAppPushNotifications | go | `b286bc2` | |
| MediaTracker | python | `2028ee4` | |
| PaperReferenceManagerAddons | python | `990d33f` | |

For each app: run `webserver-upgrade`, take the security entry, run the app's
tests, and check sign-in and Settings by eye. Then rotate anything that may
have been exposed:

- Any app with a non-owner admin: check the audit log for `login link
  reissued` entries that target the owner or another admin.
- Any app where `static_dir` was ever changed from Settings: rotate
  `secret.key` (`docker/rotate-key.sh` or `manage`).
