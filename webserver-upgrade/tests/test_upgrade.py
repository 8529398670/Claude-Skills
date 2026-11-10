"""Run with: python3 -B -m unittest discover -s tests  (from the skill directory)

One test per merge rule, then the whole prepare/apply/rollback cycle on a real
git repository, then upgrades being chosen or left out, then every skill commit
through the renderer.
"""
import argparse
import contextlib
import io
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "scripts"))
import upgrade  # noqa: E402
from upgrade import Action, File, Identity  # noqa: E402

IDENTITY = Identity("go-webserver", "Thing", "thing", "thing", 21000, 21000, ["api_keys"])
TABLE = {"api_keys": ("auth", "api_keys", ()), "webhooks": ("webhooks", "enabled", ())}


def one(path, base, new, ours, base_mode=0o644, new_mode=0o644, our_mode=0o644):
    def wrap(data, mode):
        return None if data is None else File(data, mode)
    return upgrade.plan_file(path, wrap(base, base_mode), wrap(new, new_mode), wrap(ours, our_mode))


class FileRules(unittest.TestCase):
    def test_unchanged_upstream_leaves_app_edits_alone(self):
        self.assertIsNone(one("a.go", b"x\n", b"x\n", b"mine\n"))

    def test_app_untouched_takes_new(self):
        action = one("a.go", b"x\n", b"y\n", b"x\n")
        self.assertEqual((action.kind, action.data), ("update", b"y\n"))

    def test_both_changed_different_lines_merges(self):
        base = b"1\n2\n3\n4\n5\n6\n7\n"
        new = b"1 upstream\n2\n3\n4\n5\n6\n7\n"
        ours = b"1\n2\n3\n4\n5\n6\n7 app\n"
        action = one("a.go", base, new, ours)
        self.assertEqual(action.kind, "update")
        self.assertEqual(action.data, b"1 upstream\n2\n3\n4\n5\n6\n7 app\n")

    def test_same_line_conflicts_with_base_shown(self):
        action = one("a.go", b"x\n", b"upstream\n", b"app\n")
        self.assertEqual(action.kind, "conflict")
        self.assertIn(b"<<<<<<< app", action.data)
        self.assertIn(b"||||||| base", action.data)
        self.assertIn(b">>>>>>> template", action.data)

    def test_both_made_the_same_change_is_nothing(self):
        self.assertIsNone(one("a.go", b"x\n", b"y\n", b"y\n"))

    def test_new_upstream_is_added(self):
        action = one("a.go", None, b"y\n", None)
        self.assertEqual((action.kind, action.data), ("add", b"y\n"))

    def test_app_deleted_is_not_brought_back(self):
        self.assertEqual(one("a.go", b"x\n", b"y\n", None).kind, "app-deleted")

    def test_upstream_deleted_app_unchanged_deletes(self):
        self.assertEqual(one("a.go", b"x\n", None, b"x\n").kind, "delete")

    def test_upstream_deleted_app_modified_is_kept(self):
        self.assertEqual(one("a.go", b"x\n", None, b"mine\n").kind, "kept-modified")

    def test_added_on_both_sides_keeps_app(self):
        self.assertEqual(one(".gitignore", None, b"a\n", b"b\n").kind, "both-added")

    def test_vendored_replaced_only_when_untouched(self):
        path = "static/vendor/bootstrap.min.css"
        self.assertEqual(one(path, b"v1", b"v2", b"v1").kind, "update")
        self.assertEqual(one(path, b"v1", b"v2", b"patched").kind, "opaque-kept")

    def test_binary_is_never_merged(self):
        self.assertEqual(one("x.dat", b"\0a", b"\0b", b"\0c").kind, "opaque-kept")

    def test_go_sum_is_kept(self):
        self.assertEqual(one("go.sum", b"a\n", b"b\n", b"c\n").kind, "deps-kept")

    def test_go_mod_only_raises(self):
        base = b"module t\n\ngo 1.21\n\nrequire (\n\ta.io/x v1.0.0\n\tb.io/y v1.5.0\n)\n"
        new = b"module t\n\ngo 1.23\n\nrequire (\n\ta.io/x v1.2.0\n\tb.io/y v1.0.0\n\tc.io/z v0.3.0 // indirect\n)\n"
        ours = b"module t\n\ngo 1.22\n\nrequire (\n\ta.io/x v1.0.0\n\tb.io/y v1.5.0\n\tmine.io/q v2.0.0\n)\n"
        action = one("go.mod", base, new, ours)
        self.assertEqual(action.kind, "update")
        text = action.data.decode()
        self.assertIn("go 1.23\n", text)
        self.assertIn("a.io/x v1.2.0", text)
        self.assertIn("b.io/y v1.5.0", text)  # never lowered
        self.assertIn("mine.io/q v2.0.0", text)  # the app's own dependency survives
        self.assertIn("c.io/z v0.3.0 // indirect", text)

    def test_exec_bit_merges_three_ways(self):
        action = one("s.sh", b"x\n", b"x\n", b"x\n", 0o644, 0o755, 0o644)
        self.assertEqual((action.kind, action.mode), ("mode", 0o755))
        self.assertIsNone(one("s.sh", b"x\n", b"x\n", b"x\n", 0o644, 0o755, 0o755))
        # The app changed it itself, so its choice stands.
        self.assertIsNone(one("s.sh", b"x\n", b"x\n", b"x\n", 0o755, 0o644, 0o700))

    def test_crlf_app_file_stays_crlf_and_matches_lf_base(self):
        action = one("run.bat", b"a\nb\n", b"a\nc\n", b"a\r\nb\r\n")
        self.assertEqual((action.kind, action.data), ("update", b"a\r\nc\r\n"))

    def test_crlf_alone_is_not_a_difference(self):
        self.assertIsNone(one("run.bat", b"a\n", b"a\n", b"a\r\n"))

    def test_new_bat_gets_crlf(self):
        self.assertEqual(one("x.bat", None, b"a\nb\n", None).data, b"a\r\nb\r\n")

    def test_migrations_are_flagged_even_when_clean(self):
        actions = upgrade.plan_merge(
            {"server/db/migrate.go": File(b"x\n")}, {"server/db/migrate.go": File(b"y\n")}, {"server/db/migrate.go": File(b"x\n")},
        )
        self.assertIn("schema version", actions[0].note)


