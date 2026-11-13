# Upgrading apps built with go-webserver

Read by the `webserver-upgrade` skill. Add an entry whenever
`assets/template/` changes.

## After 5c06754

**Security fixes S1-S8 (2026-11-13 review).** All fixes, so every app gets all
of them; none adds a route, screen or setting. S1 and S2 chained let a leaked
non-owner admin key download `secret.key`, so take this before anything else.
Then, per app: check the activity log for `login link reissued` entries that
target the owner or another admin, and if `server.static_dir` was ever changed
from Settings, rotate `secret.key` (`docker/rotate-key.sh` or `manage`).

- **S1 (high): reissuing or cancelling a login link follows the owner rule.**
  `server/models/owner.go` gains `mayActOnTx`, `mayActOnIDTx` and `MayActOn`:
  nobody acts on the owner, and only the owner acts on another admin (the
  operator, actor 0, and acting on yourself pass). `DeleteUser`,
  `ResetTwoFactor`, `SetUserRole`, `issueAPIKey` and `SetAPIKeyExpiry` call it
  instead of their own copies. `ErrOwnerRequired`'s text is now general.
  `LoginLinkOptions` gains `ActorID`, checked inside `IssueLoginLink`'s
  transaction; `reissueLogin` sets it (`createUser` does not: anyone may make
  an account). `CancelLoginLink( store , user_id , actor_id )` gains an
  argument -- **app code calling it no longer compiles**; pass the account
  asking, or 0 from a shell. `routes/admin.go` gains `actOnRefusal`, the
  shared 403 wording. The MCP tools `admin_reissue_login` and
  `admin_cancel_login_link` go through the same helpers.
- **S2 (medium): paths are file-only, and static_dir may not serve secrets.**
  `server/config/live.go` gains `fileOnlyKeys` (`server.app_dir`,
  `server.static_dir`, `server.language_file`, `docker.data_dir`,
  `build.output_dir`), `FileOnly`, `ErrFileOnly`; `Locked` is true for them
  everywhere, and `KeyState` gains `file_only`. `routes/settings.go` answers
  `settings_file_only`; `language.yaml` gains `settings.file_only` and
  `errors.settings_file_only`; `static/js/settings.js` shows the note.
  `server/assets/assets.go` gains `CheckStaticDir`, which `main.go` calls
  before `assets.Resolve` (and imports `path/filepath`): the server refuses to
  start when `static_dir` is, holds, or sits inside the config dir, the state
  dir, `secret.key` or the database. `server/static/static.go` never serves a
  path with a segment starting with `.` (`hasDotFile`), except `.well-known/`.
  **An app with its own path-typed key** (`*_dir`, `*_file`, `*_path`) must add
  it to `fileOnlyKeys`; `TestEveryPathKeyIsFileOnly` fails until it does. An
  app that serves its own dotfiles from `static/` must move them.
- **S3 (low): the owner rule for every other admin action.** Through the web,
  disabling or re-enabling, revoking access, ending one session, and revoking,
  deleting or restoring someone else's API key now refuse another admin's
  account unless the owner asks, and refuse the owner's. New:
  `SetUserDisabledAs`, `RevokeAllCredentialsAs`, `RevokeSessionAs`,
  `RevokeAPIKeyAs`; `DeleteAPIKey` and `RestoreAPIKey` check the key's account.
  The old names are the operator's (no rule), so the CLI and control socket are
  unchanged. `deleteKeyError` and `restoreRefusal` map the two errors to 403.
  **App routes that change another account** should do the same: call the
  model inside its transaction with `mayActOnTx`, or `models.MayActOn` first.
- **S4 (low): guess limits can no longer be raced.** `models.AttemptGuard`
  gains `Reserve` and `Settle` (`AttemptVoid`, `AttemptFailed`,
  `AttemptPassed`): an attempt is counted in flight before the guess is
  checked, so parallel guesses get at most the limit evaluated. Every caller in
  `two_factor.go`, `join.go` (PIN) and `passwords.go` (password and code) uses
  them; `Locked` and `Record` stay for app code but have the race.
- **S5 (low): `.run/` is private.** `scripts/lib.sh` gains `private_run_dir`
  (umask 077, then `chmod 700 .run`, `chmod 600 .run/server.log`), called by
  `start_in_background`. `scripts/run.bat` writes no log file and says so.
- **S6 (info): log redaction ignores case.** `server/logs/redact.go` matches
  `/login/` and `/join/` whatever their case, keeping the request's spelling,
  so `/LOGIN/<credential>` (which Fiber routes) is logged as `/LOGIN/…`. The
  Referer goes through the same function.
- **S7 (info): a Bearer header wins over the cookie.** `security.LoadUser`
  reads only the key when `Authorization: Bearer` is present (`hasBearer`); a
  bad key is no one. A signed-in browser calling with a narrow key now acts as
  that key.
- **S8: revoking a key signs out what its login links signed in.** `Session`,
  `SignInRules` and the two-factor challenge gain `MintedByKey`;
  `RedeemLoginToken` copies the link's key into the rules, and
  `VoidLinksMintedByKeyTx` also deletes those sessions and waiting challenges.
  Sessions from before this have no key and stay. Invite-link joins are not
  covered: those are new accounts, not a way into one. The MCP login-link
  description says so; `config/openapi.json`, `config/mcp.json` and
  `config/api.md` are regenerated (also for `file_only`).
- New tests, copied whole: `server/models/attempts_test.go`,
  `server/assets/assets_test.go`, `server/static/static_test.go`,
  `server/config/scripts_test.go`, `server/middleware/request_log_test.go`;
  new cases in `admin_test.go`, `api_keys_test.go`, `sessions_test.go`,
  `settings_test.go`, `mcp_test.go`, `two_factor_test.go`, `live_test.go`,
  `schema_test.go`, `logs_test.go`. `TestContainerLocksScriptKeys` now uses
  `docker.memory`, since `docker.data_dir` is file-only.

**Test:** `go test ./...` passes. As a second, non-owner admin: reissuing the
owner's link answers 403; Settings shows `static_dir` as "A path, so it can
only be changed in config/config.yaml"; disabling another admin answers 403.
`scripts/run.sh`, then `ls -ld .run .run/server.log` shows `drwx------` and
`-rw-------`. Setting `static_dir: ./config` in `config.yaml` stops the server
at startup with a `refusing to start` message. `GET /.env` answers 404.

## After d943458

**Upgrade `bootstrap-grid`: layout is Bootstrap's grid, mobile first, and a test keeps it that way.**
Marks: `aside-sticky`, `phone-only`, `server/static/frontend_rules_test.go`

The hand-written CSS grids are gone; every side-by-side layout is a Bootstrap
`.row`. Media queries only add from a breakpoint up, except the stacked phone
tables, now marked `phone-only`. Files puts Upload above its table instead of
beside it, so the table gets the width. A new test fails on a `max-width` query
without `phone-only`, a `grid-template-columns` outside the log calendar, the
theme preview and the shortcut list, or a table not right inside a
`.table-responsive`. An app with its own screens should run the test after
taking this and fix what it reports the same way.

- `static/css/style.css`:
  - The touch block is mobile first: `.btn-sm` and `.dropdown-item` get the
    44px floor as the base, and `@media (min-width: 576px) and (pointer: fine)`
    sets them back to `display: inline-block` / `block` with `min-height: 0`
    (and `min-width: 0` on `.btn-sm`). This replaces
    `@media (pointer: coarse), (max-width: 575.98px)`.
  - `.about-facts` (and its `feature:about` markers), `.view-cards`,
    `.card-span`, `.view-split`, `.view-aside`, `.mcp-clients`,
    `.theme-layout`, `.theme-edit-body` / `-aside` / `-main`, `.theme-list`,
    `.theme-colors`, `.theme-choices`, `.theme-choice` and `.settings-label`'s
    own flex rule are removed. In their place, after `.view .card-title`'s
    measure rules: `@media (min-width: 1400px) { .aside-sticky { position:
    sticky; top: calc(var(--app-navbar-height) + 1rem); } }`.
  - The scroll margin rule is `.view .card { scroll-margin-top: ... }`.
  - The stacked phone tables comment gains a `phone-only:` paragraph; the query
    itself is unchanged.
- `static/index.html`:
  - Files: the `.view-split` div and the `view-aside` class go; Upload is a
    plain card above the Files card, its contents after the `h3` wrapped in
    `<div class="row"><div class="col-md-8 col-xl-6">` so the picker does not
    stretch across the card.
  - About: `<dl class="row mb-0">`, each `dt` `col-sm-4 col-lg-3`, each `dd`
    `col-sm-8 col-lg-9 text-break`.
  - MCP: `#mcp-clients` is `class="d-flex flex-wrap gap-2"`.
- `static/js/themes_editor.js`: a `colorGrid` constant (`row row-cols-1
  row-cols-sm-2 row-cols-xxl-3 g-3`) is the class of the base, fine and button
  color groups, the button colors wrapped in a `div.mt-3`; the editor body is
  `row g-4 align-items-start` with the aside `col-12 col-xxl-4 order-xxl-last
  aside-sticky` and the main `col-12 col-xxl-8`; the theme list and the editor
  are `row g-3 align-items-start` with `col-12 col-md-4 col-xxl-3` and `col-12
  col-md-8 col-xxl-9`; the theme list is `d-flex flex-column gap-1`, the
  choices `d-flex flex-wrap row-gap-2 column-gap-4`, each choice `d-flex
  flex-column gap-1`.
- `static/js/settings.js`: the setting label is `form-label settings-label
  d-flex flex-wrap align-items-center gap-2`.
- `server/static/frontend_rules_test.go`: new, copied whole.
- `references/frontend.md` (skill only): new "Hard rules" section; Desktop
  layout and CSS structure rewritten in Bootstrap's terms.

**Test:** the new test passes. At 375px every screen is one column with no
sideways scroll; tables stack as cards. At 1440px Files shows Upload above a
full-width table; Appearance shows the theme list beside the editor, and from
1400px the preview to the right of the colors, staying in view while they
scroll. About lines its facts up in two columns from 576px.

