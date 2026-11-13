# whole-file feature:openapi
"""Hands out config/openapi.json, which scripts/api_docs.sh writes from the
routes and the docs each handler declares (the *_api.py files);
app/tests/test_api_docs.py fails while it is out of date. Signed-in callers
only: it maps the admin surface, and nobody who cannot sign in has a use for
it."""
from __future__ import annotations

from functools import cache

from starlette.requests import Request
from starlette.responses import Response

from app.core import assets


@cache
def spec() -> bytes:
    return assets.OPENAPI_SPEC.read_bytes()


async def get_openapi(request: Request) -> Response:
    return Response(spec(), media_type="application/json")
