"""The named shapes the API docs share: components/schemas in config/openapi.json.
See app/apidocs/schema.py for what the fields mean."""
from __future__ import annotations

from datetime import datetime
from typing import Annotated, Any, NotRequired, TypedDict

from app.apidocs import Field, one_of, schema


# >>> feature:api_keys
@schema(name="APIKey")
class APIKeySchema(TypedDict):
    created_at: datetime
    created_by: NotRequired[Annotated[int, Field(doc="The admin who made this key for its owner. Absent if the owner made it.")]]
    created_by_name: NotRequired[str]
    display_name: NotRequired[Annotated[str, Field(doc="The key's owner. On the admin list only.")]]
    expired: bool
    expires_at: NotRequired[datetime]
    id: str
    last_used_at: NotRequired[datetime]
    live: bool
    name: str
    revoked: bool
    revoked_at: NotRequired[datetime]
    role: Annotated[str, Field(enum=("admin", "user"))]
    user_id: int
# <<< feature:api_keys


# >>> feature:files
@schema(name="File")
class FileSchema(TypedDict):
    chunks: int
    created_at: datetime
    id: str
    name: str
    received: int
    size: int
    state: Annotated[str, Field(enum=("uploading", "ready"))]
    type: str
# <<< feature:files


# >>> feature:logs
class IPNamesSchemaNamesItem(TypedDict):
    created_at: NotRequired[datetime]
    name: NotRequired[str]
    target: NotRequired[str]
# <<< feature:logs


# >>> feature:logs
@schema(name="IPNames")
class IPNamesSchema(TypedDict):
    names: NotRequired[list[IPNamesSchemaNamesItem]]
# <<< feature:logs


# >>> feature:invite_links
@schema(name="InviteLink")
class InviteLinkSchema(TypedDict):
    created_by: int
    created_by_name: NotRequired[str]
    expires_at: NotRequired[datetime]
    id: str
    label: str
    live: bool
    max_uses: int
    remaining: int
    revoked: bool
    used: int
# <<< feature:invite_links


# >>> feature:jobs
@schema(name="Job")
class JobSchema(TypedDict):
    interrupted: bool
    interval_seconds: int
    last_error: NotRequired[str]
    last_finished: NotRequired[datetime]
    last_note: NotRequired[str]
    last_ok: NotRequired[bool]
    last_started: NotRequired[datetime]
    name: str
    next_due: datetime
    running: bool
    runs: int
# <<< feature:jobs


# >>> feature:invite_links
@schema(name="JoinPath")
class JoinPathSchema(TypedDict):
    join_path: Annotated[str, Field(doc="The link to hand out, shown in this response only. A credential: treat it like a password.")]
# <<< feature:invite_links


# >>> feature:invite_links
class JoinedSchemaShape1(TypedDict):
    display_name: str
    ok: bool
# <<< feature:invite_links


# >>> feature:invite_links
class JoinedSchemaShape2(TypedDict):
    challenge: str
    two_factor: bool
# <<< feature:invite_links


# >>> feature:invite_links
JoinedSchema = one_of("Joined", JoinedSchemaShape1, JoinedSchemaShape2, doc="Either the sign-in finished, or a two-factor code is needed.")
# <<< feature:invite_links


# >>> feature:logs
@schema(name="LogDay")
class LogDaySchema(TypedDict):
    date: str
    entries: list[dict[str, Any]]
    next: NotRequired[int | None]
# <<< feature:logs


# >>> feature:logs
@schema(name="LogDays")
class LogDaysSchema(TypedDict):
    days: list[dict[str, Any]]
    dropped: NotRequired[int]
    settings: NotRequired[dict[str, Any]]
    total_bytes: NotRequired[int]
# <<< feature:logs


@schema(name="LoginLink", doc="A login link that can still be used. One per account at most.")
class LoginLinkSchema(TypedDict):
    created_at: datetime
    expires_at: Annotated[datetime | None, Field(doc="null when it never expires.")]
    last_used_at: datetime | None
    max_uses: Annotated[int, Field(doc="Sign-ins it allows. 0 when unlimited.")]
    unlimited: bool
    uses: Annotated[int, Field(doc="Sign-ins so far.")]


