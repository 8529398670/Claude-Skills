"""How a Python type becomes JSON Schema, the same way Go's apidoc does.

str, bool, int and float map over as you would expect; datetime is a
date-time string; list[T] is an array; dict[str, T] is an object of T, and
dict[str, Any] an object of anything; Any is any value; T | None may also be
null. A TypedDict is an object: a key is required unless NotRequired. Wrap a
field's type in Annotated[T, Field(...)] to describe it:

    Field(doc="what it is")            its description
    Field(enum=("user", "admin"))      the values it may take
    Field(format="uri")                a string format
    Field(minimum=0, maximum=9)        number limits
    Field(min_length=1, max_length=9)  string limits
    Field(default=..., example=...)    a value
    Field(nullable=True)               may be null (T | None already may)

@schema(name="Me") makes a TypedDict a named schema in components, referred to
by $ref, and @schema(doc="...") gives one a description. A TypedDict
inheriting from a named one is allOf that schema and its own fields.
one_of("SignIn", A, B) is a named schema that is one of A or B.
"""
from __future__ import annotations

import types
import typing
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Union, get_args, get_origin

MISSING: Any = object()


@dataclass(frozen=True)
class Field:
    doc: str = ""
    enum: tuple = ()
    format: str = ""
    minimum: Any = None
    maximum: Any = None
    min_length: int | None = None
    max_length: int | None = None
    default: Any = MISSING
    example: Any = MISSING
    nullable: bool = False
    # Where a parameter goes, for Op.params: its name in the path, query or
    # headers.
    path: str = ""
    query: str = ""
    header: str = ""


@dataclass(frozen=True)
class OneOf:
    name: str
    shapes: tuple
    doc: str = ""


def one_of(name: str, *shapes: Any, doc: str = "") -> OneOf:
    return OneOf(name, shapes, doc)


_meta: dict[Any, dict[str, str]] = {}


def schema(name: str = "", doc: str = "") -> Any:
    """Name a TypedDict (a components entry), describe it, or both."""

    def mark(cls: Any) -> Any:
        _meta[cls] = {"name": name, "doc": doc}
        return cls

    return mark


def name_of(kind: Any) -> str:
    if isinstance(kind, OneOf):
        return kind.name
    return _meta.get(kind, {}).get("name", "") if isinstance(kind, type) else ""


def is_typed_dict(kind: Any) -> bool:
    return isinstance(kind, type) and typing.is_typeddict(kind)


SCALARS = {"string", "integer", "number", "boolean"}


class Schemas:
    def __init__(self) -> None:
        self.components: dict[str, Any] = {}
        self.building: set[str] = set()

    def of(self, kind: Any) -> dict[str, Any]:
        """The schema for a type: a $ref for a named one."""
        kind, _, _ = unwrap(kind)
        if name_of(kind):
            return self.named(kind)
        return self.inline(kind)

    def named(self, kind: Any) -> dict[str, Any]:
        from app.apidocs import ref

        name = name_of(kind)
        if name not in self.building:
            self.building.add(name)
            self.components[name] = self.inline(kind)
        return ref(name)

    def inline(self, kind: Any) -> dict[str, Any]:
        result = self.shape(kind)
        described = kind.doc if isinstance(kind, OneOf) else (_meta.get(kind, {}).get("doc", "") if isinstance(kind, type) else "")
        if described:
            result["description"] = described
        return result

    def shape(self, kind: Any) -> dict[str, Any]:
        if isinstance(kind, OneOf):
            return {"oneOf": [self.of(shape) for shape in kind.shapes]}
        if kind is datetime:
            return {"type": "string", "format": "date-time"}
        if kind is str:
            return {"type": "string"}
        if kind is bool:
            return {"type": "boolean"}
        if kind is int:
            return {"type": "integer"}
        if kind is float:
            return {"type": "number"}
        origin = get_origin(kind)
        if origin in (list, tuple):
            return {"type": "array", "items": self.of(get_args(kind)[0])}
        if origin is dict:
            result: dict[str, Any] = {"type": "object"}
            value = get_args(kind)[1]
            if value is not Any:
                result["additionalProperties"] = self.of(value)
            return result
        if kind is dict:
            return {"type": "object"}
        if is_typed_dict(kind):
            return self.object(kind)
        return {}

    def object(self, kind: Any) -> dict[str, Any]:
        hints = typing.get_type_hints(kind, include_extras=True)
        parts: list[Any] = []
        inherited: set[str] = set()
        for base in kind.__orig_bases__ if hasattr(kind, "__orig_bases__") else ():
            if is_typed_dict(base) and name_of(base):
                parts.append(self.named(base))
                inherited |= set(typing.get_type_hints(base))
        properties: dict[str, Any] = {}
        required: list[str] = []
        for name, hint in hints.items():
            if name in inherited:
                continue
            properties[name] = self.field(hint, with_doc=True)
            # Read from the hint: with string annotations, __required_keys__
            # cannot see NotRequired.
            if get_origin(hint) is not typing.NotRequired:
                required.append(name)
        own: dict[str, Any] = {"type": "object"}
        if properties:
            own["properties"] = properties
        if required:
            own["required"] = sorted(required)
        if not parts:
            return own
        if properties:
            parts.append(own)
        return {"allOf": parts}

    def field(self, hint: Any, with_doc: bool) -> dict[str, Any]:
        """A field's schema, with its Field applied. A parameter's doc
        describes the parameter rather than its schema, so with_doc is False
        for those."""
        kind, meta, nullable = unwrap(hint)
        result = dict(self.of(kind))
        nullable = nullable or meta.nullable
        if not ({"$ref", "oneOf", "allOf"} & set(result)):
            if meta.enum:
                result["enum"] = list(meta.enum)
            if meta.format:
                result["format"] = meta.format
            for key, value in (("minimum", meta.minimum), ("maximum", meta.maximum), ("minLength", meta.min_length), ("maxLength", meta.max_length)):
                if value is not None:
                    result[key] = value
            for key, value in (("default", meta.default), ("example", meta.example)):
                if value is not MISSING:
                    result[key] = value
        if nullable:
            typed = result.get("type")
            if isinstance(typed, str) and typed in SCALARS:
                result["type"] = [typed, "null"]
            else:
                result = {"oneOf": [result, {"type": "null"}]}
        if meta.doc and with_doc:
            result["description"] = meta.doc
        return result


