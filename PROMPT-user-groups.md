# Prompt: user groups in the webserver skills (paste into plan mode in ~/.claude/skills)

I want to add **user groups** to both webserver skills (`go-webserver` and
`python-webserver`), so every app built from them can host several isolated
groups of people in one running instance. I'm not sure I'm describing it
accurately, so treat what follows as the idea, not the spec. Before you write the
plan, run the `interview` skill and ask me questions until the design is pinned down.

## The idea

A group is another level of separation, above accounts and roles. Take TodoBoard:
today it has one owner, some admins, some users and viewers, and every admin sees
every user. With groups, I could make a new group in TodoBoard, put a new person in
it, and nobody outside that group can see them, not even admins of other groups.
Each admin sees and manages only the groups they belong to. The app's own data
(TodoBoard's boards, cards, labels, shares) belongs to a group too, so a group is
effectively its own small copy of the app inside the same server and database.

## What to read first

- `go-webserver/SKILL.md`, `go-webserver/references/architecture.md`,
  `python-webserver/SKILL.md` and the same references there: accounts, the owner,
  roles (viewer < user < admin), API keys, login links, invite links, sessions,
  audit log, encryption at rest, backups.
- The templates under `*/assets/template`: the user/session/key models, the
  middleware that resolves who is calling, the admin routes, the Users screen.
- `webserver-upgrade/SKILL.md` and both `UPGRADING.md` files: this has to reach
  existing apps through that skill.
- `PLAN-*.md` in this folder for how I like plans written.
- `~/WORKSPACE/PROJECTS/ProjectPortal/CROSS_PROJECT.md`, "Portal contract v2":
  the portal drives apps through their admin API, so groups change that contract.
  A separate plan covers the portal side; this plan owns the app side and the
  wire format both will follow.
- TodoBoard (`~/WORKSPACE/PROJECTS/TodoBoard`) as the first real app to adopt it,
  to check the design against app data that has to be scoped.

## Things the interview has to settle (not a complete list)

- **The owner.** Does the owner see and manage every group (the one super-admin),
  or can the owner also be kept out of a group? Who may create and delete groups?
- **Membership.** One group per account, or many? If many, is the role per group
  (admin in A, user in B), and how does a person switch groups in the UI?
- **Is a group a scope or an identity?** Same account across groups, or a separate
  account per group (same email/name allowed twice)?
- **What gets scoped.** Users list, invite links, login links, API keys, sessions,
  audit log, uploads, settings, themes, language overrides, and the app's own
  records. Which are per group, which stay global, which are owner-only.
- **API keys.** Is a key bound to one group? What does an admin key from group A
  get when it names a user in group B (404, not 403, so it can't probe)?
- **App data.** What the template gives apps so scoping their own records is the
  default and hard to forget: a group id on every record, a helper or middleware
  that injects it, tests that fail when a query leaks across groups. Bolt (Go)
  and encrypted SQLite (Python) will need different answers.
- **Off by default?** Whether groups is a feature the scaffolder includes only when
  the app needs it (like other optional features, via `prune.py`), and whether an
  app with groups off behaves exactly as today.
- **Migration.** Existing apps: every current account and record goes into one
  default group. What the upgrade does, and whether it can be undone.
- **Portal contract v3.** A `groups` capability in `/api/me`, routes to list,
  create and rename groups, and a group on Join/Open/user listing calls. Exact
  field names and error codes, written so the portal plan can copy them.
- **Deleting a group**, backups and restore per group, export.

## What the plan should produce

- The data model and rules, for both templates, in parallel.
- Every route, screen and `language.yaml` string that changes.
- Tests, especially isolation tests (nothing from group A is reachable from B by
  any route, key, link or MCP tool).
- The update to `SKILL.md`, references, `UPGRADING.md` and `webserver-upgrade`.
- The contract v3 section to add to `ProjectPortal/CROSS_PROJECT.md`.
- An order of work, and what TodoBoard needs to adopt it.

Rules: work in both skills, all on `master`, no branches or worktrees, commits and
pushes left to `gitPushUpdate.sh`. Write the final plan to
`~/.claude/skills/PLAN-user-groups.md`.