@schema(name="Me")
class MeSchema(TypedDict):
    account_role: str
    announcement: NotRequired[Annotated[str, Field(doc="server.announcement. Empty when there is none.")]]
    api_key_id: NotRequired[Annotated[str, Field(doc="API keys only.")]]
    api_key_name: NotRequired[Annotated[str, Field(doc="API keys only.")]]
    api_keys_enabled: NotRequired[bool]  # feature:api_keys
    auth: NotRequired[str]
    authenticated: bool
    csrf_token: NotRequired[Annotated[str, Field(doc="Browser sessions only.")]]
    description: NotRequired[Annotated[str, Field(doc="app.description. Empty when there is none.")]]
    display_name: str
    files_enabled: NotRequired[Annotated[bool, Field(doc="files.enabled: whether the Files screen and routes are on.")]]  # feature:files
    has_password: NotRequired[bool]  # feature:passwords
    has_pin: NotRequired[bool]  # feature:invite_links
    id: int
    invite_links_enabled: NotRequired[Annotated[bool, Field(doc="Whether invite links are on.")]]  # feature:invite_links
    is_owner: bool
    login_link: NotRequired[Annotated[LoginLinkSchema | None, Field(doc="Your own live login link, or null.")]]
    logs_enabled: NotRequired[bool]  # feature:logs
    logs_ui_clicks: NotRequired[bool]  # feature:logs
    maintenance: Annotated[str, Field(enum=("none", "read_only", "full"))]  # feature:maintenance
    mcp_enabled: NotRequired[Annotated[bool, Field(doc="server.mcp: whether POST /mcp answers.")]]  # feature:mcp
    mcp_url: NotRequired[Annotated[str, Field(doc="Where to read the MCP reference: config/mcp.json in app.repo, or /api/mcp.json when there is no repo. Only with server.mcp on.")]]  # feature:mcp
    openapi_url: NotRequired[Annotated[str, Field(doc="Where to read this API's reference: config/openapi.json in app.repo, or /api/openapi.json when there is no repo.")]]  # feature:openapi
    password_blocked: NotRequired[bool]  # feature:passwords
    passwords_enabled: NotRequired[bool]  # feature:passwords
    repo: NotRequired[Annotated[str, Field(doc="app.repo without a trailing / or .git: where the source lives. Empty when it is not set.")]]
    role: Annotated[str, Field(doc="The effective role: never higher than an API key's own.")]
    shortcuts: NotRequired[Annotated[dict[str, str], Field(doc="The keyboard shortcuts you changed, by action. Mod is Cmd on a Mac and Ctrl elsewhere.")]]  # feature:shortcuts
    show_about: NotRequired[Annotated[bool, Field(doc="app.show_about: whether the menu has About.")]]  # feature:about
    theme_id: NotRequired[Annotated[str, Field(doc="The theme you use. Empty: the first.")]]  # feature:appearance
    themes: NotRequired[Annotated[list[dict[str, Any]] | None, Field(doc="Your themes (see the Theme schema): your own list, else the app's defaults, else null for the built-in ones.")]]  # feature:appearance
    themes_custom: NotRequired[Annotated[bool, Field(doc="Whether you have your own list rather than following the defaults.")]]  # feature:appearance
    two_factor: NotRequired[bool]  # feature:two_factor
    two_factor_enabled: NotRequired[bool]  # feature:two_factor
    username: NotRequired[str]  # feature:passwords
    version: NotRequired[Annotated[str, Field(doc="The running build. dev when none was stamped.")]]
    webhooks_enabled: NotRequired[Annotated[bool, Field(doc="webhooks.enabled: whether webhooks are sent.")]]  # feature:webhooks


@schema(name="Member")
class MemberSchema(TypedDict):
    display_name: str
    id: int


# >>> feature:api_keys
@schema(name="NewAPIKey")
class NewAPIKeySchema(APIKeySchema):
    credential: Annotated[str, Field(doc="Shown in this response only, and never recoverable.")]
# <<< feature:api_keys


