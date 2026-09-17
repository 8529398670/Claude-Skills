#!/usr/bin/env python3
"""Splice the kit into an explainer document, resolve its libraries, and check it.

The document author writes only the part that is about the topic. Everything
shared -- the palette, the control styling, the runtime --
lives in assets/ and is spliced in here, so it never costs output tokens and a
fix to the stylesheet reaches every document rebuilt afterwards.

The checks exist because a generated page fails silently in very specific ways:
a slider with no readout still slides, a hardcoded hex still renders (just
outside the palette), a dead CDN path still parses. None of those raise anything. A
checklist in a prompt competes with the brief for attention; a script does not,
and it is exact about which line is wrong.

Stdlib only, by design -- a skill that needs `pip install` fails on first use.

    python3 build.py draft.html -o gravity.html
"""

import argparse
import html.parser
import os
import re
import sys
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
ASSETS = os.path.join(os.path.dirname(HERE), "assets")

# Verified against cdnjs. Each entry is (tags, global_name). Pinning exact
# versions matters more than being current: an unpinned URL that 404s next year
# turns a working document into a blank widget with no error anyone will see.
CDNJS = "https://cdnjs.cloudflare.com/ajax/libs"

LIBRARIES = {
    "d3": ([f"{CDNJS}/d3/7.9.0/d3.min.js"], "d3"),
    "chartjs": ([f"{CDNJS}/Chart.js/4.4.1/chart.umd.min.js"], "Chart"),
    "plotly": ([f"{CDNJS}/plotly.js/2.29.1/plotly.min.js"], "Plotly"),
    "three": ([f"{CDNJS}/three.js/r128/three.min.js"], "THREE"),
    "p5": ([f"{CDNJS}/p5.js/1.9.0/p5.min.js"], "p5"),
    "matter": ([f"{CDNJS}/matter-js/0.19.0/matter.min.js"], "Matter"),
    "anime": ([f"{CDNJS}/animejs/3.2.2/anime.min.js"], "anime"),
    "gsap": ([f"{CDNJS}/gsap/3.12.5/gsap.min.js"], "gsap"),
    "mathjs": ([f"{CDNJS}/mathjs/12.4.0/math.min.js"], "math"),
    "katex": (
        [
            f"{CDNJS}/KaTeX/0.16.9/katex.min.css",
            f"{CDNJS}/KaTeX/0.16.9/katex.min.js",
            f"{CDNJS}/KaTeX/0.16.9/contrib/mhchem.min.js",
            f"{CDNJS}/KaTeX/0.16.9/contrib/auto-render.min.js",
        ],
        "katex",
    ),
}

# KaTeX is the one library nobody should have to remember to ask for: a
# forgotten declaration ships `$E = mc^2$` to the reader as literal text, which
# looks like an authoring mistake rather than a missing dependency. So it is
# detected rather than declared -- which makes telling maths from money the
# thing this has to get right, in both directions. Miss the maths and the reader
# sees raw TeX; mistake "costs $5 and $10" for maths and KaTeX eats the sentence
# between the two dollar signs.
# Each level's prose budget, from references/levels.md. Length is the symptom
# the model is worst at self-checking -- it always feels like it is being brief --
# so the count belongs in a script rather than in one more line of the brief.
LEVEL_BANDS = {
    "eli5": (600, 1000),
    "high_school": (900, 1400),
    "college": (1200, 2000),
    "expert": (1200, 2000),
}

# Quantities that are genuinely unitless. Warning about a missing data-unit on a
# gain or a sample count trains the reader of the report to ignore the warning,
# which is worse than not emitting it.
DIMENSIONLESS = re.compile(
    r"\b(?:ratio|count|number|index|order|exponent|gain|factor|steps?|iterations?|"
    r"samples?|weight|fraction|probability|learning[ -]?rate|dimensionless|"
    r"per[ -]?unit|[nkmcNK]|kappa|eta|alpha|beta|gamma|lambda)\b", re.I)