**The API reference is written from the code: `config/openapi.json`, `config/mcp.json` and a new `config/api.md`.**

Nothing in `config/` is edited by hand any more. Each handler declares its docs
once with `apidoc.Doc` in a `server/routes/<area>_api.go` beside it, with Go
types for its body and reply; `scripts/api_docs.sh` writes the three files. The
template's `openapi.json` is the old one regenerated: same content, keys sorted,
`required` lists sorted, nullable numbers as `"type": [T, "null"]`, and
`x-feature` kept on paths. A few things the old file had wrong are fixed: `Me`
gains `files_enabled`, `invite_links_enabled` and `webhooks_enabled`; `Session`
gains `label` and `ip_name`; `SettingsResult`'s lists may be null.

- `server/apidoc/` (apidoc.go, schema.go, encode.go, markdown.go, check.go): new, copied whole.
- `server/routes/*_api.go` (23 files, one per area, `schemas_api.go` for the
  shared shapes): new, copied whole. An app's own routes need a Doc too.
- `server/routes/api_docs_test.go`: new. Holds `newParityFixture` (moved from
  `mcp_parity_test.go`), `TestEveryRouteIsDocumented`, `TestAPIDocsAreCurrent`
  with the `-update` flag, and `checkReplies`.
- `server/routes/mcp_parity_test.go`: `specOperations` builds the spec from the
  code; the `-update` flag, `TestMCPManifestIsCurrent` and `newParityFixture`
  move out; `mcpManifest` encodes with `apidoc.Encode`.
- `server/routes/openapi_test.go`: the route-table test goes (replaced above).
- `fixture_test.go`, `monitoring_test.go`, `mcp_test.go`, `settings_test.go`:
  `app.Use( checkReplies( t ) )` before the routes, so every 2xx JSON reply is
  checked against its docs.
- `scripts/api_docs.sh`: new. `scripts/mcp_json.sh`: removed.
- `config/api.md`: new, generated. `config/openapi.json`, `config/mcp.json`: regenerated.
- Comments naming `mcp_json.sh` or a hand-written spec: `main.go`,
  `server/routes/openapi.go`, `mcp_manifest.go`, `mcp_tools.go`, `server/mcp/registry.go`.

An app with routes of its own: `TestEveryRouteIsDocumented` names each one
without docs. Write their Docs from the app's old `openapi.json` entries (same
summary, fields and limits), run `scripts/api_docs.sh`, and compare the new
spec with the old.

**Test:** `scripts/api_docs.sh`, then `go test ./...` passes, and a second
`scripts/api_docs.sh` changes nothing.

**Upgrade `keys-manage-accounts`: API keys and MCP agents can make accounts, login links and invite links, and sign devices out; revoking a key voids the links it made.**
Marks: `VoidLinksMintedByKeyTx`, `minted_by_key`, `server/routes/mcp_tools_accounts.go`

`RequireSession` comes off the account, login-link, device and invite-link
routes, so an admin key (and the matching MCP tools) can create, rename,
re-role and delete accounts, reset two-factor, hand out and cancel login links,
manage invite links, and list or end an account's sessions; any key can list
and end its own sessions and make its own account a login link. Key
management, your own password/PIN/two-factor, other people's files and the
owner's screens stay browser-only. Two rules keep it contained: a key whose
role is below its account's may not make its own login link, and every login
link and invite link records the key that made it (an invite link a key grows,
switches on or restores becomes that key's), so revoking or deleting the key
voids the ones still usable. The MCP login-link tools answer with a full
`login_url`; the HTTP routes still answer with `login_path`.

- `server/models/login_token.go`: `LoginToken.MintedByKey` and
  `LoginLinkOptions.MintedByKey` (`json:"minted_by_key,omitempty"`), stored by
  `IssueLoginLink`; new `VoidLinksMintedByKeyTx( tx , key_id )` before
  `deleteLoginTokensForUser`, its invite-link half in a `feature:invite_links`
  block.
- `server/models/invite_link.go`: `InviteLink.MintedByKey`; `IssueInviteLink`
  gains a last `minted_by_key string` argument (`IssueInviteLinkAs` passes
  `""`); `SetInviteLinkLimits`, `SwitchOnInviteLink` and `RestoreInviteLink`
  gain a last `minted_by_key` and set it when non-empty; new
  `revokeInviteLinksMintedByKeyTx`. Update the app's own callers and tests.
- `server/models/api_key.go`: `RevokeAPIKey` and `DeleteAPIKey` run in
  `store.Write` and call `VoidLinksMintedByKeyTx` in the same transaction.
- `server/security/security.go`: new `Actor.KeyID()`; `RequireSession`'s
  comment rewritten.
- `server/routes/routes.go`: `handlers.Guard.RequireSession` removed from
  `/account/login-link`, the four `/account/sessions` routes, `/users`,
  `reissue-login`, `rename`, `role`, `delete`, `reset-two-factor`, the two
  admin `sessions` routes and the six invite-link routes; their comments
  rewritten.
- `server/routes/admin.go`, `account.go`, `sessions.go`, `invite_links.go`:
  each of those handlers keeps only reading the request and calls a new
  lower-case helper returning `( fiber.Map , *refusal )` (`createUser`,
  `reissueLogin`, `cancelLoginLink`, `renameUser`, `setUserRole`,
  `resetTwoFactor`, `deleteUser`, `createMyLoginLink`, `cancelMyLoginLink`,
  `mySessionRows`, `userSessionRows`, `labelMySession`, `revokeMySession`,
  `revokeMyOtherSessions`, `revokeUserSession`, `createInviteLink`,
  `renameInviteLink`, `setInviteLinkLimits`, `switchOnInviteLink`,
  `deleteInviteLink`, `restoreInviteLink`). `loginLinkOptions` returns a
  `*refusal` and sets `MintedByKey` from `actor.KeyID()`. `createMyLoginLink`
  refuses a key whose `actor.Role != actor.User.Role`. `sessionsTarget` takes
  the user id and returns `( by_owner , refused )`; a new `currentSessionID( c )`
  is `""` through a key, so no row is current and revoke-others ends them all.
- `server/routes/mcp_tools_accounts.go`: new, copied whole; `mcp_tools.go`
  calls `handlers.registerAccountTools( server )` before
  `registerFileTools`, and its header and `admin_revoke_access`'s description
  change.
- `server/routes/*_api.go`: those routes lose `Access: apidoc.SessionOnly`;
  `CancelMyLoginLink` and `CancelLoginLink` lose their `MCPHTTP`.
- `server/mcp/registry.go`, `api_keys.go`: comments.
- Tests: `fixture_test.go` gains `withKey` and `loginWith`;
  `TestAnAdminKeyCannotChangeAnAccount`, `TestAKeyCannotReopenOrGrowAnInviteLink`,
  `TestAnAPIKeyCannotManageDevices` and `TestAKeyCannotNameADevice` are
  replaced by tests that the key can, plus `TestOnlyAFullRoleKeyMakesItsOwnLoginLink`,
  `TestRevokingAKeyVoidsTheLinksItMade`,
  `TestDeletingAKeySwitchesOffTheInviteLinksItMade` and `TestMCPAccountTools`.
  An app with its own tests that expect 403 from these routes for a key flips
  them.
- `config/`: run `scripts/api_docs.sh`.
- `SKILL.md`, `references/architecture.md` (skill only): "Why a key cannot
  create credentials" becomes "Keys and the ways in they hand out".

**Test:** `go test ./...` passes. With an admin key, `POST /api/admin/users`
answers 201 and its `login_path` signs in; revoking the key, a second such link
no longer does. The admin's user-scoped key gets 403 from
`POST /api/account/login-link`. Over `/mcp`, `admin_create_user` answers with a
`login_url`.

## After 2028ee4

The Claude Code command on the MCP card now adds the server with `--scope user`,
so it is set up for every project, not just the folder it was pasted in.

- `static/js/mcp.js`: in `snippet()`, the `claude_code` case returns
  `"claude mcp add --scope user --transport http " + ...`.

**Test:** on Account, the MCP card with Claude Code picked shows
`claude mcp add --scope user --transport http <name> <url> --header ...`.

## After 981a796

**Upgrade `logs-date-centred`: the Logs date button sits in the middle of the search bar.**
Marks: `dropdown-center log-date-picker`, `justify-content-md-end`

The search field stays on the left and Live/Prune move to the right end, with
the date button centred between them; its calendar opens centred under it.
On a phone the three stack, the buttons centred. Bootstrap columns only, no CSS.

- `static/index.html`: in `#logs-search-form` (still `row g-2
  align-items-end`) the field's column becomes `col-md`. The
  `.log-date-picker` dropdown moves out of the `.row-actions` div into its own
  `col-md-auto text-center` column between them, and gains `dropdown-center`.
  That div, now holding only `#logs-live` and `#logs-prune`, becomes
  `col-md row-actions justify-content-center justify-content-md-end`. The two
  `col-md` columns being equal is what centres the button.

**Test:** at 1400px wide the date button is centred in the Logs card, Live and
Prune at its right edge; the calendar opens centred under the button. At
375px the field, the date button and Live/Prune stack, the buttons centred.

**Upgrade `theme-mode-colors`: every theme has light and dark colors, Light and Dark are one theme called Default, and WSU dark is WSU switched to dark.**
Marks: `setMode(`, `modeFields`, `id: "default"`, `ThemeColors`

The Light/Dark switch on a theme used to change only the mode, leaving its
light colors on a dark page. Now a theme can keep its colors for the mode not
in use (a light theme's `dark`, a dark theme's `light`) and the switch swaps
them in; switching back brings the old ones back. Every built-in carries both,
so the separate Light, Dark and WSU dark built-ins are gone: Default (first,
light) and nine others, 10 in all (was 12).

- `static/js/theme.js`: `builtIn` is replaced whole -- `default` first
  (light, with `dark`), and each of the others with its other mode's colors
  (`dark` on the light ones, `light` on midnight, slate, cerise); `wsu` takes
  the old WSU dark colors as its `dark`. `copy()` copies `light`/`dark`; new
  `modeFields` and `setMode( theme , mode )`; `current` starts `"default"`.
  `restore()` reads a kept `"light"` or `"dark"` (an id from before) as
  Default in that mode. `keep()` treats Default in light as plain. Without
  `appearance`: new `shown`, and `set`/`pick` take a mode and build Default
  in it; `watchToggle` reads `shown.mode`. Comment tweaks in the header and
  `derived()`.
- `static/js/themes_editor.js`: the mode radio calls the new `actions.mode`,
  which runs `Theme.setMode` on the edited copy.
- `scripts/check_themes.js`: also checks each built-in switched to its other
  mode.
- `server/models/themes.go`: `Theme` gains `Light`, `Dark *ThemeColors` (`json:"light,omitempty"`, `"dark"`); new `ThemeColors` (accent, second, background, surface, overrides) and `cleanThemeColors`, which `CleanThemes` uses for the theme's own colors and for the kept set. The set for the mode in use is dropped. `server/routes/themes_test.go`: two refused shapes and a kept-`dark` case in `TestThemesRefuseBadShapes`.
- `config/openapi.json`: the `Theme` schema gains `light` and `dark`.
- `config/language.yaml`: `theme.preset_light`, `preset_dark` and
  `preset_wsu_dark` go; `theme.preset_default: "Default"` comes in.

**Once, for this app's data (not a template change), only with
`appearance`:** saved lists keep what they had. Look at the admin's defaults
(meta `theme_defaults`) and each account's `themes` and `theme_id`. Nothing
saved anywhere: nothing to do. Otherwise ask the owner, then mend them once --
by a migration appended to this app's own migrations list, or by hand with the
server stopped (snapshot the database first). In each saved list:

- `light` and `dark` become one `default`: `light`'s theme (name "Default"
  unless renamed) with `dark`'s colors (`accent`, `second`, `background`,
  `surface`, `overrides`) as its `dark`. `wsu` and `wsu_dark` the same way: the
  gold-on-black colors are `wsu_dark`'s own while its `mode` is `dark`, else
  its `dark` set if it has one, else its own. Names and button styles stay with
  the theme that is kept. A pair missing one half is left alone.