# >>> feature:logs
@schema(name="PruneResult")
class PruneResultSchema(TypedDict):
    bytes_after: NotRequired[int]
    bytes_before: NotRequired[int]
    compressed: NotRequired[int]
    deleted: NotRequired[int]
# <<< feature:logs


@schema(name="Revoked")
class RevokedSchema(TypedDict):
    api_keys: NotRequired[int]
    login_links: NotRequired[int]
    passwords: NotRequired[int]
    sessions: NotRequired[int]


# >>> feature:sessions
@schema(name="Session")
class SessionSchema(TypedDict):
    created_at: datetime
    current: bool
    expires_at: datetime
    id: str
    ip: NotRequired[str]
    ip_name: NotRequired[Annotated[str, Field(doc="What the owner calls this address on the Logs screen. The owner's own list only.")]]  # feature:logs
    label: NotRequired[Annotated[str, Field(doc="The name you gave this device.")]]
    last_seen_at: NotRequired[datetime]
    user_agent: NotRequired[str]
# <<< feature:sessions


# >>> feature:settings
@schema(name="SettingsResult")
class SettingsResultSchema(TypedDict):
    applied: NotRequired[list[str] | None]
    changes: NotRequired[list[dict[str, Any]] | None]
    restart: NotRequired[list[str] | None]
    scripts: NotRequired[list[str] | None]
    unchanged: NotRequired[list[str] | None]
    version: NotRequired[str]
# <<< feature:settings


# >>> feature:settings
class SettingsSnapshotSchemaError(TypedDict):
    at: NotRequired[datetime]
    message: NotRequired[str]
    origin: NotRequired[str]
# <<< feature:settings


# >>> feature:settings
class SettingsSnapshotSchemaSectionsItemKeysItem(TypedDict):
    apply: NotRequired[str]
    default: NotRequired[Any]
    file_only: NotRequired[Annotated[bool, Field(doc="A path: change it in config.yaml, never here.")]]
    help: NotRequired[str]
    in_file: NotRequired[bool]
    key: NotRequired[str]
    locked: NotRequired[bool]
    name: NotRequired[str]
    pending_restart: NotRequired[bool]
    running: NotRequired[Any]
    type: NotRequired[str]
    value: NotRequired[Any]
# <<< feature:settings


# >>> feature:settings
class SettingsSnapshotSchemaSectionsItem(TypedDict):
    help: NotRequired[str]
    keys: NotRequired[list[SettingsSnapshotSchemaSectionsItemKeysItem]]
    name: NotRequired[str]
# <<< feature:settings


# >>> feature:settings
@schema(name="SettingsSnapshot")
class SettingsSnapshotSchema(TypedDict):
    error: NotRequired[SettingsSnapshotSchemaError | None]
    file: NotRequired[str]
    in_container: NotRequired[bool]
    secret_file: NotRequired[str]
    sections: NotRequired[list[SettingsSnapshotSchemaSectionsItem]]
    stale: NotRequired[bool]
    version: NotRequired[Annotated[str, Field(doc="Send it back when saving: a changed file is refused with a 409.")]]
    writable: NotRequired[bool]
# <<< feature:settings


class SignInSchemaShape1(TypedDict):
    ok: bool


class SignInSchemaShape2(TypedDict):
    challenge: str
    two_factor: bool


SignInSchema = one_of("SignIn", SignInSchemaShape1, SignInSchemaShape2, doc="Either the sign-in finished, or a two-factor code is needed: send it with the challenge to /api/login/two-factor.")


# >>> feature:appearance
class ThemeSchemaButton(TypedDict):
    case: NotRequired[Annotated[str, Field(enum=("normal", "upper"))]]
    depth: NotRequired[Annotated[str, Field(enum=("flat", "raised", "glossy"))]]
    shape: NotRequired[Annotated[str, Field(enum=("square", "soft", "rounded", "pill"))]]
    weight: NotRequired[Annotated[str, Field(enum=("normal", "medium", "bold"))]]
# <<< feature:appearance


