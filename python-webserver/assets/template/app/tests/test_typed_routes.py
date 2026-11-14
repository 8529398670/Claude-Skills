"""A declared route checks its request against its type before the handler
runs (see app/apidocs/route.py). Each fault answers the same 400 whichever
route it is, and the Go template answers the same words. Go's twin is
server/routes/typed_routes_test.go."""
from __future__ import annotations

import json  # feature:mcp
import unittest
from dataclasses import replace  # feature:files|webhooks
from typing import Any

from app.tests.route_support import RoutesTestCase

LINK_LIMITS = "max_uses must be 0 (unlimited) or 1-1000, expires_in_minutes 0 (never) or 1-525600"


class DeclaredRoutesTest(RoutesTestCase):
    def send(self, path: str, **body: Any) -> tuple[int, Any]:
        return self.call("POST", path, cookie=self.owner_cookie, body={**body, "csrf_token": self.csrf(self.owner_cookie)})

    def test_a_declared_route_checks_its_body(self) -> None:
        cases = [
            ("a missing field", {"role": "user"}, "display name must be 1-80 characters"),
            ("another missing field", {"display_name": "Bea"}, "role must be admin or user"),
            ("a wrong type", {"display_name": 5, "role": "user"}, "malformed request body"),
            ("a value not in the enum", {"display_name": "Bea", "role": "boss"}, "role must be admin or user"),
            ("too long", {"display_name": "x" * 81, "role": "user"}, "display name must be 1-80 characters"),
            ("out of range", {"display_name": "Bea", "role": "user", "max_uses": 5000}, LINK_LIMITS),
            ("below range", {"display_name": "Bea", "role": "user", "expires_in_minutes": -1}, LINK_LIMITS),
            ("an unknown field", {"display_name": "Bea", "role": "user", "colour": "red"}, "unknown field: colour"),
        ]
        for name, body, message in cases:
            with self.subTest(name):
                self.assertEqual(self.send("/api/admin/users", **body), (400, {"error": message}))
        status, _ = self.send("/api/admin/users", display_name="Bea", role="user", max_uses=None)
        self.assertEqual(status, 201, "a null optional field")

    def test_a_declared_route_checks_its_url(self) -> None:
        cases = [
            ("/api/admin/users?limit=0", "limit must be 1-1000"),
            ("/api/admin/users?limit=lots", "limit must be 1-1000"),
            ("/api/admin/users?offset=-1", "offset must be 0 or more"),
            ("/api/admin/users?dir=up", "dir must be asc or desc"),
            ("/api/admin/users?sort=shoe_size", "sort must be one of: display_name, role, disabled, created_at"),
        ]
        for path, message in cases:
            with self.subTest(path):
                self.assertEqual(self.call("GET", path, cookie=self.owner_cookie), (400, {"error": message}))
        self.assertEqual(self.send("/api/admin/users/abc/rename", display_name="Bea"), (400, {"error": "invalid user id"}))

    def test_a_declared_route_checks_the_sessions_csrf_token(self) -> None:
        status, reply = self.call("POST", "/api/admin/users", cookie=self.owner_cookie, body={"display_name": "Bea", "role": "user"})
        self.assertEqual((status, reply), (403, {"error": "invalid csrf token"}))
        # The token is checked before the body, as before.
        self.assertEqual(self.call("POST", "/api/admin/users", cookie=self.owner_cookie, body={"colour": "red"})[0], 403)

    # >>> feature:logs
    def test_an_optional_number_in_the_query(self) -> None:
        path = "/api/owner/logs/days/2020-01-02"
        self.assertEqual(self.call("GET", path + "?before=-1", cookie=self.owner_cookie), (400, {"error": "before must be a line number"}))
        self.assertEqual(self.call("GET", path + "?before=lots", cookie=self.owner_cookie), (400, {"error": "before must be a line number"}))
        self.assertEqual(self.call("GET", path + "?before=3", cookie=self.owner_cookie)[0], 200)
        self.assertEqual(self.call("GET", path, cookie=self.owner_cookie)[0], 200)

    # <<< feature:logs
    # >>> feature:webhooks
    def test_a_nested_object_and_a_flag(self) -> None:
        self.set_config(webhooks=replace(self.holder.get().webhooks, enabled=True))
        status, hook = self.send("/api/owner/webhooks", name="Hook", url="https://example.com/hook", events=["*"])
        self.assertEqual(status, 201, hook)
        path = f"/api/owner/webhooks/{hook['id']}"
        self.assertEqual(self.send(path + "/enabled"), (400, {"error": "enabled must be true or false"}))
        self.assertEqual(self.send(path + "/enabled", enabled="yes"), (400, {"error": "malformed request body"}))
        self.assertEqual(self.send(path + "/enabled", enabled=False, colour="red"), (400, {"error": "unknown field: colour"}))
        target = {"url": "https://example.com/hook", "events": ["*"]}
        # Keys inside expect that nobody declared are ignored, as they always were.
        self.assertEqual(self.send(path + "/target", **target, expect={**target, "note": "x"}), (200, {"ok": True}))
        self.assertEqual(self.send(path + "/target", **target, expect="old")[1], {"error": "malformed request body"})
        self.assertEqual(self.send(path + "/target", url="https://example.com/hook", events=[]), (400, {"error": "the url must be at most 2000 characters, with at least one known event picked"}))

    # <<< feature:webhooks
    # >>> feature:files
    def test_a_raw_body_takes_its_token_from_a_header(self) -> None:
        self.set_config(files=replace(self.holder.get().files, enabled=True))
        status, file = self.send("/api/files", name="a.txt", size=3)
        self.assertEqual(status, 201, file)

        def piece(index: str, token: str) -> tuple[int, Any]:
            headers = {"content-type": "application/octet-stream", "cookie": self.owner_cookie, "x-csrf-token": token}
            response = self.client.post(f"/api/files/{file['id']}/chunks/{index}", headers=headers, content=b"abc")
            return response.status_code, response.json()

        self.assertEqual(piece("0", "wrong"), (403, {"error": "invalid csrf token"}))
        self.assertEqual(piece("x", self.csrf(self.owner_cookie)), (400, {"error": "bad piece number"}))
        self.assertEqual(piece("0", self.csrf(self.owner_cookie)), (200, {"received": 1}))
        self.assertEqual(self.send("/api/files", name="b.txt"), (400, {"error": "a file needs a name and a size"}))

    # <<< feature:files

