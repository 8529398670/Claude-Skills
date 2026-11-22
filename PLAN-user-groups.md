# Plan: user groups in the go-webserver and python-webserver templates

Written 2026-11-20 from an interview, starting from `PROMPT-user-groups.md`.
On approval this file is copied to `~/.claude/skills/PLAN-user-groups.md`.

## Context

Every app built from these templates is a single tenant today. It has one owner,
global roles (viewer < user < admin), and admins who see every account. TodoBoard
is the motivating case: the owner wants to run several isolated groups of people
in one instance. Nobody outside a group can see its members or data, not even
another group's admins. A group is in effect its own small copy of the app,
inside the same server and database.

Neither template has a tenant concept yet. Every bucket is flat. The owner is a
`meta` marker. `/api/team` and the admin user list read all users. The Python
store indexes only the record key, so scoping has to go in the key.

## Scope and rules

- Phase 1 changes only `~/.claude/skills`, on `master`, with both templates in
  the same pass. Use no branches or worktrees. Commits and pushes are left to
  `gitPushUpdate.sh`.
- Phase 2 is TodoBoard adopting groups, in its own repo. It starts only after
  phase 1 is committed and the user says go.
- The portal side is a separate plan. This plan owns the app side and the wire
  format (contract v3, below).

## Decisions (from the interview)

| Topic | Decision |
|---|---|
| Owner | One super-admin. Sees, enters and manages every group. Only the owner creates, renames, archives and deletes groups. |
| Identity | One account per person. Membership is `(group, account, role)`, so someone can be admin in A and viewer in B. |
| Always in | Groups are not a prune feature. Every app has them. With one group the switcher is hidden. |
| Active group | Comes from the URL: pages at `/g/<slug>/...`, API at `/g/<slug>/api/...`, MCP at `/g/<slug>/mcp`. Even one-group apps use `/g/`. Old bare page URLs answer 301 to the default group. |
| API keys | Each key is bound to one group, and its role is capped at the account's role there. The owner may give an admin key **instance scope** (portal key), from a browser session only. |
| Group admins | Today's admin rules, inside the group: act on users and viewers, while only the owner acts on admins. They can't see outsiders and only ever create new accounts. |
| Second group | Only the owner adds an existing account to another group. |
| Usernames | Unique per group. Password sign-in is group-qualified (`/g/<slug>/login`). |
| Disable / revoke | A group admin suspends the **membership** and kills that group's keys and links. The account's sessions keep working in other groups. Only the owner disables a whole account or ends its sessions. |
| Storage | Group-scoped buckets are keyed with the group id first. The AEAD binds `bucket\0key`, so a record copied across groups fails to open. |
| App code | A `GroupStore` from the request context. The raw `Store` refuses scoped buckets. Cross-group access only through an explicit `AllGroups()`. |
| Global | Themes and language, security settings, upload quota, webhooks and jobs (owner). There is one audit log with a group column: group admins see their group's rows, the owner sees all. |
| Group owns | Name and slug only. |
| Slug | Can be renamed. An old slug stays a 301 alias until another group claims it. The numeric id is permanent and never shown in URLs. |
| Display name | One per account. Once the account is in 2+ groups, only the person or the owner can rename it. |
| Owner in lists | The owner appears in a group's lists only if added as a member. |
| Per-person app state | Per membership (`group\|user\|...`). Device-level state, such as push subscriptions and the Google account link, is per account. |
| Links | `/login/<id>.<secret>`, `/join/<id>.<secret>` and share links stay bare. The record carries its group, and the person lands on `/g/<slug>/`. Other sign-ins land on the last group used, or a picker. |
| Delete | Soft-delete (archive). Nobody can reach the group, and the owner can restore it for `groups.purge_after_days` (default 30). Housekeeping then purges its key prefix and files, and disables accounts left with no membership. |
| Export | The owner can export one group (records, files, memberships) as a sealed archive. No import. Whole-instance backup and restore are unchanged. |
| Migration | One-way schema step, after the automatic pre-migration copy. Everything goes into group 1, "Default" (slug `default`). |
| Rollout | An opt-in `groups` upgrade in `UPGRADING.md`. It lands the template side and prints an app-side checklist. New apps are scaffolded with groups. |

## Data model (both templates, same key shapes)

New global buckets:

- `groups`, keyed by group id (from a sequence, never reused). Fields: `ID`,
  `Name`, `Slug`, `CreatedAt`, `CreatedBy`, `ArchivedAt`, `PurgeAfter`.
- `group_slugs`, keyed by `LookupKey(slug)`. Value: `{group_id, alias:bool}`.
- `memberships`, keyed by `group|user`. Fields: `Role`, `CreatedAt`, `AddedBy`,
  `DisabledAt`, `ExpiresAt`, `LastSeenAt`.
- `user_groups`, keyed by `user|group` with an empty value. It is the reverse
  index, used to list my groups.
- `group_usernames`, keyed by `group|LookupKey(username)`. Value: user id. It
  replaces the global username scan.

Changes to existing records:

- **`User`**:
  - `Role` and `ExpiresAt` move to `Membership`.
  - `DisabledAt` stays and means the owner disabled the whole account.
  - New `LastGroupID`.
- **`APIKey`**: `GroupID` (0 means instance scope, owner only) and
  `InstanceScope bool`.
- **`LoginToken` and `InviteLink`**: `GroupID`. Links are still looked up by
  their global id. A mismatched group answers 404.
- **`Session`**: no group. Sessions belong to the account.
- **`files`**: becomes a scoped bucket, keyed `group|file id`. Bytes go under
  `storage/files/<group>/<id>`.
- **Owner**: still the `meta.owner_user_id` marker. The owner has effective
  `admin` in every group, with no membership row unless one was added.

The store API:

- Buckets are declared once, as `Scoped` or `Global`, in `db.go`
  `bucketNames` / `BUCKETS`.
- `GroupStore{store, groupID}` has `Put`, `Get`, `Delete`, `ForEach`,
  `UpdateValue`, `DeleteWhere`, `NextSequence` and `Write`. Each prefixes the
  group id with the existing composite-key encoding (Go `db.Key`, Python's
  twin).
- The raw `Store` returns `ErrScopedBucket` for scoped buckets.
- `store.AllGroups(func(groupID, gs))` exists for jobs and the owner.
- Go: `GroupStoreFrom(c)`. Python: `request.state.group_store`.

## Request resolution

1. `LoadUser` / `LoadActor` is unchanged: it resolves an account from the
   session or key.
2. New `ResolveGroup` runs on every `/g/{group}/*` route, in this order:
   1. **Slug lookup.** An alias answers 308 for API calls and 301 for pages.
      An unknown slug answers 404.
   2. **Archived group.** It answers 404, or 410 `group_archived` for the owner.
   3. **Who may enter.** The caller must be a member or the owner. Otherwise
      404 `not_found`. A disabled or expired membership answers 403
      `membership_disabled`.
   4. **Key scope.** The key's `GroupID` must match the group, unless the key
      has instance scope. Otherwise 404.
3. `Actor` gains `Group`, `Membership` and the effective `Role`, which is the
   lowest of the membership role, the key role and the viewer rule. The owner
   counts as admin. `RequireAdmin` and `ViewOnly` read this group role.
4. Any id from another group 404s, because the `GroupStore` simply doesn't find
   it. Credential records looked up by global id (keys, links) check `GroupID`
   and answer 404 too.

Route annotations gain `scope=GROUP|INSTANCE`, with `GROUP` as the default. The
apidoc generators mount `GROUP` routes under `/g/{group}`. `INSTANCE` routes:
health, language, login and join, logout, bare `/api/me`, the account and
security routes for one's own account, and all `/api/owner/*`. App routes need
only `scope` if they are instance-wide. The path text stays as written today.

## Routes

**Instance** (bare):

- `GET /api/me`: adds `contract: 3`, capability `groups`,
  `groups: [{id, slug, name, role}]`, `last_group`, and `key_scope`
  (`"group"` or `"instance"`).
- Owner, session or instance key:
  - `GET /api/owner/groups` → `{groups:[{id,slug,name,members,created_at,archived_at}]}`
  - `POST /api/owner/groups` `{name, slug?}` → `{group}`, or 409 `slug_taken`
  - `POST /api/owner/groups/{group_id}/rename` `{name?, slug?}`
  - `POST /api/owner/groups/{group_id}/archive`
  - `POST /api/owner/groups/{group_id}/restore`
  - `GET /api/owner/groups/{group_id}/export` (session only)
  - `POST /api/owner/groups/{group_id}/members` `{user_id, role}` adds an
    existing account. A clashing username in that group answers 409
    `username_taken`.
  - `GET /api/owner/users`: every account, with its memberships.
  - `POST /api/owner/users/{user_id}/disabled` `{disabled}`: the whole account.
  - `POST /api/owner/users/{user_id}/end-sessions`
- `POST /api/keys/{key_id}/instance-scope` `{instance_scope:bool}`: owner, from
  a session, on an admin key only.

**Group** (`/g/{group}/...`): today's routes, scoped.

- `/g/{group}/api/me` adds `group: {id, slug, name}`. `role` is the group role.
- `/api/admin/users*` now covers the group's members:
  - `POST` creates a new account plus its membership.
  - `role`, `disabled` and `expiry` act on the membership.
  - `delete` removes the membership. An account left with no membership is
    deleted.
  - `revoke-access` covers this group's keys and links only.
- Some account-wide actions need extra care: `rename`, `reset-two-factor`,
  `sessions*` and password access. A group admin may do them only while the
  target belongs to this group alone. Otherwise the route answers 403
  `account_wide` ("Only the owner can do this for this account"). This tells
  the admin that the person is also somewhere else, and nothing more. The
  plan accepts that.
- `/api/team`, invite links, keys, files and storage all come from the group.
- `/g/{group}/mcp` keeps tools at the same names, with the group from the path.

Error codes: `not_found`, `group_archived`, `membership_disabled`,
`account_wide`, `slug_taken`, `username_taken`, `group_required` (a scoped
route called bare, answering 404), and `instance_scope_off` (403).

