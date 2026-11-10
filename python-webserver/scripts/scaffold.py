#!/usr/bin/env python3
"""Copy the bundled project template into a target directory and stamp the
project name into it.

This is only the mechanical part of scaffolding -- copy plus find/replace, and
writing the project's config.yaml and a freshly generated secret.key. It
deliberately does not create a venv, install anything, start a server, or
create any users, so nothing touches the network or the outside world until
the person running it decides to.

Usage:
    python3 scaffold.py <target-dir> "<project-name>" [--features a,b,c] [--repo URL]

--features is the exact list of optional features to switch on (see FEATURES;
`none` is the core alone), and leaving it out keeps the template's defaults. A
feature left off is cut out of the project (see prune.py), not just switched off.
--without cuts other parts too (see PIECES). --prune says exactly what to cut.
"""
from __future__ import annotations

import argparse
import os
import re
import secrets
import shutil
import socket
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import prune  # noqa: E402

SKILL_DIR = Path(__file__).resolve().parent.parent
TEMPLATE_DIR = SKILL_DIR / "assets" / "template"

# Substitution is a plain text replace, so it must only touch text files.
TEXT_SUFFIXES = {".py", ".html", ".css", ".js", ".json", ".yaml", ".yml", ".sh", ".bat", ".md", ".txt", ""}
TEXT_NAMES = {"Dockerfile", ".dockerignore", ".gitignore"}

# The name lands inside Python and YAML string literals, so the characters
# that would end or escape one are refused up front rather than producing a
# project that does not parse.
UNSAFE_NAME = re.compile(r'["\\\n\r]')


def slugify(name: str) -> str:
    """The lowercase, filesystem- and Docker-safe form of the display name."""
    slug = re.sub(r"[^a-z0-9]+", "-", name.strip().lower()).strip("-")
    return slug or "app"


def is_text_file(path: Path) -> bool:
    return path.suffix in TEXT_SUFFIXES or path.name in TEXT_NAMES


def stamp(path: Path, replacements: dict[str, str]) -> None:
    if not is_text_file(path):
        return
    try:
        # Bytes, not read_text(): universal newlines would turn run.bat's CRLF
        # line endings into LF, and cmd.exe misreads labels in an LF batch file.
        text = path.read_bytes().decode("utf-8")
    except (UnicodeDecodeError, OSError):
        return
    original = text
    for token, value in replacements.items():
        text = text.replace(token, value)
    if text != original:
        path.write_bytes(text.encode("utf-8"))


# defaults.yaml must not carry a key: every config.yaml and the committed
# example are copies of it. Mirrors new_file_content in app/core/config.py.
SECRET_KEY_LINE = re.compile(r"^\s*secret_key:", re.MULTILINE)


def write_config(target_dir: Path) -> tuple[Path, Path]:
    """Write <target>/config/config.yaml (the stamped defaults) and
    <target>/config/secret.key (a fresh key), as `config init` does.

    config.yaml is the project's one configuration file -- server and Docker.
    It holds no secret, so the server can edit it while running; the key is
    in secret.key beside it. Both are created at 0600, and both are already
    in the template's .gitignore and .dockerignore.
    """
    defaults = (target_dir / "config" / "defaults.yaml").read_text(encoding="utf-8")
    if SECRET_KEY_LINE.search(defaults):
        raise SystemExit("defaults.yaml must not contain server.secret_key -- the key lives in secret.key")
    config_path = target_dir / "config" / "config.yaml"
    secret_path = target_dir / "config" / "secret.key"
    for path, content in ((config_path, defaults), (secret_path, secrets.token_hex(32) + "\n")):
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            handle.write(content)
    return config_path, secret_path


def write_example(target_dir: Path) -> Path:
    """Write <target>/config/config.example.yaml: the example header plus the stamped
    defaults.

    It is the committed counterpart of config.yaml -- every setting with its
    default, nothing secret -- so a fresh clone has something to copy. It is
    regenerated here on every scaffold from the same two files that
    `python -m app config example` uses, so it always matches the defaults
    it was scaffolded with; the copy in the template is only there to be
    read.
    """
    config_dir = target_dir / "config"
    header = (config_dir / "example_header.yaml").read_text(encoding="utf-8")
    defaults = (config_dir / "defaults.yaml").read_text(encoding="utf-8")
    example_path = config_dir / "config.example.yaml"
    example_path.write_text(header + defaults, encoding="utf-8")
    return example_path


