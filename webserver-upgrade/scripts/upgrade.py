#!/usr/bin/env python3
"""Bring template changes from the go-webserver and python-webserver skills
into apps that were scaffolded from them.

An update renders the template twice with the app's own name, port and
features: once at the skill commit the app is on (base) and once at the target
commit (new). Comparing base, new and the app, file by file, leaves only what
really changed upstream, already in the app's own names.

Commands:
    status   [--root DIR]                  which apps are behind, read-only
    baseline <app>                         the skill commit an unstamped app fits best
    stamp    <app> --commit SHA --features a,b
                                           write .skill-origin.yaml by hand
    offers   <app> [--to SHA]              the upgrades to ask about, read-only
    prepare  <app> [--to SHA] --skip a,b   merge into a staging directory
    add-feature <app> a,b                  stage a part the app was scaffolded without
    apply    <app>                         copy staging into the app
    render   <app> --out DIR [--commit SHA]
                                           the template as this app would get it
    rollback <app>                         undo the last apply, byte for byte

Fixes always land in code the app has. An upgrade, which is anything labelled as
one in the skill's UPGRADING.md, lands only if it is wanted: see Item and sort_offers.
A part cut out of the app (`pruned:` in its stamp) is cut out of both renders too, so
an update brings none of it. An app from before pruning has no such list, keeps all
its code, and goes on getting fixes in all of it. Nothing here ever removes a part.

It never commits, pushes, deploys or opens a database. Everything before
`apply` happens outside the app, so an unresolved conflict cannot reach a repo
whose push script runs `git add .`.

Only the standard library is used, because this has to run anywhere the skills
do. .skill-origin.yaml is flat, so a few lines parse it.
"""
from __future__ import annotations

import argparse
import dataclasses
import fnmatch
import hashlib
import importlib.util
import json
import os
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import time
from pathlib import Path

SKILLS_REPO = Path(__file__).resolve().parent.parent.parent
DEFAULT_ROOT = Path.home() / "WORKSPACE" / "PROJECTS"
DEFAULT_STATE = Path.home() / ".config" / "webserver-upgrade"
ORIGIN_FILE = ".skill-origin.yaml"

DEFAULTS = "config/defaults.yaml"
EXAMPLE = "config/config.example.yaml"
EXAMPLE_HEADER = "config/example_header.yaml"

# What belongs to the running app and is never read, merged or written. A copied
# secret.key would orphan the database it unlocks. Mirrors KEEP in demo.py.
# Older scaffolds kept config.yaml, with its key inside, at the repo root.
NEVER = ("config/config.yaml", "config/config.yaml.bak", "config/secret.key", "config.yaml", "config.yaml.bak", "secret.key", ORIGIN_FILE)
NEVER_DIRS = (".run", "data", "dist", ".venv", ".git")

# Replaced whole or left alone: a three-way merge of a minified file or a font
# means nothing. bootstrap.min.css is 232KB on five lines.
OPAQUE = ("static/vendor/*", "*.woff", "*.woff2", "*.ico", "*.png", "*.jpg", "*.gif", "*.min.css", "*.min.js")

# Where a data migration lives. A position in the list is the schema version,
# so a merge here always gets a human look even when it applies cleanly.
MIGRATIONS = ("server/db/migrate.go", "app/core/db.py")

PORT_LINES = (
    re.compile(r"^(\s*port:\s*)\d+[ \t]*$", re.MULTILINE),
    re.compile(r"^(\s*host_port:\s*)\d+[ \t]*$", re.MULTILINE),
)
# app.repo: scaffold.py writes the app's repository there, so a render takes
# whatever the app itself has, the way it takes the port.
REPO_LINE = re.compile(r'^(  repo:[ \t]*)"[^"\n]*"[ \t]*$', re.MULTILINE)
TOP_LEVEL_KEY = re.compile(r"^[a-z_]+:")
CONFLICT_LINE = re.compile(rb"^(<<<<<<<|\|\|\|\|\|\|\||>>>>>>>)( |$)", re.MULTILINE)
ITEM_LINE = re.compile(r"^\*\*(Upgrade|Plumbing) `([a-z0-9_-]+)`: *(.*?)\*\*\s*$")
ITEM_FIELD = re.compile(r"^(Needs|Marks|Files):")
QUOTED = re.compile(r"`([^`]+)`")


class Refused(Exception):
    """A reason to stop that the person should read, not a crash."""


@dataclasses.dataclass
class File:
    data: bytes
    mode: int = 0o644


@dataclasses.dataclass
class Identity:
    """Everything the two renders need to come out in the app's own names."""
    skill: str
    name: str
    slug: str
    module: str | None
    port: int | None
    host_port: int | None
    features: list[str]
    # What was cut out of this app when it was scaffolded. See prune.py.
    pruned: list[str] = dataclasses.field(default_factory=list)
    # app.repo as the app's defaults.yaml has it. None: no such line.
    repo: str | None = None


@dataclasses.dataclass
class Action:
    path: str
    kind: str  # add, update, delete, mode, conflict, or a notice: see NOTICES
    data: bytes | None = None
    mode: int | None = None
    note: str = ""


@dataclasses.dataclass
class Item:
    """One labelled entry in a skill's UPGRADING.md. Anything unlabelled is a fix
    and always lands. Plumbing is never offered: it comes with what needs it."""
    name: str
    title: str
    plumbing: bool = False
    needs: list[str] = dataclasses.field(default_factory=list)
    marks: list[str] = dataclasses.field(default_factory=list)
    files: list[str] = dataclasses.field(default_factory=list)


@dataclasses.dataclass
class Offers:
    ask: list[Item]  # new, and the app could use it
    off: list[tuple[Item, list[str]]]  # new, but a feature it needs is off
    plumbing: list[Item]  # new helpers, kept only if something taken needs them
    earlier: list[str]  # skipped by an earlier update, from the stamp


NOTICES = {
    "app-deleted": "the app removed this; upstream changed it; left out",
    "kept-modified": "upstream removed this but the app changed it; kept",
    "both-added": "the app and the template each added this; kept the app's",
    "opaque-kept": "upstream changed this binary or vendored file and so did the app; kept the app's",
    "deps-kept": "dependency file; kept the app's",
}
WRITES = ("add", "update", "delete", "mode", "conflict")


# ---------------------------------------------------------------- small helpers


