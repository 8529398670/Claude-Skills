# Upgrading apps built with python-webserver

Read by the `webserver-upgrade` skill. Add an entry whenever
`assets/template/` changes.

## Since ceeb9cf (the oldest commit a managed app is on)

**Switches added:** `auth.passwords`, `auth.two_factor`, `auth.invite_links`,
`backups.enabled`, `metrics.enabled`, `webhooks.enabled`, `files.enabled`.
An app that already had invite links from before the switch must list
`invite_links` in its stamp, or the update turns them off.

**New capabilities:** background jobs (`app/core/jobs.py`, `app/models/jobs.py`,
`app/routes/jobs.py`, `static/js/jobs.js`), maintenance mode, passwords, two-factor
sign-in, webhooks, file uploads (`app/routes/files.py`), metrics, backups and key
rotation (`app/core/backup.py`, `app/core/rotate.py`, `docker/restore.sh`,
`docker/rotate-key.sh`), OpenAPI (`config/openapi.json`), sessions, a server lock,
graceful shutdown, request ids.

**Name collision:** an app with its own jobs feature already has files at the
template's jobs paths. They arrive as `both-added` and need a rename, not a merge.
It may also use the same `BUCKET_JOBS = "jobs"` database bucket, so the two would
read each other's records. Rename the template's side, including its bucket, so
stored app data doesn't have to move.

**Migrations:** the list in `app/core/db.py` is empty, so nothing to append yet.
Later template steps go after the app's own.

**`.gitignore`:** tracked in the template from 6ce90b0. Apps have their own, so
it arrives as `both-added`. Compare by hand: it protects `config/secret.key`. An
app that ignores `*.json` needs an exception for `config/openapi.json`.

**Test:** sign in, an invite link, an API key, the MCP endpoint, and the app's
own features.

## Entry format

Newest first. Each entry says which commits it covers, then only what a merge
can't tell you: new config, migrations to append, renamed files, what to test.
Use SHAs, because the numbered commit messages have restarted before. New
optional features arrive switched off unless the app's stamp lists them.
