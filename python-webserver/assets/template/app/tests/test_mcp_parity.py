# whole-file feature:mcp
"""The rule these hold: everything a person can do in the app, an API key can
do over HTTP and an agent can do through /mcp -- except managing API keys, your
own password, PIN and two-factor, other people's files and the owner's screens,
which need a browser session. The API docs say which routes take
a key (their access lets apiKey in); each of those has an MCP tool naming it in
route, or an mcp_http reason for staying HTTP only. Go's twin is
server/routes/mcp_parity_test.go."""
from __future__ import annotations

import json
import re
import unittest
from dataclasses import replace
from typing import Any

from app.apidocs.generate import spec as api_spec
from app.routes.mcp_tools import build_registry
from app.tests.route_support import RoutesTestCase

SESSION_ONLY = "requires a signed-in session"


def operations(spec: dict[str, Any]) -> dict[str, dict[str, Any]]:
    """Every operation the API docs list, by "METHOD /path"."""
    found = {}
    for path, item in spec["paths"].items():
        for method, operation in item.items():
            if method.startswith("x-"):
                continue
            security = operation.get("security", [])
            found[f"{method.upper()} {path}"] = {
                "path": path,
                "method": method.upper(),
                "key_ok": any("apiKey" in scheme for scheme in security),
                "public": not security,
                "http_only": operation.get("x-mcp-http", ""),
            }
    return found


