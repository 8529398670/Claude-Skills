"""Server-side sessions. The cookie only ever holds a random opaque id --
never a signed/encoded payload -- so a session can be revoked instantly
(delete the row) and the database never has to be trusted to a client.
"""
from __future__ import annotations

import hashlib
import secrets
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Optional

from app.core.config import settings
from app.core.db import get_conn


def _hash(raw: str) -> str:
    return hashlib.sha256(raw.encode()).hexdigest()


@dataclass(frozen=True)
class Session:
    user_id: int
    csrf_token: str


def create_session(user_id: int) -> tuple[str, Session]:
    """Returns (raw_cookie_value, Session). The raw value is set on the
    response cookie and never stored -- only its hash is persisted."""
    raw = secrets.token_urlsafe(32)
    csrf_token = secrets.token_urlsafe(24)
    expires_at = (
        datetime.now(timezone.utc) + timedelta(seconds=settings.session_ttl_seconds)
    ).isoformat()
    with get_conn() as conn:
        conn.execute(
            "INSERT INTO sessions (session_hash, user_id, csrf_token, expires_at) "
            "VALUES (?, ?, ?, ?)",
            (_hash(raw), user_id, csrf_token, expires_at),
        )
    return raw, Session(user_id=user_id, csrf_token=csrf_token)


def load_session(raw_cookie_value: str) -> Optional[Session]:
    if not raw_cookie_value:
        return None
    now = datetime.now(timezone.utc).isoformat()
    with get_conn() as conn:
        row = conn.execute(
            "SELECT s.user_id, s.csrf_token FROM sessions s "
            "JOIN users u ON u.id = s.user_id "
            "WHERE s.session_hash = ? AND s.expires_at > ? AND u.disabled_at IS NULL",
            (_hash(raw_cookie_value), now),
        ).fetchone()
    if row is None:
        return None
    return Session(user_id=row["user_id"], csrf_token=row["csrf_token"])


def destroy_session(raw_cookie_value: str) -> None:
    with get_conn() as conn:
        conn.execute(
            "DELETE FROM sessions WHERE session_hash = ?", (_hash(raw_cookie_value),)
        )


def destroy_all_sessions_for_user(user_id: int) -> None:
    with get_conn() as conn:
        conn.execute("DELETE FROM sessions WHERE user_id = ?", (user_id,))
