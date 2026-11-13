# whole-file feature:mcp
"""The tools /mcp offers (see app/mcp for the protocol and the rules a tool
follows). Each one is a thin wrapper over the helper its HTTP route uses --
me_row, team_rows, set_disabled_checked, create_file_checked, ... -- so the two
doors answer the same way and refuse for the same reasons.

The rule: every route an API key may call has a tool here, named in its route,
or an mcp_http reason in its doc() for staying HTTP only.
app/tests/test_mcp_parity.py fails until that holds. What a key may not call --
managing API keys, setting your own password, PIN or two-factor, other people's
files, and the owner's screens -- needs a browser session (see
RequireSession), so it has no tool either. Accounts, login links, devices and
invite links are in mcp_tools_accounts.py.

After adding or changing a tool, run scripts/api_docs.sh to rewrite
config/mcp.json and config/api.md; app/tests/test_api_docs.py fails until
they are current.

An app's own tools go in a new module in this package (mcp_tools_<feature>.py)
with a register(registry) function called from build_registry below.
"""
from __future__ import annotations

from collections.abc import Callable
from typing import Any

from app.core.config import Config  # feature:invite_links|files
from app.core.config import FILE_CHUNK_BYTES  # feature:files
from app.core.db import Store
from app.mcp.registry import Call, Refusal, Registry, Tool
from app.models import invite_links  # feature:invite_links
from app.models import webhooks  # feature:webhooks
from app.routes import files  # feature:files
from app.routes import shortcuts  # feature:shortcuts
from app.routes import themes  # feature:appearance
from app.routes.account import me_row, rename_checked
from app.routes.admin import RuleRefusal, revoke_access_checked, set_disabled_checked, user_rows
from app.routes.helpers import PAGE_DEFAULT, PAGE_LIMIT_MESSAGE, PAGE_MAX, PAGE_OFFSET_MESSAGE, Refused, page_body
from app.routes.invite_links import invite_link_rows  # feature:invite_links
from app.routes.team import team_rows

USER_ID_SCHEMA = {"type": "integer", "minimum": 1, "description": "The account's id, from list_team or admin_list_users."}


# Both optional, so a paged tool still works with no arguments.
PAGE_SCHEMA = {
    "type": "object",
    "properties": {
        "limit": {"type": "integer", "minimum": 1, "maximum": PAGE_MAX, "description": "Rows to return. Default 200."},
        "offset": {"type": "integer", "minimum": 0, "description": "Rows to skip: the next value of the previous page."},
    },
    "additionalProperties": False,
}


def _schema(properties: dict[str, Any]) -> dict[str, Any]:
    return {"type": "object", "properties": properties, "required": list(properties), "additionalProperties": False}


def _no_args(call: Call) -> None:
    call.bind({})


def _shared(helper: Callable[..., Any], *args: Any) -> Any:
    """A helper its HTTP route shares, with its refusal in the same words."""
    try:
        return helper(*args)
    except Refused as refused:
        if refused.status >= 500:
            raise RuntimeError(refused.cause) from None
        raise Refusal(refused.message) from None


def _page(call: Call, name: str, load: Callable[[Store], list[Any]]) -> dict[str, Any]:
    """send_page for a tool, so the two doors page the same way."""
    args = call.bind({}, optional={"limit": int, "offset": int})
    limit, offset = args.get("limit", PAGE_DEFAULT), args.get("offset", 0)
    if not 1 <= limit <= PAGE_MAX:
        raise Refusal(PAGE_LIMIT_MESSAGE)
    if offset < 0:
        raise Refusal(PAGE_OFFSET_MESSAGE)
    return page_body(name, load(call.store), limit, offset)


def whoami(call: Call) -> dict[str, Any]:
    _no_args(call)
    return me_row(call.store, call.config, call.actor, call.request.app.state.version)


def list_team(call: Call) -> dict[str, Any]:
    return _page(call, "members", team_rows)


def admin_list_users(call: Call) -> dict[str, Any]:
    return _page(call, "users", user_rows)