- Each other built-in by id (ocean ... cerise) that keeps no colors for its
  other mode, or keeps a copy of its own (switched to Dark and back before
  this), gets the built-in's.
- An account whose `theme_id` was `dark` or `wsu_dark` gets `default` or
  `wsu`; if that account has its own list, that theme is also switched to dark
  (its light colors move to `light`, the dark ones on top), so it looks as it
  did. `light` becomes `default`.

"Restore the built-in themes" does the same for the defaults, at the cost of
any edits. Apps without `appearance` keep nothing on the server: a browser's
kept `"light"`/`"dark"` is read by `restore()`, and a kept whole theme from
before paints as it was until the button is pressed.

**Test:** `node scripts/check_themes.js` says `ok: 10 themes`; `go test ./server/routes -run Theme`
passes. On Appearance the list starts with Default and has no Light, Dark or
WSU dark. Edit WSU and press Dark: gold on black with dark green cards; Light:
green on off-white again. Do the same on Ocean (dark teal page) and Midnight
(press Light: purple on pale lavender). Undo after each puts the other back.
Without `appearance`: a fresh browser opens light; the navbar button goes dark
and back, and the choice survives a reload.

## After ceb3221

**Upgrade `mcp-parity`: every route a key may call has an MCP tool, `config/mcp.json` lists them, and the MCP card links it.**
Marks: `x-mcp-http`, `mcp_url`, `mcp-spec-link`, `files_start_upload`, `TestEveryKeyRouteIsAnMCPTool`

The rule is now "whatever a key can do over HTTP, an agent can do through
`/mcp`": 17 new tools (`rename_account`, `set_shortcuts`, `set_themes`,
`set_theme`, `admin_get_theme_defaults`, `admin_set_theme_defaults`, and for
files `files_list`, `files_get`, `files_start_upload`, `files_finish_upload`,
`files_download_url`, `files_rename`, `files_delete`, `files_restore`,
`my_storage`, `admin_list_files`, `admin_storage`). File bytes still go over
`/api/files` with the same key; the tools answer the addresses. Route guards
are unchanged: nothing a key could not do before.

- `server/mcp/registry.go`: `Tool.Route`, `Server.Tools()`, `Tool.Listing()`
  (tools/list uses it); the doc comment states the rule.
- `server/routes/admin.go`: `refusal.extra`, `refuse()`, `internalFailure()`;
  `send` adds `extra` to the body. `routes.go`: `restoreRefusal`, which
  `restoreRefused` now calls; `Handlers.MCPManifest`, `Handlers.MCP`;
  `GET /api/mcp.json` (`RequireMCP`). New `server/routes/mcp_manifest.go`.
- Handlers split into a helper both doors call, same answers as before:
  `renameAccount` (account.go), `setMyShortcuts` (shortcuts.go),
  `setMyThemes`, `setMyTheme`, `themeDefaults`, `setThemeDefaults`
  (themes.go), and in files.go `myStorage`, `allStorage`, `myFileRows`,
  `allFileRows`, `createFile`, `getMyFile`, `finishFile`, `renameFileAs`,
  `deleteFileAs` (replaces `removeFile`), `restoreFileAs`. An app that changed
  one of these handlers keeps its change inside the helper.
- `server/routes/mcp_tools.go`: `Route` on every tool, the new tools,
  `answer()`, `registerFileTools`. An app's own tools need a `Route` too, or
  the parity test fails: add it, or an `x-mcp-http` reason on the operation.
- `account.go`: `mcp_url` in `/api/me` and `mcpURL()`.
- `main.go`: embeds `config/mcp.json` (new, generated) into
  `handlers.MCPManifest`. New `scripts/mcp_json.sh`. Run it in the app after
  merging (its own tools then appear in the file) and commit the result.
- `config/openapi.json`: `/api/mcp.json`; `x-mcp-http` on logout,
  `logs/events`, `openapi.json`, `mcp.json`, `/mcp`, `/metrics`, the file
  chunk route and both login-link cancels; `Me.mcp_url`. Fixed: invite link
  `rename`, `limits` and `switch-on` listed `apiKey` but need a session.
- `static/index.html`: `mcp-spec-line` on the MCP card, `about-mcp-*` on
  About. `static/js/mcp.js` and `about.js` fill them. `language.yaml`:
  `mcp.spec_text`, `mcp.spec_link`, `about.mcp_reference`, `about.mcp_link`.
- Tests: new `server/routes/mcp_parity_test.go` (every key route has a tool or
  a reason; the spec's `security` matches the guards; `config/mcp.json` is
  current; `/api/mcp.json` signed in only; `mcp_url`), and
  `TestMCPFilesTravelOverHTTP` in `mcp_test.go`.

**Test:** `go test ./server/routes` passes (an app route with no tool fails
here, naming it). `scripts/mcp_json.sh` leaves `config/mcp.json` unchanged.
Account, MCP card: "What an agent can call: MCP reference (mcp.json)" opens
`<repo>/blob/HEAD/config/mcp.json`, or `/api/mcp.json` with no repo. With an
MCP client, `files_start_upload`, a curl of the piece to `chunk_url`, then
`files_finish_upload` makes a file that downloads intact.

## After 99445cc

**Upgrade `theme-buttons`: every Bootstrap button is drawn in the theme, and each theme has a button style.**
Marks: `--app-btn-radius`, `btnDanger`, `buttonChoices`, `theme-preview-buttons`

Until now only `btn-primary`, `btn-outline-primary`, `btn-outline-secondary`
and `btn-link` followed the theme; danger, success, warning, info, secondary,
light and dark (solid and outline) were stock Bootstrap. Now each has a fill,
label and outline color worked out per theme (readable, overridable), and a
theme can carry `button: { shape , weight , depth , case }`. Left out (as
in every built-in theme) it is `buttonDefault`: rounded corners, normal
weight, raised, as written -- so buttons change shape in every app that takes
this, not just their colors. The Appearance editor gains a Buttons
section and its preview shows every variant as real buttons.

- `static/js/theme.js`: `variants`, `semantic`, `buttonHarmony`,
  `buttonDefault`, `buttonChoices`; `button`, `buttonStyle`, `buttonField`,
  `asFill`; `derived()` adds `btn<V>`, `btn<V>Fg`, `btn<V>Text`; `apply()` sets
  the shape properties; `copy()` copies `button`; `keep()` compares it; a loop
  after `Theme` adds the new `vars` and `pairs`. Take the file whole if the app has not changed it.
- `static/css/style.css`: `--app-btn-*` tokens in both `:root` blocks; the
  token mapping selector gains `.theme-preview[data-bs-theme]`; a
  `.btn-<v>`/`.btn-outline-<v>` block per variant and the `.btn` shape rules
  before `.btn { --bs-btn-focus-box-shadow`; `.theme-preview-button` is
  replaced by `.theme-preview-buttons`, `.theme-choices`, `.theme-choice`.
- `static/js/themes_editor.js`: `colorLabel`, `checkLabel`, `actions.button`,
  `choiceGroup`, the Buttons section in `editPane`, and `preview()` drawing
  real buttons. Take the file whole if the app has not changed it.
- `server/models/themes.go`: button choices checked (`button`), and `btn<V>`, `btn<V>Fg`,
  `btn<V>Text` added to the override names.
- `config/language.yaml`: `undo.theme_button`; under `theme:` the
  `buttons_*`, `button_*`, `variant_*`, `color_btn_*` and
  `check_button_variant`/`check_outline_variant` keys.
- `config/openapi.json`: `Theme` gains `button`; the `overrides` description
  names the button fields.
- "Colour" is spelled "color" everywhere, in what people read and in code:
  `language.yaml` keys `theme.colour_*` -> `theme.color_*` and
  `undo.theme_colour` -> `undo.theme_color` (and "Colours" -> "Colors" in the
  text); `Theme.colours()` -> `Theme.colors()`; style.css classes
  `.theme-colours`, `.theme-colour-row`, `.theme-colour-input` -> `.theme-color*`;
  `colourRow`, `colourLabel` in themes_editor.js; the model's `themeColour` /
  `_colour` helpers and error messages. An app's own code or language.yaml
  that uses the old names needs them renamed too (search the app for
  `colour`). Older entries below keep the names as they were then.
