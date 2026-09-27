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
./scripts/vendor.sh https://cdnjs.cloudflare.com/ajax/libs/htmx.org/2.0.4/htmx.min.js
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

Every user-facing string lives in `language.yaml`. None are hardcoded in HTML
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

Everything under `static/` is compiled into the binary by `embed.go`. At
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
| `dom.js` | Element helpers. No app logic. |
| `i18n.js` | Loads and applies `language.yaml`. |
| `api.js` | Every call to the server. Attaches cookies and the CSRF token. |
| `app.js` | The account + admin screen. |
| `keys.js` | The API keys screen. An example of the rule below. |

**Give each new screen its own file with its own `init()`**, included from
that page's HTML. Do not keep appending to `app.js` -- the whole layout exists
to avoid a monolith, and the frontend is the easiest place to accidentally
build one.

`keys.js` is the worked example: it owns its own card, its own tables, and its
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

## CSS structure

`static/css/style.css` is mobile-first: base rules describe a narrow screen,
and `@media (min-width: 40rem)` adds what a wider one can afford. Written the
other way round, phones download desktop layout and then override it, which is
the usual reason a "responsive" page still feels wrong on a phone.

Things worth preserving when editing:

- **Light mode is the default and stays it.** Dark is opt-in via
  `<html data-theme="dark">`, deliberately *not* `prefers-color-scheme`, so
  the app looks the same for everyone unless someone chooses otherwise.
- **Colors are custom properties on `:root`.** Change a token, not a rule.
- **16px base font size.** Smaller makes iOS Safari zoom in on a focused
  input.
- **44px minimum button height** -- the usual floor for a reliable touch
  target.
- **`.field` wraps each label+input pair** so they move together when the
  layout switches from column to row. Its `flex: 1 1 12rem` lives only in the
  wide-screen block: in the mobile column layout a flex basis applies to
  *height*, which stretches each field to 12rem tall.
- **Wide content scrolls in its own box** (`.table-scroll`), so a table never
  makes the page itself scroll sideways.

Labels stay visible at every width. A placeholder disappears the moment
someone types and is not a substitute for a label.
