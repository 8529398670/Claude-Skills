# whole-file feature:settings
"""The API docs for settings.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, Any, NotRequired, TypedDict

from app.apidocs import Field, OWNER, SESSION_ONLY, doc
from app.routes import settings
from app.routes.schemas_api import SettingsResultSchema
from app.routes.schemas_api import SettingsSnapshotSchema


doc(
    settings.get_settings,
    summary="config.yaml, as a form",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="settings",
    reply=SettingsSnapshotSchema,
    errors=(404,),
)


class SaveSettingsBody(TypedDict):
    changes: Annotated[dict[str, Any], Field(doc="Setting paths and their new values.")]
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    version: str


doc(
    settings.save_settings,
    summary="Save settings",
    tag="Owner",
    access=SESSION_ONLY,
    role=OWNER,
    feature="settings",
    body=SaveSettingsBody,
    reply=SettingsResultSchema,
    errors=(400, 403, 404, 409),
)
