"""The hard rules for the frontend (see "Hard rules" in references/frontend.md),
the parts a test can see:

- Mobile first: media queries add from a breakpoint up (min-width). The one
  max-width query allowed is a phone-only form of a component, and says so in
  the comment right above it with the word "phone-only".
- Layout is Bootstrap's grid: no grid-template-columns, except inside the few
  components Bootstrap has no class for.
- Every table scrolls in its own .table-responsive box, so a wide one never
  makes the page scroll sideways on a phone.

How much room a table gets and which icons are used are judgement calls with
good-reason exceptions, so they are written rules and not tested here."""
from __future__ import annotations

import re
import unittest
from pathlib import Path

STATIC = Path(__file__).resolve().parents[2] / "static"

# Components that are a grid inside themselves and have no Bootstrap class:
# the calendar's seven days, the small picture of a theme, and the ? list of
# key caps beside what they do.
GRID_COMPONENTS = {".log-cal-grid", ".theme-preview", ".shortcut-list"}

CSS_COMMENT = re.compile(r"/\*.*?\*/", re.DOTALL)


def read(name: str) -> str:
    return (STATIC / name).read_text(encoding="utf-8")


def scripts() -> dict[str, str]:
    return {path.name: path.read_text(encoding="utf-8") for path in sorted((STATIC / "js").glob("*.js"))}


class FrontendRulesTest(unittest.TestCase):
    def test_mobile_first(self) -> None:
        css = read("css/style.css")
        for match in re.finditer(r"@media[^{]*", css):
            query = match.group(0)
            if "max-width" not in query:
                continue
            before = css[: match.start()].rstrip()
            comment = before[before.rfind("/*") :] if before.endswith("*/") else ""
            self.assertIn(
                "phone-only",
                comment,
                f"style.css: {query.strip()!r} is desktop first. Write the phone as the base "
                "and add from a breakpoint up with min-width (frontend.md, Hard rules)",
            )

    def test_layout_is_bootstrap_grid(self) -> None:
        css = CSS_COMMENT.sub("", read("css/style.css"))
        for selectors, body in re.findall(r"([^{}]+)\{([^{}]*)\}", css):
            if "grid-template-columns" not in body:
                continue
            for selector in (part.strip() for part in selectors.split(",")):
                self.assertIn(
                    selector,
                    GRID_COMPONENTS,
                    f"style.css: {selector} lays out with grid-template-columns. "
                    "Use Bootstrap's .row / .col-* / row-cols-* (frontend.md, Hard rules)",
                )
        sources = scripts()
        sources["index.html"] = read("index.html")
        for name, text in sources.items():
            self.assertFalse(
                "grid-template-columns" in text or "gridTemplateColumns" in text,
                f"{name} sets grid columns itself. Use Bootstrap's .row / .col-* (frontend.md, Hard rules)",
            )

    def test_every_table_scrolls_in_its_own_box(self) -> None:
        html = read("index.html")
        for match in re.finditer(r"<table\b", html):
            tags = re.findall(r"<[a-zA-Z][^>]*>", html[: match.start()])
            parent = tags[-1] if tags else ""
            line = html.count("\n", 0, match.start()) + 1
            self.assertIn(
                "table-responsive",
                parent,
                f'index.html:{line}: put this table right inside a <div class="table-responsive"> '
                "(frontend.md, Hard rules)",
            )
        for name, text in scripts().items():
            if 'Dom.el( "table"' in text:
                self.assertIn(
                    '"table-responsive"',
                    text,
                    f"{name} builds a table without a .table-responsive box around it (frontend.md, Hard rules)",
                )


if __name__ == "__main__":
    unittest.main()