- Tests: `server/routes/themes_test.go` (bad button choices refused; a button style and `btnDanger`
  kept; an empty `button` not stored). `scripts/check_themes.js` is unchanged
  and now covers the button pairs.

**Test:** `node scripts/check_themes.js` says `ok: 12 themes`. Every screen's
buttons are rounded and raised. On Appearance, pick WSU: the preview shows
every variant in WSU colors with Corners Rounded, Weight Normal, Depth Raised,
Letters As written selected. Switch Corners to Pill and Use the theme: buttons
on every screen (Users' row buttons, the danger buttons) are pill-shaped. Set
a danger fill to a pale color: the hard-to-read box names it, and Reset puts
the worked-out color back.

## After cce0220

**Upgrade `app-repo`: `app.repo` says where the source lives, and the API keys card links the API reference.**
Marks: `repo:`, `openapi_url`, `key-spec-link`

Three new `app:` keys in `config/defaults.yaml`, all `[live]`: `repo` (empty, or
an https repository address; a trailing `/` or `.git` is dropped), `description`
(one line, 500 characters at most) and `show_about` (see `about` below). The
API keys card gains one line under its description, "What a key can call: API
reference (openapi.json)", linking `<repo>/blob/HEAD/config/openapi.json` when
`app.repo` is set and this server's own `/api/openapi.json` otherwise. `scripts/scaffold.py` takes `--repo`, and without it uses a `github.com/...` module path.
webserver-upgrade keeps an app's own `app.repo` through its renders, like the port.

- `config/defaults.yaml`: the `repo` and `description` keys after `app.name`,
  then the `show_about` block in `# >>> feature:about`. Regenerate
  `config/config.example.yaml`.
- `server/config/config.go`: `AppSection` gains `Repo`, `Description` and `ShowAbout` (`// feature:about`); validation (`validRepo`), `cleanRepo`, `Config.Repo`/`Description`/`ShowAbout` and `( *Config ) RepoFile`. `server/config/live.go`: `liveConfig` copies the three.
- `server/routes/account.go`: `/api/me` gains `repo`, `description`, `show_about` (`// feature:about`) and `openapi_url` (`// feature:openapi`, from the new `openAPIURL`), after `announcement`.
- `config/openapi.json`: the `Me` schema gains `repo`, `description`,
  `show_about` and `openapi_url` (strings, `show_about` boolean).
- `static/index.html`: the `feature:openapi` `#key-spec-line` paragraph after
  `api_keys.description`.
- `static/js/keys.js`: the `feature:openapi` block in `init` that fills
  `#key-spec-link` from `me.openapi_url`.
- `config/language.yaml`: `api_keys.spec_text` and `api_keys.spec_link`, in
  `# >>> feature:openapi`.
- Tests: `server/config/live_test.go` (`TestRepoIsLiveAndCleaned`), `server/config/yamledit_test.go` (a valid `app.repo` sample), `server/routes/about_test.go` (new).

**Test:** with `app.repo` empty, the API keys card's link opens
`/api/openapi.json`. Set `app.repo` to `https://github.com/you/app.git` in
Settings and reload: the link is
`https://github.com/you/app/blob/HEAD/config/openapi.json`. `http://...` or
`github.com/you/app` is refused with a message.

**Upgrade `about`: an About screen in the menu, switched on by `app.show_about`.**
Needs: `app-repo`
Marks: `about-panel`, `About.init`, `about-facts`

A new screen, About, in the sidebar after Files, shown only while
`app.show_about` is on (off by default; the Settings screen has the switch).
It shows `app.description`, the running version, the repository link and the
API reference link (`feature:openapi`), each row only when it has a value. It
is all `feature:about`, a part every project has unless `--without about`.

- `scripts/scaffold.py`: `"about": ()` in `PIECES`.
- `static/js/about.js`: new, whole file.
- `static/index.html`: the About sidebar link after Files (`<!-- feature:about -->`),
  the `feature:about` `#about-panel` section after the Files screen, and
  `<script src="/js/about.js">` after `files.js`.
- `static/js/app.js`: `About.init( me ); // feature:about` after `Files.init`.
- `static/css/style.css`: the `feature:about` `.about-facts` block after
  `.view-description`.
- `config/language.yaml`: `nav.about`, and the `feature:about` `about:` section
  after `mcp:`.

**Test:** turn on Show About in Settings and reload: About appears in the menu
for every signed-in account, with the description, version, Source and API
reference rows. With `app.repo` empty the Source row is gone. Off again and
reloaded, the menu entry is gone and `#about` lands on Account. At 375px a
long repository address wraps instead of scrolling sideways.

## After b132ece

**Upgrade `mcp-card`: an MCP card on Account sets up Claude and other agents.**
Marks: `mcp-panel`, `mcp_enabled`, `Mcp.init`

A new card on Account, right after API keys, for connecting an agent to
`/mcp`. It shows the endpoint, lets you pick the client (Claude Code, Claude
Desktop, Other), makes an API key with the API keys card's
own create call, and shows the setup to paste for that client with the key
already in it: the `claude mcp add` command, Claude Desktop's config through
`mcp-remote`, or the plain JSON other clients read. Until a key is made there
the setup shows `YOUR_API_KEY`. The key is shown once, held only in the page,
and is listed under API keys like any other. Nothing new on the server beyond
one flag; the card is all `feature:mcp`, so an app without MCP gets none of it.

- `server/routes/account.go`: `/api/me` gains `"mcp_enabled": handlers.cfg().MCPEnabled, // feature:mcp`, after
  `api_keys_enabled`.
- `config/openapi.json`: the `/api/me` schema gains `mcp_enabled` (boolean).
- `static/js/mcp.js`: new, whole file.
- `static/index.html`: the `feature:mcp` `#mcp-panel` section after the API
  keys card, and `<script src="/js/mcp.js">` after `keys.js`.
- `static/js/app.js`: `Mcp.init( me ); // feature:mcp` after `Keys.init`.
- `static/css/style.css`: the `feature:mcp` block (`.mcp-clients`,
  `.mcp-snippet`) after `.link-box code`.
- `config/language.yaml`: the `feature:mcp` `mcp:` section after `api_keys:`.
  An app that renamed itself in `app.title` gets that name, as a slug, as the
  server name in the setup.

**Test:** on Account, the MCP card sits under API keys and shows
`<origin>/mcp`. Each client button changes the hint and the setup, and the
key name follows until you type your own. Create key fills the real key into
the setup, shows the notice, and the key appears in the API keys table;
closing the notice puts `YOUR_API_KEY` back. The Claude Code command, run as
shown, lets `claude` list the app's tools. With `server.mcp: false` (or
`auth.api_keys: false`) the card is gone. At 375px the client buttons wrap
and nothing scrolls sideways.

## After 8232d6e

**Upgrade `desktop-layout`: screens lay out for a desktop by what they are.**
Marks: `.view-split`, `.view-full`, `.theme-edit-body`, `logs-date-button`, `#signed-in) {`

Each screen's desktop layout now follows what the screen is (the table in
`references/frontend.md`, "Desktop layout"). Tables get the whole width,
and a screen goes wider only together with a layout that uses the room.
Screens nobody has laid out keep their 60rem column and look exactly as
before. Three screens change:

- Appearance (`view-full`) shows the colours in two to four columns, with the
  preview and the hard-to-read warning beside them, staying in view.
- Files (`view-wide`) puts the list in the main column and Upload in a sticky
  aside on the right from 1440px. Below that, it is exactly as before.
- Logs (`view-full`) gives the day's log the whole width. The month calendar
  moves into a popup that opens from a date button in the search bar.

Phones look the same as before.

- `static/css/style.css`:
  - In `.app-main`, drop `max-width: 60rem` and `margin: 0 auto`, and add the
    `.app-main > :not(#signed-in)` rule after it.
  - Replace the `.view` block (from "A screen is its heading" to the
    `[role="search"]` rule) with the template's: the width tiers (default
    60rem, `view-narrow` 48rem, `view-wide` 80rem, `view-full` 120rem), the
    70ch prose rule, `.view-cards`, `.card-span`, `.view-split` /
    `.view-aside` (side by side from 1440px), and the search-input cap.
  - In `feature:appearance`, take the template's `.theme-layout` media rules,
    `.theme-edit-body` / `-aside` / `-main`, `.theme-colours` and
    `.theme-colour-row`, and delete the old under-576px `.theme-colours` rule.
  - In `feature:logs`, `.log-calendar` loses its `margin-bottom`, and
    `.log-cal-menu` is added.
- `static/js/themes_editor.js`: `colourRow` answers one element (label and
  controls together) instead of two cells, and its two callers append it.
  `editPane` puts the preview and the warning box in `.theme-edit-aside` and
  the colour sections in `.theme-edit-main`, both inside `.theme-edit-body`.
  An app that changed this file merges by hand.