# Every project gets its own port, chosen once here and written into the
# project's defaults.yaml, so several local projects can run side by side and
# each keeps its own browser tab, theme, and sign-in. (The session cookie is
# named per project too -- browsers share cookies across ports on one host.)
# Writing defaults.yaml rather than only config.yaml means config.yaml,
# config.example.yaml, `config init`, and a fresh clone all agree on it.
PORT_RANGE = (20000, 39999)
PORT_LINES = (
    re.compile(r"^(\s*port:\s*)\d+[ \t]*$", re.MULTILINE),
    re.compile(r"^(\s*host_port:\s*)\d+[ \t]*$", re.MULTILINE),
)


def pick_port() -> int:
    """A random port in PORT_RANGE that nothing on this machine is listening on."""
    for _ in range(50):
        port = secrets.choice(range(PORT_RANGE[0], PORT_RANGE[1] + 1))
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
            try:
                probe.bind(("127.0.0.1", port))
            except OSError:
                continue
        return port
    raise SystemExit(f"Could not find a free port in {PORT_RANGE[0]}-{PORT_RANGE[1]}")


def stamp_port(target_dir: Path, port: int) -> None:
    """Set server.port and docker.host_port in <target>/config/defaults.yaml."""
    defaults_path = target_dir / "config" / "defaults.yaml"
    text = defaults_path.read_text(encoding="utf-8")
    for pattern in PORT_LINES:
        text, count = pattern.subn(lambda match: f"{match.group(1)}{port}", text)
        if count != 1:
            raise SystemExit(f"defaults.yaml must contain exactly one `{pattern.pattern}` line, found {count}")
    defaults_path.write_text(text, encoding="utf-8")


# app.repo, where the project's source lives. Written into defaults.yaml like
# the port, so config.yaml, the example and a fresh clone agree on it.
REPO_LINE = re.compile(r'^(  repo:[ \t]*)""[ \t]*$', re.MULTILINE)
SSH_REMOTE = re.compile(r"^(?:ssh://)?git@([^:/]+)[:/](.+)$")


def repo_url(value: str) -> str:
    """value as app.repo wants it: https, no trailing / or .git. An ssh remote
    (git@github.com:you/app.git) becomes its https address. "" when value is
    not something a browser can open."""
    value = value.strip()
    found = SSH_REMOTE.match(value)
    if found:
        value = f"https://{found.group(1)}/{found.group(2)}"
    value = value.rstrip("/")
    if value.endswith(".git"):
        value = value[: -len(".git")]
    if not re.fullmatch(r"https://[^\s/?#\"<>]+/[^\s?#\"<>]+", value):
        return ""
    return value


def stamp_repo(target_dir: Path, repo: str) -> None:
    """Set app.repo in <target>/config/defaults.yaml."""
    defaults_path = target_dir / "config" / "defaults.yaml"
    text = defaults_path.read_text(encoding="utf-8")
    text, count = REPO_LINE.subn(lambda match: f'{match.group(1)}"{repo}"', text)
    if count != 1:
        raise SystemExit(f"defaults.yaml must contain exactly one `repo: \"\"` line, found {count}")
    defaults_path.write_text(text, encoding="utf-8")


