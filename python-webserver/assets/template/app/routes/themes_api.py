# whole-file feature:appearance
"""The API docs for themes.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, NotRequired, TypedDict

from app.apidocs import ADMIN, Field, doc
from app.routes import themes
from app.routes.schemas_api import ThemeSchema


class SetMyThemeBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    theme_id: Annotated[str, Field(doc="A theme's id. Empty for the first in the list.")]


class SetMyThemeReply(TypedDict):
    ok: bool
    theme_id: str


doc(
    themes.set_my_theme,
    summary="Pick which theme you use",
    tag="Account",
    feature="appearance",
    body=SetMyThemeBody,
    reply=SetMyThemeReply,
    errors=(400, 401, 403),
)


class SetMyThemesBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    theme_id: NotRequired[Annotated[str, Field(doc="The theme you use.")]]
    themes: NotRequired[Annotated[list[ThemeSchema] | None, Field(doc="Your whole list, 1 to 40 themes. null goes back to following the app's defaults.")]]


class SetMyThemesReply(TypedDict):
    ok: bool
    theme_id: str
    themes: Annotated[list[ThemeSchema] | None, Field(doc="What you now see; null while the built-in themes apply.")]
    themes_custom: bool


doc(
    themes.set_my_themes,
    summary="Replace your own themes",
    tag="Account",
    feature="appearance",
    body=SetMyThemesBody,
    reply=SetMyThemesReply,
    errors=(400, 401, 403),
)


class GetThemeDefaultsReply(TypedDict):
    themes: Annotated[list[ThemeSchema] | None, Field(doc="null while the built-in themes apply.")]


doc(
    themes.get_theme_defaults,
    summary="The themes everyone without their own list sees",
    tag="Admin",
    role=ADMIN,
    feature="appearance",
    reply=GetThemeDefaultsReply,
    errors=(401, 403, 404),
)


class SetThemeDefaultsBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    themes: NotRequired[Annotated[list[ThemeSchema] | None, Field(doc="1 to 40 themes. null goes back to the built-in ones.")]]


class SetThemeDefaultsReply(TypedDict):
    ok: bool
    themes: list[ThemeSchema] | None


doc(
    themes.set_theme_defaults,
    summary="Replace the default themes",
    tag="Admin",
    role=ADMIN,
    feature="appearance",
    body=SetThemeDefaultsBody,
    reply=SetThemeDefaultsReply,
    errors=(400, 401, 403, 404),
)
