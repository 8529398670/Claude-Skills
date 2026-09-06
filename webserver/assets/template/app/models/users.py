"""User accounts. There is no password column anywhere in this schema --
identity is only ever proven by redeeming a login token or presenting a
valid session cookie. Keep it that way; adding a password field defeats the
point of the one-time-link design.
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

from app.core.db import get_conn


@dataclass(frozen=True)
class User:
    id: int
    display_name: str
    role: str
    disabled: bool


def _row_to_user(row) -> User:
    return User(
        id=row["id"],
        display_name=row["display_name"],
        role=row["role"],
        disabled=row["disabled_at"] is not None,
    )


def create_user(display_name: str, role: str) -> User:
    if role not in ("admin", "user"):
        raise ValueError(f"invalid role: {role!r}")
    with get_conn() as conn:
        cur = conn.execute(
            "INSERT INTO users (display_name, role) VALUES (?, ?)",
            (display_name, role),
        )
        row = conn.execute(
            "SELECT * FROM users WHERE id = ?", (cur.lastrowid,)
        ).fetchone()
    return _row_to_user(row)


def get_user(user_id: int) -> Optional[User]:
    with get_conn() as conn:
        row = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
    return _row_to_user(row) if row else None


def list_users() -> list[User]:
    with get_conn() as conn:
        rows = conn.execute("SELECT * FROM users ORDER BY created_at").fetchall()
    return [_row_to_user(r) for r in rows]


def rename_user(user_id: int, display_name: str) -> None:
    with get_conn() as conn:
        conn.execute(
            "UPDATE users SET display_name = ? WHERE id = ?",
            (display_name, user_id),
        )


def set_disabled(user_id: int, disabled: bool) -> None:
    with get_conn() as conn:
        conn.execute(
            "UPDATE users SET disabled_at = CASE WHEN ? THEN datetime('now') ELSE NULL END "
            "WHERE id = ?",
            (disabled, user_id),
        )


def any_admin_exists() -> bool:
    with get_conn() as conn:
        row = conn.execute(
            "SELECT 1 FROM users WHERE role = 'admin' LIMIT 1"
        ).fetchone()
    return row is not None
