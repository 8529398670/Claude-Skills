# Plan: open cards on the claude-skills board

Board: TodoBoard board `claude-skills` (host is in global CLAUDE.md)
Written 2026-11-06, from an interview with the owner. Line references were
checked against the working tree that day.

## Scope

- Change only `~/.claude/skills`. No app built from the templates is touched,
  stamped, trimmed or upgraded as part of this work.
- All work goes on `master`. No branches or worktrees. Commits and pushes are
  left to `gitPushUpdate.sh`.
- Card 1 (no git in `run.sh`) is out: the owner considers it already solved.
  The earlier idea of removing the commit hash from the scripts is dropped.

## Rules for every card

- Do it in both `go-webserver` and `python-webserver`. The frontends differ only
  in comments and a vendor README, so `static/` changes copy across.
- All text goes in `language.yaml`.
- New routes go in `config/openapi.json`, as POST verb routes with CSRF through
  `Api.post`. No PUT or PATCH.
- Each template change gets an `UPGRADING.md` entry in both skills. Anything
  that adds a route, screen, setting or button is labelled as an Upgrade, with
  Marks. Unlabelled items are fixes.
- Light stays the default theme. Dark is opt-in only.
- Run each skill's tests before calling a card done, and check UI changes in the
  demo app.

## Build order

1. Cards 35, 36, 37: feedback
2. Card 40: small fixes and accessibility
3. Card 38: row actions
4. Card 39: settings tools
5. Card 43: themes (preview page first)
6. Card 42: IP nicknames
7. Card 41: optional features and pruning

Card 41 goes last so its markers go around settled code once.

## Cards 35, 36, 37: feedback

Shared helpers in `dom.js`: dismiss and copy for boxes, and one field-error
helper.

**Card 35, tell people when something worked**
- Add toasts to Revoke access (`app.js:346`), Reset 2FA (`app.js:434`), Delete
  day (`logs.js:352`) and file upload (`files.js`, `uploadAll`).
- Saving an unchanged name gives feedback (`app.js:219`, returns silently).
- Remove PIN asks first (`pin.js:28`). A removed PIN can't be restored.
- New link asks first (`app.js:318`), because it kills the earlier link. Its box
  says whose link it is and scrolls into view.

**Card 36, dismiss buttons**
- All six shown-once boxes get a dismiss button: `my-link-box`, `key-secret-box`,
  `login-link-box`, `invite-link-box`, `user-key-secret-box`,
  `webhook-secret-box`.
- The error banner stays until it is dismissed or replaced, and gets a copy
  button. This replaces the 12-second `Dom.flash` at `app.js:492`.

**Card 37, field errors beside the field**
- Use the helper for the password form (`password.js:109-116`) and the other
  forms that send field problems to the banner.
- Set `aria-invalid` and `aria-describedby`.
- Clear old problem text on a failed re-save (`settings.js:221` only removes the
  class today).

## Card 40: small fixes and accessibility

- Webhook deliveries: close button, scroll into view, clear the previous hook's
  rows at once (`webhooks.js`, `showDeliveries`). Same stale-row fix for
  `Sessions.showFor`.
- Logs: copy button for an entry's JSON.
- Files: a drop during an upload is no longer silently ignored (`files.js:84`).
  Tell the person.
- API keys: clear the days field after create (`keys.js:70`, `keys.js:127`).
- Keyboard: `/` focuses search, `?` opens a shortcut list. This is an Upgrade.
- Accessibility: `th scope`, labels on both `nav` elements, row context on
  repeated row buttons, keep focus when a row refresh deletes the focused button.
- Remove the two leftover `language.yaml` comments about delete confirms.

## Card 38: row actions

- Overflow menu at every width. The one or two most used actions stay visible,
  the rest go in a ⋯ menu. A Users row can have 8 buttons (`app.js:479`).
- Stack tables on narrow screens.
- Review the 44px exemption for `.btn-sm` (`style.css:96`).
- The menu must not be clipped by `.table-responsive`.
- Test at real phone width. The original audit only read the code.

## Card 39: settings tools

- Filter box and jump links across the roughly 60 fields.
- Reset to default fills the field and marks it changed. The normal Save applies
  it, so no new route is needed.
- If a save fails on a field the filter hides, the filter clears so the error
  shows.

## Card 43: themes

Build a preview page first, showing all 12 presets with layers and tinted chrome.
The owner picks and adjusts, then it goes into the templates.

- **Roles:** the main colour stays on actions (buttons, links). A true second
  colour marks where you are: active nav, headings, progress bars, info badges.
- **Layers:** page, card and nested card each get their own shade, tinted toward
  the second colour. Worked out from the preset, so presets stay short.
- **Chrome:** lightly tinted navbar and sidebar.
- Text on the second colour is worked out automatically, as `--app-accent-fg` is.
- The swatch in the picker shows both colours.
- Add a contrast test across all presets, in light and dark.

## Card 42: IP nicknames

- Click an IP on the Logs screen to name it. A small list manages the names.
- Matches exact IPs and ranges such as `10.0.0.0/24`. The most specific match
  wins.
- Owner only, stored in the database, applied when logs are read, so old entries
  get names too. The log files themselves don't change.
- Shown on the Logs and Sessions screens. `ip:` search matches names.
- Downloads and exports gain a nickname field, worked out at export time, beside
  the raw IP.
- Logs store only IPs, so a hostname is a name someone types. No reverse DNS.
- To check first: whether `DownloadLogDay` streams the raw file today. A field
  added at export time means reading each line, including gzipped and encrypted
  ones.

## Card 41: optional features and pruning

**Goal:** an app contains only the features it uses, and upgrades bring in only
those.

- **Scope of optional:** split out as much as possible, so the 9 current switches
  plus core pieces such as logs, sessions, background jobs, maintenance mode and
  the settings screen. Probably 15 to 20 pieces. Start by mapping what depends
  on what.
- **Markers:** mark each feature's code in shared files (`routes.go`,
  `index.html`, `language.yaml`, `openapi.json`, `api.js`). Markers exist only in
  the template and are stripped from generated apps.
- **One pruning step,** used by both `scaffold.py` and `webserver-upgrade`.
- **New app defaults:** the 9 switches are worked out from what the app does, as
  now. Newly optional core pieces stay on unless told off.
- **Fix rule:** if an app still has a feature's code, fixes keep landing. Only
  absent code is skipped.
- **`add-feature`:** a new `webserver-upgrade` command. It adds the code, turns
  the switch on and adds the feature to the stamp, staged and reviewed like any
  upgrade. No data work: new buckets or tables are created at start as today.
- **Old apps:** the tool must not delete code from an app that predates markers.
  Base renders at old commits have no markers, so the merge must not read pruned
  code as removed on purpose. Cover this in `test_upgrade.py`, using a fixture,
  not a real app.
- **Test bar:** build and pass tests for all on, all off, each alone and each
  removed, in Go and Python. About 40 builds per skill.
- **Docs:** rewrite the "copies every feature but switches on only the ones it
  needs" text in both `SKILL.md` descriptions, the "Fixes always land" rule and
  the switched-off code rule in `webserver-upgrade/SKILL.md`, and the `FEATURES`
  comment in `scaffold.py`.

Turning a feature on later is an `add-feature` run, no longer one setting.
