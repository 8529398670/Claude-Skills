# Frontend reference

The rules here are what keep the frontend from drifting into a build
pipeline. They are cheap to follow from the start and expensive to retrofit.

## No build step, ever

No npm, no bundler, no transpiler, no JSX, no TypeScript compile. Files in
`static/` are served to the browser exactly as they sit on disk.

This is not nostalgia. It means:

- What you debug in the browser is what is on disk -- no source maps, no
  "works in dev, breaks in prod" from a differing build.
- A deploy is a file copy. `server/static` serves text assets with
  `no-store` and gzips them per request, so an edit is live on the next
  refresh with no cache-busting machinery.
- There is no dependency tree to audit, no lockfile to keep current, and no
  build that can break independently of the app.

The cost is real: no npm ecosystem, and you write plain DOM code. For an app
whose server is deliberately thin, that has been a good trade.

## Adding a third-party library

Download it once and serve it yourself:

```bash
misc/vendor.sh https://cdnjs.cloudflare.com/ajax/libs/htmx.org/2.0.4/htmx.min.js
```

Then `<script src="/vendor/htmx.min.js"></script>`. Commit the file -- it is
part of the app, not a build artifact. It is also embedded into any binary
`build.sh` produces, because everything under `static/` is, so a vendored
library travels with the single-file build automatically.

**Do not reference a CDN directly.** `server/middleware` sets `script-src
'self'`, so a cross-origin `<script src>` is refused by the browser. That is
the policy working, not a bug. Self-hosting also means a CDN outage, a DNS
block, or a corporate proxy cannot take the page away, and the version cannot
change under you because someone re-tagged a release.

If a library genuinely cannot be self-hosted, that is a decision to raise with
the user, not to fix by loosening the CSP.

## The language.yaml contract

Every user-facing string lives in `config/language.yaml`. None are hardcoded in HTML
or JS.

```html
<h2 data-i18n="admin.heading"></h2>
<input data-i18n-attr="placeholder:admin.new_name_placeholder" />
```

`static/js/i18n.js` fetches `/api/language` once and stamps the values in.
Two rules make this more than indirection:

1. **Rewording or translating the app is a YAML edit plus a restart.** No
   markup or code changes.
2. **A missing or empty key hides its element.** That is how you switch a
   label, a button, or a whole section off -- set it to `""` and it
   disappears, rather than rendering blank or leaking a raw key name.

Note the asymmetry in `I18n.apply()`: a missing key *hides* an element, but a
present key never *un-hides* one. Elements like the error banner and the admin
panel are shown by `app.js` based on state, and i18n has no business
overruling that. It decides what text says, not what the app reveals.

`language.yaml` is served to anyone, signed in or not, because signed-out
pages need text too. **Never put anything sensitive in it.**

Adding a screen means adding its keys to `language.yaml` in the same commit.
A `data-i18n` key with no entry renders as a hidden element, which looks
exactly like a missing feature.

## Disk during development, embedded when shipped

Everything under `static/` is compiled into the binary by the `go:embed` lines in `main.go`. At
startup the server prefers a real `./static` directory if one exists and falls
back to the embedded copy otherwise, logging which it chose:

```
[assets] static=disk /srv/app/static language=embedded
```

So editing files in a source checkout behaves exactly as described above --
edit, refresh, done -- while a binary copied somewhere on its own serves what
is inside it. Nothing about writing frontend code changes between the two; the
only thing to remember is that a new asset directory placed *outside*
`static/` would not be embedded.

## JS file conventions

Plain scripts, loaded in dependency order, each adding one global:

| File | Responsibility |
|---|---|
| `theme.js` | Light, dark, or another theme, and the colours worked out from it. Loaded in `<head>`; see below. |
| `themes_editor.js` | The Appearance screen: your themes, and the admin's defaults. See Themes below. |
| `dom.js` | Element helpers, `Dom.copy`, `Dom.toast`, `Dom.onSubmit`, `Dom.confirm`, `Dom.editable`, `Dom.form`, `Dom.fieldError`, `Dom.when`. No app logic. |
| `qr.js` | QR codes, for the authenticator app and for login and invite links. |
| `login_links.js` | How often and how long a login link works: the choices on the create-user form and the New link and Sign in on another device dialogs, and how a link is described. |
| `i18n.js` | Loads and applies `language.yaml`. |
| `undo.js` | Ctrl+Z / Ctrl+Y: the undo/redo history every screen records into. |
| `shortcuts.js` | Every keyboard shortcut, and the Shortcuts screen where each person changes theirs (saved to their account, shown for their OS). `Shortcuts.register` adds an app's own. |
| `api.js` | Every call to the server. Attaches cookies and the CSRF token. |
| `track.js` | Sends the screens and clicks the activity log records. |
| `nav.js` | The sidebar: which screen the URL hash shows. |
| `pager.js` | Fills a table from a paged route, with Load more and an empty row. |
| `keys.js` | Your API keys card on Account, and the admin's All API keys screen. An example of the rule below. |
| `sessions.js` | Signed-in devices: your own (a folding card on Account), and an admin's view of an account's. |
| `files.js` | The Files screen: uploads in pieces, with progress, resume and drops. |
| `join.js` | Joining through an invite link. |
| `pin.js` | The PIN card on Account. |
| `password.js` | Password sign-in, and the Password card on Account. |
| `two_factor.js` | The two-factor step at sign-in, and its card on Account. |
| `invites.js` | The Invite links screen. |
| `logs.js` | The owner's Logs screen, and its names for addresses. |
| `jobs.js` | The owner's Jobs screen. |
| `webhooks.js` | The owner's Webhooks screen. |
| `settings.js` | The owner's Settings screen: config.yaml as a form, with a filter, jump links and Use default. |
| `fresh.js` | Asks the server again when someone comes back to the tab. |
| `app.js` | The account + admin screen. Last, because it starts everything. |

**Give each new screen its own file with its own `init()`**, included from
that page's HTML. Do not keep appending to `app.js` -- the whole layout exists
to avoid a monolith, and the frontend is the easiest place to accidentally
build one.

`keys.js` is the worked example: it owns its own screen, its own tables, and its
own refresh, and `app.js` knows it exists in exactly one line
(`Keys.init( me )`). Copy that shape rather than the alternative.

Anything talking to the server goes through `api.js`. `Api.post` attaches the
CSRF token automatically, so a new endpoint cannot forget it.

### Never build HTML from strings containing user data

Build elements with `Dom.el` and `text:`, which sets `textContent`:

```js
Dom.el( "td" , { text: user.display_name } )
```

A display name containing `<img src=x onerror=...>` then renders as those
literal characters. The same row assembled with `innerHTML` executes it.
There is deliberately no escaping helper in `dom.js` -- the safe path should
be the only path.

## Bootstrap