class Normalise(unittest.TestCase):
    DEFAULTS = b"server:\n  port: 8080\ndocker:\n  host_port: 8080\nauth:\n  api_keys: false       # a comment\nwebhooks:\n  enabled: true\n"

    def test_secrets_and_runtime_state_are_dropped(self):
        files = {name: File(b"x") for name in ("config/secret.key", "config/config.yaml", "config.yaml", ".skill-origin.yaml", ".run/pid", "data/app.db", "dist/a", "thing", "keep.go")}
        self.assertEqual(set(upgrade.normalise(files, IDENTITY, {})), {"keep.go"})

    def test_ports_pinned_and_new_switches_off(self):
        files = {"config/defaults.yaml": File(self.DEFAULTS), "config/example_header.yaml": File(b"# header\n")}
        result = upgrade.normalise(files, IDENTITY, TABLE)
        text = result["config/defaults.yaml"].data.decode()
        self.assertIn("port: 21000", text)
        self.assertIn("host_port: 21000", text)
        self.assertIn("api_keys: true        # a comment", text)  # comment stays in its column
        self.assertIn("enabled: false", text)  # not in features: off, though the template says on
        self.assertEqual(result["config/config.example.yaml"].data, b"# header\n" + text.encode())

    def test_the_apps_own_repo_is_kept(self):
        defaults = b'app:\n  name: "Thing"\n  repo: ""\n'
        identity = Identity("go-webserver", "Thing", "thing", "thing", None, None, [], repo="https://github.com/you/thing")
        text = upgrade.normalise({"config/defaults.yaml": File(defaults)}, identity, {})["config/defaults.yaml"].data.decode()
        self.assertIn('  repo: "https://github.com/you/thing"', text)
        # An app with no repo line, from before app.repo, leaves the render alone.
        text = upgrade.normalise({"config/defaults.yaml": File(defaults)}, IDENTITY, {})["config/defaults.yaml"].data.decode()
        self.assertIn('  repo: ""', text)

    def test_lf_throughout(self):
        self.assertEqual(upgrade.normalise({"a.bat": File(b"x\r\n")}, IDENTITY, {})["a.bat"].data, b"x\n")