# What a project can have switched on: name -> (config.yaml section, key, what
# it needs). A feature a project does not use is cut out of it (see prune.py), so
# --features decides what code it gets; the rest come later through
# webserver-upgrade's add-feature. Writing defaults.yaml, like stamp_port, means
# config.yaml, config.example.yaml, `config init`, and a fresh clone all agree.
FEATURES = {
    "files": ("files", "enabled", ()),
    "api_keys": ("auth", "api_keys", ()),
    "mcp": ("server", "mcp", ("api_keys",)),
    "webhooks": ("webhooks", "enabled", ()),
    "invite_links": ("auth", "invite_links", ()),
    "passwords": ("auth", "passwords", ()),
    "two_factor": ("auth", "two_factor", ()),
    "metrics": ("metrics", "enabled", ("api_keys",)),
    "backups": ("backups", "enabled", ()),
}
# The rest of what can be cut: name -> what it needs. These have no switch, and a
# project has them unless --without says otherwise.
PIECES: dict[str, tuple[str, ...]] = {
    "logs": (),
    "sessions": (),
    "jobs": (),
    "maintenance": (),
    "settings": (),
    "appearance": (),
    "shortcuts": (),
    "openapi": (),
    "about": (),
}
# Files that cannot carry a marker, by what they belong to.
DATA_FILES: dict[str, tuple[str, ...]] = {
    "webhooks": ("app/tests/webhook_cases.json",),
    "openapi": ("config/openapi.json",),
    "mcp": ("config/mcp.json",),
}
TOP_LEVEL_KEY = re.compile(r"^[a-z_]+:")
SWITCH_VALUE = re.compile(r"\b(true|false)\b( *)")


def parse_features(value: str) -> tuple[set[str], list[str]]:
    """--features' value as the set to switch on, plus any feature added because
    another needs it. "none" is the core alone."""
    names = [name.strip() for name in value.split(",") if name.strip()]
    if names == ["none"]:
        return set(), []
    unknown = sorted(set(names) - set(FEATURES))
    if unknown or not names:
        raise SystemExit(f"Unknown feature(s): {', '.join(unknown) or '(empty)'}. Choose from: {', '.join(FEATURES)}, or none.")
    chosen = set(names)
    added = sorted({need for name in chosen for need in FEATURES[name][2]} - chosen)
    return chosen | set(added), added


def needs_of(name: str) -> tuple[str, ...]:
    return FEATURES[name][2] if name in FEATURES else PIECES[name]


def parse_names(value: str, flag: str) -> set[str]:
    """A list of features and pieces. "none" is the empty list."""
    names = {name.strip() for name in value.split(",") if name.strip()} - {"none"}
    unknown = sorted(names - set(FEATURES) - set(PIECES))
    if unknown:
        raise SystemExit(f"Unknown name(s) for {flag}: {', '.join(unknown)}. Choose from: {', '.join([*FEATURES, *PIECES])}.")
    return names


def with_dependents(removed: set[str]) -> set[str]:
    """Removing something removes what cannot work without it."""
    removed = set(removed)
    changed = True
    while changed:
        changed = False
        for name in [*FEATURES, *PIECES]:
            if name not in removed and removed & set(needs_of(name)):
                removed.add(name)
                changed = True
    return removed


def find_switch(lines: list[str], section: str, key: str) -> int:
    """The index of `  key: true|false` directly under `section:`. Section-aware
    because `enabled:` appears under several, and two spaces exactly so a deeper
    key of the same name cannot match."""
    pattern = re.compile(rf"^  {re.escape(key)}:\s*(true|false)\b")
    found: list[int] = []
    inside = False
    for index, line in enumerate(lines):
        if TOP_LEVEL_KEY.match(line):
            inside = line.startswith(f"{section}:")
        elif inside and pattern.match(line):
            found.append(index)
    if len(found) != 1:
        raise SystemExit(f"defaults.yaml must contain exactly one {section}.{key} switch, found {len(found)}")
    return found[0]


def aligned(value: str, match: re.Match[str]) -> str:
    """value with the spaces after it adjusted, so a trailing comment stays in
    its column whichever word is shorter. No comment, no spaces to adjust."""
    padding = match.group(2)
    if not padding:
        return value
    return value + " " * max(1, len(match.group(1)) + len(padding) - len(value))


def stamp_features(target_dir: Path, chosen: set[str]) -> None:
    """Set every switch in <target>/config/defaults.yaml to on or off, leaving
    its comment and alignment alone."""
    defaults_path = target_dir / "config" / "defaults.yaml"
    lines = defaults_path.read_text(encoding="utf-8").split("\n")
    for name, (section, key, _) in FEATURES.items():
        index = find_switch(lines, section, key)
        value = "true" if name in chosen else "false"
        lines[index] = SWITCH_VALUE.sub(lambda match: aligned(value, match), lines[index], count=1)
    defaults_path.write_text("\n".join(lines), encoding="utf-8")


