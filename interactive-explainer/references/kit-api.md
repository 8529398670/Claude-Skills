# The kit

A shared stylesheet and runtime that `scripts/build.py` splices into every document.
**Do not rebuild what they give you, and do not link or declare them — they arrive on
their own.**

## Already handled — do not write these

A complete palette, the page background and text colour, a centred reading column, a type
and spacing scale, 44px controls, focus rings, styled range inputs, `tabular-nums` on
readouts, scaling SVG, tables, `<pre>` overflow, and reduced-motion.

**Documents render light, whatever the reader's system theme is set to.** These pages get
printed, projected and screenshotted, and one that changes colour depending on who opens
it is one you cannot hand to anyone. Do not add a `prefers-color-scheme` block to bring
dark mode back — a document that does is the only thing that can break this.

**Never define your own `:root` palette.** Write CSS only for what your topic actually
needs — the shape of a diagram, the layout of one widget — and build it from the variables
below. A hex literal in your CSS is a colour that no longer tracks the palette: it will
not follow a change to the kit, and it is the one kind of mistake that still looks correct
on your screen today.

## Variables

**Colours** — use these, never hardcoded hex:

```
--kit-bg  --kit-surface  --kit-raised  --kit-border
--kit-text  --kit-muted  --kit-faint
--kit-accent  --kit-accent-text  --kit-accent-soft  --kit-accent-border
--kit-warn  --kit-warn-soft  --kit-warn-border
--kit-math
```

Use the accent colour to mean "this is interactive", and nothing else. The moment it also
means "important", the reader stops being able to see what is clickable.

**Spacing**: `--kit-space-1` through `--kit-space-7`.

**Other**: `--kit-radius`, `--kit-radius-sm`, `--kit-font`, `--kit-mono`, `--kit-measure`
(the reading column width), `--kit-control` (44px).

## Structure

```html
<p class="kit-lede">The one-sentence answer.</p>

<section class="widget" id="orbit">
  <h3>What the mechanism is called</h3>
  <div class="widget-stage"><svg viewBox="0 0 400 240">...</svg></div>
  <div class="control-row">
    <label for="mass">Mass</label>
    <input type="range" id="mass" min="1" max="40" step="0.5" value="12"
           data-readout="mass-out" data-unit="kg">
    <span class="readout" id="mass-out"></span>
  </div>
  <p class="try">Drag mass to maximum, then halve the distance.</p>
  <p class="notice">Force falls with the square of distance, so distance wins.</p>
</section>
```

`.try` and `.notice` print their own "Try this" and "Notice" labels. Write only the
sentence — repeating the label is a visible duplicate.

**Other classes**: `.kit-wide` (one block wider than the reading column), `.kit-scroll`
(wrap anything wide so it scrolls inside its own box rather than widening the page),
`.kit-lede`, `.kit-muted`, `.kit-small`, `.breaks`, `.recap`, `.no-math` (opt a subtree out
of maths typesetting).

## Runtime — the global `Kit`

- **`Kit.widget(id, host => { ... })`** — run one widget's setup isolated. A throw inside
  becomes a visible note in place of that widget instead of a dead page. Wrap every widget.
  Do not write your own try/catch scaffolding.
- **`Kit.slider(id, value => { ... })`** — wires the input to your callback, updates the
  `data-readout` span with units, and fires once immediately so the page opens in a live
  state. A range with `data-readout` is wired automatically even with no callback.
- **`Kit.loop(dt => { ... })`** — `requestAnimationFrame` with `.start()`, `.stop()`,
  `.toggle()` and a `.running` flag. Stops itself when the tab is hidden. Return `false`
  from the callback to end it. Use this rather than calling `requestAnimationFrame`
  yourself — the off switch is the part that gets forgotten, and an idle loop draining a
  phone battery behind a background tab is the most common sin in a generated page.
- **`Kit.rng(seed)`** — seeded PRNG returning a function that gives floats in `[0,1)`.
  `Kit.rand()` is a ready-made stream. Do not write your own, and do not use
  `Math.random()`: a page whose numbers change under the reader on reload cannot be trusted
  or referred back to.
- **`Kit.fmt(n, { unit, digits })`** — number formatting that will not jitter in width as
  digits change.
- **`Kit.math(el)`** — typeset maths in a subtree you built after load. The page is already
  typeset once automatically; you only need this for DOM you create later.

## Maths and chemistry

KaTeX with mhchem. The build script adds it when it sees maths in your document, so you can
simply write TeX:

- Inline: `$E = mc^2$`, `$H_2O$`, `$\theta$`
- Display: `$$\frac{d}{dt}\vec{p} = \vec{F}$$`
- Chemistry: `$$\ce{H2O <=> H+ + OH-}$$`, inline `$\ce{CO2}$`

### Two rules with no exceptions

**Every comma gets a space on both sides** — `word-A , word-B` — in prose and inside
maths alike. `$(2,4)$` sets the comma hard against the digits, where it is indistinguishable
from a decimal point; `$(2 , 4)$` reads at a glance. The only comma exempt is one inside a
number, where it is a thousands group rather than a separator: `1,000` stays as it is.

**A sentence never ends on an expression followed by a stop.** `The area is $x^2$.` puts
the period hard against the exponent, and the reader is left deciding whether it belongs to
the maths. There is no styling that fixes this, so the sentence has to change: either drop
the terminal punctuation, or rewrite so the expression is not the last thing in the
sentence — `The area is $x^2$ in every case.` The same goes for `!` and `?`, and for
display maths, which must never be followed by a stray period on the next line.

The build script fails on both, because both are invisible to whoever wrote them.

**Dollar signs in prose.** `$` opens an expression, so two of them in the same paragraph
will swallow the text between them — "costs $5 and $10" typesets "5 and " as maths. If a
literal dollar sign has to appear, wrap it: `<span class="no-math">$5</span>`. Writing `\$`
does **not** work in prose: outside an expression the backslash is just a backslash and the
reader sees it. Inside an expression, `$\$5$` is correct. Anything that must stay verbatim
belongs in `<code>` or `.no-math`, both of which are skipped entirely.

## Colouring the maths

Off by default, the maths renders in the body text colour. To set it in its own colour
instead, so expressions stand out from the prose around them, declare:

```html
<meta name="math-color" content="accent">
```

The build script turns that into an attribute on the root element and the kit paints every
KaTeX span `var(--kit-math)`, which is a deep violet in light mode and a pale one in dark.
Note that this is deliberately **not** `--kit-accent`: the accent colour is reserved to mean
"you can operate this", and an equation wearing it reads as a control the reader will hunt
for a way to click. `content="off"` is the explicit default.

## Craft baseline

- **Mobile first.** Fully usable at 360px wide, and the page must never scroll
  horizontally — wrap wide content in `.kit-scroll`.
- Give every SVG a `viewBox` and let it scale. Do not hardcode pixel widths on a diagram.
- Write as little CSS as the topic allows. Every rule you add is one the kit may already
  have, and one more thing that can disagree with it.
- The page is a file on disk. `localStorage` throws on a `file://` origin, so keep all state
  in JavaScript variables.
