"""The shape every list route and paged MCP tool shares."""
from __future__ import annotations

import unittest
from typing import Any

from app.models import users
from app.routes.helpers import page_body
from app.tests.route_support import RoutesTestCase


class PageBodyTests(unittest.TestCase):
    def test_cuts_and_points_at_the_next_page(self) -> None:
        rows = [1, 2, 3, 4, 5]
        for limit, offset, count, following in [(2, 0, 2, 2), (2, 2, 2, 4), (2, 4, 1, None), (5, 0, 5, None), (2, 99, 0, None)]:
            body = page_body("rows", rows, limit, offset)
            self.assertEqual(len(body["rows"]), count, (limit, offset))
            self.assertEqual(body["next"], following, (limit, offset))
            self.assertEqual(body["total"], 5)


class ListRouteTests(RoutesTestCase):
    def test_pages(self) -> None:
        for index in range(5):
            users.create_user(self.store, f"Person {index}", users.ROLE_USER)

        status, first = self.call("GET", "/api/admin/users?limit=3", key=self.admin_key)
        self.assertEqual(status, 200)
        self.assertEqual((len(first["users"]), first["next"], first["total"]), (3, 3, 8))
        _, last = self.call("GET", "/api/admin/users?limit=3&offset=6", key=self.admin_key)
        self.assertEqual((len(last["users"]), last["next"]), (2, None))
        _, past = self.call("GET", "/api/admin/users?offset=50", key=self.admin_key)
        self.assertEqual(past["users"], [])

    def test_each_list_answers_under_its_own_name(self) -> None:
        for path, name in {
            "/api/team": "members",
            "/api/admin/invite-links": "invite_links",
        }.items():
            status, reply = self.call("GET", path, key=self.admin_key)
            self.assertEqual(status, 200, path)
            self.assertIsInstance(reply[name], list, path)

    def test_key_lists_need_a_session_and_page_too(self) -> None:
        for path in ["/api/keys", "/api/admin/keys"]:
            status, reply = self.call("GET", path, cookie=self.owner_cookie)
            self.assertEqual(status, 200, path)
            self.assertIsInstance(reply["keys"], list, path)

    def test_refuses_a_bad_page(self) -> None:
        for query in ["limit=0", "limit=1001", "limit=x", "offset=-1", "offset=x"]:
            self.assertEqual(self.call("GET", "/api/admin/users?" + query, key=self.admin_key)[0], 400, query)


class MCPPagingTests(RoutesTestCase):
    def tool(self, name: str, args: dict[str, Any]) -> dict[str, Any]:
        response = self.client.post(
            "/mcp",
            headers={"authorization": "Bearer " + self.admin_key, "accept": "application/json, text/event-stream", "content-type": "application/json"},
            json={"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": name, "arguments": args}},
        )
        return response.json()

    def test_pages(self) -> None:
        page = self.tool("admin_list_users", {"limit": 1})["result"]["structuredContent"]
        self.assertEqual((len(page["users"]), page["next"], page["total"]), (1, 1, 3))

    def test_no_arguments_still_works(self) -> None:
        page = self.tool("list_team", {})["result"]["structuredContent"]
        self.assertEqual((len(page["members"]), page["next"]), (3, None))

    def test_a_limit_over_the_maximum_is_refused(self) -> None:
        self.assertTrue(self.tool("admin_list_users", {"limit": 5000})["result"]["isError"])

    def test_an_unknown_argument_is_invalid(self) -> None:
        self.assertIn("error", self.tool("admin_list_users", {"page": 1}))
