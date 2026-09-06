"""One-time login tokens -- the entire signup/login story for this app.

An admin mints a token for a user_id (new or existing). The raw token is
shown/handed out exactly once and is never stored -- only its sha256 hash
lives in the database, so a leaked database dump cannot be replayed into a
login the way a leaked password hash sometimes can. Redeeming a token
consumes it immediately, so a link that has already been used (or has
expired) is dead even if someone still has the URL.
"""
from __future__ import annotations

import hashlib
import secrets
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Optional

from app.core.config import settings
from app.core.db import get_conn


def _hash(token: str) -> str:
    return hashlib.sha256(token.encode()).hexdigest()


@dataclass(frozen=True)
class IssuedToken:
    token: str  # raw token -- caller must hand this to the user out-of-band
    user_id: int
    expires_at: str


def issue_login_token(user_id: int) -> IssuedToken:
    token = secrets.token_urlsafe(32)
    expires_at = (
        datetime.now(timezone.utc) + timedelta(seconds=settings.login_token_ttl_seconds)
    ).isoformat()
    with get_conn() as conn:
        conn.execute(
            "INSERT INTO login_tokens (token_hash, user_id, expires_at) VALUES (?, ?, ?)",
            (_hash(token), user_id, expires_at),
        )
    return IssuedToken(token=token, user_id=user_id, expires_at=expires_at)


def redeem_login_token(token: str) -> Optional[int]:
    """Validate + consume a token. Returns the user_id on success, else None.

    A token is valid exactly once: this function marks it used as part of
    the same check, so two concurrent requests racing the same link cannot
    both succeed.
    """
    token_hash = _hash(token)
    now = datetime.now(timezone.utc).isoformat()
    with get_conn() as conn:
        row = conn.execute(
            "SELECT user_id, expires_at, used_at FROM login_tokens WHERE token_hash = ?",
            (token_hash,),
        ).fetchone()
        if row is None or row["used_at"] is not None or row["expires_at"] < now:
            return None
        cur = conn.execute(
            "UPDATE login_tokens SET used_at = ? WHERE token_hash = ? AND used_at IS NULL",
            (now, token_hash),
        )
        if cur.rowcount == 0:
            return None  # lost the race to another concurrent redemption
        return row["user_id"]