def admin_set_user_disabled(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int, "disabled": bool})
    refused = set_disabled_checked(call.store, call.actor.user.id, args["user_id"], args["disabled"])
    if refused is not None:
        raise Refusal(refused.message)
    call.request.app.state.webhooks.emit(webhooks.EVENT_USER_DISABLED, {"user_id": args["user_id"], "disabled": args["disabled"]})  # feature:webhooks
    return {"ok": True}


def admin_revoke_access(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int})
    revoked = revoke_access_checked(call.store, call.actor.user.id, args["user_id"])
    if isinstance(revoked, RuleRefusal):
        raise Refusal(revoked.message)
    return {"ok": True, "revoked": revoked.to_dict()}


def rename_account(call: Call) -> dict[str, Any]:
    args = call.bind({"display_name": str}, optional={"expect": str})
    return _shared(rename_checked, call.request, args["display_name"], args.get("expect"))


# >>> feature:shortcuts
def set_shortcuts(call: Call) -> dict[str, Any]:
    args = call.bind({"shortcuts": dict})
    return _shared(shortcuts.set_shortcuts_checked, call.request, args["shortcuts"])


# <<< feature:shortcuts
# >>> feature:appearance
def set_themes(call: Call) -> dict[str, Any]:
    args = call.bind({"themes": object}, optional={"theme_id": str})
    return _shared(themes.set_my_themes_checked, call.request, args["themes"], args.get("theme_id"))


def set_theme(call: Call) -> dict[str, Any]:
    args = call.bind({"theme_id": str})
    return _shared(themes.set_my_theme_checked, call.request, args["theme_id"])


def admin_get_theme_defaults(call: Call) -> dict[str, Any]:
    _no_args(call)
    return themes.theme_defaults(call.request)


def admin_set_theme_defaults(call: Call) -> dict[str, Any]:
    args = call.bind({"themes": object})
    return _shared(themes.set_theme_defaults_checked, call.request, args["themes"])


# <<< feature:appearance
# >>> feature:files
FILE_ID_SCHEMA = {"type": "string", "description": "The file's id, from files_list or files_start_upload."}


def _files_on(cfg: Config) -> bool:
    return cfg.files.enabled


def _file_url(call: Call, path: str) -> str:
    """Where a file route is, from the address this agent reached /mcp at: the
    bytes of a file go there over plain HTTP with the same key, never through
    an MCP message (clients cap those well below a file's size)."""
    return str(call.request.base_url).rstrip("/") + path


def files_list(call: Call) -> dict[str, Any]:
    return _page(call, "files", lambda _store: files.my_file_rows(call.request))


def files_get(call: Call) -> dict[str, Any]:
    args = call.bind({"file_id": str})
    return _shared(files.get_my_file_row, call.request, args["file_id"])


def files_start_upload(call: Call) -> dict[str, Any]:
    args = call.bind({"name": str, "size": int}, optional={"type": str})
    row = _shared(files.create_file_checked, call.request, args["name"], args.get("type", ""), args["size"])
    chunk_url = _file_url(call, f"/api/files/{row['id']}/chunks/{{index}}")
    base = _file_url(call, f"/api/files/{row['id']}/chunks/")
    curl = (
        f"for ((i = 0; i < {row['chunks']}; i++)); do dd if=FILE bs={FILE_CHUNK_BYTES} skip=$i count=1 2>/dev/null | "
        f"curl -sf -o /dev/null -H \"Authorization: Bearer $KEY\" -H 'Content-Type: application/octet-stream' --data-binary @- \"{base}$i\"; done"
    )
    return {**row, "chunk_url": chunk_url, "finish_tool": "files_finish_upload", "curl": curl}


def files_finish_upload(call: Call) -> dict[str, Any]:
    args = call.bind({"file_id": str})
    return _shared(files.finish_file_checked, call.request, args["file_id"])


