"""Central configuration, loaded once from environment variables.

Every other module reads settings from here instead of touching os.environ
directly -- that keeps "what can be configured" discoverable in one place.
"""
from __future__ import annotations

import os
import secrets
from dataclasses import dataclass, field
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent.parent


def _bool_env(name: str, default: bool) -> bool:
    val = os.environ.get(name)
    if val is None:
        return default
    return val.strip().lower() in ("1", "true", "yes", "on")


@dataclass(frozen=True)
class Settings:
    project_name: str = os.environ.get("PROJECT_NAME", "{{PROJECT_NAME}}")

    # Where the sqlite file lives. Mounted as a volume in dockerRun.sh so
    # data survives container restarts/rebuilds.
    data_dir: Path = Path(os.environ.get("DATA_DIR", BASE_DIR / "data"))
    db_path: Path = field(init=False)

    static_dir: Path = Path(os.environ.get("STATIC_DIR", BASE_DIR / "static"))

    host: str = os.environ.get("HOST", "0.0.0.0")
    port: int = int(os.environ.get("PORT", "8000"))

    # Cookies default to Secure=True (HTTPS only). Set SECURE_COOKIES=false
    # only for local http:// development -- never in a real deployment.
    secure_cookies: bool = _bool_env("SECURE_COOKIES", True)
    cookie_name: str = os.environ.get("COOKIE_NAME", "session")

    session_ttl_seconds: int = int(os.environ.get("SESSION_TTL_SECONDS", str(60 * 60 * 24 * 14)))
    login_token_ttl_seconds: int = int(os.environ.get("LOGIN_TOKEN_TTL_SECONDS", str(60 * 30)))

    # Only used to sign the CSRF token, never for passwords (there are none).
    secret_key: str = os.environ.get("SECRET_KEY", "")

    def __post_init__(self):
        object.__setattr__(self, "db_path", self.data_dir / "app.db")
        if not self.secret_key:
            # Fine for local/dev runs; dockerRun.sh generates and pins a
            # persistent SECRET_KEY so sessions survive container restarts.
            object.__setattr__(self, "secret_key", secrets.token_hex(32))


settings = Settings()
settings.data_dir.mkdir(parents=True, exist_ok=True)
