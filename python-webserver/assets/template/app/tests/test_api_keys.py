"""Making a key that acts as someone else (POST /api/admin/keys): who may do it
for whom, that the key acts as them, and that they can see who made it."""
from __future__ import annotations

import unittest

from app.models import api_keys, users
from app.tests.route_support import RoutesTestCase


class MintForTest(RoutesTestCase):
    def mint(self, cookie: str, user_id: int, role: str = "user") -> tuple[int, dict]:
        return self.call("POST", "/api/admin/keys", cookie=cookie, body={
            "csrf_token": self.csrf(cookie), "user_id": user_id, "name": "deploy", "role": role,
        })

    def test_who_may_mint_for_whom(self) -> None:
        third = users.create_user(self.store, "Third admin", users.ROLE_ADMIN)
        cases = [
            ("an admin for a user", self.admin_cookie, self.user.id, "user", 201),
            ("the owner for a user", self.owner_cookie, self.user.id, "user", 201),
            ("the owner for an admin", self.owner_cookie, self.admin.id, "admin", 201),
            ("an admin for themselves", self.admin_cookie, self.admin.id, "admin", 201),
            ("an admin for another admin", self.admin_cookie, third.id, "user", 403),
            ("an admin for the owner", self.admin_cookie, self.owner.id, "user", 403),
            ("an admin role for a user", self.owner_cookie, self.user.id, "admin", 403),
            ("a user for anyone", self.user_cookie, self.admin.id, "user", 403),
            ("nobody there", self.admin_cookie, 99999, "user", 404),
        ]
        for name, cookie, target, role, status in cases:
            with self.subTest(name):
                self.assertEqual(self.mint(cookie, target, role)[0], status)

    def test_a_key_made_for_someone_acts_as_them_and_says_who_made_it(self) -> None:
        status, reply = self.mint(self.owner_cookie, self.user.id)
        self.assertEqual(status, 201)
        self.assertEqual(reply["created_by"], self.owner.id)
        self.assertEqual(self.call("GET", "/api/me", key=reply["credential"])[1]["id"], self.user.id)

        _, page = self.call("GET", "/api/keys", cookie=self.user_cookie)
        self.assertEqual(len(page["keys"]), 1)
        self.assertEqual(page["keys"][0]["created_by_name"], "Owner")
        self.assertNotIn("secret_hash", page["keys"][0])

    def test_a_key_cannot_mint_a_key_for_anyone(self) -> None:
        status, _ = self.call("POST", "/api/admin/keys", key=self.admin_key, body={"user_id": self.user.id, "name": "x"})
        self.assertEqual(status, 403)
        self.assertEqual(api_keys.list_api_keys_for_user(self.store, self.user.id), [])

    def test_a_shell_still_mints_for_the_owner(self) -> None:
        _, key = api_keys.issue_api_key(self.store, self.owner.id, "script", users.ROLE_ADMIN, 3600)
        self.assertIsNone(key.created_by)

    def test_a_row_from_before_the_field_still_loads(self) -> None:
        _, key = api_keys.issue_api_key(self.store, self.owner.id, "old", users.ROLE_USER, 3600)
        record = key.to_record()
        del record["created_by"]
        self.assertIsNone(api_keys.APIKey.from_record(record).created_by)


if __name__ == "__main__":
    unittest.main()