- `static/js/logs.js`: `init` creates the date button's `bootstrap.Dropdown`
  (`autoClose: "outside"`, 16px from the window's edges). `pick` closes it and
  focuses the button for a pick made on the calendar, instead of focusing the
  picked cell. `renderCalendar` writes the day shown into `#logs-date-label`.
- `static/index.html`:
  - Appearance gets `view-full`.
  - Files gets `view-wide`, and its two cards go in
    `<div class="view-split">`, with `view-aside` on the Upload card.
  - Logs gets `view-full`, and the search field's column becomes
    `col-md-6 col-xl-4`. The `.log-calendar` block moves out of the day card
    into a `.dropdown.log-date-picker` before `#logs-live`: a
    `#logs-date-button` toggle (with a `#logs-date-label` span) and a
    `.dropdown-menu.log-cal-menu` holding the calendar. `#logs-empty` and
    `#logs-day` stay in the day card.

An app's own screens keep their 60rem column. Each one can be given a
desktop layout later by deciding what it is, using the guide's table, and
looking at it.

**Test:** at 1600px wide:

- Appearance shows the theme list, the colours in two or more columns, and
  the preview beside them, still in view after scrolling down. Changing a
  colour to something unreadable shows the warning beside the colours, and
  Ctrl+Z puts it back.
- Files shows the list on the left and Upload on the right.
- Logs shows the day's log across the whole card. The date button opens the
  calendar; the month arrows leave it open; picking a day closes it, shows
  that day, and puts the new date on the button.
- Account, Shortcuts and Users are unchanged.

At 1280px, Files looks as it did before. At 375px, every screen is one column
with no sideways scroll, Files shows Upload first, and the calendar popup
stays inside the screen.

## After 990d33f

**`static/js/api.js`, the admin theme defaults:** `themeDefaults()` and
`setThemeDefaults()` sat inside the `feature:files` block, so an app built or
updated without `files` lost them. Its Appearance screen then failed at sign-in
with `Api.themeDefaults is not a function`, and the page stayed on the spinner.
They now sit in the `feature:appearance` block after `setTheme`. An app without
`files` that took `editable-themes` adds the two lines there by hand, unless it
already has them. An app with `files` needs nothing.

**Test:** in an app without `files`, sign in: the signed-in screens load, and
an admin's Appearance screen shows the "Defaults for everyone" tab.

## After 3d4ff05

**Upgrade `editable-themes`: Themes are kept on the account, editable, with admin defaults.**
Marks: `themes_editor.js`, `Theme.builtIn`, `/api/admin/theme-defaults`, `--app-text`

The Appearance screen was a row of fixed presets remembered in one browser.
Now each person has their own list of themes on their account -- use, new,
duplicate, rename, delete -- and edits any colour with a colour picker: the
mode, the four base colours, and every colour worked out from them (text,
links, borders, tints, sidebar...), each with a Reset back to the readable one
and a warning for text that falls under 4.5:1. Admins get a "Defaults for
everyone" tab: the list everyone starts from. People who have not changed
their own list follow it, later edits included; the first change someone
makes saves the whole list as theirs, and "Go back to the defaults" undoes
that. Light is still the first theme and the first-load default.

Server: add `server/models/themes.go`, `server/routes/themes.go` (+ `themes_test.go`) whole; add `User.Themes` / `User.ThemeID` in `server/models/user.go`; in `server/routes/account.go` (`meRow`) answer
`themes`, `themes_custom` and `theme_id`; in `server/routes/routes.go` add
`POST /api/account/themes`, `POST /api/account/theme`, and
`GET`/`POST /api/admin/theme-defaults` (admin group). `config/openapi.json`:
take the four paths, the `Theme` schema, and the three new `Me` properties.

Frontend: take `static/js/theme.js` and `static/js/themes_editor.js` whole, and
`scripts/check_themes.js`. `Theme.presets` is now `Theme.builtIn`, each `key` is
now `id`, `Theme.render` is gone (`ThemeEditor.init( me )` in `app.js` instead),
and `localStorage["theme"]` now holds the whole theme as JSON (an old preset
key is still read). An app that added its own presets moves them into
`builtIn` with `id` for `key`. In `static/js/api.js` add `setThemes`,
`setTheme`, `themeDefaults`, `setThemeDefaults`. In `static/index.html` take
the Appearance section (tabs, `#theme-editor-mine`, `#theme-editor-defaults`)
and the `themes_editor.js` script tag. In `static/css/style.css` add
`--app-text` to both `:root` blocks and `--bs-body-color: var(--app-text);` to
the hand-on block, and take the `feature:appearance` block. In `language.yaml`
take the `theme:` block and the `theme_*` labels under `undo:`.

**Test:** Pick a theme, reload, and sign in from another browser: it follows.
Make a new theme, duplicate, rename and delete one, undoing each. Set Links to
the card colour under Fine-tune: the row and a box flag it, and Reset brings
back the readable colour. As an admin, duplicate a theme on "Defaults for
everyone": a user who never changed their list sees it at once; a user who
did, does not; that user's "Go back to the defaults" brings it. A fresh
browser still opens light.

**Upgrade `steady-scroll`: Nothing moves under the reader.**
Marks: `Dom.steady`, `Dom.swap`, `Dom.reveal`, `scrollbar-gutter`

A press no longer makes the page jump. Before, toggling a kind button on Logs
("System", "Sign-in") cleared the day, waited for the server, and filled it
again, so the page shrank for a moment and the scroll position was lost. Live did
the same every two seconds and threw away loaded "Load more" pages.

Now every press goes through `Dom.steady`, which keeps the pressed button (or what
replaced it) at the same spot on screen. New content is built first and put in
with `Dom.swap`, which never lets the page shrink. Logs flips its kind buttons in
place, reads a day's first page before showing it, and Live adds only the new
rows above the ones being read. Deliberate scrolls (login link box, webhook
secret, error banner, Settings jump links, changing screen) go through
`Dom.reveal` / `Dom.movedOnPurpose`, so the guard stands back for them. A dev
build warns `[steady]` in the console when it had to correct.

Take `static/js/dom.js`, `pager.js` and `logs.js` whole. In `app.js`,
`sessions.js` and `webhooks.js`, each `scrollIntoView( { block: "nearest" } )`
becomes `Dom.reveal( element )`. In `nav.js` `render()`, the card scroll is
`Dom.reveal( card , "start" )` and the `scrollTo( 0 , 0 )` follows
`Dom.movedOnPurpose()`. In `settings.js` `jumpTo()`, call `Dom.movedOnPurpose()`
before `window.scrollTo`. In `jobs.js` the rows go in with `Dom.swap`. In
`shortcuts.js` the two refocus calls pass `{ preventScroll: true }`. In
`style.css`, add `html { scrollbar-gutter: stable; overflow-anchor: auto; }`
above `#error-banner`. An app's own screens should follow the rules in
"Nothing moves under the reader" in `references/frontend.md`: in particular, a
`Dom.clear` followed by an `await` and a fill becomes fetch, build, `Dom.swap`,
and its own `scrollIntoView` calls become `Dom.reveal`, or `Dom.steady` will
undo them.

**Test:** On Logs, scroll partway down a long day and toggle System, Sign-in and
Request a few times: the kind buttons stay put and nothing jumps. Pick another
day on the calendar while scrolled: the calendar stays put. With Live on and a
"Load more" page open, make requests elsewhere: rows being read stay where they
are and the loaded pages remain. Create a login link from a row far down Users:
the link box still comes into view. No `[steady]` warnings in a dev build's
console while doing this.

**Upgrade `logs-calendar`: Logs shows one day and a calendar to pick others.**
Marks: `log-calendar`, `log-cal-grid`, `logs.day_empty`

The Logs screen no longer lists every day on disk, which grew without end. Its
days card shows a month calendar and one day under it: today unless another
date is picked. Dates with entries are shaded, a red dot marks a day with
errors, and while searching only matching dates can be picked and the screen
moves to the newest match. The server is unchanged.

Take `static/js/logs.js` whole. In `static/index.html`, take the inside of the
Logs days card (the `.log-calendar` block, `#logs-empty`, `#logs-day`; the old
`#logs-days` is gone). In `static/css/style.css`, take the `feature:logs` block
(`.log-days` is gone, `.log-cal-*` is new). In `language.yaml` under `logs:`,
`empty` is gone and `calendar_label`, `prev_month`, `next_month`,
`today_button`, `day_button`, `day_matches` and `day_empty` are new. An app's
own code that read `#logs-days` or `details.log-day[open]` must follow.

**Test:** Logs shows only today, open, with today ringed and picked on the
calendar. Picking a shaded date shows that day; Load more, Download and Delete
day still work, and deleting returns to today. Searching `status:5xx` leaves
only matching dates pickable. No horizontal scroll at phone width.

**Upgrade `account-cards`: One Account screen, and a card per job on every screen.**
Marks: `view-title`, `card-collapse`, `sessions-card`, `sessions-open`, `account.profile_heading`

Password, PIN, Two-factor and API keys are no longer screens. They are cards on
Account, which now reads: your name, sign in on another device, password, PIN,
two-factor, API keys, where you're signed in (folded away until opened,
remembered in this browser, fetched only while open), sign out. Their sidebar
links and the `nav.pin`, `nav.password`, `nav.two_factor` and `nav.api_keys`
keys are gone. `#password`, `#pin`, `#two-factor` and `#api-keys` still work:
`Nav.moved` opens Account and scrolls to the card.

Every screen changes shape. `<section class="card view">` becomes
`<section class="view">` holding an `<h2 class="view-title">`, an optional
`<p class="view-description">`, and one `<section class="card">` per job with an
`<h3 class="card-title">`. Users, Invite links, All API keys and Webhooks split
their create form from their list. Files splits upload from the list. Logs
splits search, address names (a folding card) and the days. The user-sessions
and webhook-deliveries panels become cards. Element ids are unchanged, so screen
JS only needs the edits listed here.

Take the signed-in `<section>` and the sidebar of `static/index.html` whole. Take
the `.view` and `.card-collapse` rules in `static/css/style.css`, `moved`,
`card()` and the scroll in `render()` in `static/js/nav.js`, and `init`,
`savedOpen`, `saveOpen` and the guard at the top of `refresh` in
`static/js/sessions.js`. In `language.yaml`, take `account.profile_heading`,
`account.logout_description`, `admin.create_heading`/`list_heading`,
`files.upload_heading`/`list_heading`, `invite_links.create_heading`/`list_heading`,
`admin_keys.list_heading` and `webhooks.create_heading`/`list_heading`, each
under its feature. An app's own screens become views of cards the same way. Any
of its own code that styled `.card.view`, or looked for a screen's `h2`, must
follow the new shape.

**Test:** Account shows the cards in order, and a feature that is off has no
card. "Where you're signed in" starts shut, loads when opened and stays open
after a reload. `#two-factor` lands on Account at that card. Users, Webhooks,
Files and Logs show separate cards, with no horizontal scroll at phone width.

## After 3f84d17

**`docker/run.sh`, the settings-changed notice:** the `diff … | sed …` line now
ends in `|| true`. `diff` exits 1 whenever the `docker:` settings changed, which
is the only time the line runs, so under `pipefail` the script used to print the
was/now lines and stop with status 1, after building the image and before
replacing the container. An autodeploy reported a failed deploy; by hand it
stopped with no error.

