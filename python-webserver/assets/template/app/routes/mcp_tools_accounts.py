# whole-file feature:mcp
"""The MCP side of accounts and sign-in: login links, accounts, devices and
invite links. Each tool stands for its HTTP route and runs the route's own
handler (Tool.for_route), so the rules (who may touch the owner, what a
narrowed key may not do) hold whichever door a request comes through.

A login link or invite link made here records the key that asked, and revoking
or deleting that key voids it while it can still be used (see
login_tokens.void_links_minted_by_key). That is what keeps revoking a leaked
key meaningful now that a key can hand out ways in.

The descriptions and schemas match the Go template's mcp_tools_accounts.go
word for word: both write the same config/mcp.json.
"""
from __future__ import annotations

from typing import Any

import msgspec

from app.apidocs.route import declared_of
from app.core.config import Config  # feature:two_factor|invite_links
from app.mcp.registry import Call, Registry, Tool
from app.routes import account, admin
from app.routes import invite_links  # feature:invite_links
from app.routes import sessions  # feature:sessions

LOGIN_LINK_ANSWER = (
    "Answers with login_url (open it to sign in; it works once unless max_uses says otherwise), expires_at and max_uses. "
    "Any older link the account could still use stops working; its signed-in devices stay signed in. "
    "Revoking or deleting this API key voids the link while it is unused, and signs out every device it signed in."
)


def _base_url(call: Call) -> str:
    """The address this agent reached /mcp at, the way _file_url uses it."""
    return str(call.request.base_url).rstrip("/")


def _with_login_url(call: Call, payload: dict[str, Any]) -> dict[str, Any]:
    """A login-link payload with the full address added. The HTTP routes
    answer with login_path only (see admin.create_user for why)."""
    return {**payload, "login_url": _base_url(call) + payload["login_path"]}


def _with_route_login_url(endpoint: Any) -> Any:
    """The call of a tool for a route that mints a login link: the route's
    reply with login_url added."""
    declared = declared_of(endpoint)

    def call(tool_call: Call, body: Any) -> dict[str, Any]:
        return _with_login_url(tool_call, msgspec.to_builtins(declared.serve(tool_call.request, body)))

    return call


# >>> feature:two_factor
def _two_factor_on(cfg: Config) -> bool:
    return cfg.two_factor_enabled


# <<< feature:two_factor
# >>> feature:invite_links
def _invite_links_on(cfg: Config) -> bool:
    return cfg.invite_links_enabled


def _with_join_url(tool_call: Call, body: Any) -> dict[str, Any]:
    """admin_create_invite_link's answer: the route's, with the full address
    added, as _with_login_url does for a login link."""
    row = msgspec.to_builtins(declared_of(invite_links.create_invite_link).serve(tool_call.request, body))
    return {**row, "join_url": _base_url(tool_call) + row["join_path"]}


