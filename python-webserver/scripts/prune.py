#!/usr/bin/env python3
"""Cut the features an app does not use out of a copy of the template.

scaffold.py runs this on every new project, and webserver-upgrade reaches it
through scaffold.py, so an app and the renders it is compared against are cut
the same way. The go-webserver and python-webserver copies of this file are
identical, and a test holds them to it.

The markers live in the template, beside the code they cover. An expression is a
feature, `!feature` for code only a project without it needs, or `a|b` for either.
Every marker is removed from every project, used or not.

A whole file, in any of its first ten lines, in that file's comment syntax:

    // whole-file feature:webhooks

A block, in any comment syntax. The marker lines go too:

    // >>> feature:webhooks
    ...
    // <<< feature:webhooks

One line, as a comment at its end:

    webhooks "app/server/webhooks"  // feature:webhooks

config/api.json has no comments, so a path carries a key instead, which
the API docs' generator writes from the route's Feature (Go) or feature (Python):

    "x-feature": "webhooks"

A field of a reply or request carries the same key, from its own tag, so a
schema loses the fields of a removed feature (Go, then Python):

    Passwords int `json:"passwords" jsonschema_extras:"x-feature=passwords"` // feature:passwords
    passwords: Annotated[int, Meta(extra_json_schema={"x-feature": "passwords"})]  # feature:passwords

config/mcp.json is written from the registered tools, so it names no features:
each tool and http_only entry goes with the api.json path its route is on.
config/api.md and config/mcp.md are rendered again from those two, with the
python-webserver template's app/apidocs/markdown.py, which writes the same bytes
as Go's server/apidoc/markdown.go. So a fresh project's docs match what its
scripts/api_docs.sh would write, without running anything.

Only the standard library is used, because this has to run wherever the skills do.
"""
from __future__ import annotations

import json
import re
import shutil
import types
from pathlib import Path

# What prune_tree reads. Anything else is left alone.
SUFFIXES = {".go", ".py", ".js", ".css", ".html", ".yaml", ".yml", ".sh", ".bat", ".md", ".txt"}
NAMES = {"Dockerfile"}
OPENAPI = "config/api.json"
MCP_JSON = "config/mcp.json"
API_MD = "config/api.md"
MCP_MD = "config/mcp.md"

EXPRESSION = r"!?[a-z0-9_]+(?:\|!?[a-z0-9_]+)*"
WHOLE_FILE = re.compile(r"whole-file feature:(" + EXPRESSION + r")")
BLOCK_START = re.compile(r">>> feature:(" + EXPRESSION + r")")
BLOCK_END = re.compile(r"<<< feature:(" + EXPRESSION + r")")
# The comment that ends a line: // # /* */ or <!-- -->, holding only the tag.
LINE_TAG = re.compile(r"[ \t]*(?://|#|/\*|<!--)[ \t]*feature:(" + EXPRESSION + r")[ \t]*(?:\*/|-->)?[ \t]*$")
WHOLE_FILE_LINES = 10


class MarkerError(Exception):
    """A marker that cannot be right. It stops the scaffold rather than leaving
    half a feature behind."""


def keeps(expression: str, removed: set[str]) -> bool:
    """Whether code under this expression stays, with these features removed."""
    for part in expression.split("|"):
        name = part.lstrip("!")
        if (name not in removed) != part.startswith("!"):
            return True
    return False


def names_in(expression: str) -> set[str]:
    return {part.lstrip("!") for part in expression.split("|")}


def file_expression(text: str) -> str | None:
    """The whole-file marker's expression, if this file has one."""
    for line in text.split("\n")[:WHOLE_FILE_LINES]:
        found = WHOLE_FILE.search(line)
        if found:
            return found.group(1)
    return None