def files_download_url(call: Call) -> dict[str, Any]:
    args = call.bind({"file_id": str})
    row = _shared(files.get_my_file_row, call.request, args["file_id"])
    if row["state"] != files.models.READY:
        raise Refusal("that file has not finished uploading")
    return {"id": args["file_id"], "name": row["name"], "size": row["size"], "url": _file_url(call, f"/api/files/{args['file_id']}/download")}


def files_rename(call: Call) -> dict[str, Any]:
    args = call.bind({"file_id": str, "name": str}, optional={"expect": str})
    return _shared(files.rename_file_checked, call.request, args["file_id"], args["name"], args.get("expect"), True)


def files_delete(call: Call) -> dict[str, Any]:
    args = call.bind({"file_id": str})
    return _shared(files.delete_file_checked, call.request, args["file_id"], True)


def files_restore(call: Call) -> dict[str, Any]:
    args = call.bind({"file_id": str})
    return _shared(files.restore_file_checked, call.request, args["file_id"], False)


def my_storage(call: Call) -> dict[str, Any]:
    _no_args(call)
    return files.my_storage_row(call.request)


def admin_list_files(call: Call) -> dict[str, Any]:
    return _page(call, "files", lambda _store: files.all_file_rows(call.request))


def admin_storage(call: Call) -> dict[str, Any]:
    _no_args(call)
    return files.all_storage_row(call.request)


# <<< feature:files
# >>> feature:invite_links
def _invite_links_on(cfg: Config) -> bool:
    return cfg.invite_links_enabled


def admin_list_invite_links(call: Call) -> dict[str, Any]:
    return _page(call, "invite_links", invite_link_rows)


def admin_revoke_invite_link(call: Call) -> dict[str, Any]:
    args = call.bind({"link_id": str})
    if not invite_links.revoke_invite_link(call.store, args["link_id"]):
        raise Refusal("no such invite link")
    call.request.app.state.webhooks.emit(webhooks.EVENT_INVITE_LINK_REVOKED, {"link_id": args["link_id"]})  # feature:webhooks
    return {"ok": True}