The UI is [Bootstrap 5](https://getbootstrap.com/docs/5.3/) plus [Bootstrap
Icons](https://icons.getbootstrap.com/), vendored like any other library --
`static/vendor/README.md` lists the files and versions. Use its components
and utility classes (`card`, `btn`, `form-control`, `table`, `alert`, `badge`,
`d-flex`, `gap-2`, `mt-3`, ...) before writing CSS, and an icon is
`<i class="bi bi-<name>" aria-hidden="true"></i>`.

Two things to know when mixing it with this frontend's rules:

- **`data-i18n` replaces an element's text**, icons included. Where an icon
  sits beside a label, the key goes on an inner `<span>`:
  `<a class="nav-link"><i class="bi bi-people"></i><span data-i18n="nav.users"></span></a>`.
- **`hidden` must beat Bootstrap's display utilities.** `.d-flex` and friends
  are `!important`, so `style.css` restates `[hidden] { display: none
  !important; }` after Bootstrap. Keep that rule; the app shows and hides
  whole sections with the attribute.

To upgrade, delete the old files from `static/vendor/` and re-run
`misc/vendor.sh` for the new version (the commands are in the vendor README).

## The shell every page shares

- **Navbar on top, sidebar on the left, one screen at a time.** The app's
  name in the navbar is the home link. The sidebar is a Bootstrap
  `offcanvas-lg`: a fixed column from the `lg` breakpoint up, a slide-in panel
  on a phone. The menu button (`bi-list`, far left of the navbar) works at
  every width: on a phone it slides the panel in, on a wide screen it
  collapses and expands the column (`Nav.toggleSidebar`, a `sidebar-collapsed`
  class on `<html>`, remembered in `localStorage`). The content column stays
  centred either way.
- **A screen is a `<section class="view" data-view="name">` in `<main>`
  plus a sidebar link `<a class="nav-link" href="#name" data-nav="name">`.**
  The view is not a card itself. It holds an `<h2 class="view-title">`, an
  optional `<p class="view-description">`, and then a stack of
  `<section class="card">`s, each with an `<h3 class="card-title">`.
  **One card per job**: a form that makes something and the list it adds to
  are two cards ("Add someone" and "People" on Users), and a set of separate
  settings is one card each (Account: your name, another device, password,
  PIN, two-factor, API keys, where you're signed in, sign out). Each thing
  then has its own edge, and a feature that is off takes its whole card
  with it. A screen that does one thing is one card. Never split one card
  into parts with `<hr>`; make another card instead. A list that most visits
  don't need folds away: `<details class="card card-collapse">` with a
  `<summary class="card-header">` holding the `<h3>`, closed by default,
  and fetched only when opened (see `sessions.js`).
  Something about your own account is a card on Account, not a new screen.
  A hash can name a card (`#two-factor-panel`) and `Nav` opens its screen
  and scrolls to it; `Nav.moved` keeps the hashes of screens that became
  cards working.
  `nav.js` shows the one the URL hash names, so back/forward, reload and a
  pasted link all work with no router. The screen's own JS decides whether it
  exists for this user by un-hiding its section; `Nav.init()` (called last in
  `App.renderSignedIn`) offers only the sections that were un-hidden, and a
  hash for one that wasn't falls back to `#account`. Admin-only links go
  inside `#sidebar-admin`, whose heading disappears when it has nothing left.
- **Your account lives in the sidebar, not a header icon.** The first
  sidebar entry is Account, and its second line (`#sidebar-user`) says who is
  signed in (`App.labelAccount`). `#app-nav` on the right of the navbar is
  left for the app's own top-level links.
- **Signed-out pages have no sidebar.** The signed-out, login-confirm and
  join cards sit in `<main>` on their own.
- **Confirmations are toasts.** `Dom.toast( I18n.get( "ui.saved" ) , "ok" )`
  shows a Bootstrap toast (`#toast`) bottom right for four seconds;
  `Dom.copy( element )` toasts `ui.copied` itself. A message that must stay
  (a key shown once) gets its own `alert` on the screen instead, and
  page-level errors go in `#error-banner` at the top of `<main>`.
- **Statuses are badges.** `Dom.badge( text , "success" )` for a table cell.

### Undo and redo

Ctrl+Z (Cmd+Z on a Mac) undoes the last thing done on the page; Ctrl+Y or
Ctrl+Shift+Z redoes it. Actions that matter -- a delete, a rename -- also
show a toast with an **Undo** button, which is how a phone or anyone who does
not know the shortcut gets the same thing. `static/js/undo.js` owns the
history and the button; screens only hand it their actions. The keys are
`static/js/shortcuts.js`'s, so each person can change them on the Shortcuts
screen (an app without `shortcuts` gets the fixed ones in `Undo.onKey`). Out of
the box it covers theme picks, folding the sidebar, renaming the account,
deleting an API key, invite link, webhook or file, and saving Settings.

```js
const previous = this.current;
Undo.run( {
  label: I18n.get( "undo.theme" ),          // "Undone: theme change"
  merge: "theme",                           // clicking through five is one step
  do: () => this.pick( next ),
  undo: () => this.pick( previous ),
} );
```

- **`Undo.run`** performs `do` and records it. **`Undo.record( { label ,
  undo , redo } )`** is for a change that has already happened -- a form that
  saved to the server first (`App.onRename`), a delete (`Keys.deleteButton`).
- **`toast: true`** announces the action with an Undo button ("API key
  deleted · Undo", 8 seconds, held open while hovered or focused). Use it
  for deletes and anything whose effect is not obvious on screen; leave it
  off for things the person just watched happen (a theme, the sidebar).
  Undo and redo themselves always toast, with the opposite button.
- **`merge: "key"`** folds quick repeats of the same kind (within 1.5s) into
  one step that undoes back to where the run started. Sliders, drags and
  pickers want it.
- **Screens are not steps.** Moving between screens is Back and Forward, not
  Ctrl+Z. Each action remembers the screen it happened on, and undoing it
  from elsewhere goes back there first, so the person sees what came back.
  `screen: null` opts out (the sidebar looks the same everywhere).
- **Text fields keep their own undo.** While an `input`, `textarea`,
  `select` or `contenteditable` has focus the shortcuts are left to the
  browser. So a form that has saved calls **`Undo.saved( form )`**, which
  lets go of the focus -- without it, Ctrl+Z after pressing Enter would undo
  the typing, not the save.
- **Steps may be async.** An undo that calls `Api` is awaited before the
  next key is taken, and one that throws is dropped with an error toast.
  Undoing a server change means calling the server, not repainting the old
  value: `App.applyRename` is the example.
- **Server-backed undo checks before it writes.** Each direction sends the
  value it expects to find (`Api.rename( name , expect )`); the server
  answers 409 when someone changed it in the meantime, and the step is
  dropped with "it was changed somewhere else" instead of overwriting their
  change. Give any new server-backed undo the same `expect`.
- **A step must not record itself.** `Undo.replaying` is set while a step
  runs, so UI code it triggers does not add to the history.
- **Deletes are soft, revokes are final.** The server only marks a deleted
  key or invite link and can restore it for 10 minutes (to whoever deleted
  it, or an admin), so delete asks no confirmation and offers Undo; the
  undo calls the `.../restore` endpoint. A revoke says "stop trusting this",
  so it keeps its confirm dialog and is never undoable -- an undo would
  quietly trust a possibly leaked credential again. Disabling a user is the
  same: it asks first and has no undo, because enabling does not bring back
  the sessions and links it ended. An invite link is the exception: it can be
  switched back on, behind a confirm rather than an undo, because it is how the
  people who joined sign in on a new device. Logout and issuing a login link,
  invite or key stay out too. A new deletable thing follows the
  same soft-delete + restore pattern (`references/architecture.md`).
- History is in memory: a reload (including logout) starts it fresh.
  `Undo.canUndo()` / `canRedo()` are there for toolbar buttons.

Labels and the toast prefixes are the `undo:` block in `language.yaml`.
A label is a phrase that reads after "Undone:" ("name changed"); every new
undoable action adds its label there.

### Activity tracking and the Logs screen

`track.js` reports what people do on the page to the activity log; the
server logs requests on its own. `Track.init( me )` runs for every signed-in
user (when `me.logs_enabled`), and from then on records, with the current
screen:

- `session_start` once per tab: time zone, language, screen and viewport
  size, platform, touch points, theme. A random per-tab id rides on every
  event, so two tabs of one person read apart.
- `nav` from `Nav.render`, `undo`/`redo` from `Undo.step`, and `fetch_fail`
  from `Api.request` (no answer, or a 5xx).
- `click` on any button, link, `summary`, checkbox or radio (when
  `me.logs_ui_clicks`), named by its `data-i18n` key, else `#id`, else
  `aria-label`, else the first words of its label. Only `#hash` hrefs are
  kept: any other href could be a login link.
- `submit` with the form's id and its field **names**.
- `error` from `window.onerror` and unhandled promise rejections.

**Never send a value.** That one rule is what keeps login links, keys and
PINs, passwords and two-factor codes out of the log, since they only ever live in fields and in `<code>`
boxes. A new event follows it: `Track.event( "click" , { target , detail } )`
with names and ids only. Mark a part of the page that should not be tracked
with `data-no-track` (the Logs screen is, or reading the log would fill it).

Events go in batches every 5 seconds, at 50, and when the tab is hidden,
straight to `fetch` with `keepalive` so the last batch outlives a closing
tab. A failed send is dropped, not retried.

`logs.js` is the owner's screen (`Logs.init( me )` returns at once unless
`me.is_owner`). It shows one day at a time as a `<details>` block, today unless
another date is picked on the month calendar above it (`renderCalendar`, built
from the days listing: shaded dates have entries, a dot means errors, and while
searching only matching dates can be picked). A day's rows load when it is
opened, 200 at a time with "Load more".
The search box goes to the server as typed (debounced), and the kind buttons
add a `kind:` term. Rows are built with `Dom.el` like everything else, and a
row's JSON shows in a `<pre>` via `textContent`. Its text is the `logs:`
block in `language.yaml`.

Live on the Logs screen is an `EventSource` on `Api.logStreamPath()`, not a
timer. The server sends `changed` when new lines reach the disk, and `refresh()`
runs, at most once per `liveMs`, holding off while an entry is expanded. The
stream closes when the screen is hidden and reopens on return. A closed stream
means the server refused it, so the button turns off and `logs.live_lost` shows.
See Live updates in `architecture.md`.

### The Jobs screen

`jobs.js`, owner only, lists what `GET /api/owner/jobs` returns, so a job an app
adds appears with no frontend change. Give one a friendlier name with
`jobs.job_<name>` in `language.yaml`, dashes as underscores. A run outlasts the
call that starts it, so the screen looks again every `refreshMs` while something
is running there.

### The Files screen

`files.js`, for everyone unless `files.enabled` is off: upload, download, rename by
clicking the name, and delete, the last two with Undo. An admin gets a switch to see everyone's files, with an Owner column.
An upload retries a failed piece and goes on from the `received` the server reports,
and deletes itself if it gives up, so it does not hold the quota. `Api.request` takes
`raw` and `headers` for this: the one body that is not JSON. See
[File uploads](architecture.md#file-uploads).

### The Webhooks screen

`webhooks.js`, owner only: add an endpoint, turn it on or off, send a test, and
see how its deliveries went. Its name edits in place, and Edit changes the address
and events; both can be undone. New secret asks first and has no undo. A signing
secret appears once, in a box like a new key's. Event names are shown as they are, because they are what the receiver
sees. See [Webhooks](architecture.md#webhooks).

### Long lists

`pager.js` fills a table from a paged route and offers "Load more" under it while
there are rows left. `Pager.load( tableId , field , fetchPage , renderRow )`
answers the page, or `null` if the call failed. A refresh is a load from the start,
so a change on the screen never leaves a stale page behind. Use it for any new
list: a plain `forEach` would quietly stop at the first page. See
[Paging lists](architecture.md#paging-lists).

A column sorts on the server, because the browser only holds a page. Give its `th`
`data-sort="<row field>"` with a `.th-sort` button inside, call
`Pager.sortable( tableId , refresh )` once, pass `Pager.params( tableId )` to the Api
call, and list the field where the route sends its page. Status columns worked out
in the browser have no field to sort on, so they stay plain.

Search works the same way. `Pager.search( tableId , inputId , refresh )` wires a box,
and `Pager.filter` a checkbox such as Hide revoked and expired keys; the route calls
`matchRows` or `match_rows` with the fields to look in. An empty table then says
`ui.no_matches` rather than the table's own empty text. `Dom.typed( input , run )` is
the wait for typing to pause, for any box that loads as you type.

### Where you're signed in

`sessions.js` draws the devices table on the Account screen. It reads a short
name out of the User-Agent, only to help someone tell their devices apart. A device
can be named by clicking it, with Undo. The name worked out from the User-Agent
stays underneath, so a label cannot hide what a device says it is.
Signing a device out asks first and has no undo.

### Error references

A 5xx body carries `request_id`, and `Api.request` turns it into the error's
message using `errors.reference`, so every screen shows it through
`App.showError` with no extra code. The banner stays up 12 seconds, long enough
to copy it down.

### The Settings screen

`settings.js` is the owner's other screen (`Settings.init( me )` returns at
once unless `me.is_owner`): `config.yaml` as a form, one field per key, built
entirely from `GET /api/owner/settings`. The server works out each key's type,
value, default, help (the comment above it in `defaults.yaml`) and apply mode
(`[live]` / `[restart]` / `[scripts]`), so `settings.js` knows nothing about
any one setting -- a key added to `defaults.yaml` appears on the screen with no
frontend change. Bool is a switch, int a number field, a list a textarea with
one item per line.

Save sends only the changed fields plus the `version` the form was loaded
from. A 409 `settings_changed` means the file changed in the meantime (a hand
edit, another tab) and shows a "Reload from file" prompt instead of
overwriting; a 400 `invalid_settings` puts each problem under the field it
names (every problem starts with its `section.key`). The save is recorded in
`Undo` as a server-backed step whose undo posts the old values, expecting the
version the save produced -- so an undo after a hand edit is refused rather
than clobbering it. While the screen is showing and nothing is typed in it, it
re-reads the file every 10 seconds, which is how a hand edit or a reload error
appears without a refresh. Its text is the `settings:` block in
`language.yaml`; the help under each field is not, since it comes from
`defaults.yaml`.

### Themes

`static/js/theme.js` is loaded in `<head>`, before the stylesheet paints, and
applies the kept theme at once, so a dark choice never flashes light.
`Theme.builtIn` lists light, dark and ten ready-made themes -- ocean, forest,
sepia, grape, pink, WSU, midnight, slate, cerise, WSU dark (Wright State
University green and gold). A theme is `{ id , name , mode , accent , second ,
background? , surface? , overrides? }`: a mode, an `accent` for what you can
press, a `second` for where you are, optional page and card colours.
`Theme.derived( theme )` works out everything else: the page, card, nested card
(`.app-nested`) and navbar shades, labels that read on each fill, and text
colours pushed until they reach 4.5. `Theme.colours( theme )` is that with the
theme's `overrides` on top -- any derived colour replaced by name (`text`,
`link`, `navLink`, `border`, `info`, ...). They are set inline on `<html>` as
the `--app-*` tokens (`--app-text` feeds `--bs-body-color`). `Theme.check( theme )`
lists the pairs in `Theme.pairs` that fall under 4.5; `scripts/check_themes.js`
holds every built-in theme to none, and the tests run it. `ThemePalettes` keeps
brand colours by name, used or not. A dark mode sets Bootstrap's
`data-bs-theme="dark"` on `<html>`.

With `appearance`, themes live on the server. There are two lists: the app's
defaults, which an admin edits (meta key `theme_defaults`; nothing stored means
`Theme.builtIn`), and each person's own (`themes` on the user; nothing stored
means they follow the defaults, including later changes). `/api/me` answers
`themes` (theirs, else the defaults, else null), `themes_custom` and
`theme_id`. `Theme.load( me )` takes that; `Theme.pick( id )` changes the one
in use (`POST /api/account/theme`); `Theme.store( list , id )` replaces their
list (`POST /api/account/themes`, null for "back to the defaults"). Admins use
`GET`/`POST /api/admin/theme-defaults`. The server checks shape only (ids,
names, `#rrggbb`, known override names, 1-40 themes); contrast is the screen's
to warn about. The theme in use is also kept whole in `localStorage`, so the
first paint and the signed-out pages are already in it; `/api/me` corrects it.

The Appearance screen is `themes_editor.js` (`ThemeEditor.init( me )`): a list
(use, new, duplicate, rename, delete) beside an editor with a colour picker and
hex field for the mode's base colours and every derived one, each derived one
marked Auto until changed and with a Reset back to the worked-out colour, a
live preview, and a box naming any text that is hard to read. Admins get a
"Defaults for everyone" tab with the same editor. Every change goes through
`Undo` and sends the whole list. Names of built-in themes come from
`theme.preset_<id>` in language.yaml until a list is saved.

Without `appearance` there is no server state: the navbar button switches
light and dark in this browser's `localStorage`. Whatever you do, first load is
always light.

### Nothing moves under the reader

A press must never make the page jump: the list someone was reading stays under
their eyes, whatever the press redraws. This is a rule for every screen, and
`dom.js` carries most of it for you.

- **Every press is held still.** `Dom.press`, which every `Dom.el` button and
  every `Dom.onPress` button goes through, runs the handler inside
  `Dom.steady( button , work )`. When the work is done, the window is scrolled so
  the button sits where it sat before. If the button was redrawn, its replacement
  is used: same `id`, or same `data-key`, so give buttons that get rebuilt a
  `data-key`. Otherwise the nearest of its old ancestors still showing is used.
  It does nothing if the reader scrolled meanwhile or the screen changed. A button
  that means to take the reader elsewhere gets `data-scroll="free"`. Call
  `Dom.steady( anchor , work )` yourself for changes that do not come from a press
  (Live updates, timers).
- **Build, then swap.** Never `Dom.clear` a container and fill it after an
  `await`: for that moment the page is shorter, the browser pulls the scroll
  position up, and it does not go back down. Fetch first, build the new nodes, then
  `Dom.swap( holder , ...nodes )`. Clearing and filling in the same synchronous
  step is fine. If what is left is too short to stay where the reader was,
  `Dom.steady` gives the page blank room at its foot (`#steady-runway`), which
  goes away as they scroll up, rather than letting the browser pull the page up. A container that
  really must wait empty uses `Dom.hold( element )` and calls the `release` it
  answers (`Pager.reset` does).
- **Update in place.** A toggle flips its own classes and `aria-pressed`. It does
  not rebuild its row of buttons (see `Logs.paintKinds`).
- **Live data adds; it does not rebuild.** A screen that updates by itself adds
  what is new beside what is there, keeps the row being read where it is, and
  never throws away pages already loaded (see `Logs.liveRead`).
- **Scroll only when asked.** `Dom.reveal( element )` brings something the reader
  just opened into view (a login link, a shown-once secret, the error banner) and
  tells `Dom.steady` the move is meant. Don't call `scrollIntoView` or
  `window.scrollTo` directly (`Dom.movedOnPurpose()` first if you must), and a
  `focus()` after a redraw passes `{ preventScroll: true }`.
- **The scrollbar keeps its gutter.** `style.css` sets
  `scrollbar-gutter: stable`, so a page that crosses one screen in height does
  not shift sideways.

A dev build (version `dev`) prints `[steady] <element> moved Npx` in the console
whenever `Dom.steady` had to correct. The correction hides the jump, but the
warning means something on that screen moves the page. Fix it at the source.

## What every screen gets

These live in `dom.js`, `api.js` and `nav.js`, so a new screen has them by using
them rather than by writing them again.

- **`Dom.onSubmit( form , handler )`** is a form's submit handler, with its
  button busy and showing a spinner until `handler` finishes, so a second click
  sends nothing twice. `handler` has to be async and to await what it sends, or
  the button comes back early.
- **Buttons go busy by themselves.** One from `Dom.el` with an async click handler
  does, and `Dom.onPress( button , handler )` does that for one in the HTML. Neither
  should call `Dom.busy( button , work )` as well: the press is already busy, so
  the inner call would skip its work. A question the handler asks counts as part
  of the wait, with the spinner hidden until it is answered.
- **`Dom.confirm( question )`** answers true or false from a themed modal. An
  empty question answers true, so a `language.yaml` key set to `""` stops the
  asking. Cancel has the focus: Enter on a question nobody read deletes nothing.
- **`Dom.fieldError( input , message )`** shows a problem under one field and ties
  it to the field for screen readers; `Dom.clearFieldError( input )` takes it away.
  Typing clears it too. `App.showFieldError( code , { code: inputId } )` is the usual
  way in: a refusal about a field goes beside it, and any other goes to the banner.
- **A box that stays until dismissed** is an `.alert-dismissible` with a `.btn-close`
  carrying `data-box-close="<id to focus next>"`. Closing clears any `code` inside,
  so a shown-once value does not stay in the page.
- **`Dom.rowActions( name , buttons , keep )`** is a table row's buttons, grouped under
  the row's name so a screen reader can say whose Delete it is. It drops nulls, keeps
  the first two (or `keep`) in the row, and folds the rest into a ⋯ menu, so put the
  most used first.
- **`Pager.label( table )`** names each cell after its column, for the stacked tables
  on a phone. `Pager.load` does it; a table filled by hand calls it after.
- **`Pager.reset( table )`** empties a table at once, for when what it is about to
  hold is someone else's. `Pager.load` keeps the keyboard's place through a redraw.
- **`Dom.editable( value , options )`** is how a name gets edited in a table
  row: it answers a button showing the value, which turns into a field. Enter
  or clicking away saves, Escape cancels, and a refused save keeps the field
  open with what was typed. Give it `label` and `save`; `save` throws to refuse,
  and is where `Undo.record` goes.
- **`Dom.form( { title , fields , save } )`** is for editing more than one value:
  a dialog built from a list of fields, answering `null` if cancelled. A refused
  save says why inside the dialog and keeps the input. See the comment above it
  in `dom.js` for the field types.
- **Editing something after it is made** is a POST verb route, like every other
  change (`/api/keys/:id/rename`), never PUT or PATCH, which the spec test would
  not see. Add it to `config/openapi.json`, and give it an `expect` so its undo
  can answer 409, as `RenameUser` does.
- **`Dom.when( value , fallback )`** is a table cell's time: "3 hours ago", or a
  date past a month, with the exact time on hover. `fallback` is what a missing
  time says ("Never").
- **Empty tables.** Give a `Pager` table a `data-empty` language key and an empty
  list says so in one row. A table filled some other way calls `Pager.empty`.
- **`Qr.toggle( button , canvas , value )`** shows the link in `value` as a QR
  code. Whoever shows a new link calls `Qr.hide`, or the code on screen is still
  the old link's. A login link may work once or several times
  (`login_links.js`), and is shown only when it is made.
- **`Dom.download( name , text )`** saves text as a file.
- **A session that ends.** A 401 from any call but `/api/me`, while someone is
  signed in, goes to `/?login=ended`, which says why. 401 only ever means "no
  session": a wrong password is a 400 and a wrong code a 403. A new route must
  keep it that way, or a mistyped code would sign someone out.
- **The network.** A call that never reached the server fails with
  `errors.network`, not the browser's "Failed to fetch".
- **Coming back to a tab.** `fresh.js` asks `/api/me` again when a tab becomes
  visible after five minutes or more, and acts on the session, the maintenance
  and announcement banners, and a new `version`. Only then, and never on a timer:
  a tab nobody is looking at must not count as use, or it would keep its session
  alive.
- **Leaving.** Files and Settings each add a `beforeunload` while an upload is
  running or there are unsaved changes. A new screen with work to lose does the
  same.
- **Password fields.** `App.init` gives every `input[type=password]` a show/hide
  button through `Dom.revealable`. It goes back to dots when its form is sent.
- **Moving between screens.** `Nav.announce` puts the screen's `h2` in the tab's
  title and moves the focus to it, for a screen reader's sake. So a screen needs
  an `h2`.

Light stays the default through all of this. The Appearance picker, and the
navbar's light/dark button in an app without `appearance` (the two never ship
together), go through `Theme.pick`, which is undoable, and `prefers-color-scheme` is still never
read.

## CSS structure

`static/css/style.css` loads after Bootstrap and holds only what Bootstrap
does not: this app's colours, the sidebar shell, and touch rules. If a
utility class can do it, use the class instead of adding a rule.

Things worth preserving when editing:

- **Light mode is the default and stays it.** Dark is Bootstrap's opt-in
  `<html data-bs-theme="dark">`, set only by `theme.js`. Bootstrap's own
  colour-mode script, which follows `prefers-color-scheme`, is deliberately
  not included, so the app looks the same for everyone unless someone
  chooses otherwise.
- **Colors are the `--app-*` tokens at the top of `style.css`.** Change a
  token, not a rule. The block under them hands the tokens to Bootstrap's
  `--bs-*` variables; components Bootstrap compiles colours into
  (`.btn-primary`, `.nav-pills`, focus rings) are re-pointed explicitly.
- **16px input font size.** Smaller makes iOS Safari zoom in on a focused
  input.
- **44px minimum button height** -- the usual floor for a reliable touch
  target. `.btn-sm` (table row actions) is the exception.
- **Forms are a Bootstrap grid:** `form.row.g-3.align-items-end` with a
  `.col-md` per label+input and a `.col-md-auto` for the button, so fields
  stack on a phone and line up from `md` up.
- **Wide content scrolls in its own box** (`.table-responsive`), so a table
  never makes the page itself scroll sideways.

Labels stay visible at every width. A placeholder disappears the moment
someone types and is not a substitute for a label.

## Sign-in screens

`password.js` holds the password form on the signed-out page and the Password
card on Account; `two_factor.js` holds the code step and the Two-factor card.
Both cards
show themselves only when `/api/me` says their switch is on, so nothing else has
to know. Any sign-in call can answer `{two_factor: true, challenge}` instead of
signing in; hand that to `TwoFactor.ask( challenge )`. A password never goes
into a request: only `Password.digest()`'s result does, and there is no fallback
when WebCrypto is missing.
