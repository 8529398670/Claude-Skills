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

config/openapi.json has no comments, so a path or schema carries a key instead:

    "x-feature": "webhooks"

config/mcp.json is written from the registered tools, so it names no features:
each tool and http_only entry goes with the openapi.json path its route is on.

Only the standard library is used, because this has to run wherever the skills do.
"""
from __future__ import annotations

import json
import re
from pathlib import Path

# What prune_tree reads. Anything else is left alone.
SUFFIXES = {".go", ".py", ".js", ".css", ".html", ".yaml", ".yml", ".sh", ".bat", ".md", ".txt"}
NAMES = {"Dockerfile"}
OPENAPI = "config/openapi.json"
MCP_JSON = "config/mcp.json"

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
    """openapi.json without the paths and schemas of removed features, and with
    every x-feature key gone."""
    spec = json.loads(text)
    for group in (spec.get("paths", {}), spec.get("components", {}).get("schemas", {})):
        for name in list(group):
            expression = group[name].pop("x-feature", None)
            if expression is not None and not keeps(expression, removed):
                del group[name]
    used = {tag for item in spec.get("paths", {}).values() for operation in item.values() if isinstance(operation, dict) for tag in operation.get("tags", [])}
    if "tags" in spec:
        spec["tags"] = [tag for tag in spec["tags"] if tag["name"] in used]
    return json.dumps(spec, indent=2, ensure_ascii=False)


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
    are the files that cannot carry a marker, by feature."""
    # Read before anything is cut: mcp.json's tools go by the x-feature of
    # their route, which pruning takes out of openapi.json.
    spec_path = root / OPENAPI
    features = route_features(spec_path.read_text(encoding="utf-8")) if spec_path.is_file() else {}
    for name, paths in (data_files or {}).items():
        if name in removed:
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
    # A package or folder that was all one feature goes with it.
    for path in sorted(root.rglob("*"), reverse=True):
        if path.is_dir() and not any(path.iterdir()):
            path.rmdir()
