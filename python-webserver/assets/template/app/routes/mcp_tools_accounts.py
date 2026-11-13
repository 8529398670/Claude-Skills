# whole-file feature:mcp
"""The MCP side of accounts and sign-in: login links, accounts, devices and
invite links. Each tool calls the helper its HTTP route calls, so the rules
(who may touch the owner, what a narrowed key may not do) hold whichever door
a request comes through.

A login link or invite link made here records the key that asked, and revoking
or deleting that key voids it while it can still be used (see
login_tokens.void_links_minted_by_key). That is what keeps revoking a leaked
key meaningful now that a key can hand out ways in.

The descriptions and schemas match the Go template's mcp_tools_accounts.go
word for word: both write the same config/mcp.json.
"""
from __future__ import annotations

from typing import Any

from app.core.config import Config  # feature:two_factor|invite_links
from app.mcp.registry import Call, Registry, Tool
from app.models.invite_links import INVITE_LINK_MAX_DAYS, INVITE_LINK_MAX_USES  # feature:invite_links
from app.models.users import ROLE_ADMIN, ROLE_USER
from app.routes import account, admin
from app.routes import invite_links  # feature:invite_links
from app.routes import sessions  # feature:sessions
from app.routes.mcp_tools import PAGE_SCHEMA, USER_ID_SCHEMA, _no_args, _page, _shared

# The arguments every tool that mints a login link takes besides its own, the
# same two as the HTTP body (login_tokens.resolve_link).
LOGIN_LINK_ARGS = {
    "max_uses": {"type": "integer", "minimum": 0, "description": "How many sign-ins the link allows. Leave it out for once; 0 is unlimited until it expires."},
    "expires_in_minutes": {"type": "integer", "minimum": 0, "description": "How long the link works. Leave it out for the configured default; 0 never expires."},
}
LOGIN_LINK_OPTIONAL = {"max_uses": int, "expires_in_minutes": int}
LOGIN_LINK_ANSWER = (
    "Answers with login_url (open it to sign in; it works once unless max_uses says otherwise), expires_at and max_uses. "
    "Any older link the account could still use stops working; its signed-in devices stay signed in. "
    "Revoking or deleting this API key voids the link while it is unused."
)
ROLE_SCHEMA = {"type": "string", "enum": [ROLE_ADMIN, ROLE_USER]}


def _object(properties: dict[str, Any], required: list[str]) -> dict[str, Any]:
    schema: dict[str, Any] = {"type": "object", "properties": properties, "additionalProperties": False}
    if required:
        schema["required"] = required
    return schema


def _base_url(call: Call) -> str:
    """The address this agent reached /mcp at, the way _file_url uses it."""
    return str(call.request.base_url).rstrip("/")


def _with_login_url(call: Call, payload: dict[str, Any]) -> dict[str, Any]:
    """A login-link payload with the full address added. The HTTP routes
    answer with login_path only (see admin.create_user for why)."""
    return {**payload, "login_url": _base_url(call) + payload["login_path"]}


def create_my_login_link(call: Call) -> dict[str, Any]:
    args = call.bind({}, optional=LOGIN_LINK_OPTIONAL)
    return _with_login_url(call, _shared(account.create_my_login_link_checked, call.request, args))


def cancel_my_login_link(call: Call) -> dict[str, Any]:
    _no_args(call)
    return _shared(account.cancel_my_login_link_checked, call.request)


def admin_create_user(call: Call) -> dict[str, Any]:
    args = call.bind({"display_name": str, "role": str}, optional=LOGIN_LINK_OPTIONAL)
    return _with_login_url(call, _shared(admin.create_user_checked, call.request, args))


def admin_reissue_login(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int}, optional=LOGIN_LINK_OPTIONAL)
    return _with_login_url(call, _shared(admin.reissue_login_checked, call.request, args["user_id"], args))


def admin_cancel_login_link(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int})
    return _shared(admin.cancel_login_link_checked, call.request, args["user_id"])


def admin_rename_user(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int, "display_name": str}, optional={"expect": str})
    return _shared(admin.rename_user_checked, call.request, args["user_id"], args["display_name"], args.get("expect"))


def admin_set_user_role(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int, "role": str})
    return _shared(admin.set_user_role_checked, call.request, args["user_id"], args["role"])


def admin_delete_user(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int})
    return _shared(admin.delete_user_checked, call.request, args["user_id"])


# >>> feature:two_factor
def _two_factor_on(cfg: Config) -> bool:
    return cfg.two_factor_enabled