def features_on(target_dir: Path) -> list[str]:
    """Which switches are on in <target>/config/defaults.yaml, for the summary."""
    lines = (target_dir / "config" / "defaults.yaml").read_text(encoding="utf-8").split("\n")
    return [name for name, (section, key, _) in FEATURES.items() if re.search(r":\s*true\b", lines[find_switch(lines, section, key)])]


# Where this project came from, so `webserver-upgrade` can later render the
# template it started from and bring over only what changed since.
ORIGIN_FILE = ".skill-origin.yaml"


def skill_commit() -> str:
    """The commit of the skills repo this template is at. "unknown" outside that
    repo: a stamp must not name a commit of some other repository that happens
    to contain this directory."""
    def git(*arguments: str) -> str:
        done = subprocess.run(["git", "-C", str(SKILL_DIR), *arguments], capture_output=True, text=True)
        return done.stdout.strip() if done.returncode == 0 else ""

    top = git("rev-parse", "--show-toplevel")
    if not top or Path(top).resolve() != SKILL_DIR.parent:
        return "unknown"
    return git("rev-parse", "HEAD") or "unknown"


def write_origin(target_dir: Path, project_name: str, slug: str, module_path: str | None, port: int, features: list[str], pruned: set[str]) -> Path:
    """Write <target>/.skill-origin.yaml. It is committed with the project."""
    commit = skill_commit()
    lines = [
        "# Written by the skill that scaffolded this app; `webserver-upgrade` reads and",
        "# updates it. Commit it: it is how an update knows where this app started.",
        f"skill: {SKILL_DIR.name}",
        f"commit: {commit}",
        f'project_name: "{project_name}"',
        f"slug: {slug}",
    ]
    if module_path:
        lines.append(f"module_path: {module_path}")
    lines += [f"port: {port}", f"features: [{', '.join(features)}]", f"pruned: [{', '.join(sorted(pruned))}]"]
    origin_path = target_dir / ORIGIN_FILE
    origin_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return origin_path


# What a scripts/run.sh or docker/run.sh run inside the template itself can
# leave behind: runtime state, config/config.yaml and the secret.key beside it.
# None of it belongs in a new project. Paths are relative to the template root.
RUN_LEFTOVERS = {"config/config.yaml", "config/secret.key", "config/config.yaml.bak", ".run", ".venv", "data"}
CRUFT = shutil.ignore_patterns(".DS_Store", "Thumbs.db", "*.swp", "__pycache__", "*.pyc")


def ignore_cruft(directory: str, names: list[str]) -> set[str]:
    ignored = set(CRUFT(directory, names))
    here = Path(directory).resolve()
    if here == TEMPLATE_DIR or TEMPLATE_DIR in here.parents:
        relative = here.relative_to(TEMPLATE_DIR)
        ignored |= {name for name in names if (relative / name).as_posix() in RUN_LEFTOVERS}
    return ignored


# scripts/run.sh and scripts/run.bat carry a block, between these two marker
# lines, that turns scripts/run.sh inside the template into a demo (see demo.py). A scaffolded
# project is not the template, so the block is cut out of it.
DEMO_BLOCK = re.compile(r"^[^\n]*>>> template demo[^\n]*\n.*?^[^\n]*<<< template demo[^\n]*\n", re.MULTILINE | re.DOTALL)