## Screens (Bootstrap 5, light by default)

- **Group switcher**: a navbar dropdown listing my groups, shown only when I
  have 2 or more. The owner sees every group, with non-member groups marked.
- **Picker at `/`**: shown when there are several groups and no last group.
  Otherwise `/` redirects to `/g/<last>/`.
- **Owner "Groups" screen**: a table inside `.table-responsive`, with the
  create form above it. Columns: name, slug, members, created, status. Actions:
  rename, archive or restore, export, add an existing account.
- **Owner "All accounts" screen**: accounts with their membership chips, plus
  account-wide disable and end sessions.
- **Users screen**: scoped to the group. The page title shows the group name.
- **Sign-in page**: `/g/<slug>/login` for password sign-in. The bare login
  page asks for the group slug when passwords are on.
- **`language.yaml` keys** (both templates): `groups.*` (title, create,
  rename, archive, restore, export, add_member, slug_help, archived_banner,
  switcher_label, picker_title) and `errors.*` for each new code.

## Tests (both suites)

- **Isolation walker.** The fixture has groups A and B, each with an admin, a
  user, a viewer, keys, a login link, an invite link, a file and one sample
  scoped record. The walker goes through every route in the generated route
  table and every MCP tool. It calls with A's credentials, under A's slug,
  using B's ids, and under B's slug. It expects 404 every time and never
  2xx or 403. A new route is covered automatically.
- **Store guard.** The raw `Store` on a scoped bucket fails. A `GroupStore`
  can't read another group's prefix. A value copied across prefixes fails to
  decrypt.
- **Rules.** The admin and owner matrix per group. A membership suspend leaves
  the other group working. `account_wide` behaves as described. Usernames are
  unique per group. Slug aliases redirect, and a reclaimed slug drops its
  alias. Archive, restore and purge work. Export round-trips (open the archive
  and count its records).
- **Migration.** A v-current fixture database migrates. Every record sits
  under group 1, roles become memberships, and old URLs answer 301.
- **Contract v3.** Extend `test_portal_contract.py` and the Go twin.

## Docs and upgrade

- `SKILL.md` (both): a groups section, the scoped versus global bucket rule for
  new app data, and the "New data" pattern updated to `GroupStore`.
- `references/architecture.md`: the request flow, the key layout and the
  isolation walker. `references/frontend.md`: the switcher, and `/g/` in client
  URLs (an `api.js` base path from `location`).
- `UPGRADING.md` (both): a `groups` offer, covering the migration, the URL
  change, and an app checklist. The checklist: declare each bucket scoped or
  global, swap `Store` for `GroupStore`, fix cross-user features such as team
  lists and sharing, and make jobs use `AllGroups`.
- `webserver-upgrade/SKILL.md`: `status` shows contract 3. Note that the
  portal must speak v3 before an app takes `groups`, because bare scoped API
  paths start answering 404 `group_required`.
- `~/WORKSPACE/PROJECTS/ProjectPortal/CROSS_PROJECT.md`: add a "Portal
  contract v3" section with the routes, fields and codes above.
  - The portal discovers groups through bare `/api/me`, then calls
    `/g/<slug>/api/admin/*` with an instance-scope key.
  - Join and Open take a `group` slug.
  - User listing is per group.

## Order of work

1. **Go core**: buckets, `GroupStore`, `ResolveGroup`, the `scope=` support in
   the apidoc generator, the `Actor` changes and the migration step.
2. **Go routes, then screens and strings.**
3. **Go tests**: the isolation walker first, then the rest.
4. **Python twin** of 1–3, keeping the key shapes byte-identical.
5. **Docs**: `SKILL.md`, references, `UPGRADING.md` and `webserver-upgrade`.
6. **Contract**: the v3 section in `CROSS_PROJECT.md`.
7. **Phase 2, after a go**: TodoBoard adoption.
   - Scoped buckets: boards, cards, user_boards, user_cards, board_slugs (slug
     namespaces become per group), prefs, subscriptions, folders,
     google_links, trash, images, comments, card_links / refs, notify_sent.
   - Per account: push_subscriptions, google_accounts.
   - Share links stay global ids carrying a `GroupID`.
   - `/api/team`, `add_board_member` and `share_card_with_user` are limited
     to group members.
   - The notify scheduler and the gcal job go through `AllGroups` or the
     membership list.

## Verification

- Both template test suites pass, including the isolation walker. Run quick,
  targeted checks while building. Ask before starting any long full suite.
- Scaffold a throwaway app from each template:
  1. Create a second group as owner. Add a person to it.
  2. Sign in as that group's admin, and check in the browser pane, at 375px
     and desktop widths, that the other group's users and data are invisible.
  3. Check that the switcher appears, and that old URLs redirect.
- Run `webserver-upgrade` against a copy of an existing app in dry-run, to see
  that the `groups` offer and the migration apply.

## Risks

- Apps that skip the `groups` offer will drift further from the template on
  each later upgrade. `UPGRADING.md` should say so.
- The `account_wide` answer reveals that someone is also in another group.
  The owner accepted that.
