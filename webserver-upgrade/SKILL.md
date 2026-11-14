---
name: webserver-upgrade
description: Bring fixes, and the upgrades the user chooses, from the go-webserver and python-webserver skills into apps already built with them, or show which apps are behind. Use this when the user wants to update, upgrade, sync or push skill or template changes to an existing app, asks "which apps are behind", "is TodoBoard up to date with the skill", "pull the new template into this app", "update my apps to the latest webserver skill", or has changed a webserver template and wants existing apps to get it. Not for starting a new app (use go-webserver or python-webserver) or for features that belong to one app alone.
---

# Upgrading apps built with the webserver skills

Apps are copies of a template, so a template change reaches an app only through
this skill. It renders the template at the app's commit and at the target
commit, in the app's own names, and merges only what changed. You handle what
a merge cannot decide.

**Fixes land in code the app has. Upgrades land only if the user wants them.** An
upgrade is anything that adds a route, screen, setting or button, and `UPGRADING.md`
labels each one. A fix lands even in switched-off code, so that code is safe if someone
turns it on. An upgrade to a feature the app has switched off is left out without
asking. Code already in the app is never removed.

**Only what the app has comes through.** A part cut out of an app, listed under
`pruned:` in its stamp, is cut out of both renders, so an update brings none of it. An
app from before pruning has no such list: it keeps everything it has, and fixes still
reach all of it. To bring a part in, use `add-feature`.

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

1. **Ask which upgrades they want:** run `offers <app>`, which is read-only, and
   ask the user about everything under "Ask about", in plain words. Only they can
   answer this, even if the app seems to need something. Don't ask about what it
   lists as left out.
2. **Prepare:** `prepare <app> --skip a,b`, naming the upgrades they turned down,
   or `--skip none` if they want them all. It refuses without `--skip` while there
   is anything to ask about. It writes only to a staging directory outside the
   repo, and refuses if a file it touches has uncommitted changes.
3. **Read** `UPGRADING.md` in the app's skill for every entry since the app's
   commit, then read the prepare report in full.
4. **Resolve conflicts** in the staged files, not in the app. Each has
   `<<<<<<< app`, `||||||| base`, `=======`, `>>>>>>> template` sections. Keep
   what the app does and add what the template adds. If you can't tell what the
   app needs, ask. Generated files are never merged by hand: for
   `server/routes/routes_generated.go` (Go) and `config/openapi.json`,
   `mcp.json` and `api.md`, take either side whole, and step 8 writes them
   again.
5. **Cut out what was left out,** also in the staged files. The report lists each
   left-out upgrade and where its marks are. Marks are only a sample: use the
   upgrade's entry to find all of it, including routes (each with its declaration and request/reply types),
   `language.yaml` keys, tests, and the same code in `config.example.yaml` if you
   change `defaults.yaml`. Compare with `render` when a line could belong to either
   a fix or an upgrade, and keep the fix. A JavaScript call to something you removed
   fails only in the browser, so search for every name you cut.
6. **Check each note in the report:**
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
7. **Apply:** `apply <app>`. It refuses while any conflict marker or any mark of a
   left-out upgrade remains. It writes the left-out upgrades to `skipped:` in the
   stamp, so they are not offered again.
8. **Finish the dependencies and test:**
   - Go: `go mod tidy`, `scripts/api_docs.sh` (it rewrites the route table and
     the API docs), `go build ./...`, `go test ./...`.
   - Python: install `app/requirements.txt` in the app's venv, run
     `scripts/api_docs.sh`, then its tests.
   - Run `config check` against a **copy** of the real `config.yaml`. The decoder
     is strict, so a key the merge lost makes the app refuse to start.
9. **Report** what changed, what you decided, and anything not verified. Name
   what was left out, and any fix that changes how something behaves, such as a
   new confirmation or a different list order.

If it goes wrong, `rollback <app>` restores every file byte for byte. It refuses
if files changed after the apply, because it would lose that work.

After an apply, the next `prepare` or `apply` is refused until the first is
committed or rolled back.

## Add a part the app was scaffolded without

1. Run `add-feature <app> webhooks`, or several, comma-separated. It stages the part
   at the commit the app is on, with anything it needs, and writes nothing to the app.
2. Resolve and check as in steps 4 and 6 of an update, then `apply <app>`. A switch
   comes on, the part leaves `pruned:`, and `rollback` undoes it.
3. Test as in step 8. New buckets or tables are created at start, so no data work is
   needed.

Do this before an update, or after: the two are separate.

## Take an upgrade that was left out

`status` lists what each app left out. A merge treats left-out code as removed on
purpose, so it never comes back by itself, and this is hand work:

1. Run `render <app> --out <empty dir outside the app>` for the template at the
   app's commit.
2. Port the upgrade from there, using its `UPGRADING.md` entry. Bring in anything
   it `Needs` that the app lacks.
3. Remove its name from `skipped:` in `.skill-origin.yaml`.
4. Test as in step 8 above. Nothing is committed.

## Warn the user before they deploy

Old code refuses a database that new code has migrated. Before deploying an
update that adds a migration, the user should back up. Undoing it later needs
`app.db.before-migration-*`, because rolling back the code alone is not enough.

## Changing a template

Code that belongs to one part needs a marker (the top of `scripts/prune.py` in either
skill explains them). Then check the template still works with parts cut:

```bash
python3 <skill_dir>/scripts/check_features.py go-webserver
python3 <skill_dir>/scripts/check_features.py python-webserver
```

It runs everything, nothing, each part alone and each part removed. `--no-tests` only
compiles, and `--only webhooks` runs one part's sets.

When you change `assets/template/` in either webserver skill, add an entry to
that skill's `UPGRADING.md` in the same change. Note new config keys, migrations
to append, renamed files, and what to test. Identify commits by SHA, because the
numbered commit messages have restarted before. Label anything that isn't a fix,
as "Entry format" at the end of that file explains, or it reaches every app
without anyone being asked.

Uncommitted skill changes are not included in an update: a target is always a
commit.

## Tests

```bash
cd <skill_dir> && python3 -B -m unittest discover -s tests
```