def prune_text(text: str, removed: set[str], where: str = "") -> str:
    """text with the blocks and lines of removed features cut, and every marker
    gone. Line endings are kept, because a .bat file needs its CRLF."""
    kept: list[str] = []
    open_blocks: list[tuple[str, bool]] = []
    for number, line in enumerate(text.splitlines(keepends=True), 1):
        body = line.rstrip("\r\n")
        ending = line[len(body):]
        if WHOLE_FILE.search(body):
            continue
        start = BLOCK_START.search(body)
        end = BLOCK_END.search(body)
        if start:
            open_blocks.append((start.group(1), keeps(start.group(1), removed)))
            continue
        if end:
            if not open_blocks or open_blocks[-1][0] != end.group(1):
                raise MarkerError(f"{where}:{number}: {end.group(0)} does not close the open block")
            open_blocks.pop()
            continue
        if not all(keep for _, keep in open_blocks):
            continue
        tag = LINE_TAG.search(body)
        if tag:
            if keeps(tag.group(1), removed):
                kept.append(body[:tag.start()] + ending)
            continue
        kept.append(line)
    if open_blocks:
        raise MarkerError(f"{where}: {open_blocks[-1][0]} is never closed")
    return "".join(kept)


def prune_spec(text: str, removed: set[str]) -> str:
    """api.json without the paths of removed features, the schemas nothing
    refers to any more, or tags nothing uses. Written the way the app's own
    generator writes it, so a project with every part matches it exactly."""
    spec = json.loads(text)
    paths = spec.get("paths", {})
    for name in list(paths):
        expression = paths[name].get("x-feature")
        if expression is not None and not keeps(expression, removed):
            del paths[name]
    prune_fields(spec, removed)
    schemas = spec.get("components", {}).get("schemas", {})
    while True:
        rest = {key: value for key, value in spec.items() if key != "components"}
        rest["responses"] = spec.get("components", {}).get("responses", {})
        used = set(re.findall(r'"#/components/schemas/([^"]+)"', json.dumps([rest, schemas])))
        unused = [name for name in schemas if name not in used]
        if not unused:
            break
        for name in unused:
            del schemas[name]
    used_tags = {tag for item in paths.values() for operation in item.values() if isinstance(operation, dict) for tag in operation.get("tags", [])}
    if "tags" in spec:
        spec["tags"] = [tag for tag in spec["tags"] if tag["name"] in used_tags]
    return json.dumps(spec, indent=2, ensure_ascii=False, sort_keys=True) + "\n"


def prune_fields(node: object, removed: set[str]) -> None:
    """Take the properties whose x-feature is removed out of every schema in
    node, and out of its required list. A list or properties this empties goes, as
    the generators leave them out."""
    if isinstance(node, list):
        for item in node:
            prune_fields(item, removed)
        return
    if not isinstance(node, dict):
        return
    properties = node.get("properties")
    if isinstance(properties, dict):
        gone = {name for name, value in properties.items() if isinstance(value, dict) and isinstance(value.get("x-feature"), str) and not keeps(value["x-feature"], removed)}
        for name in gone:
            del properties[name]
        if gone and isinstance(node.get("required"), list):
            node["required"] = [name for name in node["required"] if name not in gone]
            if not node["required"]:
                del node["required"]
        if gone and not properties:
            del node["properties"]
    for value in node.values():
        prune_fields(value, removed)


def route_features(spec_text: str) -> dict[str, str]:
    """"METHOD /path" -> its path's x-feature, for every operation that has one."""
    found: dict[str, str] = {}
    for path, item in json.loads(spec_text).get("paths", {}).items():
        expression = item.get("x-feature")
        if expression is None:
            continue
        for method in item:
            if not method.startswith("x-"):
                found[f"{method.upper()} {path}"] = expression
    return found


def prune_mcp(text: str, removed: set[str], features: dict[str, str]) -> str:
    """mcp.json without the tools and http_only routes of removed features. Written
    the way the app's own generator writes it, so a fresh project's test passes."""
    manifest = json.loads(text)
    for key in ("tools", "http_only"):
        kept = []
        for entry in manifest.get(key, []):
            expression = features.get(entry.get("route", ""))
            if expression is None or keeps(expression, removed):
                kept.append(entry)
        manifest[key] = kept
    return json.dumps(manifest, indent=2, ensure_ascii=False) + "\n"