DISPLAY_MATH = re.compile(r"\$\$.+?\$\$|\\\[.+?\\\]|\\\(.+?\\\)", re.S)
INLINE_MATH = re.compile(r"\$[^$\n]{1,200}\$")


def looks_like_math(span):
    """Is this $...$ an expression, or is someone talking about money?"""
    inner = span.strip("$").strip()
    if not inner:
        return False
    if re.search(r"[\\^_{}]", inner):                  # TeX machinery, unambiguous
        return True
    if re.search(r"[=<>+/*]", inner) and len(inner.split()) <= 8:
        return True                                     # "E = mc2", "x < 1"
    if len(inner) <= 12 and " " not in inner:           # "$\theta$", "$n$"
        return True
    # House style spaces every comma, so "(2 , 4)" and "f(x , y)" arrive here
    # looking like prose. They are maths when no word survives: two or more
    # comma-separated parts, none of them an actual English word.
    if "," in inner and not re.search(r"[A-Za-z]{2,}", inner):
        parts = [part.strip() for part in inner.split(",")]
        if len(parts) >= 2 and all(parts):
            return True
    return False                                        # "5 and also " -- prose


def find_math(prose):
    """Spans that genuinely need typesetting."""
    found = [match.group(0) for match in DISPLAY_MATH.finditer(prose)]
    remainder = DISPLAY_MATH.sub(" ", prose)
    found += [m.group(0) for m in INLINE_MATH.finditer(remainder) if looks_like_math(m.group(0))]
    return found


def find_currency(prose):
    """Paragraphs where two literal $ would be swallowed as an expression."""
    hits = []
    for paragraph in re.findall(r"<p\b[^>]*>(.*?)</p>", prose, re.S | re.I):
        for match in INLINE_MATH.finditer(paragraph):
            if not looks_like_math(match.group(0)):
                hits.append(match.group(0)[:60])
                break
    return hits


def strip_regions(markup):
    """Blank out script, style, code, pre and .no-math so prose checks see prose."""
    blanked = re.sub(r"<(script|style|code|pre|textarea)\b.*?</\1>", " ", markup, flags=re.S | re.I)
    return re.sub(r'<[^>]*class="[^"]*no-math[^"]*".*?</[a-z]+>', " ", blanked, flags=re.S | re.I)


def prose_only(markup):
    """Reader-visible text: no script, style, code or tags -- but maths left intact."""
    return re.sub(r"<[^>]+>", "", strip_regions(markup))


def find_tight_commas(text):
    """Commas missing a space on either side.

    House style is "word-A , word-B", and it matters most inside maths, where
    $(2,4)$ sets the comma hard against the digits and the reader cannot tell
    a separator from a decimal point. The only exempt comma is one inside a number,
    and the test for that is exactly three digits following it: "1,000" is a
    number, while "(2,4)" is a coordinate pair and is the very case this exists
    to catch. A comma carrying a backslash in front of it is the TeX thin-space
    macro \\, and is left alone as well.
    """
    hits = []
    for match in re.finditer(r",", text):
        index = match.start()
        before = text[index - 1] if index else " "
        after = text[index + 1] if index + 1 < len(text) else " "
        if before == "\\":
            continue                                    # \, is a TeX thin space, not a comma
        if before.isdigit() and re.match(r"\d{3}(?!\d)", text[index + 1:index + 5]):
            continue
        if before != " " or after != " ":
            hits.append(re.sub(r"\s+", " ", text[max(0, index - 30):index + 30]).strip())
    return hits


def find_math_then_stop(text):
    """Sentences that end on an expression and then punctuate it.

    The stop sits hard against the last glyph of the maths, so a period after
    $x^2$ reads as part of the expression. There is no way to style out of it;
    the sentence has to not end there, or not be punctuated.
    """
    hits = []
    for match in re.finditer(r"(\$\$.+?\$\$|\$[^$\n]{1,200}\$)\s*([.!?])", text, re.S):
        span = match.group(1)
        if span.startswith("$$") or looks_like_math(span):
            hits.append(re.sub(r"\s+", " ", match.group(0))[-60:])
    return hits


