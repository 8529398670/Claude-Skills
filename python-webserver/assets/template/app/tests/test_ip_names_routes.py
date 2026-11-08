# whole-file feature:logs
"""The routes for what the owner calls an address: only the owner reaches them,
and only the owner sees a name. Kept apart from test_ip_names.py, because importing
route_support skips the whole file without httpx2. Go's twin is
server/routes/ip_names_test.go."""
from __future__ import annotations

import unittest

from app.core import db
from app.tests.route_support import RoutesTestCase

NAMES = "/api/owner/logs/ip-names"


class IPNameRoutesTest(RoutesTestCase):
    def setUp(self) -> None:
        super().setUp()
        for session_id, _ in self.store.all(db.BUCKET_SESSIONS):
            self.store.update_value(db.BUCKET_SESSIONS, session_id, lambda record: record.update(ip="10.0.0.5"))

    def post(self, path: str, cookie: str, **body: str) -> tuple[int, dict]:
        return self.call("POST", path, cookie=cookie, body={**body, "csrf_token": self.csrf(cookie)})

    # >>> feature:sessions
    def devices(self, cookie: str) -> list[dict]:
        return self.call("GET", "/api/account/sessions", cookie=cookie)[1]["sessions"]

    # <<< feature:sessions
    def test_naming_is_the_owners_alone(self) -> None:
        for cookie in (self.admin_cookie, self.user_cookie):
            self.assertEqual(self.post(NAMES, cookie, target="0.0.0.0/0", name="Mine")[0], 404)
            self.assertEqual(self.call("GET", NAMES, cookie=cookie)[0], 404)
        self.assertEqual(self.call("GET", NAMES, cookie=self.owner_cookie)[1], {"names": []})

    # >>> feature:sessions
    def test_a_name_reaches_the_owner_and_nobody_else(self) -> None:
        status, body = self.post(NAMES, self.owner_cookie, target="10.0.0.9/24", name="  Office ")
        self.assertEqual((status, body), (200, {"target": "10.0.0.0/24", "name": "Office"}))
        self.assertEqual(self.devices(self.owner_cookie)[0]["ip_name"], "Office")
        self.assertNotIn("ip_name", self.devices(self.user_cookie)[0])
        self.assertNotIn("ip_name", self.devices(self.admin_cookie)[0])

        rows = self.call("GET", f"/api/admin/users/{self.user.id}/sessions", cookie=self.owner_cookie)[1]["sessions"]
        self.assertEqual(rows[0]["ip_name"], "Office")
        rows = self.call("GET", f"/api/admin/users/{self.user.id}/sessions", cookie=self.admin_cookie)[1]["sessions"]
        self.assertNotIn("ip_name", rows[0])
        self.assertNotIn("ip", rows[0])

        self.assertEqual(self.post(NAMES + "/delete", self.owner_cookie, target="10.0.0.0/24")[0], 200)
        self.assertNotIn("ip_name", self.devices(self.owner_cookie)[0])

    # <<< feature:sessions
    def test_bad_input_is_refused(self) -> None:
        self.assertEqual(self.post(NAMES, self.owner_cookie, target="laptop", name="x"), (400, {"error": "bad_ip_target"}))
        self.assertEqual(self.post(NAMES, self.owner_cookie, target="10.0.0.1", name="  "), (400, {"error": "bad_ip_name"}))
        status, _ = self.call("POST", NAMES, cookie=self.owner_cookie, body={"target": "10.0.0.1", "name": "x"})
        self.assertEqual(status, 403)


if __name__ == "__main__":
    unittest.main()