def admin_reset_two_factor(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int})
    return _shared(admin.reset_two_factor_checked, call.request, args["user_id"])


# <<< feature:two_factor
# >>> feature:sessions
SESSION_ID_SCHEMA = {"type": "string", "description": "The device's id, from list_my_sessions or admin_list_user_sessions."}


def list_my_sessions(call: Call) -> dict[str, Any]:
    return _page(call, "sessions", lambda _store: sessions.my_session_rows(call.request))


def label_my_session(call: Call) -> dict[str, Any]:
    args = call.bind({"session_id": str, "label": str}, optional={"expect": str})
    return _shared(sessions.label_one_checked, call.request, args["session_id"], args["label"], args.get("expect"))


def revoke_my_session(call: Call) -> dict[str, Any]:
    args = call.bind({"session_id": str})
    return _shared(sessions.revoke_one_checked, call.request, args["session_id"])


def revoke_my_other_sessions(call: Call) -> dict[str, Any]:
    _no_args(call)
    return _shared(sessions.revoke_others_checked, call.request)


def admin_list_user_sessions(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int})
    return {"sessions": _shared(sessions.user_session_rows, call.request, args["user_id"])}


def admin_revoke_user_session(call: Call) -> dict[str, Any]:
    args = call.bind({"user_id": int, "session_id": str})
    return _shared(sessions.revoke_user_session_checked, call.request, args["user_id"], args["session_id"])


# <<< feature:sessions
# >>> feature:invite_links
LINK_ID_SCHEMA = {"type": "string", "description": "The id from admin_list_invite_links."}


def _invite_links_on(cfg: Config) -> bool:
    return cfg.invite_links_enabled


def admin_create_invite_link(call: Call) -> dict[str, Any]:
    args = call.bind({"label": str, "max_uses": int, "days": int})
    row = _shared(invite_links.create_invite_link_checked, call.request, args["label"], args["max_uses"], args["days"])
    return {**row, "join_url": _base_url(call) + row["join_path"]}


def admin_rename_invite_link(call: Call) -> dict[str, Any]:
    args = call.bind({"link_id": str, "label": str}, optional={"expect": str})
    return _shared(invite_links.rename_invite_link_checked, call.request, args["link_id"], args["label"], args.get("expect"))


def admin_set_invite_link_limits(call: Call) -> dict[str, Any]:
    args = call.bind({"link_id": str, "max_uses": int}, optional={"days": int})
    return _shared(invite_links.set_invite_link_limits_checked, call.request, args["link_id"], args["max_uses"], args.get("days"))


def admin_switch_on_invite_link(call: Call) -> dict[str, Any]:
    args = call.bind({"link_id": str})
    return _shared(invite_links.switch_on_invite_link_checked, call.request, args["link_id"])


def admin_delete_invite_link(call: Call) -> dict[str, Any]:
    args = call.bind({"link_id": str})
    return _shared(invite_links.delete_invite_link_checked, call.request, args["link_id"])


def admin_restore_invite_link(call: Call) -> dict[str, Any]:
    args = call.bind({"link_id": str})
    return _shared(invite_links.restore_invite_link_checked, call.request, args["link_id"])