**Test:** deploy once, change `docker.memory` in `config/config.yaml`, and run
`docker/run.sh` again: it prints the was/now lines, replaces the container and
reaches `Ready:`.

**`docker/run.sh`, the data-directory marker:** `touch "${DATA_DIR}/${DATA_MARKER}"`
is now `|| true`, and the root container that chowns the data directory touches
the marker too. On Linux the second run used to stop there, because the first had
handed `data/` and the marker to uid 10001 and the host user could no longer touch
them.

**Test:** on a Linux host, run `docker/run.sh` twice: the second run gets past the
data-directory step and `data/.<slug>-data` exists. On macOS both runs still work.

## After b286bc2

**Navbar light/dark button:** an app with the Appearance screen no longer has
the navbar's light/dark button, since the screen already picks the theme. In
`static/index.html` the `#theme-toggle` button, in `static/js/app.js` the
`Theme.watchToggle` call, in `static/js/theme.js` the `watchToggle` method, and
in `language.yaml` `nav.theme_toggle` are now `feature:!appearance`. In an app
with `appearance`, remove all four together. An app without it keeps them.

**Test:** with `appearance`, the navbar has no moon/sun button and the
Appearance screen still switches themes, with no error in the console.

**Settings highlight class:** the class that marks a changed field on the
Settings screen is now `.settings-changed`, in `static/css/style.css` and
`static/js/settings.js`. Take both files' lines together. An app with its own
CSS for the class needs the same rename.

**Test:** change a field on the Settings screen: it is highlighted until saved
or changed back.

**`go build` in `scripts/build.sh`, `lib.sh`, `run.bat` and `login.bat`:** these
now pass `-buildvcs=false`, so Go no longer records the checkout's state in the
binary. `./app version` still reports the commit, which comes from `-ldflags`.

**`scripts/login.sh` and `login.bat`:** with nothing running they now print the
link and start the server in the foreground, where Ctrl-C stops it, instead of
detaching it. A running server still just gets a link. The old behaviour is
`--background`, so any automation of the app's own that calls `login.sh` with no
server up needs that flag, or it will not return.

**Test:** `scripts/login.sh` with the server stopped: the link works and Ctrl-C
stops the server. Run it again with the server up: it prints a link and exits.

**`static/js/pager.js`:** Load more is busy until its page is in, so a double
click no longer appends the same rows twice.

**Disable on the Users screen (`static/js/app.js`):** it now asks first, and
Disable and Enable both end with a toast. Add `admin.disable_confirm`,
`admin.disabled_toast` and `admin.enabled_toast` to the app's `language.yaml`,
reworded to match the app. An app that left the keys out would get an empty
question, which skips asking, and no toast.

**Test:** in a list long enough to page, double-click Load more: no row appears
twice. Disable a user: the question appears, Cancel changes nothing, and
confirming shows the toast. Enable them: the toast appears with no question.

**Saying when something worked (`static/js/app.js`, `pin.js`, `logs.js`, `files.js`):**
Revoke access, Reset 2FA, Delete day, Remove PIN and a finished upload now end with
a toast, and saving an unchanged name says so. Remove PIN and New link now ask
first, which is a change in behaviour to name in the report. New link's box says
whose link it is and scrolls into view: `App.showLoginLink` takes the name as a
second argument, so an app's own calls need it. Add `ui.unchanged`,
`admin.revoked_toast`, `admin.two_factor_reset_toast`, `admin.reissue_confirm`,
`admin.link_for`, `pin.clear_confirm`, `pin.cleared`, `files.uploaded` and
`logs.deleted` to `language.yaml`, reworded to match the app.

**Field errors beside the field (`static/js/dom.js`, `app.js`, `password.js`,
`pin.js`, `two_factor.js`, `join.js`, `settings.js`):** `Dom.fieldError`,
`Dom.clearFieldError` and `App.showFieldError` put a refusal about one field under
that field, with `aria-invalid` and `aria-describedby`, instead of in the banner.
The Settings screen uses them too, so its old problem text no longer survives a
failed re-save. `fields[ key ].problem` is gone from `settings.js`.

**Upgrade `reusable-login-links`: Login links that work more than once, with a chosen expiry.**
Marks: `LiveLoginLinks`, `multi_use_login_token_ttl_seconds`, `cancel-login-link`, `login_links.cancel_button`
Files: `static/js/login_links.js`, `server/models/login_token_test.go`, `server/routes/login_links_test.go`

A login link can now work once (still the default), up to N times (1-1000) or any
number of times, until a chosen expiry or never, always for the same account. The
create-user form gains Works and Expires selects, **New link** and **Sign in on
another device** open a dialog with the same two, and a user's row and the Account
screen show the live link with **Cancel link**. Take `server/models/login_token.go`
whole; its rows gain `max_uses`, `unlimited` and `uses`, and old rows still read as
once, so there is no migration. The callers move from `IssueLoginToken` to
`IssueLoginLink`: `server/routes/admin.go` and `account.go` (`login_link` on user
rows and `/api/me`, `max_uses` and `expires_in_minutes` in, `max_uses`, `unlimited`
and `expires_at` out), `server/control/control.go` and `client.go`,
`server/manage/manage.go` (`-uses`, `-expires`) and `server/bootstrap/bootstrap.go`.
Two new routes, `POST /api/admin/users/:user_id/cancel-login-link` and
`POST /api/account/login-link/cancel`, in `server/routes/routes.go` and
`config/openapi.json` (plus the `LoginLink` schema and the new request and response
fields). New config key `auth.multi_use_login_token_ttl_seconds` (default 604800)
in `config/defaults.yaml`, `config.example.yaml`, `server/config/config.go` and
`live.go`. The printed banner now says `LOGIN LINK` rather than `ONE-TIME LOGIN
LINK`, so take the grep changes in `scripts/login.sh` and `docker/run.sh` with it;
`scripts/login.sh` and `login.bat` gain `--uses` and `--expires`. On screen,
`static/index.html` (the selects, `#my-live-link`, `#login-link-notice`,
`#my-link-notice` and the `login_links.js` script tag), `static/js/app.js` and
`static/js/api.js` change. In `language.yaml`, add the `login_links:` section and
`admin.reissue_heading`; `admin.link_notice` and `account.device_link_notice` now
take `{summary}`, and every string that said "one-time" or "unused login links"
is reworded (`app.tagline`, `signed_out.*`, `login_confirm.body`,
`account.device_description`, `admin.description`, `admin.reissue_confirm` and
the revoke, make-admin and disable confirms). An app that changed those strings
keeps its wording but must add `{summary}` to the two notices.

Disable, revoke access, promotion to admin and reissue now delete a link that
could still be used, not only an unused one. Without that, a reusable link that
had been used once would outlive all four, so do not take the model without it.

**Test:** create a user with Works 3 times: the link signs in three times and the
fourth is refused, and their row reads "Link: 2 of 3 used" after two. **New link**
with Any number of times and Never, use it twice, then **Cancel link**: it is
refused. Use a reusable link once, then disable and re-enable the account: the
link is refused. `manage reissue-login -user-id 2 -uses unlimited -expires 7d`
prints a banner saying so, and `scripts/login.sh 2 --uses 5` passes the flag on.

**Upgrade `dismiss-boxes`: Close buttons on shown-once boxes and the error banner.**
Marks: `data-box-close`, `error-banner-text`

All six shown-once boxes get a close button, which also clears the value from the
page. The error banner stays until it is closed or replaced, where it used to
vanish after 12 seconds, and gains a copy button. Take the markup in
`static/index.html`, the click listener at the end of `static/js/dom.js`,
`App.showError` in `static/js/app.js`, and `ui.copy` in `language.yaml`. The banner
is now a `div` holding `#error-banner-text`, so an app's own code that writes to
`#error-banner` must write there instead.

**Test:** make a user and press New link: it asks, and the box names them. Close
the box. Remove a PIN: it asks, then a toast. Set a password with two different
entries: the problem shows under the confirm field and goes as you type. Cause an
error: the banner stays past 12 seconds, Copy works, and the close button hides it.

**Small fixes (`static/js/webhooks.js`, `sessions.js`, `pager.js`, `files.js`,
`keys.js`):** the previous endpoint's or person's rows no longer show while the next
one's load, through the new `Pager.reset`. Webhook deliveries scroll into view. A
drop during an upload now says so, with `files.drop_busy` in `language.yaml`. The
days field clears after a key is made. Two leftover comments about delete
confirmations that don't exist are gone from `language.yaml`.

**Accessibility (`static/index.html`, `static/js/dom.js`, `pager.js`, and every
screen with row buttons):** headers have `scope="col"`, and both `nav` elements have
names, from `nav.top_label` and `nav.sidebar_label`. `Dom.rowActions( name , buttons )`
groups a row's buttons under the row's name, in place of a bare `.row-actions` div.
In `logs.js` that `div` moves inside an `mb-2` wrapper, and in `files.js` the download
link loses `me-1`. `Pager.load` now puts the focus back after a redraw, so an app's
own table through `Pager` gets this for free.

**Upgrade `panel-buttons`: A close button on Webhook deliveries, and Copy on log entries.**
Marks: `webhook-deliveries-close`, `log-copy`

Take the deliveries heading in `static/index.html`, the listener in
`static/js/webhooks.js`, and the button in `appendEntry` in `static/js/logs.js`. Copy
uses `ui.copy`, so it needs `dismiss-boxes`' key if that was left out.

**Upgrade `keyboard-shortcuts`: A Shortcuts screen where each person changes every key, saved to their account, with keys shown for the OS they are on.**
Marks: `shortcuts-modal`, `Shortcuts.register`, `SetMyShortcuts`
Files: `static/js/shortcuts.js`, `server/models/shortcuts.go`, `server/routes/shortcuts.go`, `server/routes/shortcuts_test.go`