class DocumentScan(html.parser.HTMLParser):
    """Element-level facts the regex checks cannot see reliably."""

    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.widgets = 0
        self.ranges = []          # (attrs dict, line)
        self.svgs_no_viewbox = []
        self.uses = []
        self.has_lede = False
        self.has_breaks = False
        self.has_recap = False
        self.title = ""
        self.level = None
        self.math_color = None
        self.prose = []
        self._in_title = False
        self._in_code = 0
        self._widget_depth = None
        self._depth = 0
        self._widget_has = {}

    def handle_starttag(self, tag, attrs):
        attributes = dict(attrs)
        classes = (attributes.get("class") or "").split()
        line = self.getpos()[0]
        self._depth += 1

        if tag == "title":
            self._in_title = True
        if "kit-lede" in classes:
            self.has_lede = True
        if "breaks" in classes:
            self.has_breaks = True
        if "recap" in classes:
            self.has_recap = True

        if "widget" in classes:
            self.widgets += 1
            self._widget_depth = self._depth
            self._widget_has = {"try": False, "notice": False, "h3": False, "line": line}
        if self._widget_depth is not None:
            if "try" in classes:
                self._widget_has["try"] = True
            if "notice" in classes:
                self._widget_has["notice"] = True
            if tag == "h3":
                self._widget_has["h3"] = True

        if tag == "input" and attributes.get("type") == "range":
            self.ranges.append((attributes, line))
        if tag == "svg" and "viewBox" not in attributes and "viewbox" not in attributes:
            self.svgs_no_viewbox.append(line)
        if tag == "meta" and (attributes.get("name") or "").lower() == "uses":
            self.uses.append((attributes.get("content") or "", line))
        if tag == "meta" and (attributes.get("name") or "").lower() == "level":
            self.level = (attributes.get("content") or "").strip().lower()
        if tag == "meta" and (attributes.get("name") or "").lower() in ("math-color", "math-colour"):
            self.math_color = (attributes.get("content") or "").strip().lower()
        if tag in ("script", "style"):
            self._in_code += 1

    def handle_endtag(self, tag):
        if tag in ("script", "style"):
            self._in_code = max(0, self._in_code - 1)
        if tag == "title":
            self._in_title = False
        if self._widget_depth is not None and self._depth == self._widget_depth:
            self.incomplete_widgets = getattr(self, "incomplete_widgets", [])
            missing = [k for k in ("h3", "try", "notice") if not self._widget_has[k]]
            if missing:
                self.incomplete_widgets.append((self._widget_has["line"], missing))
            self._widget_depth = None
        self._depth = max(0, self._depth - 1)

    def handle_data(self, data):
        if self._in_code:
            return                                      # minified JS is not prose
        self.prose.append(data)
        if self._in_title:
            self.title += data


