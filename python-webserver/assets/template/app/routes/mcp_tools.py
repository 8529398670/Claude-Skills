# whole-file feature:mcp
"""The tools /mcp offers (see app/mcp for the protocol and the rules a tool
follows). Every tool stands for a declared route, Tool.for_route(...): its
arguments are the route's own request type, and it runs the route's own
handler, so the two doors answer the same way and refuse for the same
reasons. A tool that adds to the route's answer (files_start_upload's
chunk_url, say) passes a call that builds on it.

The rule: every route an API key may call has a tool here, or an mcp_http
reason in its @route(...) for staying HTTP only.
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

from typing import Any  # feature:files

import msgspec  # feature:files

from app.apidocs.route import declared_of  # feature:files
from app.core.config import Config  # feature:invite_links|files
from app.core.config import FILE_CHUNK_BYTES  # feature:files
from app.mcp.registry import Registry, Tool
from app.mcp.registry import Call, Refusal  # feature:files
from app.routes import files  # feature:files
from app.routes import shortcuts  # feature:shortcuts
from app.routes import themes  # feature:appearance
from app.routes import account, admin, team
from app.routes import invite_links  # feature:invite_links


# >>> feature:files
def _files_on(cfg: Config) -> bool:
    return cfg.files.enabled


def _file_url(call: Call, path: str) -> str:
    """Where a file route is, from the address this agent reached /mcp at: the
    bytes of a file go there over plain HTTP with the same key, never through
    an MCP message (clients cap those well below a file's size)."""
    return str(call.request.base_url).rstrip("/") + path


def files_start_upload(call: Call, body: files.CreateFile) -> dict[str, Any]:
    """The route's answer, with where each piece goes and a shell recipe that
    sends them all."""
    row = msgspec.to_builtins(declared_of(files.create_file).serve(call.request, body))
    chunk_url = _file_url(call, f"/api/files/{row['id']}/chunks/{{index}}")
    base = _file_url(call, f"/api/files/{row['id']}/chunks/")
    curl = (
        f"for ((i = 0; i < {row['chunks']}; i++)); do dd if=FILE bs={FILE_CHUNK_BYTES} skip=$i count=1 2>/dev/null | "
        f"curl -sf -o /dev/null -H \"Authorization: Bearer $KEY\" -H 'Content-Type: application/octet-stream' --data-binary @- \"{base}$i\"; done"
    )
    return {**row, "chunk_url": chunk_url, "finish_tool": "files_finish_upload", "curl": curl}


def files_download_url(call: Call, body: files.FileIDPath) -> dict[str, Any]:
    """Where to GET a file, for the download route, whose bytes cannot come
    through MCP."""
    row = declared_of(files.get_my_file).serve(call.request, body)
    if row.state != files.models.READY:
        raise Refusal("that file has not finished uploading")
    return {"id": body.file_id, "name": row.name, "size": row.size, "url": _file_url(call, f"/api/files/{body.file_id}/download")}


# <<< feature:files
# >>> feature:invite_links
def _invite_links_on(cfg: Config) -> bool:
    return cfg.invite_links_enabled


# <<< feature:invite_links
def build_registry() -> Registry:
    registry = Registry()
    registry.register(Tool.for_route(
        account.me,
        name="whoami",
        title="Who am I",
        description="The account this API key belongs to, the role it acts with (role -- never higher than the key's own scope), and the account's own role.",
        read_only=True,
    ))
    registry.register(Tool.for_route(
        team.list_team,
        name="list_team",
        title="List team",
        description="People with an active account, a page at a time: id and display name. Use the id wherever another tool asks for a user. next is the offset of the following page, or null on the last one.",
        read_only=True,
    ))
    registry.register(Tool.for_route(
        admin.list_users,
        name="admin_list_users",
        title="List users (admin)",
        description="Accounts, a page at a time, including disabled ones, with each one's role and which is the owner. next is the offset of the following page, or null on the last one.",
        read_only=True,
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        admin.set_user_disabled,
        name="admin_set_user_disabled",
        title="Disable or enable a user (admin)",
        description="Switch an account off (disabled: true) or back on (false). Disabling signs it out everywhere and voids its live login link; its API keys stop working while it is off. Reversible. The owner and your own account cannot be disabled.",
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        admin.revoke_access,
        name="admin_revoke_access",
        title="Revoke a user's access (admin)",
        description="For an account that may be compromised: ends every session and voids its live login link and every API key it holds, at once. The account stays enabled; admin_reissue_login lets its owner back in. The owner and your own account cannot be revoked.",
        destructive=True,
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        account.rename,
        name="rename_account",
        title="Rename your account",
        description="Change your own display name (1-80 characters). expect, when given, is the name you think it has now; if someone changed it meanwhile the rename is refused with the current name, so nothing is overwritten unseen.",
    ))
    # >>> feature:shortcuts
    registry.register(Tool.for_route(
        shortcuts.set_my_shortcuts,
        name="set_shortcuts",
        title="Change your keyboard shortcuts",
        description="Replace your changed keyboard shortcuts: action name to binding, such as \"search\": \"Mod+K\" (Mod is Cmd on a Mac, Ctrl elsewhere). Send the whole set; an empty binding switches a key off, and {} puts every key back to its default. whoami shows the current set.",
    ))
    # <<< feature:shortcuts
    # >>> feature:appearance
    registry.register(Tool.for_route(
        themes.set_my_themes,
        name="set_themes",
        title="Replace your own themes",
        description="Replace your whole list of color themes (1 to 40) and pick the one you use. themes: null goes back to following the app's defaults. A theme is the Theme schema in the API reference (config/openapi.json): id, name, mode light|dark, accent, second, and optional colors as #rrggbb. whoami shows the current list.",
    ))
    registry.register(Tool.for_route(
        themes.set_my_theme,
        name="set_theme",
        title="Pick your theme",
        description="Switch to another theme from your list (or the defaults), by its id.",
    ))
    registry.register(Tool.for_route(
        themes.get_theme_defaults,
        name="admin_get_theme_defaults",
        title="Default themes (admin)",
        description="The theme list everyone without their own list sees. themes is null while the built-in ones apply.",
        read_only=True,
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        themes.set_theme_defaults,
        name="admin_set_theme_defaults",
        title="Replace the default themes (admin)",
        description="Replace the theme list everyone without their own list sees, 1 to 40 themes in the Theme schema of the API reference. null puts the built-in themes back.",
        admin_only=True,
    ))
    # <<< feature:appearance
    # Imported here: it builds on this module's helpers.
    from app.routes import mcp_tools_accounts

    mcp_tools_accounts.register(registry)
    # >>> feature:files
    registry.register(Tool.for_route(
        files.list_my_files,
        name="files_list",
        title="List your files",
        description="Your uploaded files, a page at a time: id, name, type, size, state (uploading or ready) and when. next is the offset of the following page, or null on the last one.",
        read_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.get_my_file,
        name="files_get",
        title="One of your files",
        description="One file: its details, and for an unfinished upload how many pieces have arrived (received), which is where to resume.",
        read_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.create_file,
        call=files_start_upload,
        name="files_start_upload",
        title="Start uploading a file",
        description="Reserve room for a file and get where to send it. The bytes do not go through MCP: POST each piece of chunk_bytes bytes (the last may be shorter), in order from index 0, as the raw body (Content-Type: application/octet-stream) to chunk_url with {index} filled in and the same Authorization: Bearer header this MCP connection uses. curl is a shell recipe that does all of it. Then call files_finish_upload.",
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.finish_file,
        name="files_finish_upload",
        title="Finish an upload",
        description="Seal an upload once every piece has arrived; the file is then ready to download. Asked again, it answers the same. If pieces are missing it says how many arrived (received).",
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.download_my_file,
        call=files_download_url,
        name="files_download_url",
        title="Where to download a file",
        description="The address of one of your files. GET it over HTTP with the same Authorization: Bearer header this MCP connection uses; the bytes do not come through MCP.",
        read_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.rename_my_file,
        name="files_rename",
        title="Rename a file",
        description="Give one of your files a new name. expect, when given, is the name you think it has now; a rename made meanwhile is refused with the current name.",
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.delete_my_file,
        name="files_delete",
        title="Delete a file",
        description="Move one of your files to the bin. files_restore brings it back for a while; after that it is gone.",
        destructive=True,
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.restore_my_file,
        name="files_restore",
        title="Restore a deleted file",
        description="Bring back a file you deleted, while it is still in the bin and your storage quota has room.",
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.my_storage,
        name="my_storage",
        title="Your storage",
        description="How many bytes your files use, and your quota (0 is no limit).",
        read_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.list_all_files,
        name="admin_list_files",
        title="List everyone's files (admin)",
        description="Every account's files, a page at a time, with whose each one is (user_id, user_name). next is the offset of the following page, or null on the last one.",
        read_only=True,
        admin_only=True,
        enabled=_files_on,
    ))
    registry.register(Tool.for_route(
        files.all_storage,
        name="admin_storage",
        title="Storage for everyone (admin)",
        description="How many bytes everyone's files use together, and the disk's free and total bytes where the system says.",
        read_only=True,
        admin_only=True,
        enabled=_files_on,
    ))
    # <<< feature:files
    # >>> feature:invite_links
    registry.register(Tool.for_route(
        invite_links.list_invite_links,
        name="admin_list_invite_links",
        title="List invite links (admin)",
        description="Group invite links, a page at a time: label, places used and left, expiry, and whether each still works. The links themselves are never shown again after creation. next is the offset of the following page, or null on the last one.",
        read_only=True,
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool.for_route(
        invite_links.revoke_invite_link,
        name="admin_revoke_invite_link",
        title="Switch off an invite link (admin)",
        description="Stop an invite link from letting anyone else join or sign in through it. Accounts it already made keep working. Cannot be switched back on.",
        destructive=True,
        admin_only=True,
        enabled=_invite_links_on,
    ))
    # <<< feature:invite_links
    return registry