Take `static/js/shortcuts.js`, its `<script>` tag after `undo.js`, the `#shortcuts`
sidebar link, the `shortcuts` screen section and the modal in `static/index.html`, the
`Shortcuts.init()` call in `App.init` and `Shortcuts.signedIn( me )` in
`renderSignedIn`, `Api.setShortcuts`, the `pressed` helper in `Dom.editable`, the
`typeof Shortcuts` line in `Undo.onKey`, the `.shortcut-*` rules, `nav.shortcuts`,
`undo.shortcuts` and the `shortcuts` section of `language.yaml`, the `Shortcuts`
field on `User` in `server/models/user.go`, `shortcuts` in `meRow`, the
`/account/shortcuts` route in `routes.go`, and
`/api/account/shortcuts` plus `Me.shortcuts` in `config/openapi.json`. An app that
had the earlier version drops its sidebar button (`shortcuts-button`,
`.sidebar-shortcuts`). An app with its own keys should move them into
`Shortcuts.register` so people can change them too. Nothing to migrate: an account
with no `shortcuts` uses the defaults.

**Test:** with two webhooks, open one's deliveries then the other's: the first's rows
never show under the second's name, and the close button works. Open a log entry and
Copy it. Press `/` on Users, and `?` anywhere. On Shortcuts, change search to
⌘K (Ctrl+K off a Mac): it works on Users, `/` no longer does, it is still there after a
reload and in another browser, and Ctrl+Z puts it back. Give Undo a key Help has and
accept the swap. Put every key back. Delete a row by keyboard and keep
tabbing: the focus stays in the table.

**Upgrade `row-menu`: A menu for row buttons, tables that stack on phones, and full-size taps.**
Marks: `row-menu-toggle`, `ui.more_actions`, `Pager.label`

`Dom.rowActions` keeps a row's first two buttons and puts the rest in a ⋯ menu, on
every screen width. A row with three keeps all three. Order an app's own rows with the
most used first. A button in the menu becomes a `.dropdown-item`, so an app that
restyles a row button through its class must stop. Below 576px each table row turns
into a card with its column names beside the values, through `Pager.label`, which
`Pager.load` calls by itself. An app's own table filled without `Pager` should call
`Pager.label( id )` after it. The Logs table is left alone. On touch screens `.btn-sm`
and menu items are now 44px, so an app's own layouts with small buttons need a look at
phone width. Take `static/js/dom.js`, `pager.js`, `jobs.js`, `webhooks.js`,
`static/css/style.css` and `ui.more_actions` in `language.yaml`.

**Test:** on Users, open the ⋯ menu on a row at the bottom of the table: it is not
cut off. Use Revoke access from it by keyboard: the focus returns to the ⋯ button.
At phone width, every table but Logs is cards, sort buttons still work, and no page
scrolls sideways.

**Upgrade `settings-tools`: A filter, jump links and Use default on the Settings screen.**
Marks: `settings-filter`, `settings.reset_button`

Take `static/js/settings.js`, the `.settings-tools` block in `static/index.html`, the
rules after `.settings-help` in `static/css/style.css`, and the new `settings` keys in
`language.yaml`. Use default fills the field in, and Save writes it, so nothing new
reaches the server. If a save fails on a field the filter is hiding, the filter clears.

**Test:** type `rate` in the filter: only matching settings and their jump links stay,
and a change made before filtering still saves. A jump link lands with the section's
heading in view. Use default appears only on a value that differs, marks the field as
changed, and writes nothing until Save.

**Brand colours (`static/js/theme.js`):** `ThemePalettes` keeps all ten Wright State
colours by name, and the two WSU presets now draw from it. Their colours are
unchanged. Take the block above `Theme`.

**Upgrade `theme-layers`: A second colour, layered cards and tinted chrome in every theme.**
Marks: `--app-second`, `Theme.colours`, `check_themes.js`
Files: `scripts/check_themes.js`

Every preset gains `second`, for where you are: the active nav item, headings,
progress bars, info badges and info boxes. The main colour stays on buttons and links.
`Theme.colours( preset )` works out the rest, and the page, a card, a nested card
(`.app-nested`) and the navbar and sidebar each get their own shade. Take
`static/js/theme.js`, the new rules and the token block in `static/css/style.css`, and
the two `.app-nested` panels in `static/index.html`. An app's own presets need a
`second`, and need `accent` even for a plain mode. An app's own CSS using
`--app-accent` for a heading or the active nav item should move to `--app-second-text`
or `--app-second`. Forest's main colour darkened a little (`#28843b`) to pass contrast.
`.text-bg-primary` now follows the theme. It used to stay Bootstrap blue.
`.btn-outline-secondary` is stronger, because Bootstrap's grey was too faint on dark
cards, and `--app-border` is now drawn from the card rather than the page.
`scripts/check_themes.js` holds every preset to 4.5, and the tests run it when Node is
there.

**Test:** go through every theme on the Appearance screen: the active nav item and
headings take the second colour, buttons the main one, and a saved dark theme still
does not flash light. Opening with nothing saved is light.

**Upgrade `ip-names`: Nicknames for addresses and ranges.**
Marks: `BucketIPNames`, `ip-names-table`
Files: `server/models/ip_name.go`, `server/routes/ip_names_test.go`, `server/models/ip_name_test.go`

Names an address or a range on the Logs screen, so the log says who is who. Clicking
an address names it, and a list under the search manages them. The most specific name
wins, and `ip:` and free text search them. Names are applied as the log is read, so
the files do not change and old entries get names too. Downloads gain `ip_name` beside
`ip`. Only the owner sees names, on Logs and on Sessions. No migration: the bucket is
created at start. Take the model, the three routes under `/api/owner/logs/ip-names`
and their `config/openapi.json` entries, the `.log-ip` rules, the panel in
`static/index.html`, the changes to `static/js/logs.js`, `sessions.js` and `api.js`,
and the `logs.ip_*`, `logs.table_ip_*`, `undo.ip_name`, `errors.bad_ip_*` keys in
`language.yaml`. `Logger.Download` now takes a `logs.Query`, and `scanRaw` is gone, so an app's own caller needs the extra argument.

**Test:** click an address in the log and name it, then try a range. Search `ip:` with
part of the name. A narrower range beats a wider one. Download a day: `ip_name` is
there and the file on disk has none. Sign in as someone else: their Sessions show no
names.

**Parts can be cut (`scripts/prune.py`, every template file):** the template now carries
markers saying which code belongs to which part, and `scaffold.py` removes them all, so
none reach an app. Some code moved so that parts can stand alone, with no change in
behaviour. An app from before this has no `pruned:` in its stamp, keeps everything it
has, and goes on getting fixes in all of it. The moves arrive as ordinary changes, so
expect conflicts where an app edited those files.
Cutting `backups` removes only the schedule: `manage backup` and `manage restore` stay in
every app.

**Plumbing `edit-helpers`: Dom.editable and Dom.form.**
Marks: `Dom.editable`, `inline-edit`

Shared helpers for editing
something in place or in a dialog. They need the `#form-modal` block from
`static/index.html`, the `.inline-edit` rules from `static/css/style.css`, and
`ui.save` and `ui.edit` in `language.yaml`.

**Upgrade `rename-keys`: Renaming API keys and changing their expiry.**
Needs: `api_keys`, `edit-helpers`.
Marks: `RenameMyAPIKey`, `undo.rename_key`, `rename_key:`

Four new routes
(`/api/keys/:id/rename` and `/expiry`, and the same under `/api/admin/keys`), so
take the key model and route changes and the `config/openapi.json` entries. On
screen, `static/js/keys.js` and `static/js/api.js` change, and All keys gains an
Expires column in `static/index.html`. Add `undo.rename_key` and the
`api_keys.expiry_*` keys to `language.yaml`.

**Test:** rename a key by clicking its name, then Ctrl+Z puts the old name back.
Press Expiry on a live key and enter 30: the date moves. An expired or revoked
key has no Expiry button. As a plain admin, the owner's keys have none either.

**Upgrade `edit-invites`: Editing invite links.**
Needs: `invite_links`, `edit-helpers`.
Marks: `SwitchOnInviteLink`, `undo.rename_invite`, `rename_invite:`
Files: `server/routes/invite_links_test.go`

Three new routes under `/api/admin/invite-links/:id`
(`rename`, `limits`, `switch-on`), so take the invite model and route changes and
the `config/openapi.json` entries. On screen, `static/js/invites.js` and
`static/js/api.js` change. Add `undo.rename_invite` and the new `invite_links`
keys to `language.yaml`. A switched-off link can now be switched back on: an app
that promised its users otherwise should empty `invite_links.switch_on_button`
and take the route out.

**Test:** rename a link by clicking its label, then Ctrl+Z puts it back. Press
Edit, change Places, and leave the days blank: the expiry stays. Places below the
number joined is refused. Switch a link off, then Switch on: the same link joins
again.

**Upgrade `edit-webhooks`: Editing webhooks.**
Needs: `webhooks`, `edit-helpers`.
Marks: `RotateWebhookSecret`, `undo.rename_webhook`, `rename_webhook:`

Three new routes under `/api/owner/webhooks/:id` (`rename`,
`target`, `rotate-secret`), so take the webhook model and route changes and the
`config/openapi.json` entries. On screen, `static/js/webhooks.js` and
`static/js/api.js` change, and the secret box's notice in `static/index.html` gains
the id `webhook-secret-notice`. Add `undo.rename_webhook`, `undo.edit_webhook` and
the new `webhooks` keys to `language.yaml`. An app with its own events needs nothing
extra: the dialog offers what `/api/owner/webhooks/events` lists.

**Test:** rename a webhook by clicking its name, then Ctrl+Z puts it back. Press
Edit, change the address and tick one event: the row shows the new address, and
Undo restores both. An `http://` address is refused in the dialog. Press New secret:
it asks first, then the box shows a new secret, and the next test delivery is signed
with it.

**Activity log (`server/logs/entry.go`, `server/logs/writer.go`):** `Logger.Log` now
copies an entry's strings before queueing it. Lines are written after the request
ends, and Fiber reuses the buffer those strings view, so a line could hold the
next request's bytes: a wrong id, or part of its headers. Take both files. An app
with its own `Entry` fields that hold request strings should copy them in `owned`.

**Test:** `go test ./server/logs`. An app's old log lines cannot be repaired, so an
odd id or user agent in them may be this.

