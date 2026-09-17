# Declarable libraries

Declare what you need and the build script emits a pinned `<script>` tag for it in `<head>`:

```html
<meta name="uses" content="d3,chartjs">
```

The library is then simply present as a global by the time your end-of-body script runs.
You never write the `<script src>` yourself — a hand-written CDN path that is subtly wrong
still parses, and the widget depending on it dies in front of the reader with nothing in
the console to explain why. Declaring it means the URL is one this skill has verified.

| Name | Global | What it is for |
|---|---|---|
| `katex` | — | Maths and chemistry typesetting. Added automatically when maths is detected. |
| `d3` | `d3` | Scales, axes, shapes, force layouts, geo projections. |
| `chartjs` | `Chart` | Conventional charts on a canvas, fast to stand up. |
| `plotly` | `Plotly` | Scientific plots — 3D surfaces, contours, heatmaps. Heavy (~1MB). |
| `three` | `THREE` | 3D scenes with WebGL. |
| `p5` | `p5` | Generative and particle sketches on a canvas. |
| `matter` | `Matter` | 2D rigid-body physics — collisions, constraints, gravity. |
| `anime` | `anime` | Tweening and timeline animation. |
| `gsap` | `gsap` | Tweening and timelines, more capable than anime. |
| `mathjs` | `math` | Symbolic maths, matrices, unit arithmetic, expression parsing. |

An unknown name is a build error rather than a silent no-op, because the failure it would
otherwise cause is invisible.

## Reach for none of them first

Most widgets here are a `viewBox`, a few `<path>` elements and some arithmetic, and that
version is smaller, has no load order to get wrong, and looks like the rest of the page
because it is built from the kit's variables. A charting library pulled in for one chart
usually costs more in fighting its defaults — its own fonts, its own colours, its own dark
mode — than the chart was worth.

Good reasons to declare one:

- **d3** — you need real axes with sensible ticks, a `scaleLog`, or a force/geo layout.
  Note you can use `d3.scaleLinear` and `d3.line` to generate `<path>` data and still render
  the SVG yourself, which is often the best of both.
- **matter** / **three** / **p5** — the topic *is* physics, 3D or emergent behaviour, and
  hand-rolling the integrator would be the whole document.
- **mathjs** — the reader types an expression and you have to evaluate it.

If you declare a library, use it. The build script flags a declaration whose global never
appears in your script, because that is a page paying for weight it does not use.

## Something not on the list

Ask the user before reaching outside this table. Adding an entry to `LIBRARIES` in
`scripts/build.py` is two lines and a URL — but it is a decision about what every future
document may depend on, and pinned versions are the reason a document built today still
works next year. Verify any new URL resolves before pinning it:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -I https://cdnjs.cloudflare.com/ajax/libs/...
```