# <<< feature:invite_links
def register(registry: Registry) -> None:
    registry.register(Tool.for_route(
        account.create_my_login_link,
        call=_with_route_login_url(account.create_my_login_link),
        name="create_my_login_link",
        title="A login link for yourself",
        description="A link that signs your own account in on a browser or another device. Only a key with your account's full role may ask: one scoped down to user on an admin account is refused. " + LOGIN_LINK_ANSWER,
    ))
    registry.register(Tool.for_route(
        account.cancel_my_login_link,
        name="cancel_my_login_link",
        title="Stop your own login link working",
        description="Void your account's live login link without making a new one. Devices it already signed in stay signed in. Answers with removed: 0 or 1.",
        destructive=True,
    ))
    registry.register(Tool.for_route(
        admin.create_user,
        call=_with_route_login_url(admin.create_user),
        name="admin_create_user",
        title="Create an account (admin)",
        description="Make a new account and its first login link, to hand to the person. role is admin or user. Answers with user_id and the link. " + LOGIN_LINK_ANSWER,
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        admin.reissue_login,
        call=_with_route_login_url(admin.reissue_login),
        name="admin_reissue_login",
        title="A new login link for an account (admin)",
        description="For someone who lost their device or their link: a fresh login link for their account. " + LOGIN_LINK_ANSWER,
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        admin.cancel_login_link,
        name="admin_cancel_login_link",
        title="Stop an account's login link working (admin)",
        description="Void an account's live login link without making a new one: a link that went somewhere it should not have. Devices it already signed in stay signed in; admin_revoke_access ends those too. Answers with removed: 0 or 1.",
        destructive=True,
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        admin.rename_user,
        name="admin_rename_user",
        title="Rename someone (admin)",
        description="Change another account's display name (1-80 characters). Only the owner renames an admin. expect, when given, is the name you think it has now; if it changed meanwhile the rename is refused with the current name.",
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        admin.set_user_role,
        name="admin_set_user_role",
        title="Change an account's role (admin)",
        description="Make an account an admin or an ordinary user. Only the owner takes an admin's rights away; the owner is always an admin, and nobody changes their own role.",
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        admin.delete_user,
        name="admin_delete_user",
        title="Delete an account (admin)",
        description="Remove an account for good, with its sessions, login link and API keys. Cannot be undone: admin_set_user_disabled is the reversible version. Only the owner deletes an admin; nobody deletes the owner or themselves.",
        destructive=True,
        admin_only=True,
    ))
    # >>> feature:two_factor
    registry.register(Tool.for_route(
        admin.reset_two_factor,
        name="admin_reset_two_factor",
        title="Turn off an account's two-factor (admin)",
        description="For someone who lost both their phone and their recovery codes: switches their two-factor sign-in off so a login link lets them in again. Only the owner resets an admin's; the owner's is reset from a shell.",
        destructive=True,
        admin_only=True,
        enabled=_two_factor_on,
    ))
    # <<< feature:two_factor
    # >>> feature:sessions
    registry.register(Tool.for_route(
        sessions.list_my_sessions,
        name="list_my_sessions",
        title="Your signed-in devices",
        description="Where your account is signed in, a page at a time: id, when it started and was last seen, the browser, and the name you gave it. Through an API key no device is current. next is the offset of the following page, or null on the last one.",
        read_only=True,
    ))
    registry.register(Tool.for_route(
        sessions.label_one,
        name="label_my_session",
        title="Name one of your devices",
        description="Give one of your devices a name (at most 80 characters; empty clears it). expect, when given, is the name you think it has now.",
    ))
    registry.register(Tool.for_route(
        sessions.revoke_one,
        name="revoke_my_session",
        title="Sign out one of your devices",
        description="End one of your signed-in sessions; that browser has to sign in again.",
        destructive=True,
    ))
    registry.register(Tool.for_route(
        sessions.revoke_others,
        name="revoke_my_other_sessions",
        title="Sign out all your devices",
        description="End every session your account has: through an API key there is no current device to keep, so every browser signs in again. API keys keep working. Answers with how many were removed.",
        destructive=True,
    ))
    registry.register(Tool.for_route(
        sessions.list_user_sessions,
        name="admin_list_user_sessions",
        title="An account's signed-in devices (admin)",
        description="Where an account is signed in: every session, with when it started and was last seen. Addresses are shown to the owner only, and only the owner sees the owner's.",
        read_only=True,
        admin_only=True,
    ))
    registry.register(Tool.for_route(
        sessions.revoke_user_session,
        name="admin_revoke_user_session",
        title="Sign out one of an account's devices (admin)",
        description="End one of an account's sessions, rather than all of them with admin_revoke_access.",
        destructive=True,
        admin_only=True,
    ))
    # <<< feature:sessions
    # >>> feature:invite_links
    registry.register(Tool.for_route(
        invite_links.create_invite_link,
        call=_with_join_url,
        name="admin_create_invite_link",
        title="Create an invite link (admin)",
        description="One link for a group: each person who opens it joins as a new user, up to max_uses people, for days days. Answers with join_url (shown only now) and the link's details. Revoking or deleting this API key switches the link off.",
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool.for_route(
        invite_links.rename_invite_link,
        name="admin_rename_invite_link",
        title="Rename an invite link (admin)",
        description="Change the label people see on an invite link's join page (1-80 characters). expect, when given, is the label you think it has now.",
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool.for_route(
        invite_links.set_invite_link_limits,
        name="admin_set_invite_link_limits",
        title="Change an invite link's places and expiry (admin)",
        description="Set how many people the link may take in (never fewer than have joined) and, when days is given, a new expiry counted from now. The link then counts as this key's: revoking the key switches it off.",
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool.for_route(
        invite_links.switch_on_invite_link,
        name="admin_switch_on_invite_link",
        title="Switch an invite link back on (admin)",
        description="Undo admin_revoke_invite_link: anyone still holding the link can use it again. The link then counts as this key's: revoking the key switches it off.",
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool.for_route(
        invite_links.delete_invite_link,
        name="admin_delete_invite_link",
        title="Delete an invite link (admin)",
        description="Take an invite link off the list and stop it working. The accounts it made stay. admin_restore_invite_link undoes it for a few minutes.",
        destructive=True,
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool.for_route(
        invite_links.restore_invite_link,
        name="admin_restore_invite_link",
        title="Undo deleting an invite link (admin)",
        description="Bring back an invite link deleted in the last few minutes, working as before. The link then counts as this key's: revoking the key switches it off.",
        admin_only=True,
        enabled=_invite_links_on,
    ))
    # <<< feature:invite_links