def check(markup, scan, own_style, own_script, declared):
    """Return (errors, warnings). Errors mean the page is broken; warnings mean look."""
    errors, warnings = [], []

    # --- libraries -------------------------------------------------------
    for content, line in scan.uses:
        for name in (part.strip().lower() for part in content.split(",")):
            if name and name not in LIBRARIES:
                errors.append(
                    f'line {line}: uses="{name}" is not a library this skill knows. '
                    f"Known: {', '.join(sorted(LIBRARIES))}. An unknown name splices "
                    "nothing and leaves the widget dead."
                )

    for name in declared:
        if name not in LIBRARIES:
            continue                                    # already reported just above
        _, global_name = LIBRARIES[name]
        if name != "katex" and not re.search(rf"\b{re.escape(global_name)}\b", own_script):
            warnings.append(
                f'"{name}" is declared but `{global_name}` never appears in your script. '
                "Drop the declaration or use it."
            )

    for global_name, name in (("d3", "d3"), ("Chart", "chartjs"), ("THREE", "three"),
                              ("Plotly", "plotly"), ("Matter", "matter"), ("gsap", "gsap")):
        if re.search(rf"\b{re.escape(global_name)}\.", own_script) and name not in declared:
            errors.append(
                f'Your script uses `{global_name}` but "{name}" is not in '
                '<meta name="uses">. It will be undefined at runtime.'
            )

    # --- the kit's job, done twice ---------------------------------------
    if re.search(r":root\s*\{", own_style):
        warnings.append(
            "Your <style> defines :root. The kit already ships a full palette -- "
            "redefining it means two sources of truth for the same colours."
        )
    if "prefers-color-scheme" in own_style:
        warnings.append(
            "Your <style> has a prefers-color-scheme block. These documents render light "
            "for every reader on purpose, so this makes the page change colour depending "
            "on who opens it."
        )

    hexes = sorted(set(re.findall(r"#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})\b", own_style)))
    if hexes:
        warnings.append(
            f"Hardcoded colours in your CSS: {', '.join(hexes[:8])}"
            f"{' ...' if len(hexes) > 8 else ''}. These sit outside the palette and "
            "will not follow a change to the kit. Use var(--kit-*)."
        )

    if re.search(r"\bMath\.random\s*\(", own_script):
        warnings.append("Math.random() is not seeded, so a reload changes the page under the reader. Use Kit.rand().")
    if re.search(r"\bsetInterval\s*\(", own_script):
        warnings.append("setInterval keeps running in a hidden tab. Use Kit.loop() for motion.")
    if re.search(r"requestAnimationFrame\s*\(", own_script) and "Kit.loop" not in own_script:
        warnings.append("Raw requestAnimationFrame has no off switch. Use Kit.loop().")

    # --- things that throw or rot ----------------------------------------
    for pattern, message in (
        (r"\b(?:localStorage|sessionStorage)\b", "Storage throws when the file is opened from file://."),
        (r"\bdocument\.cookie\b", "document.cookie is empty here."),
        (r"\b(?:fetch|XMLHttpRequest|EventSource|WebSocket)\s*\(", "There is no API to call; a network read rots or fails offline."),
        (r"\b(?:alert|confirm|prompt)\s*\(", "Modal dialogs interrupt the reader. Put it on the page."),
    ):
        if re.search(pattern, own_script):
            warnings.append(message)

    # --- widgets ---------------------------------------------------------
    if scan.widgets == 0:
        errors.append("No .widget sections. An explainer with nothing to operate is just an article.")
    elif scan.widgets < 3:
        errors.append(
            f"Only {scan.widgets} widget(s). The floor is 3 and the target is 4-6 -- "
            "below that a mechanism is going unexplored."
        )

    for line, missing in getattr(scan, "incomplete_widgets", []):
        warnings.append(f"line {line}: widget is missing {', '.join('<h3>' if m == 'h3' else '.' + m for m in missing)}.")

    wrapped = len(re.findall(r"Kit\.widget\s*\(", own_script))
    if scan.widgets and wrapped < scan.widgets:
        warnings.append(
            f"{scan.widgets} widgets but {wrapped} Kit.widget() call(s). An unwrapped widget "
            "that throws takes the whole page down with it."
        )

    for attributes, line in scan.ranges:
        ident = attributes.get("id") or "(no id)"
        if not attributes.get("data-readout"):
            warnings.append(f"line {line}: range #{ident} has no data-readout, so the reader cannot see what they set.")
        if not attributes.get("data-unit"):
            label = re.search(rf'<label[^>]*for="{re.escape(ident)}"[^>]*>(.*?)</label>',
                              markup, re.S | re.I)
            label_text = re.sub(r"<[^>]+>", " ", label.group(1)) if label else ""
            if not DIMENSIONLESS.search(f"{ident} {label_text}"):
                warnings.append(
                    f"line {line}: range #{ident} has no data-unit. Fine if the quantity is "
                    "genuinely dimensionless -- a ratio, a gain, a count -- but if it has "
                    "units the reader needs them on the readout."
                )
        value, low = attributes.get("value"), attributes.get("min")
        if value is None:
            warnings.append(f"line {line}: range #{ident} has no starting value, so it opens mid-scale by accident.")
        elif low is not None and value == low:
            warnings.append(
                f"line {line}: range #{ident} opens at its minimum. Open somewhere already "
                "interesting -- a page that shows nothing until touched teaches nothing."
            )

    if re.search(r'<div[^>]*\brole="(?:slider|button)"', markup):
        warnings.append("A div with role=slider/button. Native <input>/<button> are keyboard- and screen-reader-ready for free.")

    # --- structure -------------------------------------------------------
    if not scan.title.strip():
        errors.append("No <title>.")
    if not scan.has_lede:
        warnings.append('No <p class="kit-lede">. The first screen should answer the question outright.')
    if not scan.has_breaks:
        warnings.append('No .breaks section. Naming where the model stops applying is most of the honesty.')
    if not scan.has_recap:
        warnings.append("No .recap section.")
    for line in scan.svgs_no_viewbox:
        warnings.append(f"line {line}: <svg> with no viewBox will not scale on a phone.")

    # --- length -----------------------------------------------------------
    words = len(re.findall(r"[A-Za-z][A-Za-z'-]+", " ".join(scan.prose)))
    if scan.level and scan.level in LEVEL_BANDS:
        low, high = LEVEL_BANDS[scan.level]
        if words > high * 1.2:
            warnings.append(
                f"{words} words of prose for level '{scan.level}' (band {low}-{high}). "
                "Going long is rarely fixed by shorter sentences -- it usually means one "
                "more thing was explained than the reader needed."
            )
        elif words < low * 0.8:
            warnings.append(
                f"only {words} words of prose for level '{scan.level}' (band {low}-{high}). "
                "Check a mechanism has not been left as a widget with no explanation."
            )
    elif scan.level:
        errors.append(
            f'<meta name="level" content="{scan.level}"> is not one of: '
            f"{', '.join(LEVEL_BANDS)}."
        )
    else:
        warnings.append(
            'No <meta name="level"> so the prose budget was not checked. '
            f"Add one of: {', '.join(LEVEL_BANDS)}."
        )

    # --- house typography -------------------------------------------------
    prose = prose_only(markup)

    tight = find_tight_commas(prose)
    for hit in tight[:5]:
        errors.append(
            f"Comma with no space around it: \u2026{hit}\u2026 -- house style is "
            '"word-A , word-B", in prose and inside maths alike.'
        )
    if len(tight) > 5:
        errors.append(f"...and {len(tight) - 5} further comma(s) missing their spaces.")

    stops = find_math_then_stop(prose)
    for hit in stops[:5]:
        errors.append(
            f"A sentence ends on an expression and then punctuates it: \u2026{hit} -- "
            "the stop bleeds into the maths. Drop the punctuation, or rewrite so the "
            "sentence does not end on the expression."
        )
    if len(stops) > 5:
        errors.append(f"...and {len(stops) - 5} further expression(s) followed by a stop.")

    if scan.math_color is not None and scan.math_color not in ("accent", "off"):
        errors.append(
            f'<meta name="math-color" content="{scan.math_color}"> is not one of: '
            "accent, off."
        )

    # --- maths ------------------------------------------------------------
    for span in find_currency(strip_regions(markup)):
        warnings.append(
            f"A paragraph reads {span!r} -- two literal $ signs, which KaTeX takes as an "
            'expression and swallows the text between them. Wrap them in <span class="no-math">.'
        )

    return errors, warnings


