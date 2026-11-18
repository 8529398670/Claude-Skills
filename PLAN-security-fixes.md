# Plan: security fixes in the go-webserver and python-webserver templates, round 2

Written 2026-11-15, from a third security review of E2EEDB. E2EEDB is built
from go-webserver at `236539f`, the current `HEAD` of this repo. Round 1
(S1-S8) is done and is in git history. Every finding below is in template
code, not in E2EEDB's own code, so each app built from these templates
probably has it too. The E2EEDB-only fixes are a separate plan, in
`~/WORKSPACE/PROJECTS/E2EEDB/PLAN-security-fixes.md` (G0-G13).

## Scope and rules

- Phase 1 changes only `~/.claude/skills`, on `master`. Use no branches or
  worktrees. Commits and pushes are left to `gitPushUpdate.sh`.
- Fix both templates in the same pass.
  - **Go: confirmed** means the finding was reproduced with a probe against
    E2EEDB.
  - **Python: check first** means it was found by reading the code. Write a
    failing test before fixing it.
- Every fix gets a regression test in that template's own suite. The test
  must fail before the fix and pass after it.
- Add one entry to each `UPGRADING.md` covering all of these fixes, so
  `webserver-upgrade` offers them as one security update.
- Phase 2, rolling the fixes out to the apps, starts only after phase 1 is
  committed. See the end of this plan.

## S9 (high): an API key can turn itself into a browser session

**Go: confirmed. Python: same shape (`test_only_a_full_role_key_makes_its_own_login_link` asserts it).**

`POST /api/account/login-link` accepts any key that carries its account's full
role. Such a key can:

1. ask for `{"max_uses":0,"expires_in_minutes":0}` and get an unlimited link
   that never expires;
2. redeem the link, which gives an ordinary browser session (unless 2FA is
   on).

Everything marked session-only then opens to that key. In E2EEDB that means:

- the keyring (an offline passphrase guess, or replacing the keyring);
- the owner screens and Settings, which also chains with S2's class of bug;
- minting new keys (201).

An admin key could also reach `reissue-login` for ordinary users.

Code:

- Go: `server/routes/account.go`, `createMyLoginLink`.
- Python: the matching handler in `app/routes/account.py`.
- Apps with MCP: check whether an MCP tool (TodoBoard has
  `create_my_login_link`) offers the same thing to a key.

Fix:

1. Make the route session-only, using the same guard the other session-only
   routes use. Do the same for any MCP tool that mints a login link for the
   caller.
2. If a script really needs a sign-in link, keep a separate opt-in. A key
   flag, `may_sign_in`, off by default, set only from a session. Its links
   are single-use, at most 15 minutes. Done after commit 43, with one change:
   the owner and admins may switch it on for their own keys too (the owner's
   agent key is the reason it exists), and only the key's own account can.
3. Tests:
   - A full-role key gets 403 from the route, and from the MCP tool where
     there is one.
   - A session still gets its link.
   - Update the existing Python test.

## S10 (medium): the rate limiter's key is wrong behind a proxy and on IPv6

**Go: confirmed. Python: check first.**

- **Behind a proxy with `trust_proxy: false`.** Every visitor shares the
  proxy's address, which is the default config behind Cloudflare Tunnel or a
  local proxy. 60 POSTs a minute to `/api/login` then block every sign-in and
  every 2FA code, for everyone.
- **On IPv6.** Each /128 has its own budget. Twelve addresses in one /64 got
  twelve tries against a limit of five.
- **Not logged.** 429s never reach the activity log, because `RequestLog`
  runs after `RateLimit` (`main.go`). So the lockout above cannot be seen.

Code: `server/middleware/middleware.go`, `KeyGenerator`
(`prefix + c.IP()`).

Fix:

1. Key IPv6 clients by their /64 (`netip.Addr.Prefix(64)`). Leave IPv4 as
   it is.
2. When `trust_proxy` is false and a request carries `CF-Connecting-IP`,
   `X-Forwarded-For` or `X-Real-IP`, log one warning per process. The warning
   names the header and points to `server.trust_proxy`.
3. Log 429s, at most one line per bucket per window.
4. Tests:
   - Two addresses in one /64 share a bucket.
   - A warning is logged once.
   - A 429 shows up in the log.

## S11 (medium): API responses never say `Cache-Control: no-store`

**Go: confirmed. Python: check first.**

No API response sends `Cache-Control` or `Vary`. That covers `/api/me`,
`/api/owner/settings`, `/api/account/sessions` and
`/api/owner/logs/days/<date>`, download included. Only static files, SSE,
`/api/language` and error pages send it. The risks:

