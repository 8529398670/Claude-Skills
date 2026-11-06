---
name: webserver-upgrade
description: Bring new features and fixes from the go-webserver and python-webserver skills into apps already built with them, or show which apps are behind. Use this when the user wants to update, upgrade, sync or push skill or template changes to an existing app, asks "which apps are behind", "is TodoBoard up to date with the skill", "pull the new template into this app", "update my apps to the latest webserver skill", or has changed a webserver template and wants existing apps to get it. Not for starting a new app (use go-webserver or python-webserver) or for features that belong to one app alone.
---

# Upgrading apps built with the webserver skills

Apps are copies of a template, so a template change reaches an app only through
this skill. It renders the template at the app's commit and at the target
commit, in the app's own names, and merges only what changed. You handle what
a merge cannot decide.

All commands are `python3 <skill_dir>/scripts/upgrade.py <command>`.

## Hard rules

- **Never commit, push, branch or deploy.** Changes stay in the working tree on
  `master`. The user reviews them and runs `gitPushUpdate.sh`.
- **Never touch** `config/config.yaml`, `config/secret.key`, or anything under
  the app's data directory. The tool refuses to; do not do it by hand either.
- **One app at a time**, and finish it before starting the next.
- **Leave unstamped apps alone.** `status` lists them as unmanaged. An app gets
  a stamp only when the user asks for it.

## See what is behind

```bash
python3 <skill_dir>/scripts/upgrade.py status
```

This is read-only. Report the table and stop unless the user asked for an update.

## Stamp an older app (once per app)

An app scaffolded before stamps existed needs one.

1. Run `baseline <app>`. It scores the app against every skill commit. Below
   25% of shared files exact, there is no reliable starting point: tell the
   user and leave the app unmanaged.
2. Work out which feature switches the app **already uses**. The stamp's
   feature list decides which switches come out on, so a capability the app has
   from before the switch existed must be listed, or the update turns it off.
   Read the app's routes, not just its config, and watch for false matches:
   the word "backup" in a comment is not the backups feature, and an app's own
   upload code is not the template's `files` feature.
3. Show the user the commit and feature list, and get their confirmation.
4. Run `stamp <app> --commit <sha> --features a,b` (or `--features none`).

## Update an app

1. **Prepare:** `prepare <app>`. This writes only to a staging directory
   outside the repo, and refuses if a file it touches has uncommitted changes.
2. **Read** `UPGRADING.md` in the app's skill for every entry since the app's
   commit, then read the prepare report in full.
3. **Resolve conflicts** in the staged files, not in the app. Each has
   `<<<<<<< app`, `||||||| base`, `=======`, `>>>>>>> template` sections. Keep
   what the app does and add what the template adds. If you can't tell what the
   app needs, ask.
4. **Check each note in the report:**
   - `both-added`: the app and the template each have a file at this path. If
     they are the same feature, merge by hand. If they are different things
     that share a name, the template's code that expects its own file will
     break: rename one side and fix what calls it.
   - `app-deleted`: the app removed this on purpose. Check that new template
     code does not depend on it.
   - `kept-modified`, `opaque-kept`, `deps-kept`: decide whether the app needs
     anything from upstream, and say so in your report.
   - `migrations`: template steps go after the app's own, never before.
     Positions in the list are version numbers, so an existing step never moves.
   - `git would ignore`: fix the app's `.gitignore`, or the file is never
     committed and the app breaks on a fresh clone.
5. **Apply:** `apply <app>`. It refuses while any conflict marker remains.
6. **Finish the dependencies and test:**
   - Go: `go mod tidy`, `go build ./...`, `go test ./...`.
   - Python: install `app/requirements.txt` in the app's venv, then run its tests.
   - Run `config check` against a **copy** of the real `config.yaml`. The decoder
     is strict, so a key the merge lost makes the app refuse to start.
7. **Report** what changed, what you decided, and anything not verified.

If it goes wrong, `rollback <app>` restores every file byte for byte. It refuses
if files changed after the apply, because it would lose that work.

After an apply, the next `prepare` or `apply` is refused until the first is
committed or rolled back.

## Warn the user before they deploy

Old code refuses a database that new code has migrated. Before deploying an
update that adds a migration, the user should back up. Undoing it later needs
`app.db.before-migration-*`, because rolling back the code alone is not enough.

## Changing a template

When you change `assets/template/` in either webserver skill, add an entry to
that skill's `UPGRADING.md` in the same change. Note new config keys, migrations
to append, renamed files, and what to test. Identify commits by SHA, because the
numbered commit messages have restarted before.

Uncommitted skill changes are not included in an update: a target is always a
commit. The stamp in a new app has `dirty: true` if the template had uncommitted
changes when it was scaffolded, and the commit is then only close.

## Tests

```bash
cd <skill_dir> && python3 -B -m unittest discover -s tests
```
