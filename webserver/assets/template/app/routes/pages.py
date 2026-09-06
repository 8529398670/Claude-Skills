"""Wires the static-file serving policy (see app.core.static_files) up as
the catch-all route for anything not claimed by the auth/account/admin
APIs. Kept separate from those API route modules so "how pages are served"
and "what the app can do" don't get tangled together.
"""
from __future__ import annotations

from app.core.config import settings
from app.core.static_files import serve_static

static_page = serve_static(settings.static_dir)
