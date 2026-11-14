# Frontend reference

The rules here are what keep the frontend from drifting into a build
pipeline. They are cheap to follow from the start and expensive to retrofit.

## Hard rules

Every screen, card and dialog follows these. They are not style advice: a
screen that breaks one is not done.

1. **Bootstrap for everything it has.** Layout is Bootstrap's grid -- `.row`,
   `.col-*`, `row-cols-*`, `g-*` -- and spacing, flex, display, text and width
   are its utility classes. Components are Bootstrap components. `style.css`
   holds only what Bootstrap has no class for: the color tokens, the shell
   (navbar, sidebar, the `.view` width tiers, `aside-sticky`), touch sizes, and
   the inside of a few components (the log calendar's seven days, the theme
   preview, the stacked phone tables). Never write a layout grid or flex row in
   CSS that a class can say.
2. **Mobile first.** The base is the phone, at 375px, and it is checked first.
   Wider screens only add, through Bootstrap's breakpoint classes
   (`col-lg-6`, `row-cols-xxl-3`, `order-xxl-last`) or `@media (min-width: ...)`.
   No `max-width` media queries for layout. The one allowed is a phone-only
   form of a component that cannot be undone from a breakpoint up (the stacked
   tables), with `phone-only` in the comment right above it.
3. **A table takes the most room on its screen.** Its card spans the whole row
   and the table sits right inside a `.table-responsive`, so a wide one scrolls
   in its own box and never the page. Something beside a table needs a good
   reason that still looks good when you look at it at 1440 and 1920: two or
   three small tables of a few short columns, read together, can sit side by
   side (`row g-3`, each `col-12 col-xl-6`). A form, a filter or a thin tool is
   never that reason; it goes above the table. On a phone every table is the
   full width.
4. **Icons are Bootstrap Icons by default:**
   `<i class="bi bi-<name>" aria-hidden="true"></i>`. Another icon (another
   set, an inline SVG) only when Bootstrap Icons has nothing that fits, with a
   one-line comment saying so. Never emoji as icons.

`app/tests/test_frontend_rules.py` fails on a `max-width` query without `phone-only`, a
`grid-template-columns` outside the components above, and a table not right
inside a `.table-responsive`. How much room a table gets and which icons are
used are judgement calls, so those are checked by eye.

## No build step, ever

No npm, no bundler, no transpiler, no JSX, no TypeScript compile. Files in
`static/` are served to the browser exactly as they sit on disk.

This is not nostalgia. It means:

- What you debug in the browser is what is on disk -- no source maps, no
  "works in dev, breaks in prod" from a differing build.
- A deploy is a file copy. `app/core/static_files.py` serves text assets
  with `no-store` and gzips them per request, so an edit is live on the next
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
part of the app, not a build artifact, and it ships in the Docker image with
the rest of `static/`.

**Do not reference a CDN directly.** `app/core/middleware.py` sets
`script-src 'self'`, so a cross-origin `<script src>` is refused by the browser. That is
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
   markup or code changes. A `language.yaml` dropped into the app directory
   (`/app/data/` in the container) overrides the bundled one, so a deployment
   can reword the UI without touching the code.
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

## Which static/ is served

At startup the server serves `server.static_dir` (default `./static` beside
`config.yaml`) if that directory exists, and the `static/` that ships beside
the `app/` package otherwise, logging which it chose:

```
[assets] static=/srv/app/static (server.static_dir) language=/srv/app/language.yaml
```

In a source checkout those are the same directory, so editing files behaves
exactly as described above -- edit, refresh, done. The Docker image copies
`static/` in, so anything you add there ships; a new asset directory placed
*outside* `static/` would need its own `COPY` line in the Dockerfile and is
not served by the catch-all -- putting it under `static/` is simpler.

## JS file conventions

Plain scripts, loaded in dependency order, each adding one global:

| File | Responsibility |
|---|---|
| `theme.js` | Light, dark, or another theme, and the colors worked out from it. Loaded in `<head>`; see below. |
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
| `mcp.js` | The MCP card on Account: an API key and the setup to paste for Claude Code, Claude Desktop or another client. |
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
`d-flex`, `gap-2`, `mt-3`, `row`, `col-md-6`, ...) before writing CSS, and an
icon is `<i class="bi bi-<name>" aria-hidden="true"></i>` (see
[Hard rules](#hard-rules)).

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
  class on `<html>`, remembered in `localStorage`). `<main>` takes all the
  space beside the sidebar; each screen decides how much of it to use and is
  centred in it (see [Desktop layout](#desktop-layout)).
- **A screen is a `<section class="view" data-view="name">` in `<main>`
  plus a sidebar link `<a class="nav-link" href="#name" data-nav="name">`.**
  The view is not a card itself. It holds an `<h2 class="view-title">`, an
  optional `<p class="view-description">`, and then its
  `<section class="card">`s, each with an `<h3 class="card-title">` -- one
  column, unless what the screen is calls for another layout on a desktop
  ([Desktop layout](#desktop-layout)).
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

### Desktop layout

A wide screen is room, not an obligation. What decides a screen's desktop
layout is **what the screen is** -- the shape of its content -- not how many
pixels are spare. It is specific to each screen, and there is no formula: the
table below is the record of what has worked and what has not. Width used
badly is worse than width left empty: a short table stretched across 1100px
pulls each row's buttons away from its name, a lone input that long reads as
a banner, unrelated cards of different heights side by side leave ragged
edges and holes, and a thin column squeezed beside a table looks broken. So
the default is the width every screen was designed at, and a screen goes
wider only together with a layout its content calls for. Phones are never
affected: the base is the phone, and everything here only adds from a
breakpoint up.

**A table takes the most room** ([Hard rules](#hard-rules), 3). A table is
the thing people scan across, and it wants the width of its screen. By default
its card spans the row and nothing sits beside it: no rail, no calendar, no
filter column, no form squeezed into a strip. A control that picks what a table
shows (a date, a filter) is a button in the bar above it that opens a popup:
the Logs calendar is the worked example (`#logs-date-button`, a Bootstrap
dropdown with `autoClose: "outside"`, closed by `Logs.pick`). A form or tool
that adds rows goes above the table, full width, as on Users and Files (both
once had it beside the table; both read better with it above). Tables side by
side need a good reason and have to look good: a few small tables of a few
short columns, read together.

**First, say what the screen is,** and pick from this table:

| The screen is... | Example | Layout from a breakpoint up | Width |
|---|---|---|---|
| A list of separate settings, read top to bottom | Account, Shortcuts | One column, as on a phone | default |
| One small form | a sign-in step | One column | `view-narrow` |
| A table plus the form or tool that adds to it | Users, Files | Form above, table full width -- as on a phone | default, `view-wide` |
| A big table or log people scan across | Logs | The table gets the width; what picks the rows sits in the bar above, the calendar in a popup | `view-full` |
| A few small tables read together | totals by day and by user | `row g-3`, each table's card `col-12 col-xl-6` | `view-wide` |
| An editor and what shows its effect | Appearance (colors + preview) | `row`: editor `col-xxl-8`, preview `col-xxl-4 order-xxl-last aside-sticky` | `view-full` |
| Several things of the same kind and size | stat tiles, one card per item | `row row-cols-1 row-cols-lg-2 row-cols-xxl-3 g-3`, each card in a `.col` with `h-100` | `view-wide` |

If a screen is none of these, or two of them, it stays one column until
someone decides what it is, and looks at it. That decision is the design
work; the classes below are only how it is written down.

1. **Width tiers** go on the `<section class="view">`. The shell does not cap
   the width; the view does.

   | Class | Max width | Use with |
   |---|---|---|
   | *(none)* | 60rem | one column -- the width every screen was built at |
   | `view-narrow` | 48rem | one small form |
   | `view-wide` | 80rem | a main + aside, cards or small tables side by side |
   | `view-full` | 120rem | an editor with panes, or a table people scan across |

   Never widen a screen without changing its layout: the same column, wider,
   only stretches it. What sits in `<main>` outside a screen (signed-out, join
   and confirm cards, banners) keeps 60rem.
2. **Main + aside** is a Bootstrap row: `<div class="row g-3
   align-items-start">` with the main part `col-12 col-xxl-8` and the aside
   `col-12 col-xxl-4`. Below `xxl` they stack in page order, so put the one a
   phone needs first, first; if that is the aside, give it `order-xxl-last` to
   move it right on a wide screen without changing the keyboard order. Add
   `aside-sticky` to the aside's column to keep it in view under the navbar
   while the main part scrolls (Appearance does this with its preview). Use it
   beside an editor or other non-table content, never beside a table (see
   above).
3. **Card grids** are Bootstrap `row-cols`: `<div class="row row-cols-1
   row-cols-lg-2 row-cols-xxl-3 g-3">`, each card inside a `.col` and `h-100`
   so each row's cards share one height. Only for cards of the same kind and
   similar size. Unrelated cards never go in a grid: that was tried on Account
   and read as chaos, so Account stays one column. A card that needs the width
   is in a `col-12`. The grid sits outside any `feature:` markers.
4. **Inside a card, repeated fields wrap.** A run of the same kind of field
   (colors, toggles) in a wide card is a row-cols grid of units, `row
   row-cols-1 row-cols-sm-2 row-cols-xxl-3 g-3`, each its label over its
   control (the theme colors). A single input never stretches across a wide
   card: a search box (`input[type=search]` inside `[role="search"]`) stops at
   28rem, and a form field in a wide card is `col-md-6 col-xl-4`, not a bare
   `col-md`.
5. **Prose keeps its measure.** `.view-description`, `.card-text`, `.form-text`
   and paragraphs in an alert stop at 70ch.
6. **Breakpoints are Bootstrap's.** The base is the phone. From `sm`: fields
   and repeated units go two across. From `lg`: card grids, and the sidebar
   becomes a column. From `xl`: small tables side by side. From `xxl` (1400px):
   main + aside and editor panes. Each starts where the part that matters most
   (the table, the colors) still has its room beside the new column, not where
   the new column first fits.

**Check every new or changed screen** at 375 first, then 768, 1280, 1440 and
1920 wide, by looking at it -- the numbers above are where to start, not proof:

- Can you say what the screen is, in the table's terms? Does its layout match?
- Does a table have the most room? If anything sits beside it, is there a good
  reason, and does it look good?
- Is everything side by side a Bootstrap `.row`, and one column on a phone?
- Does any row put its buttons far from the name they act on, or wrap?
- Is any lone input or line of text stretched past ~40rem / ~90 characters?
- Do cards side by side leave ragged edges or holes?
- Does a preview or summary scroll out of view while you edit what it shows?
- Does the page scroll sideways anywhere at 375?

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
another date is picked on the month calendar. The calendar opens from a
button in the search bar that names the day shown (`#logs-date-button`), so
the log keeps the whole width; picking a day closes it. It is drawn by
`renderCalendar`, from the days listing: shaded dates have entries, a dot
means errors, and while searching only matching dates can be picked. A day's rows load when it is
opened, 200 at a time with "Load more".
The search box goes to the server as typed (debounced), and the kind buttons
add a `kind:` term. Rows are built with `Dom.el` like everything else, and a
row's JSON shows in a `<pre>` via `textContent`. Its text is the `logs:`
block in `language.yaml`.

**Live** opens an `EventSource` on `/api/owner/logs/stream` and runs the screen's
ordinary `refresh()`, throttled, on each `changed` event, so an idle server
costs no requests. The stream closes when the Logs screen is left and reopens
on return. A lost stream turns Live off and says so (`logs.live_lost`): the
server ends one when the session is revoked.

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

### The Jobs and Account sessions screens

`jobs.js` is the owner's Jobs screen: each job from `GET /api/owner/jobs`, how
it last went, when it is next due, and a Run now button
(`POST /api/owner/jobs/<name>/run`, 202 at once; the list shows the outcome).

`sessions.js` is the sessions list on the Account screen, with `current` marking
this device. Ending one, or all the others, asks first and is not undoable: a
revoke stays out of `Undo`, like revoking a key. A device can be named by clicking
it, with Undo. The name worked out from the User-Agent stays underneath, so a label
cannot hide what a device says it is.

### Quoting a request

A 5xx body carries `request_id`. `Api.request` puts it after the error message
(`errors.reference`), and the error flash stays up for about 12 seconds, long
enough to copy. Pasted into the Logs search it finds that request's line.

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
`Theme.builtIn` lists Default (first, and light: what a new browser shows) and
nine ready-made themes -- ocean, forest, sepia, grape, pink, WSU (Wright State
University green and gold), midnight, slate, cerise. Each has colors for both
light and dark. A theme is `{ id , name , mode , accent , second , background? ,
surface? , overrides? , light? , dark? }`: a mode, an `accent` for what you can
press, a `second` for where you are, optional page and card colors. `light` or
`dark` keeps the theme's colors for the mode not in use (`{ accent , second ,
background? , surface? , overrides? }`), and `Theme.setMode( theme , mode )`
swaps them in, so the Appearance screen's Light/Dark switch turns WSU gold on
black and back again. A theme with nothing kept for the other mode keeps its
colors, and from then on remembers them for the mode it left.
`scripts/check_themes.js` holds every built-in to 4.5 in both modes.
`Theme.derived( theme )` works out everything else: the page, card, nested card
(`.app-nested`) and navbar shades, labels that read on each fill, and text
colors pushed until they reach 4.5. `Theme.colors( theme )` is that with the
theme's `overrides` on top -- any derived color replaced by name (`text`,
`link`, `navLink`, `border`, `info`, ...). They are set inline on `<html>` as
the `--app-*` tokens (`--app-text` feeds `--bs-body-color`). `Theme.check( theme )`
lists the pairs in `Theme.pairs` that fall under 4.5; `scripts/check_themes.js`
holds every built-in theme to none, and the tests run it. `ThemePalettes` keeps
brand colors by name, used or not. A dark mode sets Bootstrap's
`data-bs-theme="dark"` on `<html>`.

Every Bootstrap button is in the theme. `primary` is the accent; the others
(`Theme.variants`: secondary, success, danger, warning, info, light, dark) each
get three derived colors -- `btnDanger` (the fill: a semantic base per mode
with `buttonHarmony` percent of the accent, or the second color for info,
moved until black or white reads on it), `btnDangerFg` (its label) and
`btnDangerText` (the outline button's label and border, read on a card) --
as `--app-btn-danger`, `--app-btn-danger-fg` and `--app-btn-danger-text`, held
to 4.5 by `pairs` (`button_<v>`, `outline_<v>`) like the rest, and overridable
by name. A theme's optional `button: { shape , weight , depth , case }` picks
from `Theme.buttonChoices` (square/soft/rounded/pill, normal/medium/bold,
flat/raised/glossy, normal/upper; missing is `buttonDefault`: rounded,
normal, raised, as written -- which every built-in theme uses); `Theme.buttonStyle( theme )` turns it into `--app-btn-radius`,
`-weight`, `-shadow`, `-image`, `-transform` and `-spacing`, which style.css
puts on every `.btn` (sizes scale the radius; the sheen is for filled buttons
only).

With `appearance`, themes live on the server. There are two lists: the app's
defaults, which an admin edits (meta key `theme_defaults`; nothing stored means
`Theme.builtIn`), and each person's own (`themes` on the user; nothing stored
means they follow the defaults, including later changes). `/api/me` answers
`themes` (theirs, else the defaults, else null), `themes_custom` and
`theme_id`. `Theme.load( me )` takes that; `Theme.pick( id )` changes the one
in use (`POST /api/account/theme`); `Theme.store( list , id )` replaces their
list (`POST /api/account/themes`, null for "back to the defaults"). Admins use
`GET`/`POST /api/admin/theme-defaults`. The server checks shape only (ids,
names, `#rrggbb`, known override names, known button choices, 1-40 themes); contrast is the screen's
to warn about. The theme in use is also kept whole in `localStorage`, so the
first paint and the signed-out pages are already in it; `/api/me` corrects it.

The Appearance screen is `themes_editor.js` (`ThemeEditor.init( me )`): a list
(use, new, duplicate, rename, delete) beside an editor with a color picker and
hex field for the mode's base colors and every derived one, each derived one
marked Auto until changed and with a Reset back to the worked-out color, a
Buttons section (toggles for the four button choices, then each variant's
fill, label and outline color), a live preview drawn with real Bootstrap
buttons of every variant (the preview carries the theme's own `--app-*`
properties and `data-bs-theme`), and a box naming any text that is hard to read. Admins get a
"Defaults for everyone" tab with the same editor. Every change goes through
`Undo` and sends the whole list. Names of built-in themes come from
`theme.preset_<id>` in language.yaml until a list is saved.

Without `appearance` there is no server state and one theme, Default: the
navbar button switches it between light and dark, kept in this browser's
`localStorage`. Whatever you do, first load is
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
  not see. Declare it with `@route` (its docs come from its types), and an `expect` so its undo
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
does not: this app's colors, the sidebar shell, and touch rules. If a
utility class can do it, use the class instead of adding a rule.

Things worth preserving when editing:

- **Light mode is the default and stays it.** Dark is Bootstrap's opt-in
  `<html data-bs-theme="dark">`, set only by `theme.js`. Bootstrap's own
  color-mode script, which follows `prefers-color-scheme`, is deliberately
  not included, so the app looks the same for everyone unless someone
  chooses otherwise.
- **Colors are the `--app-*` tokens at the top of `style.css`.** Change a
  token, not a rule. The block under them hands the tokens to Bootstrap's
  `--bs-*` variables; components Bootstrap compiles colors into
  (`.btn-primary`, `.nav-pills`, focus rings) are re-pointed explicitly.
- **16px input font size.** Smaller makes iOS Safari zoom in on a focused
  input.
- **44px minimum button height** -- the usual floor for a reliable touch
  target. `.btn-sm` (table row actions) is the exception.
- **Mobile first.** Base rules are the phone; wider screens add with
  `min-width` queries or Bootstrap's breakpoint classes ([Hard rules](#hard-rules)).
- **Forms are a Bootstrap grid:** `form.row.g-3.align-items-end` with a
  `.col-md` per label+input and a `.col-md-auto` for the button, so fields
  stack on a phone and line up from `md` up.
- **Wide content scrolls in its own box** (`.table-responsive`), so a table
  never makes the page itself scroll sideways.
- **Layout is Bootstrap's grid.** The shell adds only the width tiers
  (`view-narrow`, `view-wide`, `view-full`) and `aside-sticky`. A screen uses
  them and `.row` / `.col-*` rather than writing its own widths or grids; see
  [Desktop layout](#desktop-layout).

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