class Example(unittest.TestCase):
    def test_example_is_built_from_merged_defaults(self):
        ours = {"config/defaults.yaml": File(b"old\n"), "config/example_header.yaml": File(b"# h\n"), "config/config.example.yaml": File(b"stale\n")}
        new = {"config/defaults.yaml": File(b"new\n"), "config/example_header.yaml": File(b"# h\n")}
        actions = [Action("config/defaults.yaml", "update", b"merged\n")]
        upgrade.regenerate_example(actions, ours, new)
        self.assertEqual(actions[-1].path, "config/config.example.yaml")
        self.assertEqual(actions[-1].data, b"# h\nmerged\n")


def run_git(directory, *arguments):
    subprocess.run(["git", "-C", str(directory), "-c", "user.name=t", "-c", "user.email=t@t", *arguments], check=True, capture_output=True)


class Repo(unittest.TestCase):
    """prepare, apply and rollback on a real repository, with the two renders
    stood in for so the test controls exactly what changed upstream."""

    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        root = Path(self.scratch.name)
        self.app = root / "app"
        self.state = root / "state"
        self.app.mkdir()
        (self.app / "config").mkdir()
        (self.app / "go.mod").write_text("module thing\n")
        (self.app / "same.go").write_text("old\n")
        (self.app / "both.go").write_text("mine\n")
        (self.app / "gone.go").write_text("bye\n")
        (self.app / "config" / "secret.key").write_text("secret\n")
        (self.app / ".gitignore").write_text("config/secret.key\n*.json\n")
        (self.app / ".skill-origin.yaml").write_text('skill: go-webserver\ncommit: aaaa\nproject_name: "Thing"\nslug: thing\nport: 21000\nfeatures: [api_keys]\n')
        run_git(self.app, "init", "-q", "-b", "master")
        run_git(self.app, "add", ".")
        run_git(self.app, "commit", "-qm", "start")

        base = {"same.go": File(b"old\n"), "both.go": File(b"x\n"), "gone.go": File(b"bye\n")}
        new = {"same.go": File(b"new\n"), "both.go": File(b"y\n"), "added.go": File(b"hello\n"), "config/openapi.json": File(b"{}\n")}
        self.renders = {"aaaa": base, "bbbb": new}
        self.saved = (upgrade.render, upgrade.resolve_commit)
        upgrade.render = lambda commit, identity, table=None: (self.renders[commit], {})
        upgrade.resolve_commit = lambda reference: "bbbb"
        self.addCleanup(self.restore)

    def restore(self):
        upgrade.render, upgrade.resolve_commit = self.saved

    def run_command(self, function, **extra):
        extra.setdefault("skip", None)
        arguments = argparse.Namespace(app=str(self.app), state=str(self.state), to="HEAD", force=False, **extra)
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            function(arguments)
        return output.getvalue()

    def staged(self, path):
        return self.state / "thing" / "staging" / "files" / path

    def snapshot(self):
        return {path.relative_to(self.app).as_posix(): (path.read_bytes(), path.stat().st_mode) for path in self.app.rglob("*") if path.is_file() and ".git/" not in path.as_posix()}

    def resolve_both(self):
        self.staged("both.go").write_text("resolved\n")