def strip_demo_block(path: Path) -> None:
    if not path.exists():
        return
    text = path.read_bytes().decode("utf-8")
    stripped, count = DEMO_BLOCK.subn("", text)
    if count:
        path.write_bytes(stripped.encode("utf-8"))


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("target_dir")
    parser.add_argument("project_name")
    parser.add_argument("--features", help=f"comma-separated, or none; leave it out for the template's defaults. Choose from: {', '.join(FEATURES)}")
    parser.add_argument("--without", default="", help=f"parts a project normally has to leave out, comma-separated. Choose from: {', '.join(PIECES)}")
    parser.add_argument("--repo", help="where the source lives, e.g. https://github.com/you/app (an ssh remote is fine); becomes app.repo")
    parser.add_argument("--prune", help="exactly what to cut, comma-separated, or none, in place of working it out from --features and --without; for webserver-upgrade and demo.py")
    args = parser.parse_args()
    # Before anything is written: a typo should not leave half a project.
    chosen, added = parse_features(args.features) if args.features is not None else (None, [])
    without = parse_names(args.without, "--without")
    exact = parse_names(args.prune, "--prune") if args.prune is not None else None

    target_dir = Path(args.target_dir).resolve()
    project_name = args.project_name.strip()
    repo = repo_url(args.repo) if args.repo else ""
    if args.repo and not repo:
        raise SystemExit(f"--repo {args.repo!r} is not a repository address like https://github.com/you/app")
    if not project_name or UNSAFE_NAME.search(project_name):
        print('The project name must be non-empty and contain no double quote, backslash, or newline.')
        sys.exit(1)

    # Refusing here rather than merging is the point: scaffolding on top of
    # someone's existing work would silently overwrite files, and the mistake
    # is only obvious later.
    if target_dir.exists() and any(target_dir.iterdir()):
        print(f"Refusing to scaffold into non-empty directory: {target_dir}")
        sys.exit(1)

    # Skip editor/OS cruft rather than copying it into a fresh project. A
    # stray .DS_Store in the template would otherwise end up committed in
    # every scaffolded repo. RUN_LEFTOVERS go too, at the top level only.
    shutil.copytree(
        TEMPLATE_DIR,
        target_dir,
        dirs_exist_ok=True,
        ignore=ignore_cruft,
    )

    slug = slugify(project_name)
    replacements = {
        "{{PROJECT_NAME}}": project_name,
        # Docker rejects uppercase repository names, and the slug also names
        # the app's state directory (~/.config/<slug>/), so anything that
        # becomes a path or an image/container name uses it rather than the
        # display name.
        "{{PROJECT_SLUG}}": slug,
    }
    for path in sorted(target_dir.rglob("*")):
        if path.is_file():
            stamp(path, replacements)
    for name in ("run.sh", "run.bat", "login.sh", "login.bat"):
        strip_demo_block(target_dir / "scripts" / name)

    # lib.sh files are sourced, not run, so they stay non-executable.
    for folder in ("scripts", "docker", "misc"):
        for script in (target_dir / folder).glob("*.sh"):
            if script.name != "lib.sh":
                script.chmod(0o755)

    port = pick_port()
    stamp_port(target_dir, port)
    if repo:
        stamp_repo(target_dir, repo)
    if chosen is not None:
        stamp_features(target_dir, chosen)
    on = features_on(target_dir)
    pruned = exact if exact is not None else with_dependents(set(FEATURES) - set(on) | without)
    on = [name for name in on if name not in pruned]
    prune.prune_tree(target_dir, pruned, DATA_FILES)
    config_path, secret_path = write_config(target_dir)
    example_path = write_example(target_dir)
    write_origin(target_dir, project_name, slug, None, port, on, pruned)

    print(f"Scaffolded '{project_name}' into {target_dir}")
    print(f"  slug (state dir, docker image/container name): {slug}")
    print(f"  port: {port} (random per project, so local projects never collide)")
    print(f"  repo: {repo or '(none -- set app.repo in Settings or config.yaml)'}")
    print(f"  config: {config_path} (mode 0600, gitignored; editable while the server runs)")
    print(f"  secret: {secret_path} (mode 0600, fresh key, gitignored -- back it up with the database)")
    print(f"  example: {example_path} (commit it -- `cp` it to config/config.yaml on a fresh clone)")
    off = [name for name in FEATURES if name not in on and name not in pruned]
    print(f"  features on: {', '.join(on) or '(core only)'}")
    if off:
        print(f"  features off: {', '.join(off)} -- in the project; switch one on in Settings")
    if pruned:
        print(f"  left out: {', '.join(sorted(pruned))} -- webserver-upgrade's add-feature brings one in")
    for name in added:
        print(f"  added {name}: another feature you chose needs it")
    print("")
    print("Next:")
    print(f"  cd {target_dir}")
    print("  scripts/run.sh            # sets up .venv if needed, (re)starts the server, prints its URL")
    print("  scripts/login.sh          # a login link for the owner (user 1), wherever it runs")
    print("  scripts/run.sh manage ... # admin CLI, works while it is running")
    print("  (scripts\\run.bat and scripts\\login.bat on Windows, same arguments)")


if __name__ == "__main__":
    main()
