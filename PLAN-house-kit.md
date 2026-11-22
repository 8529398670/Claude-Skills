# Plan: the house kit

Written 2026-11-22. Questions answered by the owner the same day (see Decisions).

## The problem

Every screen builds its own tiles, button rows and filter bars out of
`Dom.el`. Each one is a fresh chance to get the layout wrong, and nothing
catches it but the owner's eyes. Example from ProjectPortal: a join tile put a
long text button ("Use my own key") in a card footer next to the Join, Board
and GitHub buttons. It wrapped onto its own line on every width. The static
rules test cannot see that: the problem only shows up in a real browser.

## The idea

1. **A kit**: a small set of finished pieces on top of Bootstrap, in one
   file, so a screen asks for "a tile with these actions" instead of building
   one.
2. **A gallery**: a page in the demo app that shows every piece, filled with
   worst-case data.
3. **A layout check**: a script that opens the gallery and every screen in
   headless Chrome at phone, tablet and desktop widths, and fails on the
   things I keep getting wrong.

Bootstrap stays the base. The kit doesn't replace it: it is our decisions
about how to use it, written down once as code.

## 1. The kit: `static/js/kit.js`

A new `Kit` object next to `Dom`, which it uses. Every piece takes data and
answers an element. Its classes are Bootstrap's, plus a `kit-*` class so the
layout check can find it.