- **A cache-everything rule.** If a Cloudflare rule caches everything, the
  owner's log for a day is served from the cache at a URL anyone can guess.
- **File extensions.** By default Cloudflare caches any path ending in a
  file extension.

Fix:

1. One middleware, early in the chain, sets `Cache-Control: no-store,
   private` and `Vary: Cookie, Authorization` on every response that the
   static handler did not serve.
2. Test: each route in the generated route table answers with `no-store`.

## S12 (medium): encrypted database records are not bound to where they sit

**Go: confirmed. Python: check how SQLite records are sealed.**

`db.encode` / `db.open` (`server/db/db.go:214-233`) seal every bbolt value
with `ChaChaEncryptBytes(secretKey, raw)` and no associated data.

The attack needs someone who can write `app.db` but does not have
`secret.key`, for example through a backup, a restore pipeline or shared
storage. That person copies the owner's `users` record over another user's
key. The other user's cookie then passes `RequireOwner`, because the
`User.ID` inside the copied record is 1. The same swap works between
sessions, API keys and meta records.

Fix:

1. Seal each value with associated data `bucket || 0x00 || key`, through
   `ChaChaEncryptBound`.
2. Derive one subkey per purpose from `secret.key`, using BLAKE2b or HKDF
   with a label: `db`, `logs`, `cookie`, `totp`, `files`. Then a value from
   one purpose never opens in another.
3. Make the key-rotation path (`db.go:192`) and backup/restore use the same
   functions.
4. Tests:
   - Copying a value to another key fails to open.
   - A cookie sealed for one purpose does not open as a log line.
5. In `UPGRADING.md`, the entry re-seals the database once, as `rotate-key`
   already does.

## S13 (low): the check that refuses trusting every proxy compares strings

**Go: confirmed. Python: check first.**

`server/config/config.go` refuses only the strings `0.0.0.0/0` and `::/0`.
These all pass, and each one trusts every address:

- `1.2.3.4/0`;
- `::1/0`;
- `[0.0.0.0/1, 128.0.0.0/1]`.

The value can be set from Settings and takes effect on restart.

The documented Docker example, `172.16.0.0/12`, also trusts every container
on every bridge network.

Fix:

1. After `ParseCIDR`, refuse a prefix narrower than /8 for IPv4 and /16 for
   IPv6. Cloudflare's widest ranges are /13 and /29.
2. Change the comments in `defaults.yaml` and `config.example.yaml` to
   suggest the exact gateway, `172.17.0.1`. Point to
   `docker network inspect bridge` to find it.
3. Test: each entry above is refused.

## S14 (low): two admin actions skip the rule about acting on another account

**Go: confirmed. Python: check first.**

- **Renaming.** `renameKey` with `own_only=false`
  (`server/routes/api_keys.go:361`) has no ownership check. A non-owner admin
  renamed the owner's `prod-deploy` key to `unused-delete-me`. That could
  lead the owner to revoke the wrong key.
- **Restoring.** Any admin may restore a soft-deleted key
  (`server/models/soft_delete.go`). The owner deleted admin Bob's key, Bob
  restored it, and it was live again.

Fix:

1. **Renaming.** Call `mayActOnIDTx` in `RenameAPIKey`.
2. **Restoring.** Record who deleted the key. If the deleter outranks the
   restorer, only the deleter or the owner may restore it.
3. Tests: both cases above are refused.

## S15 (low): turning on 2FA does not end other sessions

**Go: confirmed. Python: check first.**

An attacker's session stayed signed in after the victim enrolled 2FA
(`server/models/two_factor.go`). The same happens when 2FA is reset.

Fix:

1. When 2FA is confirmed, call `RevokeOtherSessions(user, current)`.
2. When an admin resets 2FA, end every session of that account.
3. Test: after enrolment, the other session gets 401.

## S16 (info): small hardening

1. **Bearer header.** `hasBearer` (`server/security/security.go`) splits only
   on a space. A request sending `Bearer<TAB>key` falls back to the cookie
   and runs as the signed-in account. Split on any whitespace, and treat any
   `Authorization` header as a bearer attempt: a bad one is nobody.
2. **Cookie prefix.** When `secure_cookies` is on, name the cookie
   `__Host-<cookie_name>`. Then an app on a sibling subdomain cannot plant a
   session cookie and sign the visitor in as the attacker. This signs everyone
   out once; say so in `UPGRADING.md`.