class Cycle(Repo):
    def test_prepare_leaves_the_app_alone(self):
        before = self.snapshot()
        output = self.run_command(upgrade.command_prepare)
        self.assertEqual(self.snapshot(), before)
        self.assertIn("both.go", output)
        self.assertIn("config/openapi.json is new and git would ignore it", output)

    def test_apply_refuses_conflict_markers(self):
        self.run_command(upgrade.command_prepare)
        before = self.snapshot()
        with self.assertRaisesRegex(upgrade.Refused, "Conflict markers remain in: both.go"):
            self.run_command(upgrade.command_apply)
        self.assertEqual(self.snapshot(), before)

    def test_apply_then_rollback_is_byte_for_byte(self):
        before = self.snapshot()
        self.run_command(upgrade.command_prepare)
        self.resolve_both()
        self.run_command(upgrade.command_apply)
        self.assertEqual((self.app / "same.go").read_text(), "new\n")
        self.assertEqual((self.app / "both.go").read_text(), "resolved\n")
        self.assertEqual((self.app / "added.go").read_text(), "hello\n")
        self.assertFalse((self.app / "gone.go").exists())
        self.assertEqual((self.app / "config" / "secret.key").read_text(), "secret\n")
        self.assertIn("commit: bbbb", (self.app / ".skill-origin.yaml").read_text())
        self.run_command(upgrade.command_rollback)
        self.assertEqual(self.snapshot(), before)

    def test_uncommitted_touched_file_refuses_but_unrelated_does_not(self):
        (self.app / "notes.txt").write_text("unrelated\n")
        self.run_command(upgrade.command_prepare)
        (self.app / "same.go").write_text("edited\n")
        with self.assertRaisesRegex(upgrade.Refused, "same.go"):
            self.run_command(upgrade.command_prepare)

    def test_second_apply_waits_for_the_first(self):
        self.run_command(upgrade.command_prepare)
        self.resolve_both()
        self.run_command(upgrade.command_apply)
        with self.assertRaisesRegex(upgrade.Refused, "not committed yet"):
            self.run_command(upgrade.command_prepare)
        run_git(self.app, "add", ".")
        run_git(self.app, "commit", "-qm", "done")
        self.renders["cccc"] = dict(self.renders["bbbb"])
        self.assertIn("already at", self.run_command(upgrade.command_prepare))

    def test_new_commit_after_prepare_refuses(self):
        self.run_command(upgrade.command_prepare)
        self.resolve_both()
        (self.app / "other.txt").write_text("x\n")
        run_git(self.app, "add", ".")
        run_git(self.app, "commit", "-qm", "more")
        with self.assertRaisesRegex(upgrade.Refused, "new commits"):
            self.run_command(upgrade.command_apply)

    def test_rollback_will_not_lose_later_work(self):
        self.run_command(upgrade.command_prepare)
        self.resolve_both()
        self.run_command(upgrade.command_apply)
        (self.app / "added.go").write_text("my later edit\n")
        with self.assertRaisesRegex(upgrade.Refused, "added.go"):
            self.run_command(upgrade.command_rollback)

    def test_no_branch_is_ever_created(self):
        self.run_command(upgrade.command_prepare)
        self.resolve_both()
        self.run_command(upgrade.command_apply)
        branches = subprocess.run(["git", "-C", str(self.app), "branch", "--format=%(refname:short)"], capture_output=True, text=True).stdout.split()
        self.assertEqual(branches, ["master"])
        log = subprocess.run(["git", "-C", str(self.app), "rev-list", "--count", "HEAD"], capture_output=True, text=True).stdout.strip()
        self.assertEqual(log, "1")  # nothing committed


ENTRIES = """## After abc

**Upgrade `rename-things`: Renaming things.**
Needs: `helpers`.
Marks: `rename_thing`
Files: `added.go`

Prose that says how, with `quoted` words that are not marks.

**Upgrade `hooks-edit`: Editing webhooks.**
Needs: `webhooks`, `helpers`.
Marks: `edit_hook`

**Plumbing `helpers`: Shared helpers.**
Marks: `helper_fn`

**Some fix:** not labelled, so it always lands.
"""


