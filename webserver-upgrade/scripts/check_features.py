#!/usr/bin/env python3
"""Build and test a webserver skill's template with parts cut out.

Every combination of 17 parts cannot be tested, so this runs the ones that find
broken dependencies: everything, nothing, each part alone, and each part removed.

    python3 check_features.py go-webserver [--only webhooks,logs] [--set all,none] [--jobs 3]
        [--no-tests] [--frontend-only] [--python PATH]

--only runs a part's "alone" and "removed" sets. --no-tests stops after compiling,
which is quick. --frontend-only stops after the checks on static/, quicker still.
Python needs an interpreter with the app's requirements and httpx2, given by
--python; the default is the skill's demo virtualenv.

A failing set leaves its project in place and prints where. It exits 1 if any fails.
"""
from __future__ import annotations

import argparse
import concurrent.futures
import importlib.util
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

SKILLS = Path(__file__).resolve().parent.parent.parent
LANGUAGE_SECTION = re.compile(r"([a-z0-9_]+):")
LANGUAGE_KEY = re.compile(r"  ([a-z0-9_]+):")
LITERAL_KEY = re.compile(r'I18n\.get\(\s*"([^"]+)"\s*\)')
LITERAL_ID = re.compile(r'Dom\.get\(\s*"([^"]+)"\s*\)')
GLOBAL_OBJECT = re.compile(r"^const ([A-Z]\w*) = \{", re.MULTILINE)
COMMENT = re.compile(r"(?:^|[ \t])//(?:[ \t].*)?$", re.MULTILINE)