# What renders api.md and mcp.md. A Python project has its own; a Go project
# borrows the python-webserver template's, which writes the same bytes.
OWN_RENDERER = "app/apidocs/markdown.py"
SHARED_RENDERER = Path(__file__).resolve().parent.parent.parent / "python-webserver" / "assets" / "template" / OWN_RENDERER


def renderer(root: Path, removed: set[str]) -> types.ModuleType:
    """The markdown module, loaded from its file rather than imported, so
    nothing of the project runs. It needs only the standard library."""
    path = root / OWN_RENDERER
    if not path.is_file():
        path = SHARED_RENDERER
    if not path.is_file():
        raise MarkerError(f"{SHARED_RENDERER} is missing: it renders config/api.md and config/mcp.md")
    module = types.ModuleType("apidoc_markdown")
    module.__file__ = str(path)
    exec(compile(prune_text(path.read_text(encoding="utf-8"), removed, str(path)), str(path), "exec"), module.__dict__)
    return module


def render_docs(root: Path, removed: set[str], spec_text: str) -> None:
    """Write config/api.md and config/mcp.md from the pruned api.json and
    mcp.json, where the project has them. spec_text is api.json as it was
    copied: a project without openapi has no api.json, but api.md still
    describes its routes."""
    mcp_path = root / MCP_JSON
    manifest = json.loads(mcp_path.read_text(encoding="utf-8")) if mcp_path.is_file() else None
    api_md = (root / API_MD).is_file()
    mcp_md = manifest is not None and (root / MCP_MD).is_file()
    if not api_md and not mcp_md:
        return
    spec = json.loads(prune_spec(spec_text, removed))
    markdown = renderer(root, removed)
    if api_md:
        (root / API_MD).write_text(markdown.render(spec, manifest), encoding="utf-8", newline="")
    if mcp_md:
        (root / MCP_MD).write_text(markdown.render_mcp(spec, manifest), encoding="utf-8", newline="")


def markers_in(text: str) -> set[str]:
    """Every feature this text names in a marker."""
    found: set[str] = set()
    for pattern in (WHOLE_FILE, BLOCK_START, BLOCK_END, LINE_TAG):
        for line in text.split("\n"):
            match = pattern.search(line)
            if match:
                found |= names_in(match.group(1))
    return found


def prunable(path: Path) -> bool:
    return path.suffix in SUFFIXES or path.name in NAMES


def prune_tree(root: Path, removed: set[str], data_files: dict[str, tuple[str, ...]] | None = None) -> None:
    """Cut removed features out of the project at root, in place. It runs with an
    empty set too: a project with everything still loses the markers. data_files
    are the files that cannot carry a marker, by feature expression (`a|b` goes
    only when both do)."""
    # Read before anything is cut: mcp.json's tools go by the x-feature of
    # their route, which pruning takes out of api.json.
    spec_path = root / OPENAPI
    spec_text = spec_path.read_text(encoding="utf-8") if spec_path.is_file() else None
    features = route_features(spec_text) if spec_text is not None else {}
    for expression, paths in (data_files or {}).items():
        if not keeps(expression, removed):
            for relative in paths:
                (root / relative).unlink(missing_ok=True)
    for path in sorted(root.rglob("*")):
        if not path.is_file() or path.is_symlink():
            continue
        relative = path.relative_to(root).as_posix()
        if relative == OPENAPI:
            path.write_text(prune_spec(path.read_text(encoding="utf-8"), removed), encoding="utf-8", newline="")
            continue
        if relative == MCP_JSON:
            path.write_text(prune_mcp(path.read_text(encoding="utf-8"), removed, features), encoding="utf-8", newline="")
            continue
        if relative in (API_MD, MCP_MD):
            continue
        if not prunable(path):
            continue
        try:
            text = path.read_bytes().decode("utf-8")
        except UnicodeDecodeError:
            continue
        expression = file_expression(text)
        if expression is not None and not keeps(expression, removed):
            path.unlink()
            continue
        pruned = prune_text(text, removed, relative)
        if pruned != text:
            path.write_bytes(pruned.encode("utf-8"))
    if spec_text is not None:
        render_docs(root, removed, spec_text)
    # A package or folder that was all one feature goes with it.
    for path in sorted(root.rglob("*"), reverse=True):
        if path.is_dir() and not any(path.iterdir()):
            path.rmdir()


