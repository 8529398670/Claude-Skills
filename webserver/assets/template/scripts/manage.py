"""Command-line management for the two things that can't happen through
the web UI: creating the very first admin (nobody is logged in yet to do
it from the admin panel) and, as a fallback, minting logins from a shell
without going through the browser.

Usage:
    python -m scripts.manage bootstrap-admin --name "Ada"
    python -m scripts.manage create-login --name "Grace" --role user
    python -m scripts.manage reissue-login --user-id 3
"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from app.core.config import settings  # noqa: E402
from app.core.db import init_db  # noqa: E402
from app.models import login_tokens, users  # noqa: E402


def _print_login_url(token: str) -> None:
    # base_url isn't known outside a request; print the path and let the
    # operator prepend whatever host/port/scheme this deployment uses.
    print(f"One-time login link (expires in {settings.login_token_ttl_seconds // 60} min):")
    print(f"  /login/{token}")


def bootstrap_admin(args: argparse.Namespace) -> None:
    if users.any_admin_exists() and not args.force:
        print("An admin already exists. Re-run with --force to create another anyway.")
        return
    user = users.create_user(args.name, "admin")
    issued = login_tokens.issue_login_token(user.id)
    print(f"Created admin user #{user.id} ({args.name}).")
    _print_login_url(issued.token)


def create_login(args: argparse.Namespace) -> None:
    user = users.create_user(args.name, args.role)
    issued = login_tokens.issue_login_token(user.id)
    print(f"Created {args.role} user #{user.id} ({args.name}).")
    _print_login_url(issued.token)


def reissue_login(args: argparse.Namespace) -> None:
    user = users.get_user(args.user_id)
    if user is None:
        print(f"No user with id {args.user_id}", file=sys.stderr)
        sys.exit(1)
    issued = login_tokens.issue_login_token(user.id)
    print(f"New login link for #{user.id} ({user.display_name}).")
    _print_login_url(issued.token)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)

    p = sub.add_parser("bootstrap-admin", help="Create the first admin account.")
    p.add_argument("--name", required=True)
    p.add_argument("--force", action="store_true", help="Create even if an admin already exists.")
    p.set_defaults(func=bootstrap_admin)

    p = sub.add_parser("create-login", help="Create a new user and print a one-time login link.")
    p.add_argument("--name", required=True)
    p.add_argument("--role", choices=["admin", "user"], default="user")
    p.set_defaults(func=create_login)

    p = sub.add_parser("reissue-login", help="Mint a fresh login link for an existing user.")
    p.add_argument("--user-id", type=int, required=True)
    p.set_defaults(func=reissue_login)

    args = parser.parse_args()
    init_db()
    args.func(args)


if __name__ == "__main__":
    main()