| Piece | What it fixes |
|---|---|
| `Kit.actions( name , { primary , icons , menu } )` | A button row. **One** text button (the primary), then icon-only buttons, then a ⋯ menu on the far right. Icon buttons always get `title` and `aria-label` from one language key. Passing a second text button is a console error in dev. One line at every width. |
| `Kit.iconButton( icon , labelKey , onClick )` / `Kit.iconLink( icon , labelKey , href )` | The only way to make an icon-only button or link. You can't forget the label. |
| `Kit.tile( { icon , color , title , subtitle , status , badges , body , tags , actions } )` | A card in a `row-cols` grid: header, badges, text, tags, and a `Kit.actions` footer. Even height (`h-100`), long names break rather than push. |
| `Kit.tileGrid( id )` | The `row row-cols-1 row-cols-md-2 row-cols-xxl-3 g-3` holder, so every tile screen uses the same steps. |
| `Kit.filterBar( { search , selects , toggles } )` | The search/select/switch row above a list: stacked on a phone, one line from `md` up, the switch lined up with the inputs. |
| `Kit.empty( icon , textKey , action )` | An empty list or grid: icon, a sentence, one optional button. |
| `Kit.stat( labelKey , value , hint )` / `Kit.statRow( stats )` | A number tile and a row of them (dashboards). |
| `Kit.section( titleKey , { description , actions , body } )` | The standard card with heading, an optional line under it, and optional header actions on the right. |
| `Kit.keyValue( pairs )` | A label/value list (account details, a project's settings): two columns from `sm`, stacked on a phone. |

`Dom.rowActions` stays for table rows. `Kit.actions` is its cousin for card
footers and section headers, and they share `Dom.rowMenu`.

Only pieces we have needed more than once go in. A new one is added when a
second screen needs it, not before.

## 2. The rule that makes it work

Added to "Hard rules" in `references/frontend.md`:

> **6. If the kit has it, use it.** A tile, a button row, a filter bar, an
> empty state, a stat or a section card comes from `Kit`. A screen that needs
> something the kit lacks adds it to `kit.js` (and to the gallery) first, then
> uses it. A screen never builds one by hand.

`frontend_rules_test.go` (and the Python twin) gets a check for what a test
can see: `card-footer`, `row-cols-` and `form-switch` appear only in
`kit.js`, `index.html` holders, and the listed exceptions. Hand-built copies
fail the test.

A new section, "The house kit", in `frontend.md` lists each piece with one
example call and the rule it follows. That's what I read before writing a
screen.

## 3. The gallery: `/kit`

A screen in the template, admin only, and dev only: behind
`server.kit_gallery` (auto: on for an unstamped `dev` build, like
`live_reload`; never served by a stamped or Docker build). It shows every piece twice:

- **normal**: realistic data
- **worst case**: a 60-character name with no spaces, every badge, every
  action, 12 tags, an empty description, a dead health dot, and a long
  translation-length label

It's the page the layout check loads first. It's also where you look when you
want to see the whole house style at once.

## 4. The layout check: `scripts/check_layout.py`

Dev only, never shipped in a build or Docker image. It opens the gallery and
every `data-path` screen in `index.html`, signed in as the demo admin (through
`scripts/login.sh`), at **375**, **768** and **1440** wide, in light and dark.
It fails on:

- **The page scrolls sideways** (`scrollWidth > innerWidth`).
- **A `kit-actions` row wrapped**: its children don't all share one `offsetTop`.
- **A button's label is cut off or wraps** inside the button.
- **Something sticks out of its card**: a child's right edge is past the card's.
- **An icon-only button has no `aria-label`.**
- **Tiles in one grid row aren't the same height.**
- **Console errors** on load, including the kit's "second text button" error.

Every run writes one screenshot per screen and width to `.run/layout/`, so
the eye check is a folder of pictures, not a manual click-through. In a
failure, the report names the screen, width, element and rule.

**Browser:** Playwright for Python in one venv at `~/.claude/skills/.venv`,
driving the installed Google Chrome (`channel="chrome"`), so no browser
download. `check_layout.py` makes the venv on first run if it's missing. One script for both skills, since their `static/` is the same.
Each app gets a thin `scripts/check_layout.sh` that starts the app if it
isn't running and calls it.

## 5. Order of work

1. **`kit.js` and the gallery** in `go-webserver`, with `Kit.actions`,
   `iconButton`, `tile`, `tileGrid` and `empty` first: the pieces behind
   today's bug.
2. **`check_layout.py`**, run against the gallery until it passes at all
   three widths. Then break a piece on purpose to prove each rule fails.
3. **The rest of the kit** (`filterBar`, `section`, `stat`, `keyValue`).
4. **Move the template's own screens onto the kit**, and run the check over
   all of them.
5. **The rule** in `frontend.md` and the static test, plus the "check before
   done" rule in `frontend.md` and the global CLAUDE.md, and the skipping
   test hook (see Decisions, 4).
6. **Copy to `python-webserver`** (the `static/` change copies across; the
   test is ported).
7. **`UPGRADING.md`** in both skills, marked as an Upgrade, so
   `webserver-upgrade` can bring it to existing apps. `webserver-upgrade`
   runs the check as the last step of an upgrade that changes `static/`.
8. **Pilot on ProjectPortal**: upgrade it, move `projects.js` tiles onto
   `Kit.tile`, run the check over its screens, and fix what it finds. Then
   the other apps, one at a time, when you choose.

Same rules as every skills plan: `master` only, commits left to
`gitPushUpdate.sh`, all text in `language.yaml`, light theme by default, both
skills' tests pass.

## Decisions

1. **Name:** `Kit` in code, "house kit" in the docs.
2. **Browser:** Playwright for Python, in one shared venv at
   `~/.claude/skills/.venv`, driving the installed Google Chrome.
3. **Gallery:** dev only. `server.kit_gallery` defaults to auto (on for an
   unstamped `dev` build only). A stamped or Docker build never serves `/kit`.
4. **When the check runs** (the owner left this to me):
   - **Always before I call UI work done.** Any change to `static/` in a
     skill template or an app isn't finished until `check_layout` passes at
     all three widths and I've looked at the screenshots for the screens I
     touched. This goes in `frontend.md` next to the hard rules, and in the
     global CLAUDE.md's UI layout section, so it applies in every project.
   - **In the skills' test suite, when it can run.** The test calls the
     check if Playwright and Chrome are there, and skips with one line
     saying why if they aren't. A machine without Chrome can still run the
     tests, and this Mac always runs the check.
   - **After `webserver-upgrade` changes an app's frontend**, as the last
     step of the upgrade, so pulled-in changes are checked in that app.
   - **Not in Docker or the build.** The check is never a reason a build
     fails.