# ---------------------------------------------------------------- tests on or off
#
# An app is scaffolded without most of its tests unless asked for them (--tests,
# or `tests: true` in the skills repo's config.yaml). It always keeps the
# security and sign-in tests, the ones that catch an upgrade or an edit
# quietly breaking audited code, plus what scripts/api_docs.sh (Go) runs and the
# helpers all of those need.
KEEP_WITHOUT_TESTS = {
    # Go: API docs, and the fixtures the route tests share.
    "server/routes/api_docs_test.go",
    "server/routes/fixture_test.go",
    "server/routes/mcp_parity_test.go",
    "server/routes/monitoring_test.go",
    # Go: sign-in, sessions, roles, keys, limits, secrets.
    "server/routes/sessions_test.go",
    "server/routes/login_links_test.go",
    "server/routes/invite_links_test.go",
    "server/routes/two_factor_test.go",
    "server/routes/passwords_test.go",
    "server/routes/admin_test.go",
    "server/routes/api_keys_test.go",
    "server/middleware/ratelimit_test.go",
    "server/models/login_token_test.go",
    "server/models/invite_link_test.go",
    "server/models/attempts_test.go",
    "server/models/helpers_test.go",
    "server/config/secret_test.go",
    "server/config/helpers_test.go",
    "server/config/public_url_test.go",
    "server/config/scripts_test.go",
    "server/routes/no_store_test.go",
    "server/middleware/no_store_test.go",
    "server/db/sealing_test.go",
    "server/encryption/subkey_test.go",
    "server/models/reseal_test.go",
    # Python: the same, and the shared support modules.
    "app/tests/__init__.py",
    "app/tests/support.py",
    "app/tests/route_support.py",
    "app/tests/test_sessions.py",
    "app/tests/test_login_tokens.py",
    "app/tests/test_invite_links.py",
    "app/tests/test_passwords.py",
    "app/tests/test_attempts.py",
    "app/tests/test_rate_limit.py",
    "app/tests/test_api_keys.py",
    "app/tests/test_admin_users.py",
    "app/tests/test_config_secret.py",
    "app/tests/test_public_url.py",
    "app/tests/test_scripts.py",
    "app/tests/test_no_store.py",
    "app/tests/test_sealing.py",
}
PYTHON_TESTS = "app/tests"
SKILLS_CONFIG = Path(__file__).resolve().parent.parent.parent / "config.yaml"


def tests_by_default(config: Path = SKILLS_CONFIG) -> bool:
    """`tests:` in the skills repo's config.yaml. No file or no key is off."""
    try:
        lines = config.read_text(encoding="utf-8").splitlines()
    except OSError:
        return False
    for line in lines:
        key, _, value = line.split("#", 1)[0].partition(":")
        if key.strip() == "tests":
            return value.strip().lower() in {"true", "yes", "on"}
    return False


def strip_tests(root: Path) -> None:
    """Take the tests out of the project at root, in place."""
    python_tests = root / PYTHON_TESTS
    tests = list(python_tests.rglob("*")) if python_tests.is_dir() else []
    for path in sorted(root.rglob("*_test.go")) + sorted(item for item in tests if item.is_file()):
        if path.relative_to(root).as_posix() not in KEEP_WITHOUT_TESTS:
            path.unlink()
    for path in sorted(tests, reverse=True):
        if path.is_dir() and not any(path.iterdir()):
            path.rmdir()
    for path in sorted(root.rglob("testdata"), reverse=True):
        if path.is_dir() and not any(path.parent.glob("*_test.go")):
            shutil.rmtree(path)
