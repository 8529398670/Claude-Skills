"""The pruning step both webserver skills use (scripts/prune.py in each)."""
from __future__ import annotations

import filecmp
import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path

SKILLS = Path(__file__).resolve().parent.parent.parent
FILE = SKILLS / "go-webserver" / "scripts" / "prune.py"

spec = importlib.util.spec_from_file_location("prune", FILE)
prune = importlib.util.module_from_spec(spec)
sys.modules["prune"] = prune
spec.loader.exec_module(prune)


class ExpressionTest(unittest.TestCase):
    def test_keeps(self) -> None:
        self.assertTrue(prune.keeps("a", set()))
        self.assertFalse(prune.keeps("a", {"a"}))
        self.assertTrue(prune.keeps("!a", {"a"}))
        self.assertFalse(prune.keeps("!a", set()))
        self.assertTrue(prune.keeps("a|b", {"a"}))
        self.assertFalse(prune.keeps("a|b", {"a", "b"}))


class TextTest(unittest.TestCase):
    def test_a_block_goes_with_its_markers(self) -> None:
        text = "one\n// >>> feature:a\ntwo\n// <<< feature:a\nthree\n"
        self.assertEqual(prune.prune_text(text, {"a"}), "one\nthree\n")
        self.assertEqual(prune.prune_text(text, set()), "one\ntwo\nthree\n")

    def test_every_comment_style(self) -> None:
        for opening, closing in (("//", ""), ("#", ""), ("/*", " */"), ("<!--", " -->")):
            text = f"{opening} >>> feature:a{closing}\ntwo\n{opening} <<< feature:a{closing}\nthree {opening} feature:a{closing}\nfour\n"
            self.assertEqual(prune.prune_text(text, {"a"}), "four\n", opening)
            self.assertEqual(prune.prune_text(text, set()), "two\nthree\nfour\n", opening)

    def test_blocks_nest(self) -> None:
        text = "# >>> feature:a\none\n# >>> feature:b\ntwo\n# <<< feature:b\n# <<< feature:a\n"
        self.assertEqual(prune.prune_text(text, {"b"}), "one\n")
        self.assertEqual(prune.prune_text(text, {"a"}), "")

    def test_the_opposite_of_a_feature(self) -> None:
        text = "// >>> feature:!a\nstub\n// <<< feature:!a\nreal // feature:a\n"
        self.assertEqual(prune.prune_text(text, {"a"}), "stub\n")
        self.assertEqual(prune.prune_text(text, set()), "real\n")

    def test_crlf_survives(self) -> None:
        text = "rem >>> feature:a\r\ntwo\r\nrem <<< feature:a\r\nthree\r\n"
        self.assertEqual(prune.prune_text(text, set()), "two\r\nthree\r\n")

    def test_a_string_that_mentions_a_feature_is_left_alone(self) -> None:
        text = 'key: "feature:a is not a marker here"\n'
        self.assertEqual(prune.prune_text(text, {"a"}), text)

    def test_broken_markers_stop_it(self) -> None:
        with self.assertRaises(prune.MarkerError):
            prune.prune_text("# >>> feature:a\n", set())
        with self.assertRaises(prune.MarkerError):
            prune.prune_text("# <<< feature:a\n", set())
        with self.assertRaises(prune.MarkerError):
            prune.prune_text("# >>> feature:a\n# <<< feature:b\n", set())


class SpecTest(unittest.TestCase):
    def test_paths_and_schemas(self) -> None:
        spec_text = json.dumps({
            "paths": {"/a": {"x-feature": "a", "get": {}}, "/b": {"get": {}}},
            "components": {"schemas": {"A": {"x-feature": "a"}, "B": {"type": "object"}}},
        })
        kept = json.loads(prune.prune_spec(spec_text, set()))
        self.assertEqual(kept["paths"]["/a"], {"get": {}})
        gone = json.loads(prune.prune_spec(spec_text, {"a"}))
        self.assertEqual(list(gone["paths"]), ["/b"])
        self.assertEqual(list(gone["components"]["schemas"]), ["B"])


class TreeTest(unittest.TestCase):
    def test_a_tree(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "pkg").mkdir()
            (root / "pkg" / "only.go").write_text("// whole-file feature:a\npackage pkg\n")
            (root / "shared.go").write_text("keep\ncut // feature:a\n")
            (root / "off.go").write_text("// whole-file feature:!a\nstub\n")
            (root / "logo.png").write_bytes(b"\x89PNG feature:a")
            prune.prune_tree(root, {"a"})
            self.assertEqual(sorted(path.name for path in root.iterdir()), ["logo.png", "off.go", "shared.go"])
            self.assertEqual((root / "shared.go").read_text(), "keep\n")
            self.assertEqual((root / "off.go").read_text(), "stub\n")

    def test_nothing_removed_still_clears_the_markers(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "only.go").write_text("// whole-file feature:a\npackage pkg\n")
            prune.prune_tree(root, set())
            self.assertEqual((root / "only.go").read_text(), "package pkg\n")


class CopiesTest(unittest.TestCase):
    def test_both_skills_have_the_same_file(self) -> None:
        self.assertTrue(filecmp.cmp(FILE, SKILLS / "python-webserver" / "scripts" / "prune.py", shallow=False))


if __name__ == "__main__":
    unittest.main()