class Items(unittest.TestCase):
    def test_labels_are_read_and_prose_is_not(self):
        items = upgrade.parse_items(ENTRIES)
        self.assertEqual([item.name for item in items], ["rename-things", "hooks-edit", "helpers"])
        self.assertEqual(items[0].needs, ["helpers"])
        self.assertEqual(items[0].marks, ["rename_thing"])
        self.assertEqual(items[0].files, ["added.go"])
        self.assertTrue(items[2].plumbing)

    def test_an_example_in_a_code_block_is_not_an_item(self):
        self.assertEqual(upgrade.parse_items("```\n**Upgrade `x`: X.**\nMarks: `m`\n```\n"), [])

    def offers(self, base=b"", new=b"rename_thing edit_hook helper_fn", ours=b"", earlier=()):
        table = {"api_keys": ("auth", "api_keys", ()), "webhooks": ("webhooks", "enabled", ())}
        return upgrade.sort_offers(upgrade.parse_items(ENTRIES), {"a": File(base)}, {"a": File(new)}, {"a": File(ours)}, IDENTITY, table, list(earlier))

    def test_needing_an_off_feature_is_not_asked_about(self):
        offers = self.offers()
        self.assertEqual([item.name for item in offers.ask], ["rename-things"])
        self.assertEqual([(item.name, missing) for item, missing in offers.off], [("hooks-edit", ["webhooks"])])

    def test_already_in_the_template_the_app_is_on_is_not_new(self):
        self.assertEqual(self.offers(base=b"rename_thing").ask, [])

    def test_already_in_the_app_is_not_new(self):
        self.assertEqual(self.offers(ours=b"rename_thing").ask, [])

    def test_skipped_earlier_is_not_asked_again(self):
        offers = self.offers(earlier=["rename-things"])
        self.assertEqual(offers.ask, [])
        self.assertEqual(offers.earlier, ["rename-things"])

    def test_plumbing_stays_only_for_what_needs_it(self):
        taken, left = upgrade.settle(self.offers(), set())
        self.assertEqual({item.name for item in taken}, {"rename-things", "helpers"})
        self.assertEqual({item.name for item in left}, {"hooks-edit"})
        taken, left = upgrade.settle(self.offers(), {"rename-things"})
        self.assertEqual(taken, [])
        self.assertEqual({item.name for item in left}, {"rename-things", "hooks-edit", "helpers"})

    def test_needing_a_skipped_upgrade_skips_it_too(self):
        entries = "**Upgrade `one`: One.**\nMarks: `m1`\n\n**Upgrade `two`: Two.**\nNeeds: `one`.\nMarks: `m2`\n"
        offers = upgrade.sort_offers(upgrade.parse_items(entries), {}, {"a": File(b"m1 m2")}, {}, IDENTITY, {}, [])
        taken, left = upgrade.settle(offers, {"one"})
        self.assertEqual(taken, [])
        self.assertEqual({item.name for item in left}, {"one", "two"})

    def test_a_whole_feature_the_stamp_lists_is_not_offered(self):
        entries = "**Upgrade `api_keys`: Keys.**\nMarks: `m1`\n"
        table = {"api_keys": ("auth", "api_keys", ())}
        offers = upgrade.sort_offers(upgrade.parse_items(entries), {}, {"a": File(b"m1")}, {}, IDENTITY, table, [])
        self.assertEqual(offers.ask, [])


class Entries(unittest.TestCase):
    """Labels in the real UPGRADING.md files have to match the real templates."""

    def test_every_mark_and_file_is_in_the_template(self):
        repo = Path(upgrade.SKILLS_REPO)
        for skill in ("go-webserver", "python-webserver"):
            template = repo / skill / "assets" / "template"
            items = upgrade.parse_items((repo / skill / "UPGRADING.md").read_text(encoding="utf-8"))
            self.assertTrue(items)
            text = b"\n".join(path.read_bytes() for path in template.rglob("*") if path.is_file())
            for item in items:
                self.assertTrue(item.marks, item.name)
                for mark in item.marks:
                    self.assertIn(mark.encode(), text, f"{skill} {item.name}")
                for name in item.files:
                    self.assertTrue((template / name).exists(), f"{skill} {name}")


