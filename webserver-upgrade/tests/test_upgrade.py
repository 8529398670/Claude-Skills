"""Run with: python3 -B -m unittest discover -s tests  (from the skill directory)

One test per merge rule, then the whole prepare/apply/rollback cycle on a real
git repository, then every skill commit through the renderer.
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


class Cycle(unittest.TestCase):
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
        (self.app / ".skill-origin.yaml").write_text('skill: go-webserver\ncommit: aaaa\ndirty: false\nproject_name: "Thing"\nslug: thing\nport: 21000\nfeatures: [api_keys]\n')
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

    def test_dirty_touched_file_refuses_but_unrelated_does_not(self):
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
