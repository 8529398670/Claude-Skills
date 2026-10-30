# Frontend reference

The rules here are what keep the frontend from drifting into a build
pipeline. They are cheap to follow from the start and expensive to retrofit.

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
| `theme.js` | Light, dark, or a preset theme. Loaded in `<head>`; see below. |
| `dom.js` | Element helpers, `Dom.copy`, `Dom.toast`. No app logic. |
| `i18n.js` | Loads and applies `language.yaml`. |
| `api.js` | Every call to the server. Attaches cookies and the CSRF token. |
| `nav.js` | The sidebar: which screen the URL hash shows. |
| `app.js` | The account + admin screen. |
| `keys.js` | The API keys screen. An example of the rule below. |

**Give each new screen its own file with its own `init()`**, included from
that page's HTML. Do not keep appending to `app.js` -- the whole layout exists
to avoid a monolith, and the frontend is the easiest place to accidentally
build one.

`keys.js` is the worked example: it owns its own screen, its own tables, and its
own refresh, and `app.js` knows it exists in exactly one line
(`await Keys.init( me )`). Copy that shape rather than the alternative.

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
  behind the navbar's menu button on a phone.
- **A screen is a `<section class="card view" data-view="name">` in `<main>`
  plus a sidebar link `<a class="nav-link" href="#name" data-nav="name">`.**
  `nav.js` shows the one the URL hash names, so back/forward, reload and a
  pasted link all work with no router. The screen's own JS decides whether it
  exists for this user by un-hiding its section; `Nav.init()` (called last in
  `App.renderSignedIn`) offers only the sections that were un-hidden, and a
  hash for one that wasn't falls back to `#account`. Admin-only links go
  inside `#sidebar-admin`, whose heading disappears when it has nothing left.
- **A profile icon, not "Name (role)" text.** `#app-nav` on the right of the
  navbar holds the `bi-person-circle` link to `#account`; its
  `title`/`aria-label` say who is signed in (`App.labelProfile`). The app's
  own top-level links go in the same `<div>`, before the icon.
- **Signed-out pages have no sidebar.** The signed-out, login-confirm and
  join cards sit in `<main>` on their own.
- **Confirmations are toasts.** `Dom.toast( I18n.get( "ui.saved" ) , "ok" )`
  shows a Bootstrap toast (`#toast`) bottom right for four seconds;
  `Dom.copy( element )` toasts `ui.copied` itself. A message that must stay
  (a key shown once) gets its own `alert` on the screen instead, and
  page-level errors go in `#error-banner` at the top of `<main>`.
- **Statuses are badges.** `Dom.badge( text , "success" )` for a table cell.

### Themes

`static/js/theme.js` is loaded in `<head>`, before the stylesheet paints, and
applies the saved theme at once, so a dark choice never flashes light.
`Theme.presets` lists light, dark and eight ready-made themes -- ocean, forest, sepia, grape, pink, midnight, slate, dark pink -- (each a mode plus
optional `accent`, `background`, `surface` colours -> `--app-accent`,
`--app-bg`, `--app-surface`, set inline on `<html>`; `--app-accent-fg` is
picked for contrast). A dark mode sets Bootstrap's `data-bs-theme="dark"` on
`<html>`. The Appearance screen is `Theme.render( root )`; names come from
`theme.preset_<key>` in language.yaml.

The choice lives in this browser's `localStorage` only -- no server state.
To make it follow a person across devices, store the preset key on the
account and call `Theme.set( key )` after `/api/me`. Whatever you do, first
load is always light.

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