# >>> feature:mcp
class ToolArgumentsTest(RoutesTestCase):
    config_changes = {"mcp_enabled": True}

    def tool(self, name: str, arguments: Any) -> dict[str, Any]:
        status, reply = self.call("POST", "/mcp", key=self.admin_key, body={"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": name, "arguments": arguments}})
        self.assertEqual(status, 200, reply)
        return reply

    def failed(self, reply: dict[str, Any]) -> str:
        result = reply.get("result", {})
        return json.loads(result["content"][0]["text"]).get("error", "") if result.get("isError") else ""

    def test_a_tool_checks_its_arguments_like_its_route(self) -> None:
        self.assertIn(LINK_LIMITS, self.failed(self.tool("admin_create_user", {"display_name": "Bea", "role": "user", "max_uses": 5000})))
        self.assertIn("display name must be 1-80 characters", self.failed(self.tool("admin_create_user", {"role": "user"})))
        self.assertIn("error", self.tool("admin_create_user", {"display_name": 5, "role": "user"}))
        self.assertIn("sort must be one of", self.failed(self.tool("admin_list_users", {"sort": "shoe_size"})))
        self.assertEqual(self.failed(self.tool("admin_list_users", {"sort": "display_name", "dir": "desc", "q": "use"})), "")


# <<< feature:mcp
if __name__ == "__main__":
    unittest.main()