class Skipping(Repo):
    def setUp(self):
        super().setUp()
        self.renders["bbbb"]["same.go"] = File(b"new rename_thing\n")
        self.items = upgrade.parse_items("**Upgrade `rename-things`: Renaming things.**\nMarks: `rename_thing`\nFiles: `added.go`\n")
        self.saved_items = upgrade.upgrade_items
        upgrade.upgrade_items = lambda commit, skill: self.items
        self.addCleanup(lambda: setattr(upgrade, "upgrade_items", self.saved_items))

    def test_prepare_wants_a_decision(self):
        with self.assertRaises(upgrade.Refused):
            self.run_command(upgrade.command_prepare)

    def test_unknown_name_refuses(self):
        with self.assertRaises(upgrade.Refused):
            self.run_command(upgrade.command_prepare, skip="nope")

    def test_taken_lands_whole(self):
        self.run_command(upgrade.command_prepare, skip="none")
        self.assertTrue(self.staged("added.go").exists())

    def test_left_out_file_stays_out_and_fixes_land(self):
        self.run_command(upgrade.command_prepare, skip="rename-things")
        self.assertFalse(self.staged("added.go").exists())
        self.staged("same.go").write_text("new\n")
        self.staged("both.go").write_text("mine\ny\n")
        self.run_command(upgrade.command_apply)
        self.assertFalse((self.app / "added.go").exists())
        self.assertEqual((self.app / "same.go").read_text(), "new\n")
        self.assertIn("skipped: [rename-things]", (self.app / ".skill-origin.yaml").read_text())

    def test_apply_refuses_a_leftover_mark(self):
        self.run_command(upgrade.command_prepare, skip="rename-things")
        self.staged("both.go").write_text("mine\ny\n")
        with self.assertRaises(upgrade.Refused) as caught:
            self.run_command(upgrade.command_apply)
        self.assertIn("rename_thing", str(caught.exception))

    def test_the_apps_own_use_of_a_mark_is_not_a_leftover(self):
        self.items[0].marks.append("hello")
        (self.app / "same.go").write_text("old\n# rename_thing of our own\n")
        run_git(self.app, "add", ".")
        run_git(self.app, "commit", "-qm", "own")
        self.run_command(upgrade.command_prepare, skip="rename-things")
        self.staged("same.go").write_text("new\n# rename_thing of our own\n")
        self.staged("both.go").write_text("mine\ny\n")
        self.run_command(upgrade.command_apply)

    def test_rollback_restores_the_stamp(self):
        before = (self.app / ".skill-origin.yaml").read_text()
        self.run_command(upgrade.command_prepare, skip="rename-things")
        self.staged("same.go").write_text("new\n")
        self.staged("both.go").write_text("mine\ny\n")
        self.run_command(upgrade.command_apply)
        self.run_command(upgrade.command_rollback)
        self.assertEqual((self.app / ".skill-origin.yaml").read_text(), before)

    def test_a_second_update_does_not_bring_it_back(self):
        self.run_command(upgrade.command_prepare, skip="rename-things")
        self.staged("same.go").write_text("new\n")
        self.staged("both.go").write_text("mine\ny\n")
        self.run_command(upgrade.command_apply)
        run_git(self.app, "add", ".")
        run_git(self.app, "commit", "-qm", "first")
        later = dict(self.renders["bbbb"])
        later["added.go"] = File(b"hello again\n")
        later["same.go"] = File(b"new rename_thing\nmore\n")
        self.renders["cccc"] = later
        upgrade.resolve_commit = lambda reference: "cccc"
        output = self.run_command(upgrade.command_prepare)
        self.assertNotIn("Refused", output)
        self.assertFalse(self.staged("added.go").exists())
        self.assertNotIn("added.go", output)


