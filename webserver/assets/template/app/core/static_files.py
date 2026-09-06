"""Static asset serving with one deliberate rule: text assets (html/js/css)
are never cached by the browser and are re-read from disk + gzipped on
every request, so a deploy is visible on the very next refresh with no
cache-busting query strings or asset hashing to manage. Images are the
opposite -- gzip barely helps already-compressed formats, and users expect
images to load instantly on repeat views, so they get normal long-lived
caching instead.

This trade-off (always-fresh markup/code vs. cached binary assets) is the
whole reason this module exists instead of just using Starlette's built-in
StaticFiles app.
"""
from __future__ import annotations

import gzip
import mimetypes
from pathlib import Path

from starlette.requests import Request
from starlette.responses import FileResponse, Response

# Extensions that are small, text-based, and change often during development.
_GZIP_ALWAYS_FRESH = {".html", ".js", ".css", ".json", ".svg"}

_IMAGE_CACHE_SECONDS = 60 * 60 * 24 * 7  # 7 days


def _resolve_safe(root: Path, requested_path: str) -> Path | None:
    """Resolve requested_path under root, refusing any ../ escape."""
    candidate = (root / requested_path.lstrip("/")).resolve()
    try:
        candidate.relative_to(root.resolve())
    except ValueError:
        return None
    return candidate


def serve_static(root: Path, default_file: str = "index.html"):
    """Returns a Starlette endpoint function that serves files under root."""

    async def endpoint(request: Request) -> Response:
        raw_path = request.path_params.get("path", "") or default_file
        file_path = _resolve_safe(root, raw_path)
        if file_path is None or not file_path.is_file():
            # SPA-style fallback: unknown paths get index.html so client-side
            # routing (if any) can take over.
            file_path = _resolve_safe(root, default_file)
            if file_path is None or not file_path.is_file():
                return Response("Not found", status_code=404)

        suffix = file_path.suffix.lower()
        content_type = mimetypes.guess_type(file_path.name)[0] or "application/octet-stream"

        if suffix in _GZIP_ALWAYS_FRESH:
            accepts_gzip = "gzip" in request.headers.get("accept-encoding", "")
            body = file_path.read_bytes()
            headers = {
                "Cache-Control": "no-store, no-cache, must-revalidate",
                "Pragma": "no-cache",
            }
            if accepts_gzip:
                body = gzip.compress(body, compresslevel=6)
                headers["Content-Encoding"] = "gzip"
            return Response(body, media_type=content_type, headers=headers)

        # Binary/image assets: let the OS/browser cache do its job.
        return FileResponse(
            file_path,
            headers={"Cache-Control": f"public, max-age={_IMAGE_CACHE_SECONDS}"},
        )

    return endpoint
