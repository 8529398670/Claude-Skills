"""The API reference -- config/openapi.json and config/api.md -- written from
the code, so nobody edits either by hand.

Every route's handler declares its docs once, with doc(), in an *_api.py file
next to it (app/routes/account_api.py for app/routes/account.py): a summary,
who may call it, and the types of what it takes and gives back. Those types
are TypedDicts whose fields carry the rest (see schema.py). build() walks the
routes the app really registers and pairs each with its handler's docs, so the
paths and methods come from the route table, and a route without docs, or
docs without a route, is reported.

Nothing here runs while the server is up. scripts/api_docs.sh runs
app/tests/test_api_docs.py with --update, which writes the files; the same
test fails while a committed file is out of date. Go's twin is server/apidoc,
and the two write the same bytes.
"""
from __future__ import annotations

import importlib
import json
import pkgutil
import re
from dataclasses import dataclass, field
from typing import Annotated, Any, Callable, NotRequired, TypedDict

from app.apidocs.schema import Field, OneOf, Schemas, one_of, schema  # noqa: F401  (the public names)
from app.apidocs.schema import operation as schema_operation

TITLE = "{{PROJECT_NAME}} API"
VERSION = "1.0.0"
ABOUT = (
    "Every route this server answers, apart from the pages at /login/* and /join/*. Generated from the code by "
    "scripts/api_docs.sh: an app's own routes get their docs the same way, and a test fails until they do."
)

# Who may call a route. SIGNED, the default, is anyone signed in, by a browser
# session or an API key.
SIGNED = "signed"
PUBLIC = "public"  # no sign-in at all
SESSION_ONLY = "session"  # a browser session: an API key is turned away
KEY_ONLY = "key"  # an API key

# Roles for Op.role. Empty is any signed-in user.
ADMIN = "admin"
OWNER = "owner"


@dataclass
class Op:
    """One route's docs. The fields match Go's apidoc.Op."""

    summary: str = ""
    tag: str = ""
    description: str = ""
    # method picks the route when one handler serves several methods.
    method: str = ""
    access: str = SIGNED
    role: str = ""
    # feature is the optional part of the template the route belongs to; it
    # becomes x-feature on the path, which scaffold.py's prune step reads.
    feature: str = ""
    # mcp_http is why a route an API key may call has no MCP tool.
    mcp_http: str = ""
    # params is a TypedDict whose fields carry Field(path=...), Field(query=...)
    # or Field(header=...). A path parameter it leaves out is a plain string.
    params: Any = None
    # body is the request body's type; body_type its content type when it is
    # not JSON, when it is raw bytes and body stays None.
    body: Any = None
    body_type: str = ""
    # status is the success status. reply is that reply's type; reply_type its
    # content type when it is not JSON (then reply is None, and the reply is
    # text, or bytes for application/octet-stream).
    status: int = 200
    reply: Any = None
    reply_type: str = ""
    reply_description: str = ""
    # errors are the error statuses the route can answer with, besides 429
    # and 503, which every route can.
    errors: tuple[int, ...] = ()
    used: bool = field(default=False, repr=False)


_registry: dict[str, list[Op]] = {}


def doc(handler: Callable, **fields: Any) -> None:
    """Record a handler's docs: doc(account.me, summary=..., tag=..., ...)."""
    _registry.setdefault(handler_name(handler), []).append(Op(**fields))


def handler_name(handler: Any) -> str:
    """The function behind a handler, the same for a bound method (what a
    route holds) and the plain function (what doc() may be given)."""
    function = getattr(handler, "__func__", handler)
    return f"{getattr(function, '__module__', '')}.{getattr(function, '__qualname__', '')}"


def load_docs() -> None:
    """Import every *_api module in app.routes, where the doc() calls are."""
    import app.routes as package

    for module in pkgutil.iter_modules(package.__path__):
        if module.name.endswith("_api"):
            importlib.import_module(f"app.routes.{module.name}")


ROUTE_PARAMETER = re.compile(r"\{(\w+):\w+\}")
SPEC_PARAMETER = re.compile(r"\{(\w+)\}")
NOT_WORD = re.compile(r"[^a-z0-9]+")


def spec_path(path: str) -> str:
    """A route's path the way the spec writes it: /users/{user_id}."""
    return ROUTE_PARAMETER.sub(r"{\1}", path)