class Pruned(Repo):
    """An app scaffolded without a part: the renders stand in for what scaffold.py
    produces with and without it."""

    def setUp(self):
        super().setUp()
        stamp = self.app / ".skill-origin.yaml"
        stamp.write_text(stamp.read_text() + "pruned: [metrics, webhooks]\n")
        (self.app / "same.go").write_text("// mine\n\nold\n")
        run_git(self.app, "add", ".")
        run_git(self.app, "commit", "-qm", "stamped")
        (self.app / "both.go").write_text("x\n")
        (self.app / "gone.go").unlink()
        run_git(self.app, "add", ".")
        run_git(self.app, "commit", "-qm", "tidy")
        self.calls = []

        def render(commit, identity, table=None):
            self.calls.append((commit, list(identity.pruned), list(identity.features)))
            files = {"same.go": File(b"old\n"), "both.go": File(b"x\n")}
            if "webhooks" not in identity.pruned:
                files["hooks.go"] = File(b"hooks\n")
                files["same.go"] = File(b"old\nwire\n")
            if "metrics" not in identity.pruned:
                files["metrics.go"] = File(b"metrics\n")
            return files, {"webhooks": ("webhooks", "enabled", ()), "metrics": ("metrics", "enabled", ("api_keys",))}

        upgrade.render = render
        saved = upgrade.needs_table
        upgrade.needs_table = lambda commit, skill: {"webhooks": (), "metrics": ("api_keys",), "logs": ()}
        self.addCleanup(lambda: setattr(upgrade, "needs_table", saved))

    def add(self, features):
        arguments = argparse.Namespace(app=str(self.app), state=str(self.state), features=features)
        with contextlib.redirect_stdout(io.StringIO()):
            upgrade.command_add_feature(arguments)

    def test_identify_reads_the_list(self):
        self.assertEqual(upgrade.identify(self.app).pruned, ["metrics", "webhooks"])

    def test_adding_brings_the_part_and_keeps_what_the_app_changed(self):
        self.add("webhooks")
        self.assertEqual(self.calls[-1][1], ["metrics"])
        self.assertIn("webhooks", self.calls[-1][2])
        self.assertFalse((self.app / "hooks.go").exists())
        with contextlib.redirect_stdout(io.StringIO()):
            upgrade.command_apply(argparse.Namespace(app=str(self.app), state=str(self.state)))
        self.assertEqual((self.app / "hooks.go").read_text(), "hooks\n")
        self.assertFalse((self.app / "metrics.go").exists())
        self.assertIn("wire", (self.app / "same.go").read_text())
        self.assertIn("mine", (self.app / "same.go").read_text())
        stamp = (self.app / ".skill-origin.yaml").read_text()
        self.assertIn("pruned: [metrics]", stamp)
        self.assertIn("webhooks", stamp.split("features:")[1].split("\n")[0])
        self.assertIn("commit: aaaa", stamp)

    def test_something_the_app_has_cannot_be_added(self):
        with self.assertRaisesRegex(upgrade.Refused, "not scaffolded without logs"):
            self.add("logs")


class OldApps(Repo):
    def test_an_app_from_before_pruning_has_nothing_pruned(self):
        self.assertEqual(upgrade.identify(self.app).pruned, [])

    def test_an_update_never_takes_a_part_out_of_it(self):
        # The new render is the same template with every marker gone, so nothing
        # that the app holds is read as removed upstream.
        self.renders["aaaa"] = {"hooks.go": File(b"hooks\n"), "same.go": File(b"old\n")}
        self.renders["bbbb"] = {"hooks.go": File(b"hooks\nfixed\n"), "same.go": File(b"old\n")}
        (self.app / "hooks.go").write_text("hooks\n")
        run_git(self.app, "add", ".")
        run_git(self.app, "commit", "-qm", "hooks")
        self.run_command(upgrade.command_prepare)
        self.assertEqual(self.staged("hooks.go").read_text(), "hooks\nfixed\n")


class EveryCommit(unittest.TestCase):
    def test_every_commit_renders_the_same_twice(self):
        for skill, module in (("go-webserver", "thing"), ("python-webserver", None)):
            identity = Identity(skill, "Thing", "thing", module, 21000, 21000, ["api_keys"])
            for commit in upgrade.skill_commits(skill):
                with self.subTest(skill=skill, commit=commit[:7]):
                    first, _ = upgrade.render(commit, identity)
                    second, _ = upgrade.render(commit, identity)
                    self.assertEqual({k: (v.data, v.mode) for k, v in first.items()}, {k: (v.data, v.mode) for k, v in second.items()})
                    self.assertNotIn("config/secret.key", first)
                    self.assertFalse([path for path in first if "{{" in path])


if __name__ == "__main__":
    unittest.main()
