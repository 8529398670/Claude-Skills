# Upgrading apps built with python-webserver

Read by the `webserver-upgrade` skill. Add an entry whenever
`assets/template/` changes.

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

Server: add `app/models/themes.py`, `app/routes/themes.py` (+ `app/tests/test_theme_routes.py`) whole; add `themes` / `theme_id` on `User` in `app/models/users.py`; in `app/routes/account.py` (`me_row`) answer
`themes`, `themes_custom` and `theme_id`; in `app/routes/__init__.py` add
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
Marks: `live_login_links`, `multi_use_login_token_ttl_seconds`, `cancel-login-link`, `login_links.cancel_button`
Files: `static/js/login_links.js`, `app/tests/test_login_tokens.py`

A login link can now work once (still the default), up to N times (1-1000) or any
number of times, until a chosen expiry or never, always for the same account. The
create-user form gains Works and Expires selects, **New link** and **Sign in on
another device** open a dialog with the same two, and a user's row and the Account
screen show the live link with **Cancel link**. Take `app/models/login_tokens.py`
whole; its records gain `max_uses`, `unlimited` and `uses`, and old records still
read as once, so there is no migration. Callers move to `issue_login_link`:
`app/routes/admin.py` (`link_options`, `link_response`, `cancel_login_link`,
`login_link` on user rows) and `account.py` (`cancel_my_login_link`, `login_link`
in `me_row`), `app/manage.py` (`--uses`, `--expires`) and `app/core/bootstrap.py`
(`print_login_link` takes the link's info; `describe_seconds` is gone). Two new
routes, `POST /api/admin/users/{user_id}/cancel-login-link` and
`POST /api/account/login-link/cancel`, in `app/routes/__init__.py` and
`config/openapi.json` (plus the `LoginLink` schema and the new request and response
fields). New config key `auth.multi_use_login_token_ttl_seconds` (default 604800)
in `config/defaults.yaml`, `config.example.yaml`, `app/core/config.py` and
`config_live.py`. The printed banner now says `LOGIN LINK` rather than `ONE-TIME
LOGIN LINK`, so take the grep change in `docker/run.sh` with it;
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
link is refused. `python -m app manage reissue-login --user-id 2 --uses unlimited
--expires 7d` prints a banner saying so, and `scripts/login.sh 2 --uses 5` passes
the flag on.

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
Marks: `shortcuts-modal`, `Shortcuts.register`, `set_my_shortcuts`
Files: `static/js/shortcuts.js`, `app/models/shortcuts.py`, `app/routes/shortcuts.py`, `app/tests/test_shortcuts.py`

Take `static/js/shortcuts.js`, its `<script>` tag after `undo.js`, the `#shortcuts`
sidebar link, the `shortcuts` screen section and the modal in `static/index.html`, the
`Shortcuts.init()` call in `App.init` and `Shortcuts.signedIn( me )` in
`renderSignedIn`, `Api.setShortcuts`, the `pressed` helper in `Dom.editable`, the
`typeof Shortcuts` line in `Undo.onKey`, the `.shortcut-*` rules, `nav.shortcuts`,
`undo.shortcuts` and the `shortcuts` section of `language.yaml`, the `shortcuts`
field on `User` in `app/models/users.py` (and its `field` import's marker),
`shortcuts` in `me_row`, the route and import in `app/routes/__init__.py`, and
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
Marks: `BUCKET_IP_NAMES`, `ip-names-table`
Files: `app/models/ip_names.py`, `app/tests/test_ip_names.py`, `app/tests/test_ip_names_routes.py`

Names an address or a range on the Logs screen, so the log says who is who. Clicking
an address names it, and a list under the search manages them. The most specific name
wins, and `ip:` and free text search them. Names are applied as the log is read, so
the files do not change and old entries get names too. Downloads gain `ip_name` beside
`ip`. Only the owner sees names, on Logs and on Sessions. No migration: the bucket is
created at start. Take the model, the three routes under `/api/owner/logs/ip-names`
and their `config/openapi.json` entries, the `.log-ip` rules, the panel in
`static/index.html`, the changes to `static/js/logs.js`, `sessions.js` and `api.js`,
and the `logs.ip_*`, `logs.table_ip_*`, `undo.ip_name`, `errors.bad_ip_*` keys in
`language.yaml`. `logs.Query` takes the names as a second argument, and `Logger.download` takes a query.

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
Marks: `rename_my_key`, `undo.rename_key`, `rename_key:`

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
Marks: `switch-on`, `undo.rename_invite`, `rename_invite:`
Files: `app/tests/test_invite_links.py`

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
Marks: `rotate-secret`, `undo.rename_webhook`, `rename_webhook:`

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

**Upgrade `rename-files`: Renaming files.**
Needs: `files`, `edit-helpers`.
Marks: `rename_my_file`, `undo.rename_file`, `rename_file:`

Two new routes (`/api/files/:id/rename`, and the same under
`/api/admin/files`), so take the file model and route changes and the
`config/openapi.json` entries. On screen, `static/js/files.js` and `static/js/api.js`
change. Add `undo.rename_file` to `language.yaml`.

**Test:** rename a file by clicking its name, then Ctrl+Z puts it back. Rename it to
`a/b.txt`: the row shows `b.txt`. Download it: it has the new name and the same
contents. As an admin, Show everyone's files and rename someone else's.

**Upgrade `edit-users`: Admins editing a user's name and role.**
Needs: `edit-helpers`.
Marks: `set_user_role`, `role_changed_toast`
Files: `app/tests/test_admin_users.py`

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
Marks: `label_on`, `undo.rename_session`, `rename_session:`

One new route (`/api/account/sessions/:id/label`), so take
the session model and route changes and the `config/openapi.json` entry. The record
gains an optional `label`, and sessions from before it simply have none. On screen,
`static/js/sessions.js` and `static/js/api.js` change. Add `undo.rename_session` to
`language.yaml`.

**Test:** on the Account screen, click a device's name, type "Work laptop", and the
browser and system stay underneath. Ctrl+Z puts it back. Clear the name and the
worked-out one returns. An admin's Sessions view shows the name but cannot edit it.

**Upgrade `sort-columns`: Sorting table columns.**
Marks: `Pager.sortable`, `th-sort`, `sort_rows`

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
Marks: `Pager.search`, `match_rows`

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

**Whole features.** Each is its own choice. Everything else in this entry is
foundation that later fixes are built on, so it always lands.

**Upgrade `passwords`: Password login.**
Marks: `passwords:`
Files: `app/models/passwords.py`, `app/routes/passwords.py`, `app/tests/test_passwords.py`, `static/js/password.js`

**Upgrade `two_factor`: Two-factor sign-in.**
Marks: `two_factor:`
Files: `app/models/two_factor.py`, `app/routes/two_factor.py`, `static/js/two_factor.js`

**Upgrade `webhooks`: Webhooks.**
Marks: `webhooks:`
Files: `app/core/webhooks.py`, `app/models/webhooks.py`, `app/routes/webhooks.py`, `app/tests/test_webhooks.py`, `app/tests/webhook_cases.json`, `static/js/webhooks.js`

**Upgrade `files`: File uploads.**
Marks: `files_enabled`
Files: `app/models/files.py`, `app/routes/files.py`, `app/tests/test_files.py`, `app/tests/test_files_routes.py`, `static/js/files.js`

**Upgrade `metrics`: Metrics.**
Needs: `api_keys`.
Marks: `metrics:`
Files: `app/core/metrics.py`, `app/routes/metrics.py`

**Upgrade `backups`: Backups and key rotation.**
Marks: `backups:`
Files: `app/core/backup.py`, `app/core/rotate.py`, `app/tests/test_backup.py`, `app/tests/test_rotate.py`, `docker/restore.sh`, `docker/rotate-key.sh`

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