def build(routes: list[tuple[str, str, Any]]) -> tuple[dict[str, Any], list[str]]:
    """openapi.json for these (method, path, endpoint) routes, and what is
    wrong with the docs: a route nobody documented, or docs for a route that
    is not registered."""
    load_docs()
    for entries in _registry.values():
        for entry in entries:
            entry.used = False
    builder = Schemas()
    paths: dict[str, Any] = {}
    tags: set[str] = set()
    problems: list[str] = []
    for method, raw_path, handler in routes:
        name = handler_name(handler)
        path = spec_path(raw_path)
        label = f"{method} {path}"
        if "<lambda>" in name or "<locals>" in name:
            problems.append(f"{label}: its handler is an unnamed function; give it a name and docs")
            continue
        chosen = None
        for entry in _registry.get(name, []):
            if entry.method in ("", method):
                chosen = entry
                if entry.method:
                    break
        if chosen is None:
            problems.append(f"{label}: no docs; add doc( {short_name(name)} , ... ) in an *_api.py next to it")
            continue
        chosen.used = True
        item = paths.setdefault(path, {})
        if chosen.feature:
            item["x-feature"] = chosen.feature
        operation, operation_problems = schema_operation(builder, method, path, chosen)
        problems += [f"{label}: {problem}" for problem in operation_problems]
        item[method.lower()] = operation
        tags.add(chosen.tag)
    for name, entries in _registry.items():
        for entry in entries:
            if not entry.used:
                problems.append(f"docs for {short_name(name)} {entry.method}, which is no route's handler")
    problems.sort()
    builder.named(ErrorReply)
    builder.named(MaintenanceReply)
    responses = {}
    for status in ERROR_STATUSES:
        target = "Maintenance" if status == 503 else "Error"
        responses[f"E{status}"] = {"description": ERROR_MEANINGS[status], "content": json_content(ref(target))}
    spec = {
        "openapi": "3.1.0",
        "info": {"title": TITLE, "version": VERSION, "description": ABOUT},
        "tags": [{"name": name} for name in tag_order(tags)],
        "paths": paths,
        "components": {
            "securitySchemes": {
                "sessionCookie": {
                    "type": "apiKey", "in": "cookie", "name": "session",
                    "description": "A browser session. Its POSTs carry csrf_token in the body. The cookie's name is server.cookie_name.",
                },
                "apiKey": {"type": "http", "scheme": "bearer", "description": "An API key, as Authorization: Bearer <id>.<secret>."},
            },
            "responses": responses,
            "schemas": builder.components,
        },
    }
    return spec, problems


def short_name(name: str) -> str:
    return name.rsplit(".", 2)[-2] + "." + name.rsplit(".", 1)[-1] if name.count(".") > 1 else name


# The order tags appear in, in both files. A tag not listed here comes after
# these, alphabetically.
TAG_ORDER = ["Account", "Sign-in", "API keys", "Files", "Admin", "Invite links", "Owner", "Webhooks", "Monitoring"]


def tag_order(used: set[str]) -> list[str]:
    return [name for name in TAG_ORDER if name in used] + sorted(name for name in used if name not in TAG_ORDER)


ERROR_STATUSES = (400, 401, 403, 404, 409, 429, 503)
ERROR_MEANINGS = {
    400: "The request was refused as it stands.",
    401: "Not signed in.",
    403: "Not allowed, or a bad csrf_token.",
    404: "No such thing, or this feature is switched off.",
    409: "The record has moved on: an undo too late, or a stale edit.",
    429: "Too many requests.",
    503: "In maintenance mode. Retry-After says when to try again.",
}


@schema(name="Error")
class ErrorReply(TypedDict):
    """The body of every error."""

    error: str
    request_id: NotRequired[Annotated[str, Field(doc="On a 5xx: the reference that finds this failure in the log.")]]


@schema(name="Maintenance")
class MaintenanceReply(TypedDict):
    """The body of a 503 in maintenance mode."""

    error: str
    maintenance: Annotated[str, Field(enum=("read_only", "full"))]


def json_content(schema_: dict[str, Any]) -> dict[str, Any]:
    return {"application/json": {"schema": schema_}}


def raw_schema(content_type: str) -> dict[str, Any]:
    result: dict[str, Any] = {"type": "string"}
    if content_type == "application/octet-stream":
        result["format"] = "binary"
    return result


def ref(name: str, section: str = "schemas") -> dict[str, Any]:
    return {"$ref": f"#/components/{section}/{name}"}


def operation_id(method: str, path: str) -> str:
    return NOT_WORD.sub("_", f"{method} {path}".lower()).strip("_")


def encode(document: Any) -> str:
    """How every generated JSON file is written: keys sorted, two-space
    indent, and a newline at the end -- the bytes Go's apidoc.Encode writes."""
    return json.dumps(document, indent=2, ensure_ascii=False, sort_keys=True) + "\n"
