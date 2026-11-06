# Upgrading apps built with go-webserver

Read by the `webserver-upgrade` skill. Add an entry whenever
`assets/template/` changes.

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
Use SHAs, because the numbered commit messages have restarted before. New
optional features arrive switched off unless the app's stamp lists them.