def git(directory: Path, *arguments: str, stdin: bytes | None = None, check: bool = True) -> bytes:
    done = subprocess.run(["git", "-C", str(directory), *arguments], input=stdin, capture_output=True)
    if check and done.returncode != 0:
        raise Refused(f"git {' '.join(arguments)} failed in {directory}: {done.stderr.decode(errors='replace').strip()}")
    return done.stdout


def slugify(name: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", name.strip().lower()).strip("-") or "app"


def is_text(data: bytes) -> bool:
    if b"\0" in data:
        return False
    try:
        data.decode("utf-8")
    except UnicodeDecodeError:
        return False
    return True


def to_lf(data: bytes) -> bytes:
    return data.replace(b"\r\n", b"\n") if is_text(data) else data


def uses_crlf(data: bytes) -> bool:
    return is_text(data) and data.count(b"\r\n") > data.count(b"\n") / 2


def is_opaque(path: str) -> bool:
    return any(fnmatch.fnmatch(path, pattern) for pattern in OPAQUE)


def is_never(path: str, slug: str = "") -> bool:
    if path in NEVER or path in (slug, slug + ".exe"):
        return True
    return path.split("/")[0] in NEVER_DIRS


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


# ---------------------------------------------------------------- the origin file


def read_origin(app: Path) -> dict[str, str] | None:
    path = app / ORIGIN_FILE
    if not path.exists():
        return None
    values: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        match = re.match(r"^([a-z_]+):\s*(.*?)\s*$", line)
        if match:
            values[match.group(1)] = match.group(2).strip('"')
    return values


def origin_text(identity: Identity, commit: str) -> str:
    lines = [
        "# Written by the skill that scaffolded this app; `webserver-upgrade` reads and",
        "# updates it. Commit it: it is how an update knows where this app started.",
        f"skill: {identity.skill}",
        f"commit: {commit}",
        f'project_name: "{identity.name}"',
        f"slug: {identity.slug}",
    ]
    if identity.module:
        lines.append(f"module_path: {identity.module}")
    lines.append(f"port: {identity.port}")
    lines.append(f"features: [{', '.join(identity.features)}]")
    if identity.pruned:
        lines.append(f"pruned: [{', '.join(identity.pruned)}]")
    return "\n".join(lines) + "\n"


def parse_features(value: str) -> list[str]:
    return [name.strip() for name in value.strip("[] ").split(",") if name.strip()]


def parse_items(text: str) -> list[Item]:
    items: list[Item] = []
    current: Item | None = None
    field = ""
    fenced = False
    for line in text.splitlines():
        if line.startswith("```"):  # the format's own example is not an item
            fenced = not fenced
            current = None
        if fenced:
            continue
        match = ITEM_LINE.match(line)
        if match:
            current = Item(match.group(2), match.group(3).rstrip(". "), match.group(1) == "Plumbing")
            items.append(current)
            field = ""
            continue
        if current is None:
            continue
        if not line.strip():
            current = None
            continue
        start = ITEM_FIELD.match(line)
        if start:
            field = start.group(1).lower()
        if field:
            getattr(current, field).extend(QUOTED.findall(line))
    return items


def upgrade_items(commit: str, skill: str) -> list[Item]:
    """The labelled items in the skill's UPGRADING.md at this commit. A commit
    from before the labels has none, so everything in it counts as a fix."""
    text = git(SKILLS_REPO, "show", f"{commit}:{skill}/UPGRADING.md", check=False)
    return parse_items(text.decode("utf-8", errors="replace"))


def contains(files: dict[str, File], mark: str) -> bool:
    data = mark.encode()
    return any(data in item.data for item in files.values())


def sort_offers(items: list[Item], base: dict[str, File], new: dict[str, File], ours: dict[str, File], identity: Identity, table: dict[str, tuple], earlier: list[str]) -> Offers:
    """An item is new to this update if none of its marks are in the template the
    app is on and one is in the target, so an entry's place in UPGRADING.md does
    not matter. The app already has a whole feature its stamp lists, and anything
    with all its marks in the app's files: an app scaffolded from uncommitted
    work can be ahead of its stamped commit."""
    fresh = [item for item in items
             if item.name not in earlier
             and not (item.name in table and item.name in identity.features)
             and any(contains(new, mark) for mark in item.marks)
             and not any(contains(base, mark) for mark in item.marks)
             and not all(contains(ours, mark) for mark in item.marks)]
    names = {item.name for item in fresh}
    offers = Offers([], [], [], list(earlier))
    for item in fresh:
        missing = [need for need in item.needs if need in identity.pruned or (need in table and need not in identity.features and need not in names)]
        if item.plumbing:
            offers.plumbing.append(item)
        elif missing:
            offers.off.append((item, missing))
        else:
            offers.ask.append(item)
    return offers


def settle(offers: Offers, unwanted: set[str]) -> tuple[list[Item], list[Item]]:
    """What is taken and what is skipped. An upgrade that needs a skipped one is
    skipped with it, and plumbing stays only if something taken needs it."""
    skipped = unwanted | {item.name for item, _ in offers.off}
    changed = True
    while changed:
        changed = False
        for item in offers.ask:
            if item.name not in skipped and skipped & set(item.needs):
                skipped.add(item.name)
                changed = True
    taken = [item for item in offers.ask if item.name not in skipped]
    wanted = {need for item in taken for need in item.needs}
    changed = True
    while changed:
        changed = False
        for item in offers.plumbing:
            if item.name in wanted and item not in taken:
                taken.append(item)
                wanted |= set(item.needs)
                changed = True
    left = [item for item in offers.ask + [entry for entry, _ in offers.off] + offers.plumbing if item not in taken]
    return taken, left


def identify(app: Path, features: list[str] | None = None) -> Identity:
    """Work out what the app was scaffolded with, from the stamp when there is
    one and from the app's own files when there is not."""
    origin = read_origin(app) or {}
    defaults_path = app / DEFAULTS
    defaults = defaults_path.read_text(encoding="utf-8") if defaults_path.exists() else ""
    module = origin.get("module_path")
    go_mod = app / "go.mod"
    if go_mod.exists():
        found = re.search(r"^module\s+(\S+)", go_mod.read_text(encoding="utf-8"), re.MULTILINE)
        module = found.group(1) if found else module
    skill = origin.get("skill") or ("go-webserver" if go_mod.exists() else "python-webserver")
    name = origin.get("project_name")
    if not name:
        found = re.search(r"^app:\s*\n(?:\s*#.*\n)*\s+name:\s*\"?([^\"\n#]+?)\"?\s*(?:#.*)?$", defaults, re.MULTILINE)
        header = re.match(r"^#\s*(.+?)\s+--", defaults)
        if found:
            name = found.group(1)
        elif header:
            name = header.group(1)
        else:
            name = app.name
    port = re.search(PORT_LINES[0].pattern.replace(r"[ \t]*$", ""), defaults, re.MULTILINE)
    host_port = re.search(PORT_LINES[1].pattern.replace(r"[ \t]*$", ""), defaults, re.MULTILINE)

    def number(match: re.Match[str] | None) -> int | None:
        return int(match.group(0).split(":")[1]) if match else None

    if features is None:
        features = parse_features(origin.get("features", ""))
    repo = REPO_LINE.search(defaults)
    return Identity(skill, name, origin.get("slug") or slugify(name), module, number(port), number(host_port), features, parse_features(origin.get("pruned", "")),
                    repo.group(0).split('"')[1] if repo else None)


# ---------------------------------------------------------------- rendering


def export_commit(commit: str, skill: str, into: Path) -> Path:
    """Write the skill at this commit into a directory, byte for byte.

    Raw blobs rather than `git archive`, which applies .gitattributes and turns
    run.bat into CRLF on one side of the comparison only.
    """
    listing = git(SKILLS_REPO, "ls-tree", "-r", "-z", commit, "--", f"{skill}/scripts", f"{skill}/assets/template")
    entries = []
    for record in listing.split(b"\0"):
        if record:
            meta, path = record.split(b"\t", 1)
            mode, kind, blob = meta.decode().split()
            if kind == "blob":
                entries.append((mode, blob, path.decode()))
    if not entries:
        raise Refused(f"{skill} does not exist at {commit}")
    request = "".join(f"{blob}\n" for _, blob, _ in entries).encode()
    stream = git(SKILLS_REPO, "cat-file", "--batch", stdin=request)
    offset = 0
    for mode, _, path in entries:
        end = stream.index(b"\n", offset)
        size = int(stream[offset:end].split()[2])
        data = stream[end + 1:end + 1 + size]
        offset = end + 2 + size
        target = into / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
        target.chmod(0o755 if mode == "100755" else 0o644)
    return into / skill


def features_table(scaffold: Path) -> dict[str, tuple]:
    """FEATURES from a scaffold.py. Older ones have none."""
    spec = importlib.util.spec_from_file_location("scaffold_at_commit", scaffold)
    module = importlib.util.module_from_spec(spec)
    sys.dont_write_bytecode = True
    spec.loader.exec_module(module)
    return getattr(module, "FEATURES", {})


def set_switch(text: str, section: str, key: str, on: bool) -> str:
    """Set `key` directly under `section:`, keeping a trailing comment in its
    column. A file without the switch is returned as it was."""
    pattern = re.compile(rf"^(  {re.escape(key)}:\s*)(true|false)\b( *)")
    lines = text.split("\n")
    inside = False
    for index, line in enumerate(lines):
        if TOP_LEVEL_KEY.match(line):
            inside = line.startswith(f"{section}:")
            continue
        match = pattern.match(line) if inside else None
        if match:
            value = "true" if on else "false"
            padding = match.group(3)
            if padding:
                padding = " " * max(1, len(match.group(2)) + len(padding) - len(value))
            lines[index] = match.group(1) + value + padding + line[match.end():]
            break
    return "\n".join(lines)


def normalise(files: dict[str, File], identity: Identity, table: dict[str, tuple]) -> dict[str, File]:
    """Make a render comparable: nothing random, nothing secret, LF throughout."""
    result = {path: File(to_lf(item.data), item.mode) for path, item in files.items() if not is_never(path, identity.slug)}
    if DEFAULTS in result:
        text = result[DEFAULTS].data.decode("utf-8")
        for pattern, value in zip(PORT_LINES, (identity.port, identity.host_port)):
            if value is not None:
                text = pattern.sub(lambda match: f"{match.group(1)}{value}", text)
        if identity.repo is not None:
            text = REPO_LINE.sub(lambda match: f'{match.group(1)}"{identity.repo}"', text, count=1)
        for name, (section, key, *_) in table.items():
            text = set_switch(text, section, key, name in identity.features)
        result[DEFAULTS].data = text.encode("utf-8")
        if EXAMPLE_HEADER in result:
            result[EXAMPLE] = File(result[EXAMPLE_HEADER].data + result[DEFAULTS].data)
    return result


def render(commit: str, identity: Identity, table: dict[str, tuple] | None = None) -> tuple[dict[str, File], dict[str, tuple]]:
    """Run this commit's own scaffold.py and return what it produced, normalised,
    plus the feature table of that commit."""
    with tempfile.TemporaryDirectory(prefix="webserver-upgrade-") as scratch:
        root = Path(scratch)
        skill_dir = export_commit(commit, identity.skill, root / "skill")
        scaffold = skill_dir / "scripts" / "scaffold.py"
        arguments = [sys.executable, "-B", str(scaffold), str(root / "out"), identity.name]
        if identity.skill == "go-webserver" and identity.module:
            arguments.append(identity.module)
        # A commit from before pruning has nothing to cut, and takes no such flag.
        if "--prune" in scaffold.read_text(encoding="utf-8"):
            arguments += ["--prune", ",".join(identity.pruned) or "none"]
        elif identity.pruned:
            raise Refused(f"{commit} is older than pruning, so it cannot be rendered without {', '.join(identity.pruned)}.")
        done = subprocess.run(arguments, capture_output=True, cwd=root)
        if done.returncode != 0:
            raise Refused(f"scaffold.py at {commit} failed: {(done.stderr or done.stdout).decode(errors='replace').strip()}")
        own_table = features_table(scaffold)
        files: dict[str, File] = {}
        out = root / "out"
        for path in sorted(out.rglob("*")):
            if path.is_file() and not path.is_symlink():
                mode = 0o755 if path.stat().st_mode & stat.S_IXUSR else 0o644
                files[path.relative_to(out).as_posix()] = File(path.read_bytes(), mode)
    return normalise(files, identity, own_table if table is None else table), own_table


def skill_commits(skill: str) -> list[str]:
    """Commits that changed this skill's template, oldest first."""
    log = git(SKILLS_REPO, "log", "--reverse", "--format=%H", "--", f"{skill}/assets/template", f"{skill}/scripts/scaffold.py")
    return log.decode().split()


def resolve_commit(reference: str) -> str:
    return git(SKILLS_REPO, "rev-parse", "--verify", f"{reference}^{{commit}}").decode().strip()


def short(commit: str) -> str:
    return commit[:7]


def behind_count(old: str, new: str, skill: str) -> int:
    log = git(SKILLS_REPO, "rev-list", "--count", f"{old}..{new}", "--", f"{skill}/assets/template", f"{skill}/scripts/scaffold.py")
    return int(log.decode().strip() or 0)


# ---------------------------------------------------------------- reading an app


def read_app(app: Path, paths) -> dict[str, File]:
    files: dict[str, File] = {}
    for path in paths:
        full = app / path
        if full.is_file():
            mode = 0o755 if full.stat().st_mode & stat.S_IXUSR else 0o644
            files[path] = File(full.read_bytes(), mode)
    return files


def uncommitted_paths(app: Path) -> set[str]:
    output = git(app, "status", "--porcelain", "-z", "--untracked-files=all")
    paths: set[str] = set()
    records = output.split(b"\0")
    index = 0
    while index < len(records):
        record = records[index]
        index += 1
        if len(record) < 4:
            continue
        paths.add(record[3:].decode())
        if record[:2].strip().startswith((b"R", b"C")):
            paths.add(records[index].decode())
            index += 1
    return paths


# ---------------------------------------------------------------- merging


def merge_text(base: bytes, new: bytes, ours: bytes) -> tuple[bytes, bool]:
    """Three-way merge of LF text. Returns the result and whether it conflicted."""
    with tempfile.TemporaryDirectory(prefix="webserver-merge-") as scratch:
        root = Path(scratch)
        for name, data in (("ours", ours), ("base", base), ("new", new)):
            (root / name).write_bytes(data)
        done = subprocess.run(
            ["git", "merge-file", "-p", "--diff3", "-L", "app", "-L", "base", "-L", "template", "ours", "base", "new"],
            capture_output=True, cwd=root,
        )
    if done.returncode < 0 or done.returncode >= 128:
        raise Refused(f"git merge-file failed: {done.stderr.decode(errors='replace').strip()}")
    return done.stdout, done.returncode > 0


def version_key(version: str) -> tuple:
    match = re.match(r"v(\d+)\.(\d+)\.(\d+)(.*)", version)
    if not match:
        return (0, 0, 0, version)
    major, minor, patch, rest = match.groups()
    return (int(major), int(minor), int(patch), rest)


REQUIRE_LINE = re.compile(r"^(\s*(?:require\s+)?)(\S+)\s+(v\S+)(.*)$")


def merge_go_mod(ours: bytes, new: bytes) -> bytes:
    """Keep the app's go.mod and raise it to what the template needs: anything
    missing is added, nothing is lowered. `go mod tidy` settles the rest."""
    lines = ours.decode("utf-8").split("\n")
    have: dict[str, int] = {}
    block_end = None
    in_block = False
    for index, line in enumerate(lines):
        if line.startswith("require ("):
            in_block = True
            continue
        if in_block and line.startswith(")"):
            in_block = False
            block_end = block_end if block_end is not None else index
            continue
        match = REQUIRE_LINE.match(line)
        if match and (in_block or line.startswith("require ")):
            have[match.group(2)] = index
    wanted = new.decode("utf-8").split("\n")
    missing: list[str] = []
    in_block = False
    for line in wanted:
        if line.startswith("require ("):
            in_block = True
            continue
        if in_block and line.startswith(")"):
            in_block = False
            continue
        match = REQUIRE_LINE.match(line)
        if not match or not (in_block or line.startswith("require ")):
            go_line = re.match(r"^go (\S+)$", line)
            if go_line:
                for index, existing in enumerate(lines):
                    current = re.match(r"^go (\S+)$", existing)
                    if current and version_key("v" + current.group(1)) < version_key("v" + go_line.group(1)):
                        lines[index] = line
            continue
        module, version = match.group(2), match.group(3)
        if module in have:
            existing = REQUIRE_LINE.match(lines[have[module]])
            if version_key(existing.group(3)) < version_key(version):
                lines[have[module]] = f"{existing.group(1)}{module} {version}{existing.group(4)}"
        else:
            missing.append(f"\t{module} {version}{match.group(4)}")
    if missing:
        if block_end is None:
            lines += ["", "require ("] + missing + [")"]
        else:
            lines[block_end:block_end] = missing
    return "\n".join(lines).encode("utf-8")


def finished(data: bytes, ours: bytes | None, path: str) -> bytes:
    """Back to the line endings the file should have on disk."""
    wants_crlf = uses_crlf(ours) if ours is not None else path.endswith(".bat")
    if wants_crlf and is_text(data):
        return to_lf(data).replace(b"\n", b"\r\n")
    return data


def merged_mode(base: File | None, new: File, ours: File | None) -> int:
    if ours is None:
        return new.mode
    if base is None or base.mode == new.mode:
        return ours.mode
    return new.mode if ours.mode == base.mode else ours.mode


def plan_file(path: str, base: File | None, new: File | None, ours: File | None) -> Action | None:
    """What one file needs, by comparing base, new and the app. None means
    nothing: most files land here."""
    base_data = base.data if base else None
    new_data = new.data if new else None
    our_data = to_lf(ours.data) if ours else None

    if base_data == new_data:
        if new and base and ours and base.mode != new.mode:
            mode = merged_mode(base, new, ours)
            return Action(path, "mode", mode=mode) if mode != ours.mode else None
        return None

    if new is None:  # removed upstream
        if ours is None:
            return None
        if our_data == base_data:
            return Action(path, "delete")
        return Action(path, "kept-modified")

    if base is None:  # added upstream
        if ours is None:
            return Action(path, "add", finished(new_data, None, path), new.mode)
        if our_data == new_data:
            return None
        return Action(path, "both-added")

    if ours is None:
        return Action(path, "app-deleted")
    if our_data == new_data:
        return None

    mode = merged_mode(base, new, ours)
    if path == "go.mod":
        return Action(path, "update", merge_go_mod(ours.data, new_data), mode, "raised to what the template needs; run go mod tidy")
    if path == "go.sum":
        return Action(path, "deps-kept")
    if our_data == base_data:
        return Action(path, "update", finished(new_data, ours.data, path), mode)
    if is_opaque(path) or not is_text(new_data) or not is_text(our_data):
        return Action(path, "opaque-kept")
    merged, conflicted = merge_text(base_data, new_data, our_data)
    return Action(path, "conflict" if conflicted else "update", finished(merged, ours.data, path), mode)


def plan_merge(base: dict[str, File], new: dict[str, File], ours: dict[str, File]) -> list[Action]:
    actions = []
    for path in sorted(set(base) | set(new)):
        if path == EXAMPLE:  # generated from the merged defaults, in regenerate_example
            continue
        action = plan_file(path, base.get(path), new.get(path), ours.get(path))
        if action:
            if path in MIGRATIONS and action.kind in WRITES:
                action.note = (action.note + " " if action.note else "") + "migrations: check the order, the list position is the schema version"
            actions.append(action)
    return actions


def regenerate_example(actions: list[Action], ours: dict[str, File], new: dict[str, File]) -> None:
    """config.example.yaml is the header plus the defaults, so it is built from
    the merged result and never merged itself."""
    if DEFAULTS not in new or EXAMPLE_HEADER not in new or DEFAULTS not in ours:
        return
    staged = {action.path: action for action in actions}

    def current(path: str) -> bytes | None:
        action = staged.get(path)
        if action and action.data is not None:
            return action.data
        return ours[path].data if path in ours else None

    header, defaults = current(EXAMPLE_HEADER), current(DEFAULTS)
    if header is None or defaults is None or CONFLICT_LINE.search(defaults):
        return
    data = header + defaults
    if EXAMPLE not in ours or ours[EXAMPLE].data != data:
        kind = "update" if EXAMPLE in ours else "add"
        actions.append(Action(EXAMPLE, kind, data, 0o644, "regenerated from the merged defaults"))


def needs_table(commit: str, skill: str) -> dict[str, tuple[str, ...]]:
    """Everything that can be cut at this commit, and what each part needs."""
    with tempfile.TemporaryDirectory(prefix="webserver-upgrade-") as scratch:
        scaffold = export_commit(commit, skill, Path(scratch)) / "scripts" / "scaffold.py"
        spec = importlib.util.spec_from_file_location("scaffold_at_commit", scaffold)
        module = importlib.util.module_from_spec(spec)
        sys.dont_write_bytecode = True
        spec.loader.exec_module(module)
    table = {name: tuple(values[2]) for name, values in getattr(module, "FEATURES", {}).items()}
    table.update({name: tuple(needs) for name, needs in getattr(module, "PIECES", {}).items()})
    return table


def render_pair(identity: Identity, old: str, new_commit: str) -> tuple[dict[str, File], dict[str, File], dict[str, tuple]]:
    new_files, table = render(new_commit, identity)
    base_files, _ = render(old, identity, table)
    return base_files, new_files, table


def compute(app: Path, base_files: dict[str, File], new_files: dict[str, File], left_out: set[str] = frozenset()) -> tuple[list[Action], list[str]]:
    """The merge. `left_out` is the whole files of skipped upgrades: they are not
    added, and one the app is already without is not reported as removed."""
    ours = read_app(app, set(base_files) | set(new_files))
    actions = plan_merge(base_files, new_files, ours)
    actions = [action for action in actions if not (action.path in left_out and action.kind in ("add", "app-deleted"))]
    regenerate_example(actions, ours, new_files)
    added = [action.path for action in actions if action.kind == "add"]
    ignored = []
    if added:
        output = git(app, "check-ignore", "--stdin", stdin="\n".join(added).encode(), check=False)
        ignored = output.decode().split()
    return actions, ignored


def tally(actions: list[Action]) -> dict[str, int]:
    counts: dict[str, int] = {}
    for action in actions:
        counts[action.kind] = counts.get(action.kind, 0) + 1
    return counts


# ---------------------------------------------------------------- commands


def app_dir(value: str) -> Path:
    path = Path(value).expanduser().resolve()
    if not path.is_dir():
        candidate = DEFAULT_ROOT / value
        if candidate.is_dir():
            return candidate
        raise Refused(f"No such app: {value}")
    return path


def require_origin(app: Path) -> dict[str, str]:
    origin = read_origin(app)
    if not origin:
        raise Refused(f"{app.name} has no {ORIGIN_FILE}. Run `baseline` first, then `stamp`.")
    return origin


def files_of(items: list[Item], names) -> set[str]:
    return {path for item in items if item.name in names for path in item.files}


def looks_scaffolded(path: Path) -> bool:
    return (path / "scripts" / "login.sh").exists() or (path / "config" / "language.yaml").exists() or (path / "language.yaml").exists()


def command_status(arguments: argparse.Namespace) -> None:
    root = Path(arguments.root).expanduser()
    rows: list[tuple[str, ...]] = []
    unmanaged: list[str] = []
    skipped_notes: list[str] = []
    for path in sorted(root.iterdir()):
        if not path.is_dir():
            continue
        origin = read_origin(path)
        if origin is None:
            if looks_scaffolded(path):
                unmanaged.append(path.name)
            continue
        identity = identify(path)
        target = resolve_commit("HEAD")
        count = behind_count(origin["commit"], target, identity.skill)
        earlier = parse_features(origin.get("skipped", ""))
        if earlier:
            skipped_notes.append(f"{path.name}: {', '.join(earlier)}")
        if count == 0:
            rows.append((path.name, identity.skill, short(origin["commit"]), "up to date", ""))
            continue
        base, new, table = render_pair(identity, origin["commit"], target)
        items = upgrade_items(target, identity.skill)
        offers = sort_offers(items, base, new, read_app(path, new), identity, table, earlier)
        actions, ignored = compute(path, base, new, files_of(items, earlier))
        counts = tally(actions)
        detail = ", ".join(f"{value} {key}" for key, value in sorted(counts.items()))
        if ignored:
            detail += f"; {len(ignored)} new file(s) git would ignore"
        if offers.ask:
            detail += f"; {len(offers.ask)} upgrade(s) to choose from"
        rows.append((path.name, identity.skill, short(origin["commit"]), f"{count} commit(s) behind", detail))
    if rows:
        width = max(len(row[0]) for row in rows)
        for row in rows:
            print(f"{row[0]:<{width}}  {row[1]:<16}  {row[2]}  {row[3]}" + (f"  ({row[4]})" if row[4] else ""))
    else:
        print("No stamped apps found.")
    if skipped_notes:
        print("\nUpgrades left out:")
        for note in skipped_notes:
            print(f"  {note}")
    if unmanaged:
        print(f"\nUnmanaged, never touched: {', '.join(unmanaged)}")
    if git(SKILLS_REPO, "status", "--porcelain").strip():
        print("\nNote: the skills repo has uncommitted changes, and those are not included.")


def score(app: Path, identity: Identity, commit: str) -> tuple[int, int, int]:
    """How many of a commit's files the app has exactly, and of how many it
    could have. Only files it shares count: an app is not penalised for lacking
    what a commit had not grown yet, or for files it has dropped."""
    files, _ = render(commit, identity, {})
    ours = read_app(app, files)
    exact = sum(1 for path, item in files.items() if path in ours and to_lf(ours[path].data) == item.data)
    return exact, len(ours), len(files)


def command_baseline(arguments: argparse.Namespace) -> None:
    app = app_dir(arguments.app)
    identity = identify(app)
    print(f"{app.name}: {identity.skill}, name {identity.name!r}, slug {identity.slug}, module {identity.module}, port {identity.port}")
    results = []
    for commit in skill_commits(identity.skill):
        try:
            results.append((score(app, identity, commit), commit))
        except Refused as error:
            print(f"  {short(commit)} would not render: {error}")
    best = max(results, key=lambda item: item[0][0])
    for (exact, shared, total), commit in results:
        marker = "  <- best" if commit == best[1] else ""
        print(f"  {short(commit)}  {exact:>3} exact of {shared:>3} shared ({total} in template){marker}")
    (exact, shared, _), commit = best
    strength = exact / shared if shared else 0
    print(f"Best fit: {commit} ({strength:.0%} of shared files exact)")
    if strength < 0.25:
        print("That is too weak to merge from. Treat this app as unmanaged.")


def command_stamp(arguments: argparse.Namespace) -> None:
    app = app_dir(arguments.app)
    features = parse_features(arguments.features) if arguments.features != "none" else []
    identity = identify(app, features)
    commit = resolve_commit(arguments.commit)
    (app / ORIGIN_FILE).write_text(origin_text(identity, commit), encoding="utf-8")
    print(f"Wrote {app / ORIGIN_FILE}: {short(commit)}, features {features or '(none)'}")


def state_dir(arguments: argparse.Namespace, identity: Identity) -> Path:
    return Path(arguments.state).expanduser() / identity.slug


def pending_journal(state: Path) -> Path | None:
    journals = sorted((state / "journal").glob("*")) if (state / "journal").exists() else []
    return journals[-1] if journals else None


def refuse_if_unsettled(app: Path, state: Path) -> None:
    """An apply that is still uncommitted has to be committed or rolled back
    first, or a second one would stack on it and neither could be undone."""
    journal = pending_journal(state)
    if not journal:
        return
    recorded = json.loads((journal / "journal.json").read_text(encoding="utf-8"))
    touched = set(recorded["written"]) | set(recorded["created"]) | set(recorded["deleted"])
    if touched & uncommitted_paths(app):
        raise Refused(f"The last apply to {app.name} is not committed yet. Commit it, or run `rollback`.")
    shutil.rmtree(journal)


def stage(app: Path, state: Path, old: str, target: str, actions: list[Action], ignored: list[str], skipped: list[str], identity: Identity, marks: dict[str, list[str]]) -> tuple[Path, list[dict]]:
    """Write what apply needs. Nothing here touches the app."""
    staging = state / "staging"
    shutil.rmtree(staging, ignore_errors=True)
    (staging / "files").mkdir(parents=True)
    records = []
    for action in actions:
        if action.data is not None:
            target_path = staging / "files" / action.path
            target_path.parent.mkdir(parents=True, exist_ok=True)
            target_path.write_bytes(action.data)
        records.append({"path": action.path, "kind": action.kind, "mode": action.mode, "note": action.note})
    head = git(app, "rev-parse", "HEAD").decode().strip()
    plan = {
        "app": str(app), "from": old, "to": target, "head": head, "ignored": ignored, "actions": records,
        "skipped": skipped, "features": identity.features, "pruned": identity.pruned, "marks": marks,
    }
    (staging / "plan.json").write_text(json.dumps(plan, indent=2) + "\n", encoding="utf-8")
    return staging, records


def report(app: Path, old: str, target: str, actions: list[Action], ignored: list[str]) -> None:
    counts = tally(actions)
    print(f"{app.name}: {short(old)} -> {short(target)}")
    for kind, count in sorted(counts.items()):
        print(f"  {count:>4} {kind}")
    conflicts = [action.path for action in actions if action.kind == "conflict"]
    if conflicts:
        print("\nConflicts to resolve in place, under the staging files directory:")
        for path in conflicts:
            print(f"  {path}")
    for action in actions:
        if action.kind in NOTICES:
            print(f"  note: {action.path}: {NOTICES[action.kind]}")
        elif action.note:
            print(f"  note: {action.path}: {action.note}")
    for path in ignored:
        print(f"  warning: {path} is new and git would ignore it, so it would never be committed")


def command_add_feature(arguments: argparse.Namespace) -> None:
    """Stage a part the app was scaffolded without, at the commit it is on: two
    renders that differ only by that part, merged like any update. A switch comes
    on with it. Anything it needs comes too."""
    app = app_dir(arguments.app)
    origin = require_origin(app)
    identity = identify(app)
    state = state_dir(arguments, identity)
    refuse_if_unsettled(app, state)
    commit = origin["commit"]
    wanted = set(parse_features(arguments.features))
    extra = wanted - set(identity.pruned)
    if extra:
        raise Refused(f"{app.name} was not scaffolded without {', '.join(sorted(extra))}, so there is nothing to add.")
    table = needs_table(commit, identity.skill)
    adding: set[str] = set()
    waiting = list(wanted)
    while waiting:
        name = waiting.pop()
        if name in adding or name not in identity.pruned:
            continue
        adding.add(name)
        waiting.extend(table.get(name, ()))
    base, _ = render(commit, identity)
    _, switches = render(commit, dataclasses.replace(identity, pruned=[]))
    after = dataclasses.replace(
        identity,
        features=identity.features + sorted(name for name in adding if name in switches),
        pruned=[name for name in identity.pruned if name not in adding],
    )
    new, _ = render(commit, after)
    actions, ignored = compute(app, base, new)
    uncommitted = sorted(uncommitted_paths(app) & {action.path for action in actions if action.kind in WRITES})
    if uncommitted:
        raise Refused(f"{app.name} has uncommitted changes in files this touches: {', '.join(uncommitted)}")
    staging, _ = stage(app, state, commit, commit, actions, ignored, parse_features(origin.get("skipped", "")), after, {})
    report(app, commit, commit, actions, ignored)
    print(f"\nAdding: {', '.join(sorted(adding))}")
    print(f"Staged in {staging}")


def command_prepare(arguments: argparse.Namespace) -> None:
    app = app_dir(arguments.app)
    origin = require_origin(app)
    identity = identify(app)
    state = state_dir(arguments, identity)
    refuse_if_unsettled(app, state)
    target = resolve_commit(arguments.to)
    if origin["commit"] == target:
        print(f"{app.name} is already at {short(target)}.")
        return
    earlier = parse_features(origin.get("skipped", ""))
    items = upgrade_items(target, identity.skill)
    base, new, table = render_pair(identity, origin["commit"], target)
    offers = sort_offers(items, base, new, read_app(app, new), identity, table, earlier)
    if offers.ask and arguments.skip is None:
        names = ", ".join(item.name for item in offers.ask)
        raise Refused(f"Upgrades need a decision first: {names}. Run `offers`, ask, then pass --skip a,b or --skip none.")
    unwanted = set(parse_features(arguments.skip or "")) - {"none"}
    known = {item.name for item in offers.ask} | {item.name for item, _ in offers.off}
    if unwanted - known:
        raise Refused(f"Not an upgrade in this update: {', '.join(sorted(unwanted - known))}")
    taken, left = settle(offers, unwanted)
    switched_on = [item.name for item in taken if item.name in table]
    if switched_on:  # a whole feature they chose comes on, as it would in a new app
        identity = dataclasses.replace(identity, features=identity.features + switched_on)
        base, new, table = render_pair(identity, origin["commit"], target)
    skipped = sorted(set(earlier) | {item.name for item in left})
    actions, ignored = compute(app, base, new, files_of(items, skipped))
    uncommitted = sorted(uncommitted_paths(app) & {action.path for action in actions if action.kind in WRITES})
    if uncommitted:
        raise Refused(f"{app.name} has uncommitted changes in files this update touches: {', '.join(uncommitted)}")

    staging, records = stage(app, state, origin["commit"], target, actions, ignored, skipped, identity, {item.name: item.marks for item in left})
    report(app, origin["commit"], target, actions, ignored)
    if taken:
        print(f"\nTaken: {', '.join(item.name for item in taken)}")
    if left:
        print("\nLeft out. Remove these from the staged files, along with the rest of each upgrade:")
        for item in left:
            hits = leftovers(staging / "files", {item.name: item.marks}, records, app)
            print(f"  {item.name}: {item.title}")
            for path, mark in hits:
                print(f"    {path}: {mark}")
    print(f"\nStaged in {staging}")


def leftovers(files: Path, marks: dict[str, list[str]], records: list[dict], app: Path) -> list[tuple[str, str]]:
    """Marks of left-out upgrades in staged files, unless the app's own copy
    already had that text, which makes it the app's and not the template's."""
    found = []
    for record in records:
        staged = files / record["path"]
        if record["kind"] not in WRITES or not staged.exists():
            continue
        data = staged.read_bytes()
        own = app / record["path"]
        ours = own.read_bytes() if own.exists() else b""
        for mark in (mark for group in marks.values() for mark in group):
            if mark.encode() in data and mark.encode() not in ours:
                found.append((record["path"], mark))
    return found


def command_offers(arguments: argparse.Namespace) -> None:
    app = app_dir(arguments.app)
    origin = require_origin(app)
    identity = identify(app)
    target = resolve_commit(arguments.to)
    if origin["commit"] == target:
        print(f"{app.name} is already at {short(target)}.")
        return
    earlier = parse_features(origin.get("skipped", ""))
    base, new, table = render_pair(identity, origin["commit"], target)
    offers = sort_offers(upgrade_items(target, identity.skill), base, new, read_app(app, new), identity, table, earlier)
    print(f"{app.name}: {short(origin['commit'])} -> {short(target)}")
    print("\nAsk about:" if offers.ask else "\nNothing to ask about.")
    for item in offers.ask:
        print(f"  {item.name}: {item.title}")
    if offers.off:
        print("\nLeft out without asking, because a feature it needs is off:")
        for item, missing in offers.off:
            print(f"  {item.name}: {item.title} (needs {', '.join(missing)})")
    if offers.earlier:
        print(f"\nLeft out by an earlier update: {', '.join(offers.earlier)}")


def command_render(arguments: argparse.Namespace) -> None:
    app = app_dir(arguments.app)
    origin = require_origin(app)
    out = Path(arguments.out).expanduser().resolve()
    if out.is_relative_to(app) or (out.exists() and any(out.iterdir())):
        raise Refused(f"{out} must be empty and outside the app.")
    commit = resolve_commit(arguments.commit) if arguments.commit else origin["commit"]
    files, _ = render(commit, identify(app))
    for path, item in files.items():
        target = out / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(item.data)
    print(f"Wrote {len(files)} file(s) to {out}, the template at {short(commit)}.")


def command_apply(arguments: argparse.Namespace) -> None:
    app = app_dir(arguments.app)
    identity = identify(app)
    state = state_dir(arguments, identity)
    refuse_if_unsettled(app, state)
    staging = state / "staging"
    if not (staging / "plan.json").exists():
        raise Refused(f"Nothing staged for {app.name}. Run `prepare` first.")
    plan = json.loads((staging / "plan.json").read_text(encoding="utf-8"))
    if git(app, "rev-parse", "HEAD").decode().strip() != plan["head"]:
        raise Refused(f"{app.name} has new commits since `prepare`. Run `prepare` again.")
    writes = [record for record in plan["actions"] if record["kind"] in WRITES]
    uncommitted = sorted(uncommitted_paths(app) & {record["path"] for record in writes})
    if uncommitted:
        raise Refused(f"{app.name} changed since `prepare`: {', '.join(uncommitted)}")
    unresolved = []
    for record in writes:
        staged = staging / "files" / record["path"]
        if staged.exists() and CONFLICT_LINE.search(staged.read_bytes()):
            unresolved.append(record["path"])
    if unresolved:
        raise Refused("Conflict markers remain in: " + ", ".join(unresolved))
    remaining = leftovers(staging / "files", plan.get("marks", {}), plan["actions"], app)
    if remaining:
        listed = ", ".join(f"{mark} in {path}" for path, mark in remaining)
        raise Refused(f"Left-out upgrades are still in the staged files: {listed}")

    journal = state / "journal" / time.strftime("%Y%m%d-%H%M%S")
    (journal / "files").mkdir(parents=True)
    recorded = {"from": plan["from"], "to": plan["to"], "written": [], "created": [], "deleted": [], "modes": {}, "after": {}}

    def keep(path: str) -> None:
        full = app / path
        copy = journal / "files" / path
        copy.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(full, copy)
        recorded["modes"][path] = stat.S_IMODE(full.stat().st_mode)

    for record in writes:
        path, kind = record["path"], record["kind"]
        full = app / path
        if kind == "delete":
            keep(path)
            recorded["deleted"].append(path)
            full.unlink()
            continue
        if full.exists():
            keep(path)
            recorded["written"].append(path)
        else:
            recorded["created"].append(path)
            full.parent.mkdir(parents=True, exist_ok=True)
        staged = staging / "files" / path
        if staged.exists():
            full.write_bytes(staged.read_bytes())
        if record["mode"] is not None:
            full.chmod(record["mode"])
        recorded["after"][path] = sha256(full.read_bytes())

    origin_path = app / ORIGIN_FILE
    keep(ORIGIN_FILE)
    recorded["written"].append(ORIGIN_FILE)
    text = origin_path.read_text(encoding="utf-8")
    text = re.sub(r"^commit: .*$", f"commit: {plan['to']}", text, flags=re.MULTILINE)
    text = re.sub(r"^features: .*$", f"features: [{', '.join(plan['features'])}]", text, flags=re.MULTILINE)
    text = re.sub(r"^pruned: .*\n", "", text, flags=re.MULTILINE)
    if plan.get("pruned"):
        text = text.rstrip("\n") + f"\npruned: [{', '.join(plan['pruned'])}]\n"
    text = re.sub(r"^skipped: .*\n", "", text, flags=re.MULTILINE)
    if plan["skipped"]:
        text = text.rstrip("\n") + f"\nskipped: [{', '.join(plan['skipped'])}]\n"
    origin_path.write_text(text, encoding="utf-8")
    recorded["after"][ORIGIN_FILE] = sha256(origin_path.read_bytes())

    (journal / "journal.json").write_text(json.dumps(recorded, indent=2) + "\n", encoding="utf-8")
    shutil.rmtree(staging)
    print(f"Applied {len(writes)} change(s) to {app.name}, now at {short(plan['to'])}. Nothing is committed.")
    print(f"Undo with `rollback`. Journal: {journal}")
    if (app / "go.mod").exists():
        print("Next: go mod tidy, go build ./..., go test ./...")


def command_rollback(arguments: argparse.Namespace) -> None:
    app = app_dir(arguments.app)
    identity = identify(app)
    state = state_dir(arguments, identity)
    journal = pending_journal(state)
    if not journal:
        raise Refused(f"No apply to undo for {app.name}.")
    recorded = json.loads((journal / "journal.json").read_text(encoding="utf-8"))
    changed = [
        path for path, digest in recorded["after"].items()
        if (app / path).exists() and sha256((app / path).read_bytes()) != digest
    ]
    if changed and not arguments.force:
        raise Refused("These changed after the apply, and rollback would lose that work: " + ", ".join(changed) + ". Use --force to lose it.")
    for path in recorded["created"]:
        (app / path).unlink(missing_ok=True)
        parent = (app / path).parent
        while parent != app and not any(parent.iterdir()):
            parent.rmdir()
            parent = parent.parent
    for path in recorded["written"] + recorded["deleted"]:
        full = app / path
        full.parent.mkdir(parents=True, exist_ok=True)
        full.write_bytes((journal / "files" / path).read_bytes())
        full.chmod(recorded["modes"][path])
    shutil.rmtree(journal)
    print(f"Rolled {app.name} back to {short(recorded['from'])}.")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--state", default=str(DEFAULT_STATE), help="where staging and journals live")
    commands = parser.add_subparsers(dest="command", required=True)

    status = commands.add_parser("status")
    status.add_argument("--root", default=str(DEFAULT_ROOT))
    status.set_defaults(run=command_status)

    baseline = commands.add_parser("baseline")
    baseline.add_argument("app")
    baseline.set_defaults(run=command_baseline)

    stamp = commands.add_parser("stamp")
    stamp.add_argument("app")
    stamp.add_argument("--commit", required=True)
    stamp.add_argument("--features", required=True, help="comma-separated, or none")
    stamp.set_defaults(run=command_stamp)

    prepare = commands.add_parser("prepare")
    prepare.add_argument("app")
    prepare.add_argument("--to", default="HEAD")
    prepare.add_argument("--skip", help="upgrades to leave out, comma-separated, or none")
    prepare.set_defaults(run=command_prepare)

    add_feature = commands.add_parser("add-feature")
    add_feature.add_argument("app")
    add_feature.add_argument("features", help="what to add, comma-separated")
    add_feature.set_defaults(run=command_add_feature)

    offers = commands.add_parser("offers")
    offers.add_argument("app")
    offers.add_argument("--to", default="HEAD")
    offers.set_defaults(run=command_offers)

    rendered = commands.add_parser("render")
    rendered.add_argument("app")
    rendered.add_argument("--out", required=True)
    rendered.add_argument("--commit", help="defaults to the commit the app is on")
    rendered.set_defaults(run=command_render)

    apply = commands.add_parser("apply")
    apply.add_argument("app")
    apply.set_defaults(run=command_apply)

    rollback = commands.add_parser("rollback")
    rollback.add_argument("app")
    rollback.add_argument("--force", action="store_true")
    rollback.set_defaults(run=command_rollback)

    arguments = parser.parse_args()
    try:
        arguments.run(arguments)
    except Refused as error:
        print(f"Refused: {error}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