def load_scaffold(skill: str):
    folder = SKILLS / skill / "scripts"
    sys.path.insert(0, str(folder))
    spec = importlib.util.spec_from_file_location("scaffold", folder / "scaffold.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def plans(scaffold, only: set[str], names: set[str]) -> dict[str, set[str]]:
    """Set name -> what is cut."""
    parts = [*scaffold.FEATURES, *scaffold.PIECES]

    def needed(name: str) -> set[str]:
        found = {name}
        for need in scaffold.needs_of(name):
            found |= needed(need)
        return found

    sets: dict[str, set[str]] = {}
    if not only:
        sets["all"] = set()
        sets["none"] = set(parts)
    for name in parts:
        if not only or name in only:
            sets[f"only-{name}"] = set(parts) - needed(name)
            sets[f"without-{name}"] = scaffold.with_dependents({name})
    return {key: value for key, value in sets.items() if not names or key in names}


def run(command: list[str], cwd: Path, timeout: int = 3000) -> tuple[bool, str]:
    try:
        done = subprocess.run(command, cwd=cwd, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        return False, f"timed out: {' '.join(command)}"
    return done.returncode == 0, done.stdout + done.stderr


def tail(text: str) -> str:
    lines = [line for line in text.splitlines() if line.startswith(("FAIL", "--- FAIL", "ERROR:", "#", "vet:")) or ".go:" in line or "Error" in line]
    return "\n".join((lines or text.splitlines())[-12:])


def language_keys(path: Path) -> set[str]:
    """Every section.key in language.yaml, which is only ever two levels deep."""
    keys: set[str] = set()
    section = ""
    for line in path.read_text(encoding="utf-8").splitlines():
        top = LANGUAGE_SECTION.match(line)
        if top:
            section = top.group(1)
        key = LANGUAGE_KEY.match(line)
        if key:
            keys.add(f"{section}.{key.group(1)}")
    return keys


def frontend_problems(skill: str, project: Path) -> list[str]:
    """What the pruned page would trip over: a key, an element, a script or a
    global that went with a feature while something still asks for it. A key built
    at run time, like "logs.kind_" + kind, is not literal, so it is not looked at."""
    page = (project / "static" / "index.html").read_text(encoding="utf-8")
    scripts = {path.name: COMMENT.sub("", path.read_text(encoding="utf-8")) for path in (project / "static" / "js").glob("*.js")}
    keys = language_keys(project / "config" / "language.yaml")
    ids = set(re.findall(r'\bid="([^"]+)"', page))
    problems: list[str] = []

    wanted = set(re.findall(r'\bdata-i18n="([^"]+)"', page)) | set(re.findall(r'\bdata-empty="([^"]+)"', page))
    for value in re.findall(r'\bdata-i18n-attr="([^"]+)"', page):
        wanted |= {pair.split(":", 1)[1] for pair in value.split(",")}
    problems += [f"index.html: no language key {key}" for key in sorted(wanted - keys)]
    for script, text in sorted(scripts.items()):
        problems += [f"{script}: no language key {key}" for key in sorted(set(LITERAL_KEY.findall(text)) - keys)]
        problems += [f"{script}: no element #{element}" for element in sorted(set(LITERAL_ID.findall(text)) - ids)]

    for source in sorted(set(re.findall(r'src="/js/([^"]+)"', page)) - set(scripts)):
        problems.append(f"index.html: loads /js/{source}, which is gone")

    template = SKILLS / skill / "assets" / "template" / "static" / "js"
    defined = {name for text in scripts.values() for name in GLOBAL_OBJECT.findall(text)}
    every = {name for path in template.glob("*.js") for name in GLOBAL_OBJECT.findall(path.read_text(encoding="utf-8"))}
    for name in sorted(every - defined):
        reference = re.compile(rf"\b{name}\.")
        problems += [f"{script}: {name} is gone" for script, text in sorted(scripts.items()) if reference.search(text)]
    return problems


def check(skill: str, name: str, cut: set[str], arguments: argparse.Namespace, base: Path) -> tuple[str, bool, str]:
    project = base / name
    scaffold = SKILLS / skill / "scripts" / "scaffold.py"
    ok, output = run([sys.executable, "-B", str(scaffold), str(project), "Check", "--prune", ",".join(sorted(cut)) or "none"], base)
    if not ok:
        return name, False, tail(output)
    for script in sorted((project / "static" / "js").glob("*.js")):
        ok, output = run(["node", "--check", str(script)], project)
        if not ok:
            return name, False, output[-400:]
    problems = frontend_problems(skill, project)
    if problems:
        return name, False, "\n".join(problems[:20])
    if arguments.frontend_only:
        shutil.rmtree(project, ignore_errors=True)
        return name, True, ""
    if skill == "go-webserver":
        steps = [["go", "vet", "./..."]] + ([] if arguments.no_tests else [["go", "test", "./..."]])
    else:
        steps = [[arguments.python, "-m", "compileall", "-q", "app"]] + ([] if arguments.no_tests else [[arguments.python, "-m", "unittest", "discover", "app/tests"]])
    for step in steps:
        ok, output = run(step, project)
        if not ok:
            return name, False, tail(output)
    shutil.rmtree(project, ignore_errors=True)
    return name, True, ""


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("skill", choices=["go-webserver", "python-webserver"])
    parser.add_argument("--only", default="")
    parser.add_argument("--set", default="", help="run only these sets, by name")
    parser.add_argument("--jobs", type=int, default=3)
    parser.add_argument("--no-tests", action="store_true")
    parser.add_argument("--frontend-only", action="store_true", help="stop after node --check and the checks on static/")
    parser.add_argument("--python", default=str(Path.home() / ".cache" / "python-webserver-demo" / ".venv" / "bin" / "python"))
    arguments = parser.parse_args()
    scaffold = load_scaffold(arguments.skill)
    only = {name for name in arguments.only.split(",") if name}
    names = {name for name in arguments.set.split(",") if name}
    sets = plans(scaffold, only, names)
    base = Path(tempfile.mkdtemp(prefix="check-features-"))
    failed = 0
    with concurrent.futures.ThreadPoolExecutor(arguments.jobs) as pool:
        work = [pool.submit(check, arguments.skill, name, cut, arguments, base) for name, cut in sets.items()]
        for future in concurrent.futures.as_completed(work):
            name, ok, detail = future.result()
            print(f"{'ok  ' if ok else 'FAIL'} {name}", flush=True)
            if not ok:
                failed += 1
                print(f"     {base / name}\n     " + detail.replace("\n", "\n     "), flush=True)
    print(f"{len(sets) - failed} of {len(sets)} sets passed")
    if failed == 0:
        shutil.rmtree(base, ignore_errors=True)
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