3. **The `auth.two_factor` setting.** Switching it off lets enrolled accounts
   sign in without a code. Say that in its comment and on the Settings screen.

## S17: document running behind Cloudflare

Add `references/deploy-cloudflare.md` to both templates, and link to it from
the `trust_proxy` comment.

- **Tunnel, recommended.**
  - Set `trust_proxy: true`, `trusted_proxies: [127.0.0.1, "::1"]` (or the
    exact Docker gateway), `proxy_header: CF-Connecting-IP`,
    `secure_cookies: true`, and `public_url`.
  - Check in Logs that real visitor IPs show up.
- **Proxied DNS.**
  - Use Full (strict) TLS with an Origin CA certificate.
  - Turn on Authenticated Origin Pulls.
  - Firewall the origin so only Cloudflare's ranges reach it.
  - Never use Flexible.
  - Trusting Cloudflare's ranges means trusting every Cloudflare customer,
    which is why Origin Pulls are needed.
- **Dashboard.**
  - Add a Cache Rule that bypasses the hostname, with Cache Deception Armor
    on.
  - Turn off Rocket Loader, Email Obfuscation and automatic analytics
    injection.
  - Add a rate-limit rule on `POST /api/login*`.
  - Keep challenges off `/api/*` when API keys are used.
  - Leave Cloudflare's HSTS off, since the app sends its own.

## Order

S9, S11, S10, S12, then S13, S14, S15, S16 and S17. Run the full test suite of
both templates, then add the `UPGRADING.md` entry.

## Phase 2: roll out with `webserver-upgrade` (after phase 1 is committed)

For each app built from these templates:

1. Run `webserver-upgrade` and take the security entry.
2. Run the app's tests.
3. Check sign-in, Settings and the API keys card by eye.

Start with E2EEDB, where S9 exposes the keyring: that is step G0 of its plan.
TodoBoard has MCP, so test S9 through its MCP login-link tool.

Afterwards:

- **Audit log.** In any app where full-role keys were handed out, look for
  `own login link created` entries made by a key (`by_key`). Revoke those
  keys if any of the entries are unexplained.
- **Database re-seal.** S12 re-seals the database. Take a backup first.

# Round 3 (S18-S21)

Written 2026-11-15, from the fourth security review of E2EEDB. S19-S21 are in
template code (`reissueLogin` and `resetTwoFactor` in
`server/routes/admin.go` are identical to the Go template). The same rules
as above: both templates, a failing test first, one `UPGRADING.md` entry,
then E2EEDB takes it as step H0 of its plan.

## S18: dropped (E2EEDB only)

Not a template change. Keys making login links (`reissue-login`, the
account link route, `may_sign_in`) is a deliberate template option and stays
as it is. E2EEDB turns it off for itself, because a session there reads the
keyring: step H0 of its own plan.

## S19 (medium): an admin can sign in as any account below them

**Go: confirmed.** `reset-two-factor` then `reissue-login` on a user gives
the admin a session as that user: their keyring (offline passphrase
guesses), their plaintext folder names, and replacing the keyring. That is
how links work, so the fix is to make it visible:

1. Record who issued a link on the session it makes (`IssuedBy`), and audit
   "signed in with a link issued by <admin>" on the account's own log.
2. Audit every keyring read, with the session's origin.
3. Apps document it (E2EEDB README, "Known limits").

## S20 (low): disabling or demoting an admin leaves their key's work alive

**Go: confirmed.** `KeyLineageLive` checks only that the key is live. After
the owner disabled Bob, a session from a link Bob's key made stayed signed
in, the link still redeemed, and a key minted from that session worked.
After demotion, a link Bob made as admin still redeemed.

Fix: `KeyLineageLive` also refuses a key whose account is missing, disabled,
or below the role it had when it made the link; or void every link and
session a user's keys made when that user is disabled or demoted. Test: the
three cases above end.

## S21 (info)

1. `skipRequestLog` (`server/middleware/request_log.go`) matches the raw,
   case-sensitive path; normalise it as `budgetFor` does.
2. The `RequireSession` comment in `server/security/security.go` still says
   any key may make its own login link.

## S22 (low): revoke-access and delete-user leave key-made work alive

**Done 2026-11-18** (both templates, `UPGRADING.md` "After 145e4d4"). Found by the round-3 session. `RevokeAllCredentials`
(revoke-access) and delete-user stop a user's keys but do not void the login
links and sessions those keys made in other accounts. Same class as S20: run
`voidWhatKeysMadeTx` / `void_what_keys_made_tx` there too, in both templates,
with a test like S20's.
