"""Cross-cutting hardening applied to every response/request, regardless of
which route handles it. Route modules should never need to think about
these headers or about basic abuse-throttling themselves.
"""
from __future__ import annotations

import time
from collections import defaultdict, deque

from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import Response

from app.core.config import settings


class SecurityHeadersMiddleware(BaseHTTPMiddleware):
    """Baseline headers recommended by OWASP's secure headers project.
    Tighten CSP further once you know exactly which origins your pages
    need (fonts, analytics, etc.) -- this default assumes everything is
    same-origin.
    """

    async def dispatch(self, request: Request, call_next) -> Response:
        response = await call_next(request)
        response.headers["X-Content-Type-Options"] = "nosniff"
        response.headers["X-Frame-Options"] = "DENY"
        response.headers["Referrer-Policy"] = "same-origin"
        response.headers["Permissions-Policy"] = "geolocation=(), microphone=(), camera=()"
        response.headers["Content-Security-Policy"] = (
            "default-src 'self'; img-src 'self' data:; object-src 'none'; "
            "base-uri 'self'; frame-ancestors 'none'"
        )
        if settings.secure_cookies:
            response.headers["Strict-Transport-Security"] = "max-age=63072000; includeSubDomains"
        return response


class RateLimitMiddleware(BaseHTTPMiddleware):
    """A deliberately simple in-memory sliding-window limiter. It resets on
    every restart and doesn't share state across multiple worker processes
    -- fine for the single-process deployment this template ships with. If
    you scale to multiple workers/replicas, move this to something shared
    (e.g. Redis) instead of trusting per-process memory.
    """

    def __init__(self, app, max_requests: int = 60, window_seconds: int = 60):
        super().__init__(app)
        self.max_requests = max_requests
        self.window_seconds = window_seconds
        self._hits: dict[str, deque] = defaultdict(deque)

    async def dispatch(self, request: Request, call_next) -> Response:
        client_ip = request.client.host if request.client else "unknown"
        now = time.monotonic()
        hits = self._hits[client_ip]
        while hits and now - hits[0] > self.window_seconds:
            hits.popleft()
        if len(hits) >= self.max_requests:
            return Response("Too many requests", status_code=429)
        hits.append(now)
        return await call_next(request)
