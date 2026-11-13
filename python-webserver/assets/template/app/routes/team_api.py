"""The API docs for team.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, TypedDict

from app.apidocs import Field, doc
from app.routes import team
from app.routes.schemas_api import MemberSchema


class ListTeamParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]


class ListTeamReply(TypedDict):
    members: list[MemberSchema]
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    total: int


doc(
    team.list_team,
    summary="Everyone's names",
    tag="Account",
    params=ListTeamParams,
    reply=ListTeamReply,
    errors=(400, 401),
)