class MCPParity(RoutesTestCase):
    """The app with every switch on, so no feature hides behind a 404."""

    config_changes = {
        "mcp_enabled": True,
        "api_keys_enabled": True,
        "invite_links_enabled": True,  # feature:invite_links
        "passwords_enabled": True,  # feature:passwords
        "two_factor_enabled": True,  # feature:two_factor
        "metrics_enabled": True,  # feature:metrics
    }

    def setUp(self) -> None:
        super().setUp()
        cfg = self.holder.get()
        self.set_config(files=replace(cfg.files, enabled=True))  # feature:files
        self.set_config(webhooks=replace(self.holder.get().webhooks, enabled=True))  # feature:webhooks

    def test_every_key_route_is_an_mcp_tool(self) -> None:
        spec = operations(api_spec()[0])
        problems, covered = [], set()
        for tool in build_registry().tools():
            operation = spec.get(tool.route)
            if not tool.route:
                problems.append(f"tool {tool.name} has no route")
            elif operation is None:
                problems.append(f"tool {tool.name} names {tool.route}, which has no API docs")
            elif not operation["key_ok"]:
                problems.append(f"tool {tool.name} names {tool.route}, which an API key may not call")
            elif operation["http_only"]:
                problems.append(f"{tool.route} has a tool ({tool.name}) and an x-mcp-http reason; drop one")
            covered.add(tool.route)
        for name, operation in spec.items():
            if operation["key_ok"] and name not in covered and not operation["http_only"]:
                problems.append(f"{name} takes an API key but has no MCP tool: add one in mcp_tools.py, or an mcp_http reason in its doc()")
            if not operation["key_ok"] and operation["http_only"]:
                problems.append(f"{name} has an x-mcp-http reason but no key may call it; drop the reason")
        self.assertEqual(sorted(problems), [])

    def test_the_spec_says_who_may_call_each_route(self) -> None:
        """A route listed for API keys is not turned away for being called with
        one, and a session-only route is. With the owner's admin key, so only
        RequireSession and RequireOwner can say no to the key itself."""
        problems = []
        for name, operation in sorted(operations(api_spec()[0]).items()):
            if operation["public"] or operation["path"] in ("/api/logout", "/api/owner/logs/stream"):
                continue
            path = re.sub(r"\{\w+\}", "999", operation["path"])
            status, reply = self.call(operation["method"], path, key=self.admin_key, body={})
            message = reply.get("error", "") if isinstance(reply, dict) else ""
            refused = (status == 403 and SESSION_ONLY in message) or (status == 404 and message == "not_found")
            if operation["key_ok"] and refused:
                problems.append(f"{name}: the API docs let an API key in, but a key is turned away")
            if not operation["key_ok"] and not refused:
                problems.append(f"{name}: the API docs say session only, but a key got {status}")
        self.assertEqual(problems, [])

    def test_the_mcp_manifest_is_for_signed_in_callers(self) -> None:
        self.assertEqual(self.call("GET", "/api/mcp.json")[0], 401)
        status, reply = self.call("GET", "/api/mcp.json", key=self.user_key)
        self.assertEqual((status, reply["endpoint"]), (200, "/mcp"))

    def test_me_says_where_the_mcp_reference_is(self) -> None:
        self.assertEqual(self.call("GET", "/api/me", cookie=self.user_cookie)[1]["mcp_url"], "/api/mcp.json")
        self.set_config(repo="https://github.com/you/app")
        self.assertEqual(self.call("GET", "/api/me", cookie=self.user_cookie)[1]["mcp_url"], "https://github.com/you/app/blob/HEAD/config/mcp.json")

    def test_account_tools(self) -> None:
        def tool(name: str, arguments: dict[str, Any], as_key: str) -> tuple[bool, dict[str, Any]]:
            status, reply = self.call("POST", "/mcp", key=as_key, body={"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": name, "arguments": arguments}})
            self.assertEqual(status, 200, reply)
            result = reply["result"]
            return bool(result.get("isError")), json.loads(result["content"][0]["text"])

        # An admin key makes an account and gets a full address to hand over.
        failed, reply = tool("admin_create_user", {"display_name": "Bob", "role": "user", "max_uses": 2}, self.admin_key)
        self.assertFalse(failed, reply)
        self.assertTrue(reply["login_url"].startswith("http") and "/login/" in reply["login_url"], reply)

        # The narrowed key on the owner's admin account may not make itself a
        # login link: the session would be an admin's.
        failed, reply = tool("create_my_login_link", {}, self.user_key)
        self.assertTrue(failed)
        self.assertIn("less than your account's role", reply["error"])
        failed, reply = tool("create_my_login_link", {}, self.admin_key)
        self.assertFalse(failed, reply)
        self.assertIn("login_url", reply)

        # The owner rules hold through the tools.
        self.assertTrue(tool("admin_set_user_role", {"user_id": self.owner.id, "role": "user"}, self.admin_key)[0])
        self.assertFalse(tool("admin_rename_user", {"user_id": self.user.id, "display_name": "Mem"}, self.admin_key)[0])

    # >>> feature:files
    def test_files_travel_over_http(self) -> None:
        """Start and finish through /mcp, the bytes over plain HTTP with the
        same key."""
        key = self.user_key

        def tool(name: str, arguments: dict[str, Any], as_key: str = key) -> tuple[bool, dict[str, Any]]:
            status, reply = self.call("POST", "/mcp", key=as_key, body={"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": name, "arguments": arguments}})
            self.assertEqual(status, 200, reply)
            result = reply["result"]
            return bool(result.get("isError")), json.loads(result["content"][0]["text"])

        content = b"agent upload " * 100
        failed, started = tool("files_start_upload", {"name": "notes.txt", "size": len(content)})
        self.assertFalse(failed, started)
        self.assertEqual(started["chunks"], 1)
        self.assertTrue(started["chunk_url"].endswith(f"/api/files/{started['id']}/chunks/{{index}}"), started)
        response = self.client.post(
            f"/api/files/{started['id']}/chunks/0", headers={"authorization": "Bearer " + key, "content-type": "application/octet-stream"}, content=content
        )
        self.assertEqual(response.status_code, 200, response.text)
        failed, finished = tool("files_finish_upload", {"file_id": started["id"]})
        self.assertEqual((failed, finished["state"]), (False, "ready"), finished)
        failed, where = tool("files_download_url", {"file_id": started["id"]})
        self.assertTrue(where["url"].endswith(f"/api/files/{started['id']}/download"), where)
        response = self.client.get(where["url"][where["url"].index("/api/"):], headers={"authorization": "Bearer " + key})
        self.assertEqual((response.status_code, response.content), (200, content))

        # The same refusals as the route: someone else's file is no file.
        from app.models import api_keys, users

        member_key, _ = api_keys.issue_api_key(self.store, self.user.id, "member", users.ROLE_USER, 3600)
        failed, reply = tool("files_rename", {"file_id": started["id"], "name": "x.txt"}, as_key=member_key)
        self.assertEqual((failed, reply["error"]), (True, "no such file"))
        self.assertFalse(tool("files_delete", {"file_id": started["id"]})[0])
        self.assertFalse(tool("files_restore", {"file_id": started["id"]})[0])

        # Switched off, the tools are gone like the routes.
        self.set_config(files=replace(self.holder.get().files, enabled=False))
        _, reply = self.call("POST", "/mcp", key=key, body={"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}})
        self.assertFalse([tool["name"] for tool in reply["result"]["tools"] if tool["name"].startswith("files_")])

    # <<< feature:files


if __name__ == "__main__":
    unittest.main()