def unwrap(hint: Any) -> tuple[Any, Field, bool]:
    """A hint's type with NotRequired, Annotated and "| None" taken off, its
    Field, and whether it may be null."""
    meta = Field()
    nullable = False
    while True:
        origin = get_origin(hint)
        if origin in (typing.NotRequired, typing.Required):
            hint = get_args(hint)[0]
        elif origin is typing.Annotated:
            args = get_args(hint)
            hint = args[0]
            for extra in args[1:]:
                if isinstance(extra, Field):
                    meta = extra
        elif origin in (Union, types.UnionType) and type(None) in get_args(hint):
            others = [arg for arg in get_args(hint) if arg is not type(None)]
            nullable = True
            hint = others[0]
        else:
            return hint, meta, nullable


def operation(builder: Schemas, method: str, path: str, op: Any) -> tuple[dict[str, Any], list[str]]:
    """One operation of openapi.json, and what is wrong with its docs."""
    from app.apidocs import ERROR_MEANINGS, KEY_ONLY, PUBLIC, SESSION_ONLY, json_content, operation_id, raw_schema, ref

    problems: list[str] = []
    result: dict[str, Any] = {"summary": op.summary, "operationId": operation_id(method, path), "tags": [op.tag]}
    if not op.summary:
        problems.append("no summary")
    if not op.tag:
        problems.append("no tag")
    if op.description:
        result["description"] = op.description
    if op.access == PUBLIC:
        result["security"] = []
    elif op.access == SESSION_ONLY:
        result["security"] = [{"sessionCookie": []}]
    elif op.access == KEY_ONLY:
        result["security"] = [{"apiKey": []}]
    else:
        result["security"] = [{"sessionCookie": []}, {"apiKey": []}]
    if op.role:
        result["x-required-role"] = op.role
    if op.mcp_http:
        result["x-mcp-http"] = op.mcp_http
    found, parameter_problems = parameters(builder, path, op.params)
    problems += parameter_problems
    if found:
        result["parameters"] = found
    if op.body_type:
        result["requestBody"] = {"required": True, "content": {op.body_type: {"schema": raw_schema(op.body_type)}}}
    elif op.body is not None:
        result["requestBody"] = {"required": True, "content": json_content(builder.of(op.body))}
    reply: dict[str, Any] = {"description": op.reply_description or "Success"}
    if op.reply_type:
        reply["content"] = {op.reply_type: {"schema": raw_schema(op.reply_type)}}
    elif op.reply is not None:
        reply["content"] = json_content(builder.of(op.reply))
    else:
        problems.append("no reply")
    responses: dict[str, Any] = {str(op.status): reply}
    for code in (429, 503, *op.errors):
        if code not in ERROR_MEANINGS:
            problems.append(f"error status {code} has no shared response")
            continue
        responses[str(code)] = ref(f"E{code}", "responses")
    result["responses"] = responses
    return result, problems


def parameters(builder: Schemas, path: str, params: Any) -> tuple[list[dict[str, Any]], list[str]]:
    from app.apidocs import SPEC_PARAMETER

    problems: list[str] = []
    declared: dict[str, dict[str, Any]] = {}
    order: list[str] = []
    if params is not None:
        for field_name, hint in typing.get_type_hints(params, include_extras=True).items():
            _, meta, _ = unwrap(hint)
            place, name = "", ""
            for candidate in ("path", "query", "header"):
                if getattr(meta, candidate):
                    place, name = candidate, getattr(meta, candidate)
            if not place:
                problems.append(f"parameter field {field_name} has no path, query or header")
                continue
            parameter: dict[str, Any] = {"name": name, "in": place, "schema": builder.field(hint, with_doc=False)}
            if meta.doc:
                parameter["description"] = meta.doc
            if place == "path":
                parameter["required"] = True
            declared[f"{place} {name}"] = parameter
            order.append(f"{place} {name}")
    result = []
    for name in SPEC_PARAMETER.findall(path):
        key = f"path {name}"
        if key in declared:
            result.append(declared.pop(key))
            continue
        result.append({"name": name, "in": "path", "required": True, "schema": {"type": "string"}})
    for key in order:
        if key in declared:
            if key.startswith("path "):
                problems.append(f"path parameter {key[5:]} is not in the path")
                continue
            result.append(declared[key])
    return result, problems