# <<< feature:invite_links
def register(registry: Registry) -> None:
    user_only = _object({"user_id": USER_ID_SCHEMA}, ["user_id"])
    registry.register(Tool(
        name="create_my_login_link",
        route="POST /api/account/login-link",
        title="A login link for yourself",
        description="A link that signs your own account in on a browser or another device. Only a key with your account's full role may ask: one scoped down to user on an admin account is refused. " + LOGIN_LINK_ANSWER,
        call=create_my_login_link,
        input_schema=_object(dict(LOGIN_LINK_ARGS), []),
    ))
    registry.register(Tool(
        name="cancel_my_login_link",
        route="POST /api/account/login-link/cancel",
        title="Stop your own login link working",
        description="Void your account's live login link without making a new one. Devices it already signed in stay signed in. Answers with removed: 0 or 1.",
        call=cancel_my_login_link,
        destructive=True,
    ))
    registry.register(Tool(
        name="admin_create_user",
        route="POST /api/admin/users",
        title="Create an account (admin)",
        description="Make a new account and its first login link, to hand to the person. role is admin or user. Answers with user_id and the link. " + LOGIN_LINK_ANSWER,
        call=admin_create_user,
        input_schema=_object({**LOGIN_LINK_ARGS, "display_name": {"type": "string", "description": "1-80 characters."}, "role": ROLE_SCHEMA}, ["display_name", "role"]),
        admin_only=True,
    ))
    registry.register(Tool(
        name="admin_reissue_login",
        route="POST /api/admin/users/{user_id}/reissue-login",
        title="A new login link for an account (admin)",
        description="For someone who lost their device or their link: a fresh login link for their account. " + LOGIN_LINK_ANSWER,
        call=admin_reissue_login,
        input_schema=_object({**LOGIN_LINK_ARGS, "user_id": USER_ID_SCHEMA}, ["user_id"]),
        admin_only=True,
    ))
    registry.register(Tool(
        name="admin_cancel_login_link",
        route="POST /api/admin/users/{user_id}/cancel-login-link",
        title="Stop an account's login link working (admin)",
        description="Void an account's live login link without making a new one: a link that went somewhere it should not have. Devices it already signed in stay signed in; admin_revoke_access ends those too. Answers with removed: 0 or 1.",
        call=admin_cancel_login_link,
        input_schema=user_only,
        destructive=True,
        admin_only=True,
    ))
    registry.register(Tool(
        name="admin_rename_user",
        route="POST /api/admin/users/{user_id}/rename",
        title="Rename someone (admin)",
        description="Change another account's display name (1-80 characters). Only the owner renames an admin. expect, when given, is the name you think it has now; if it changed meanwhile the rename is refused with the current name.",
        call=admin_rename_user,
        input_schema=_object({
            "user_id": USER_ID_SCHEMA,
            "display_name": {"type": "string"},
            "expect": {"type": "string", "description": "The current name, from admin_list_users. Optional."},
        }, ["user_id", "display_name"]),
        admin_only=True,
    ))
    registry.register(Tool(
        name="admin_set_user_role",
        route="POST /api/admin/users/{user_id}/role",
        title="Change an account's role (admin)",
        description="Make an account an admin or an ordinary user. Only the owner takes an admin's rights away; the owner is always an admin, and nobody changes their own role.",
        call=admin_set_user_role,
        input_schema=_object({"user_id": USER_ID_SCHEMA, "role": ROLE_SCHEMA}, ["user_id", "role"]),
        admin_only=True,
    ))
    registry.register(Tool(
        name="admin_delete_user",
        route="POST /api/admin/users/{user_id}/delete",
        title="Delete an account (admin)",
        description="Remove an account for good, with its sessions, login link and API keys. Cannot be undone: admin_set_user_disabled is the reversible version. Only the owner deletes an admin; nobody deletes the owner or themselves.",
        call=admin_delete_user,
        input_schema=user_only,
        destructive=True,
        admin_only=True,
    ))
    # >>> feature:two_factor
    registry.register(Tool(
        name="admin_reset_two_factor",
        route="POST /api/admin/users/{user_id}/reset-two-factor",
        title="Turn off an account's two-factor (admin)",
        description="For someone who lost both their phone and their recovery codes: switches their two-factor sign-in off so a login link lets them in again. Only the owner resets an admin's; the owner's is reset from a shell.",
        call=admin_reset_two_factor,
        input_schema=user_only,
        destructive=True,
        admin_only=True,
        enabled=_two_factor_on,
    ))
    # <<< feature:two_factor
    # >>> feature:sessions
    registry.register(Tool(
        name="list_my_sessions",
        route="GET /api/account/sessions",
        title="Your signed-in devices",
        description="Where your account is signed in, a page at a time: id, when it started and was last seen, the browser, and the name you gave it. Through an API key no device is current. next is the offset of the following page, or null on the last one.",
        call=list_my_sessions,
        input_schema=PAGE_SCHEMA,
        read_only=True,
    ))
    registry.register(Tool(
        name="label_my_session",
        route="POST /api/account/sessions/{session_id}/label",
        title="Name one of your devices",
        description="Give one of your devices a name (at most 80 characters; empty clears it). expect, when given, is the name you think it has now.",
        call=label_my_session,
        input_schema=_object({"session_id": SESSION_ID_SCHEMA, "label": {"type": "string"}, "expect": {"type": "string"}}, ["session_id", "label"]),
    ))
    registry.register(Tool(
        name="revoke_my_session",
        route="POST /api/account/sessions/{session_id}/revoke",
        title="Sign out one of your devices",
        description="End one of your signed-in sessions; that browser has to sign in again.",
        call=revoke_my_session,
        input_schema=_object({"session_id": SESSION_ID_SCHEMA}, ["session_id"]),
        destructive=True,
    ))
    registry.register(Tool(
        name="revoke_my_other_sessions",
        route="POST /api/account/sessions/revoke-others",
        title="Sign out all your devices",
        description="End every session your account has: through an API key there is no current device to keep, so every browser signs in again. API keys keep working. Answers with how many were removed.",
        call=revoke_my_other_sessions,
        destructive=True,
    ))
    registry.register(Tool(
        name="admin_list_user_sessions",
        route="GET /api/admin/users/{user_id}/sessions",
        title="An account's signed-in devices (admin)",
        description="Where an account is signed in: every session, with when it started and was last seen. Addresses are shown to the owner only, and only the owner sees the owner's.",
        call=admin_list_user_sessions,
        input_schema=user_only,
        read_only=True,
        admin_only=True,
    ))
    registry.register(Tool(
        name="admin_revoke_user_session",
        route="POST /api/admin/users/{user_id}/sessions/{session_id}/revoke",
        title="Sign out one of an account's devices (admin)",
        description="End one of an account's sessions, rather than all of them with admin_revoke_access.",
        call=admin_revoke_user_session,
        input_schema=_object({"user_id": USER_ID_SCHEMA, "session_id": SESSION_ID_SCHEMA}, ["user_id", "session_id"]),
        destructive=True,
        admin_only=True,
    ))
    # <<< feature:sessions
    # >>> feature:invite_links
    link_only = _object({"link_id": LINK_ID_SCHEMA}, ["link_id"])
    registry.register(Tool(
        name="admin_create_invite_link",
        route="POST /api/admin/invite-links",
        title="Create an invite link (admin)",
        description="One link for a group: each person who opens it joins as a new user, up to max_uses people, for days days. Answers with join_url (shown only now) and the link's details. Revoking or deleting this API key switches the link off.",
        call=admin_create_invite_link,
        input_schema=_object({
            "label": {"type": "string", "description": "1-80 characters; people see it on the join page."},
            "max_uses": {"type": "integer", "minimum": 1, "maximum": INVITE_LINK_MAX_USES},
            "days": {"type": "integer", "minimum": 1, "maximum": INVITE_LINK_MAX_DAYS},
        }, ["label", "max_uses", "days"]),
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool(
        name="admin_rename_invite_link",
        route="POST /api/admin/invite-links/{link_id}/rename",
        title="Rename an invite link (admin)",
        description="Change the label people see on an invite link's join page (1-80 characters). expect, when given, is the label you think it has now.",
        call=admin_rename_invite_link,
        input_schema=_object({"link_id": LINK_ID_SCHEMA, "label": {"type": "string"}, "expect": {"type": "string"}}, ["link_id", "label"]),
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool(
        name="admin_set_invite_link_limits",
        route="POST /api/admin/invite-links/{link_id}/limits",
        title="Change an invite link's places and expiry (admin)",
        description="Set how many people the link may take in (never fewer than have joined) and, when days is given, a new expiry counted from now. The link then counts as this key's: revoking the key switches it off.",
        call=admin_set_invite_link_limits,
        input_schema=_object({
            "link_id": LINK_ID_SCHEMA,
            "max_uses": {"type": "integer", "minimum": 1, "maximum": INVITE_LINK_MAX_USES},
            "days": {"type": "integer", "minimum": 1, "maximum": INVITE_LINK_MAX_DAYS, "description": "Leave it out to keep the expiry."},
        }, ["link_id", "max_uses"]),
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool(
        name="admin_switch_on_invite_link",
        route="POST /api/admin/invite-links/{link_id}/switch-on",
        title="Switch an invite link back on (admin)",
        description="Undo admin_revoke_invite_link: anyone still holding the link can use it again. The link then counts as this key's: revoking the key switches it off.",
        call=admin_switch_on_invite_link,
        input_schema=link_only,
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool(
        name="admin_delete_invite_link",
        route="POST /api/admin/invite-links/{link_id}/delete",
        title="Delete an invite link (admin)",
        description="Take an invite link off the list and stop it working. The accounts it made stay. admin_restore_invite_link undoes it for a few minutes.",
        call=admin_delete_invite_link,
        input_schema=link_only,
        destructive=True,
        admin_only=True,
        enabled=_invite_links_on,
    ))
    registry.register(Tool(
        name="admin_restore_invite_link",
        route="POST /api/admin/invite-links/{link_id}/restore",
        title="Undo deleting an invite link (admin)",
        description="Bring back an invite link deleted in the last few minutes, working as before. The link then counts as this key's: revoking the key switches it off.",
        call=admin_restore_invite_link,
        input_schema=link_only,
        admin_only=True,
        enabled=_invite_links_on,
    ))
    # <<< feature:invite_links
