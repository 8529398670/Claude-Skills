# whole-file feature:shortcuts
"""Keyboard shortcuts: kept per account, read back through /api/me, and only
in a shape a keyboard can send."""
from __future__ import annotations

import unittest

from app.models.shortcuts import valid_binding
from app.tests.route_support import RoutesTestCase


class ShortcutRoutesTest(RoutesTestCase):
    def mine(self, cookie: str) -> dict:
        status, me = self.call("GET", "/api/me", cookie=cookie)
        self.assertEqual(status, 200)
        self.assertIsInstance(me["shortcuts"], dict)
        return me["shortcuts"]

    def save(self, cookie: str, shortcuts: object, csrf: bool = True) -> tuple[int, object]:
        body: dict = {"shortcuts": shortcuts}
        if csrf:
            body["csrf_token"] = self.csrf(cookie)
        return self.call("POST", "/api/account/shortcuts", cookie=cookie, body=body)

    def test_they_are_kept_per_account(self) -> None:
        self.assertEqual(self.mine(self.user_cookie), {})
        wanted = {"search": "Mod+K", "redo": "Mod+Shift+Z", "help": ""}
        status, reply = self.save(self.user_cookie, wanted)
        self.assertEqual(status, 200, reply)
        self.assertEqual(self.mine(self.user_cookie), wanted)
        self.assertEqual(self.mine(self.owner_cookie), {})
        # {} puts everything back.
        self.assertEqual(self.save(self.user_cookie, {})[0], 200)
        self.assertEqual(self.mine(self.user_cookie), {})

    def test_they_need_the_csrf_token(self) -> None:
        self.assertEqual(self.save(self.user_cookie, {"search": "K"}, csrf=False)[0], 403)
        self.assertEqual(self.mine(self.user_cookie), {})

    def test_what_no_keyboard_sends_is_refused(self) -> None:
        for shortcuts in (
            {"Search": "K"},
            {"search": "Shift+Mod+K"},
            {"search": "Mod+Mod+K"},
            {"search": "Mod+"},
            {"search": "Hyper+K"},
            {"search": "Mod+ab"},
            {"search": " "},
            {"search": 5},
            ["search"],
        ):
            status, reply = self.save(self.user_cookie, shortcuts)
            self.assertEqual(status, 400, (shortcuts, reply))
        self.assertEqual(self.mine(self.user_cookie), {})


class BindingTest(unittest.TestCase):
    def test_bindings(self) -> None:
        cases = {
            "/": True, "?": True, "Mod+Z": True, "Mod+Shift+Z": True, "Mod+Ctrl+Alt+Shift+F5": True,
            "Enter": True, "Escape": True, "Space": True, "Shift++": True, "+": True, "": True, "é": True,
            "Alt+Mod+K": False, "Shift+": False, "enter": False, "\t": False,
        }
        for binding, want in cases.items():
            self.assertEqual(valid_binding(binding), want, binding)


if __name__ == "__main__":
    unittest.main()