# <<< feature:invite_links
def build_registry() -> Registry:
    registry = Registry()
    registry.register(Tool(
        name="whoami",
        route="GET /api/me",
        title="Who am I",
        description="The account this API key belongs to, the role it acts with (role -- never higher than the key's own scope), and the account's own role.",
        call=whoami,
        read_only=True,
    ))
    registry.register(Tool(
        name="list_team",
        route="GET /api/team",
        title="List team",
        description="People with an active account, a page at a time: id and display name. Use the id wherever another tool asks for a user. next is the offset of the following page, or null on the last one.",
        call=list_team,
        input_schema=PAGE_SCHEMA,
        read_only=True,
    ))
    registry.register(Tool(
        name="admin_list_users",
        route="GET /api/admin/users",
        title="List users (admin)",
        description="Accounts, a page at a time, including disabled ones, with each one's role and which is the owner. next is the offset of the following page, or null on the last one.",
        call=admin_list_users,
        input_schema=PAGE_SCHEMA,
        read_only=True,
        admin_only=True,
    ))
    registry.register(Tool(
        name="admin_set_user_disabled",
        route="POST /api/admin/users/{user_id}/disabled",
        title="Disable or enable a user (admin)",
        description="Switch an account off (disabled: true) or back on (false). Disabling signs it out everywhere and voids its live login link; its API keys stop working while it is off. Reversible. The owner and your own account cannot be disabled.",
        call=admin_set_user_disabled,
        input_schema=_schema({"user_id": USER_ID_SCHEMA, "disabled": {"type": "boolean"}}),
        admin_only=True,
    ))
    registry.register(Tool(
        name="admin_revoke_access",
        route="POST /api/admin/users/{user_id}/revoke-access",
        title="Revoke a user's access (admin)",
        description="For an account that may be compromised: ends every session and voids its live login link and every API key it holds, at once. The account stays enabled; admin_reissue_login lets its owner back in. The owner and your own account cannot be revoked.",
        call=admin_revoke_access,
        input_schema=_schema({"user_id": USER_ID_SCHEMA}),
        destructive=True,
        admin_only=True,
    ))
    registry.register(Tool(
        name="rename_account",
        route="POST /api/account/rename",
        title="Rename your account",
        description="Change your own display name (1-80 characters). expect, when given, is the name you think it has now; if someone changed it meanwhile the rename is refused with the current name, so nothing is overwritten unseen.",
        call=rename_account,
        input_schema={
            "type": "object",
            "properties": {"display_name": {"type": "string"}, "expect": {"type": "string", "description": "The current name, from whoami. Optional."}},
            "required": ["display_name"],
            "additionalProperties": False,
        },
    ))
    # >>> feature:shortcuts
    registry.register(Tool(
        name="set_shortcuts",
        route="POST /api/account/shortcuts",
        title="Change your keyboard shortcuts",
        description="Replace your changed keyboard shortcuts: action name to binding, such as \"search\": \"Mod+K\" (Mod is Cmd on a Mac, Ctrl elsewhere). Send the whole set; an empty binding switches a key off, and {} puts every key back to its default. whoami shows the current set.",
        call=set_shortcuts,
        input_schema=_schema({"shortcuts": {"type": "object", "additionalProperties": {"type": "string"}}}),
    ))
    # <<< feature:shortcuts
    # >>> feature:appearance
    registry.register(Tool(
        name="set_themes",
        route="POST /api/account/themes",
        title="Replace your own themes",
        description="Replace your whole list of color themes (1 to 40) and pick the one you use. themes: null goes back to following the app's defaults. A theme is the Theme schema in the API reference (config/openapi.json): id, name, mode light|dark, accent, second, and optional colors as #rrggbb. whoami shows the current list.",
        call=set_themes,
        input_schema={
            "type": "object",
            "properties": {"themes": {"type": ["array", "null"], "items": {"type": "object"}}, "theme_id": {"type": "string"}},
            "required": ["themes"],
            "additionalProperties": False,
        },
    ))
    registry.register(Tool(
        name="set_theme",
        route="POST /api/account/theme",
        title="Pick your theme",
        description="Switch to another theme from your list (or the defaults), by its id.",
        call=set_theme,
        input_schema=_schema({"theme_id": {"type": "string"}}),
    ))
    registry.register(Tool(
        name="admin_get_theme_defaults",
        route="GET /api/admin/theme-defaults",
        title="Default themes (admin)",
        description="The theme list everyone without their own list sees. themes is null while the built-in ones apply.",
        call=admin_get_theme_defaults,
        read_only=True,
        admin_only=True,
    ))
    registry.register(Tool(
        name="admin_set_theme_defaults",
        route="POST /api/admin/theme-defaults",
        title="Replace the default themes (admin)",
        description="Replace the theme list everyone without their own list sees, 1 to 40 themes in the Theme schema of the API reference. null puts the built-in themes back.",
        call=admin_set_theme_defaults,
        input_schema=_schema({"themes": {"type": ["array", "null"], "items": {"type": "object"}}}),
        admin_only=True,
    ))
    # <<< feature:appearance
    # Imported here: it builds on this module's helpers.
    from app.routes import mcp_tools_accounts

    mcp_tools_accounts.register(registry)
    # >>> feature:files
    file_id_only = _schema({"file_id": FILE_ID_SCHEMA})
    registry.register(Tool(
        name="files_list",
        route="GET /api/files",
        title="List your files",
        description="Your uploaded files, a page at a time: id, name, type, size, state (uploading or ready) and when. next is the offset of the following page, or null on the last one.",
        call=files_list,
        input_schema=PAGE_SCHEMA,
        read_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="files_get",
        route="GET /api/files/{file_id}",
        title="One of your files",
        description="One file: its details, and for an unfinished upload how many pieces have arrived (received), which is where to resume.",
        call=files_get,
        input_schema=file_id_only,
        read_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="files_start_upload",
        route="POST /api/files",
        title="Start uploading a file",
        description="Reserve room for a file and get where to send it. The bytes do not go through MCP: POST each piece of chunk_bytes bytes (the last may be shorter), in order from index 0, as the raw body (Content-Type: application/octet-stream) to chunk_url with {index} filled in and the same Authorization: Bearer header this MCP connection uses. curl is a shell recipe that does all of it. Then call files_finish_upload.",
        call=files_start_upload,
        input_schema={
            "type": "object",
            "properties": {
                "name": {"type": "string", "description": "The file's name, as it should be shown."},
                "size": {"type": "integer", "minimum": 1, "description": "Exact size in bytes."},
                "type": {"type": "string", "description": "Its media type, such as application/pdf. Optional."},
            },
            "required": ["name", "size"],
            "additionalProperties": False,
        },
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="files_finish_upload",
        route="POST /api/files/{file_id}/finish",
        title="Finish an upload",
        description="Seal an upload once every piece has arrived; the file is then ready to download. Asked again, it answers the same. If pieces are missing it says how many arrived (received).",
        call=files_finish_upload,
        input_schema=file_id_only,
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="files_download_url",
        route="GET /api/files/{file_id}/download",
        title="Where to download a file",
        description="The address of one of your files. GET it over HTTP with the same Authorization: Bearer header this MCP connection uses; the bytes do not come through MCP.",
        call=files_download_url,
        input_schema=file_id_only,
        read_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="files_rename",
        route="POST /api/files/{file_id}/rename",
        title="Rename a file",
        description="Give one of your files a new name. expect, when given, is the name you think it has now; a rename made meanwhile is refused with the current name.",
        call=files_rename,
        input_schema={
            "type": "object",
            "properties": {"file_id": FILE_ID_SCHEMA, "name": {"type": "string"}, "expect": {"type": "string"}},
            "required": ["file_id", "name"],
            "additionalProperties": False,
        },
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="files_delete",
        route="POST /api/files/{file_id}/delete",
        title="Delete a file",
        description="Move one of your files to the bin. files_restore brings it back for a while; after that it is gone.",
        call=files_delete,
        input_schema=file_id_only,
        destructive=True,
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="files_restore",
        route="POST /api/files/{file_id}/restore",
        title="Restore a deleted file",
        description="Bring back a file you deleted, while it is still in the bin and your storage quota has room.",
        call=files_restore,
        input_schema=file_id_only,
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="my_storage",
        route="GET /api/account/storage",
        title="Your storage",
        description="How many bytes your files use, and your quota (0 is no limit).",
        call=my_storage,
        read_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="admin_list_files",
        route="GET /api/admin/files",
        title="List everyone's files (admin)",
        description="Every account's files, a page at a time, with whose each one is (user_id, user_name). next is the offset of the following page, or null on the last one.",
        call=admin_list_files,
        input_schema=PAGE_SCHEMA,
        read_only=True,
        admin_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool(
        name="admin_storage",
        route="GET /api/admin/storage",
        title="Storage for everyone (admin)",
        description="How many bytes everyone's files use together, and the disk's free and total bytes where the system says.",
        call=admin_storage,
        read_only=True,
        admin_only=True,
        enabled=_files_on,
    ))
    # <<< feature:files
    # >>> feature:invite_links
    registry.register(Tool(
        name="admin_list_invite_links",
        route="GET /api/admin/invite-links",
        title="List invite links (admin)",
        description="Group invite links, a page at a time: label, places used and left, expiry, and whether each still works. The links themselves are never shown again after creation. next is the offset of the following page, or null on the last one.",
        call=admin_list_invite_links,
        input_schema=PAGE_SCHEMA,
        read_only=True,
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool(
        name="admin_revoke_invite_link",
        route="POST /api/admin/invite-links/{link_id}/revoke",
        title="Switch off an invite link (admin)",
        description="Stop an invite link from letting anyone else join or sign in through it. Accounts it already made keep working. Cannot be switched back on.",
        call=admin_revoke_invite_link,
        input_schema=_schema({"link_id": {"type": "string", "description": "The id from admin_list_invite_links."}}),
        destructive=True,
        admin_only=True,
        enabled=_invite_links_on,
    ))
    # <<< feature:invite_links
    return registry