def verify(urls):
    """HEAD every remote URL. A 404 here is a widget that dies in front of the reader."""
    broken = []
    for url in urls:
        request = urllib.request.Request(url, method="HEAD")
        try:
            with urllib.request.urlopen(request, timeout=12) as response:
                if response.status >= 400:
                    broken.append((url, str(response.status)))
        except urllib.error.HTTPError as error:
            broken.append((url, str(error.code)))
        except Exception as error:                      # offline is not a build failure
            broken.append((url, f"unreachable ({type(error).__name__})"))
    return broken


def build(source, out_path, do_verify=True):
    markup = open(source, encoding="utf-8").read()

    if "<head" not in markup.lower():
        print("error: no <head> to splice into. Write a complete HTML document.", file=sys.stderr)
        return 1

    scan = DocumentScan()
    scan.feed(markup)

    own_style = "\n".join(re.findall(r"<style\b[^>]*>(.*?)</style>", markup, re.S | re.I))
    own_script = "\n".join(re.findall(r"<script\b(?![^>]*\bsrc=)[^>]*>(.*?)</script>", markup, re.S | re.I))

    declared = []
    for content, _ in scan.uses:
        declared += [name.strip().lower() for name in content.split(",") if name.strip()]

    notes = []
    maths = find_math(strip_regions(markup))
    if maths and "katex" not in declared:
        declared.append("katex")                        # detected, so it cannot be forgotten
        notes.append(f"katex added automatically ({len(maths)} expression(s), e.g. {maths[0][:40]!r}).")

    errors, warnings = check(markup, scan, own_style, own_script, declared)

    kit_css = open(os.path.join(ASSETS, "kit.css"), encoding="utf-8").read()
    kit_js = open(os.path.join(ASSETS, "kit.js"), encoding="utf-8").read()

    # Order matters twice over. The kit's CSS goes in before the document's own
    # <style>, so an equal-specificity rule in the document wins -- the kit sets
    # the floor, the document decorates. Library scripts are synchronous rather
    # than deferred because the document's own script sits at the end of <body>
    # and runs during parsing: a deferred library would still be undefined when
    # it got there.
    head = ["<style>\n" + kit_css + "\n</style>"]
    remote = []
    for name in dict.fromkeys(declared):
        if name not in LIBRARIES:
            continue
        for url in LIBRARIES[name][0]:
            remote.append(url)
            head.append(
                f'<link rel="stylesheet" href="{url}">' if url.endswith(".css")
                else f'<script src="{url}"></script>'
            )
    head.append("<script>\n" + kit_js + "\n</script>")

    spliced = re.sub(r"(<head\b[^>]*>)", lambda m: m.group(1) + "\n" + "\n".join(head), markup, count=1, flags=re.I)

    # The option lives with `level` and `uses` as a meta tag, and becomes an
    # attribute here so the stylesheet can key off it without the author having
    # to remember to decorate <html> by hand.
    if scan.math_color == "accent":
        spliced = re.sub(r"<html\b([^>]*)>",
                         lambda m: "<html" + re.sub(r'\s*data-math="[^"]*"', "", m.group(1))
                         + ' data-math="accent">',
                         spliced, count=1, flags=re.I)
    with open(out_path, "w", encoding="utf-8") as handle:
        handle.write(spliced)

    resolved = [name for name in dict.fromkeys(declared) if name in LIBRARIES]
    size = os.path.getsize(out_path) / 1024
    print(f"built {out_path}  ({size:.0f} KB, {scan.widgets} widgets"
          + (f", level: {scan.level}" if scan.level else "")
          + (f", libraries: {', '.join(resolved)}" if resolved else ", no libraries")
          + (", maths in accent colour" if scan.math_color == "accent" else "") + ")")
    for note in notes:
        print(f"  note   {note}")

    if remote and do_verify:
        for url, why in verify(remote):
            warnings.append(f"CDN check: {url} -> {why}")

    for message in errors:
        print(f"  ERROR  {message}")
    for message in warnings:
        print(f"  warn   {message}")
    if not errors and not warnings:
        print("  clean.")
    elif not errors:
        print(f"\n{len(warnings)} warning(s). Fix what is real, then rebuild.")
    else:
        print(f"\n{len(errors)} error(s) must be fixed. Edit the source and rebuild.")

    return 1 if errors else 0


def main():
    parser = argparse.ArgumentParser(description="Splice the kit into an explainer and check it.")
    parser.add_argument("source", help="the document you wrote")
    parser.add_argument("-o", "--output", help="where the finished file goes (default: <source>.built.html)")
    parser.add_argument("--no-verify", action="store_true", help="skip the CDN HEAD checks (offline)")
    arguments = parser.parse_args()

    out_path = arguments.output or re.sub(r"\.html?$", "", arguments.source) + ".built.html"
    return build(arguments.source, out_path, do_verify=not arguments.no_verify)


if __name__ == "__main__":
    sys.exit(main())