**Upgrade `rename-files`: Renaming files.**
Needs: `files`, `edit-helpers`.
Marks: `RenameMyFile`, `undo.rename_file`, `rename_file:`

Two new routes (`/api/files/:id/rename`, and the same under
`/api/admin/files`), so take the file model and route changes and the
`config/openapi.json` entries. On screen, `static/js/files.js` and `static/js/api.js`
change. Add `undo.rename_file` to `language.yaml`.

**Test:** rename a file by clicking its name, then Ctrl+Z puts it back. Rename it to
`a/b.txt`: the row shows `b.txt`. Download it: it has the new name and the same
contents. As an admin, Show everyone's files and rename someone else's.

**Upgrade `edit-users`: Admins editing a user's name and role.**
Needs: `edit-helpers`.
Marks: `role_changed_toast`
Files: `server/routes/admin_test.go`

Two new routes
(`/api/admin/users/:id/rename` and `/role`), so take the user and credential model
changes, the admin routes and the `config/openapi.json` entries. On screen,
`static/js/app.js` and `static/js/api.js` change. Add the `admin.make_*` keys and
`admin.role_changed_toast` to `language.yaml`. An app whose admin rights mean more
than the template's should check that "any admin may make an admin" suits it.

**Test:** on the Users screen, click someone's name, rename them, and Ctrl+Z puts it
back. Press Make admin: it asks first, and that person is signed out. Press Make
user on them: their next request is an ordinary user's. As a plain admin, another
admin's row has neither.

**Upgrade `name-sessions`: Naming your sessions.**
Needs: `edit-helpers`.
Marks: `LabelMySession`, `undo.rename_session`, `rename_session:`

One new route (`/api/account/sessions/:id/label`), so take
the session model and route changes and the `config/openapi.json` entry. The record
gains an optional `label`, and sessions from before it simply have none. On screen,
`static/js/sessions.js` and `static/js/api.js` change. Add `undo.rename_session` to
`language.yaml`.

**Test:** on the Account screen, click a device's name, type "Work laptop", and the
browser and system stay underneath. Ctrl+Z puts it back. Clear the name and the
worked-out one returns. An admin's Sessions view shows the name but cannot edit it.

**Upgrade `sort-columns`: Sorting table columns.**
Marks: `Pager.sortable`, `th-sort`, `sendSorted`

List routes take `?sort=` and `?dir=`, so take the paging
code (the sorting in `server/routes/pagination.go` or `app/routes/helpers.py`), each
list route's field list, and the `config/openapi.json` parameters. On screen, take
`static/js/pager.js`, `static/js/api.js`, the sortable `th`s in `static/index.html`,
the `.th-sort` rules in `static/css/style.css`, and the `Pager.sortable` calls.

**API keys now list newest first,** in the routes and `manage`, like every other
list. Anything of the app's own that reads the first key as the oldest needs a look.

**Test:** press a column header on Users: up, down, then back to the list's own
order. Sort All keys by Last used: keys never used stay last both ways. Sort a list
long enough to page, press Load more: the order carries on.

**Upgrade `search-lists`: Search on keys, invites, files and webhooks.**
Marks: `Pager.search`, `matchRows`

Those lists take `?q=`, and the key
lists take `?live=true`, so take the routes, `matchRows` or `match_rows`, and the
`config/openapi.json` parameters. On screen, take `static/js/pager.js`,
`static/js/dom.js` (`Dom.typed`), `static/js/api.js`, the search boxes in
`static/index.html`, and the `Pager.search` and `Pager.filter` calls. `Api.listUsers`
now takes `Pager.params`, so an app's own call with a search string needs changing.
Add `ui.no_matches` and the `search_label` keys to `language.yaml`.

**Test:** type in the search box on each screen: the list narrows as you pause, and
Load more stays inside the search. A search with no matches says so rather than
"No API keys yet." Tick Hide revoked and expired keys: only working keys stay.

**Upgrade `show-more`: Showing more on each list.**
Marks: `table_created`, `two_factor_badge`

The Users rows gain `username` and `created_at`, so take
the row in `server/routes/admin.go` or `app/routes/admin.py` and the `User` schema in
`config/openapi.json`. An admin key and the MCP `admin_list_users` tool get them too.
Invite links also search by maker. On screen, `static/js/app.js`, `keys.js`,
`invites.js`, `files.js` and `webhooks.js` change, and Users gains a Created column
in `static/index.html`. Add `admin.table_created`, `admin.two_factor_badge`,
`api_keys.created` and `invite_links.made_by` to `language.yaml`.

**Test:** Users shows a username under a name, a 2FA badge for someone with it on,
and a Created column that sorts. A key shows when it was made, an invite link who
made it, a file its type, and a webhook its events.

**Upgrade `busy-buttons`: Buttons show when they are working.**
Marks: `Dom.onPress`, `.modal-open .btn`

A button made with `Dom.el` now goes busy by itself while an async click handler
runs, and `Dom.onPress` does the same for a button in the page's HTML. Take
`static/js/dom.js`, the `.modal-open` rule in `static/css/style.css`, and the
`Dom.onPress` calls in `app.js`, `two_factor.js`, `pin.js`, `password.js`, `logs.js`,
`sessions.js` and `settings.js`. An upgrade, though it adds no button, because it
changes every button: an app's own `Dom.el` button that calls `Dom.busy` on itself
in its click handler must stop, since the press is already busy and that inner call
would skip its work. `Dom.busy` no longer sets `disabled`, so check any code of the
app's that reads it.

**Test:** on a slow connection, press a row button: it shows a spinner and a second
press does nothing. Press one that asks first: no spinner behind the question, Cancel
puts the button back and the focus on it. Load more still loads.

## Since 88bf66f (the oldest commit a managed app is on)

**Switches added:** `auth.passwords`, `auth.two_factor`, `auth.invite_links`,
`backups.enabled`, `metrics.enabled`, `webhooks.enabled`, `files.enabled`.
An app that already had invite links from before the switch must list
`invite_links` in its stamp, or the update turns them off.

**New capabilities (each with its own server, route and static files):**
background jobs (`server/jobs`), the activity log and Logs screen
(`server/logs`, `server/routes/logs.go`), a live Settings screen
(`server/config/live.go`, `schema.go`, `yamledit.go`), maintenance mode, passwords,
two-factor sign-in, webhooks, file uploads, metrics, backups and key rotation
(`server/backup`, `server/rotate`, `docker/restore.sh`, `docker/rotate-key.sh`),
OpenAPI (`config/openapi.json`), sessions, soft delete, pagination, undo.

**Whole features.** Each is its own choice. Everything else in this entry is
foundation that later fixes are built on, so it always lands.

**Upgrade `passwords`: Password login.**
Marks: `passwords:`
Files: `server/models/password.go`, `server/routes/passwords.go`, `server/routes/passwords_test.go`, `static/js/password.js`

**Upgrade `two_factor`: Two-factor sign-in.**
Marks: `two_factor:`
Files: `server/models/two_factor.go`, `server/routes/two_factor.go`, `static/js/two_factor.js`

**Upgrade `webhooks`: Webhooks.**
Marks: `webhooks:`
Files: `server/models/webhook.go`, `server/models/webhook_test.go`, `server/routes/webhooks.go`, `server/routes/webhooks_test.go`, `server/webhooks/webhook_cases.json`, `server/webhooks/webhooks.go`, `server/webhooks/webhooks_test.go`, `static/js/webhooks.js`

**Upgrade `files`: File uploads.**
Marks: `files_enabled`
Files: `server/jobs/files.go`, `server/models/file.go`, `server/models/file_test.go`, `server/routes/files.go`, `server/routes/files_test.go`, `static/js/files.js`

**Upgrade `metrics`: Metrics.**
Needs: `api_keys`.
Marks: `metrics:`
Files: `server/metrics/metrics.go`, `server/middleware/metrics.go`, `server/routes/metrics.go`

**Upgrade `backups`: Backups and key rotation.**
Marks: `backups:`
Files: `docker/restore.sh`, `docker/rotate-key.sh`, `server/backup/backup.go`, `server/backup/backup_test.go`, `server/rotate/rotate.go`, `server/rotate/rotate_test.go`

**Secrets:** `server/config/secret.go` moves the key out of `config.yaml` into
`config/secret.key`. `scripts/run.sh` does this through `config migrate` on the
next start. An app with `secret_key` still in `config.yaml` will change on disk
at that point: back up `config.yaml` first.

**Migrations:** `server/db/migrate.go` is new, and its list is empty. A database
with no version number is treated as version 0, so this adds no steps, but every
later template step is appended after the app's own.

**`.gitignore`:** tracked in the template from 6ce90b0. Apps have their own, so
it arrives as `both-added`. Compare by hand: it protects `config/secret.key`.

**Test:** sign in, an invite link, an API key, the MCP endpoint, the Logs screen,
the Settings screen, and the app's own features.

## Entry format

Newest first. Each entry says which commits it covers, then only what a merge
can't tell you: new config, migrations to append, renamed files, what to test.
Use SHAs, because the numbered commit messages have restarted before.

**Label everything that isn't a fix.** An unlabelled item is a fix and lands in
every app without asking. Anything that adds a route, screen, setting or button is
an upgrade, and the user chooses it, so write it like this, with no blank line
inside the block:

```
**Upgrade `rename-keys`: Renaming API keys and changing their expiry.**
Needs: `api_keys`, `edit-helpers`.
Marks: `RenameMyAPIKey`, `undo.rename_key`
Files: `server/routes/keys_extra.go`
```

- `Needs` (optional) is a feature switch or another item. An upgrade whose feature
  is off is left out without asking.
- `Marks` are strings found only in that item's code, and in no earlier template.
  They decide whether the item is new to an app, and `apply` refuses while a
  left-out item's marks are in the staged files.
- `Files` (optional) are whole files that exist only for this item.
- Use `Plumbing` in place of `Upgrade` for shared helpers that nobody would choose
  on their own. They are never offered, and come with whatever needs them.
- An upgrade named after a feature switch is that whole feature. Taking it switches
  it on and adds it to the stamp.
- `Needs` can also be a part an app can be scaffolded without, such as `logs`. An
  upgrade to a part cut out of an app is left out without asking.