# >>> feature:appearance
class ThemeSchemaDark(TypedDict):
    accent: Annotated[str, Field(doc="The main color: what you can press.")]
    background: NotRequired[Annotated[str, Field(doc="The page. Left out: the mode's own.")]]
    overrides: NotRequired[Annotated[dict[str, str], Field(doc="Worked-out colors replaced by name: text, accent, accentFg, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, info, infoBorder, border, and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back.")]]
    second: Annotated[str, Field(doc="Says where you are: active nav item, headings, info.")]
    surface: NotRequired[Annotated[str, Field(doc="Cards. Left out: the mode's own.")]]
# <<< feature:appearance


# >>> feature:appearance
class ThemeSchemaLight(TypedDict):
    accent: Annotated[str, Field(doc="The main color: what you can press.")]
    background: NotRequired[Annotated[str, Field(doc="The page. Left out: the mode's own.")]]
    overrides: NotRequired[Annotated[dict[str, str], Field(doc="Worked-out colors replaced by name: text, accent, accentFg, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, info, infoBorder, border, and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back.")]]
    second: Annotated[str, Field(doc="Says where you are: active nav item, headings, info.")]
    surface: NotRequired[Annotated[str, Field(doc="Cards. Left out: the mode's own.")]]
# <<< feature:appearance


# >>> feature:appearance
@schema(name="Theme", doc="A color scheme on the Appearance screen. Colors are #rrggbb. Anything not given is worked out by static/js/theme.js at readable contrast.")
class ThemeSchema(TypedDict):
    accent: Annotated[str, Field(doc="The main color: what you can press.")]
    background: NotRequired[Annotated[str, Field(doc="The page. Left out: the mode's own.")]]
    button: NotRequired[Annotated[ThemeSchemaButton, Field(doc="How the theme's buttons look. A choice left out is the default (rounded, normal, raised, normal).")]]
    dark: NotRequired[Annotated[ThemeSchemaDark, Field(doc="A light theme's colors for dark mode, kept while it is light. Switching the theme to dark swaps them in. A light theme's light (or a dark theme's dark) is dropped.")]]
    id: Annotated[str, Field(doc="Lower-case letters, digits, - and _, up to 32.")]
    light: NotRequired[Annotated[ThemeSchemaLight, Field(doc="A dark theme's colors for light mode, kept while it is dark. Switching the theme to light swaps them in.")]]
    mode: Annotated[str, Field(enum=("light", "dark"))]
    name: Annotated[str, Field(doc="1 to 60 characters.")]
    overrides: NotRequired[Annotated[dict[str, str], Field(doc="Worked-out colors replaced by name: text, accent, accentFg, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, info, infoBorder, border, and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back.")]]
    second: Annotated[str, Field(doc="Says where you are: active nav item, headings, info.")]
    surface: NotRequired[Annotated[str, Field(doc="Cards. Left out: the mode's own.")]]
# <<< feature:appearance


@schema(name="User")
class UserSchema(TypedDict):
    created_at: NotRequired[datetime]
    disabled: bool
    display_name: str
    has_password: NotRequired[bool]  # feature:passwords
    id: int
    login_link: NotRequired[Annotated[LoginLinkSchema | None, Field(doc="The account's live login link, or null.")]]
    owner: bool
    password_blocked: NotRequired[bool]  # feature:passwords
    role: Annotated[str, Field(enum=("admin", "user"))]
    two_factor: NotRequired[bool]  # feature:two_factor
    username: NotRequired[Annotated[str, Field(doc="Empty until a password is set.")]]  # feature:passwords


# >>> feature:webhooks
@schema(name="Webhook")
class WebhookSchema(TypedDict):
    created_at: datetime
    enabled: bool
    events: list[str]
    id: str
    name: str
    url: str
# <<< feature:webhooks


# >>> feature:webhooks
@schema(name="WebhookDelivery")
class WebhookDeliverySchema(TypedDict):
    attempts: int
    created_at: datetime
    event: str
    finished_at: NotRequired[datetime]
    id: str
    last_error: NotRequired[str]
    last_status: NotRequired[int]
    next_attempt_at: NotRequired[datetime]
    status: str
# <<< feature:webhooks
