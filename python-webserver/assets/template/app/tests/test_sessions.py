"""The active sessions list (app/routes/sessions.py): every way of signing in
notes the device, the list shows only your own and nothing secret, and signing
another device out is checked against who is asking."""
from __future__ import annotations

import time
import unittest
from datetime import timedelta
from unittest import mock

from app.core import db, encryption, logs, security
from app.models import credentials, invite_links, login_tokens, sessions, to_iso, users, utc_now
from app.models.sign_in import SignInRules
from app.tests.route_support import RoutesTestCase, TestClient

BROWSER = "Mozilla/5.0 (Test) " + "x" * 400


class SessionRecordTest(RoutesTestCase):
    def test_a_record_from_before_these_fields_still_loads(self) -> None:
        now = utc_now()
        secret = "s3cret"
        self.store.put(db.BUCKET_SESSIONS, "old", {
            "user_id": self.owner.id,
            "secret_hash": encryption.sha256_hex(secret),
            "csrf_token": "t",
            "created_at": to_iso(now),
            "expires_at": to_iso(now + timedelta(hours=1)),
        })
        session, user = sessions.load_session(self.store, f"old.{secret}")
        self.assertEqual(user.id, self.owner.id)
        self.assertEqual(session.id, "old")
        self.assertEqual((session.ip, session.user_agent, session.last_seen_at), ("", "", None))

    def test_the_id_is_never_stored(self) -> None:
        _, _, session = sessions.new_session(self.owner.id, 3600)
        session.id = "transient"
        self.assertNotIn("id", session.to_record())
        self.assertNotIn("ip", session.to_record())

    def test_the_optional_fields_round_trip(self) -> None:
        _, _, session = sessions.new_session(self.owner.id, 3600)
        session.ip, session.user_agent, session.last_seen_at = "10.0.0.1", "curl", utc_now()
        back = sessions.Session.from_record(session.to_record())
        self.assertEqual((back.ip, back.user_agent, back.last_seen_at), (session.ip, session.user_agent, session.last_seen_at))


class TouchTest(RoutesTestCase):
    def record(self, session_id: str) -> dict:
        return self.store.get(db.BUCKET_SESSIONS, session_id)

    def credential(self) -> tuple[str, str]:
        login = login_tokens.issue_login_token(self.store, self.owner.id, 3600)
        credential = login_tokens.redeem_login_token(self.store, login, SignInRules(3600, False)).session_credential
        return credential, credential.partition(".")[0]

    def test_loading_a_session_is_not_activity(self) -> None:
        credential, session_id = self.credential()
        sessions.load_session(self.store, credential)
        self.assertNotIn("last_seen_at", self.record(session_id))

    def test_a_request_touches_at_most_every_interval(self) -> None:
        credential, session_id = self.credential()
        cookie = {self.cfg.cookie_name: encryption.xchacha_encrypt_string(self.cfg.secret_key, credential)}
        security.resolve_actor(self.store, self.cfg, cookie, "")
        first = self.record(session_id)["last_seen_at"]

        security.resolve_actor(self.store, self.cfg, cookie, "")
        self.assertEqual(self.record(session_id)["last_seen_at"], first)

        stale = to_iso(utc_now() - sessions.SESSION_TOUCH_INTERVAL - timedelta(seconds=1))
        self.store.update_value(db.BUCKET_SESSIONS, session_id, lambda record: record.update(last_seen_at=stale))
        security.resolve_actor(self.store, self.cfg, cookie, "")
        self.assertGreater(self.record(session_id)["last_seen_at"], stale)

    def test_a_failed_touch_does_not_fail_the_request(self) -> None:
        credential, _ = self.credential()
        cookie = {self.cfg.cookie_name: encryption.xchacha_encrypt_string(self.cfg.secret_key, credential)}
        with mock.patch.object(self.store, "update_value", side_effect=RuntimeError("locked")):
            self.assertIsNotNone(security.resolve_actor(self.store, self.cfg, cookie, ""))


class CaptureTest(RoutesTestCase):
    config_changes = {"passwords_enabled": True, "two_factor_enabled": True}

    def new_client(self) -> TestClient:
        return TestClient(self.app, base_url="https://testserver", headers={"user-agent": BROWSER})

    def assert_captured(self, client: TestClient) -> None:
        rows = client.get("/api/account/sessions").json()["sessions"]
        current = next(row for row in rows if row["current"])
        self.assertEqual(current["ip"], "testclient")
        self.assertEqual(len(current["user_agent"]), 256)
        self.assertTrue(current["user_agent"].startswith("Mozilla/5.0 (Test)"))
        self.assertIn("last_seen_at", current)

    def sign_in(self, user_id: int) -> TestClient:
        client = self.new_client()
        credential = login_tokens.issue_login_token(self.store, user_id, 600)
        self.assertEqual(client.post("/api/login", json={"credential": credential}).status_code, 200)
        return client

    def test_a_login_link(self) -> None:
        self.assert_captured(self.sign_in(self.user.id))

    def test_a_password(self) -> None:
        client = self.sign_in(self.user.id)
        token = client.get("/api/me").json()["csrf_token"]
        digest = encryption.sha256_hex("correct horse")[:43]
        self.assertEqual(
            client.post("/api/account/password", json={"username": "user", "password_digest": digest, "csrf_token": token}).status_code,
            200,
        )
        fresh = self.new_client()
        self.assertEqual(fresh.post("/api/login/password", json={"username": "user", "password_digest": digest}).status_code, 200)
        self.assert_captured(fresh)

    def test_two_factor(self) -> None:
        client = self.sign_in(self.user.id)
        token = client.get("/api/me").json()["csrf_token"]
        secret = client.post("/api/account/two-factor/start", json={"csrf_token": token}).json()["secret"]
        code = encryption.totp_code(secret, int(time.time()) // encryption.TOTP_PERIOD_SECONDS)
        self.assertEqual(client.post("/api/account/two-factor/confirm", json={"code": code, "csrf_token": token}).status_code, 200)

        fresh = self.new_client()
        credential = login_tokens.issue_login_token(self.store, self.user.id, 600)
        challenge = fresh.post("/api/login", json={"credential": credential}).json()["challenge"]
        # The session only exists once the code is given, so this is the step
        # that has to capture. The code that confirmed setup cannot be used
        # again; the next window's is accepted.
        next_code = encryption.totp_code(secret, int(time.time()) // encryption.TOTP_PERIOD_SECONDS + 1)
        response = fresh.post("/api/login/two-factor", json={"challenge": challenge, "code": next_code})
        self.assertEqual(response.status_code, 200, response.text)
        self.assert_captured(fresh)

    def test_an_invite_link(self) -> None:
        link, _ = invite_links.issue_invite_link(self.store, "Group", 5, timedelta(days=1), self.owner.id)
        joined = self.new_client()
        response = joined.post("/api/join/create", json={"credential": link, "display_name": "Newcomer"})
        self.assertEqual(response.status_code, 201, response.text)
        self.assert_captured(joined)

        returning = self.new_client()
        user_id = joined.get("/api/me").json()["id"]
        response = returning.post("/api/join/claim", json={"credential": link, "user_id": user_id})
        self.assertEqual(response.status_code, 200, response.text)
        self.assert_captured(returning)

    def test_a_failure_to_note_it_does_not_fail_the_sign_in(self) -> None:
        with mock.patch.object(sessions, "remember_client", side_effect=RuntimeError("locked")):
            client = self.sign_in(self.user.id)
        self.assertEqual(client.get("/api/me").status_code, 200)


class ListTest(RoutesTestCase):
    def rows(self, cookie: str) -> list[dict]:
        status, reply = self.call("GET", "/api/account/sessions", cookie=cookie)
        self.assertEqual(status, 200)
        return reply["sessions"]

    def test_needs_a_session(self) -> None:
        self.assertEqual(self.call("GET", "/api/account/sessions")[0], 401)
        self.assertEqual(self.call("GET", "/api/account/sessions", key=self.admin_key)[0], 403)
        self.assertEqual(self.call("POST", "/api/account/sessions/revoke-others", key=self.admin_key, body={})[0], 403)
        self.assertEqual(self.call("POST", "/api/account/sessions/x/revoke", key=self.admin_key, body={})[0], 403)

    def test_shows_only_your_own_newest_first_and_marks_this_one(self) -> None:
        second = self.session(self.owner.id)
        rows = self.rows(second)
        self.assertEqual(len(rows), 2)
        self.assertEqual([row["current"] for row in rows], [True, False])
        self.assertGreaterEqual(rows[0]["created_at"], rows[1]["created_at"])
        self.assertEqual(len(self.rows(self.user_cookie)), 1)

    def test_shows_nothing_secret(self) -> None:
        response = self.request("GET", "/api/account/sessions", cookie=self.owner_cookie)
        self.assertEqual(set(response.json()["sessions"][0]) - {"id", "current", "created_at", "expires_at", "last_seen_at", "ip", "user_agent"}, set())
        record = self.store.get(db.BUCKET_SESSIONS, response.json()["sessions"][0]["id"])
        self.assertNotIn("secret_hash", response.text)
        self.assertNotIn("csrf_token", response.text)
        self.assertNotIn(record["secret_hash"], response.text)
        self.assertNotIn(record["csrf_token"], response.text)

    def test_leaves_out_what_has_expired(self) -> None:
        session_id, _, session = sessions.new_session(self.owner.id, -10)
        self.store.put(db.BUCKET_SESSIONS, session_id, session.to_record())
        self.assertNotIn(session_id, [row["id"] for row in self.rows(self.owner_cookie)])


class RevokeTest(RoutesTestCase):
    def revoke(self, cookie: str, session_id: str, token: str | None = None) -> tuple[int, dict]:
        token = self.csrf(cookie) if token is None else token
        return self.call("POST", f"/api/account/sessions/{session_id}/revoke", cookie=cookie, body={"csrf_token": token})

    def current_id(self, cookie: str) -> str:
        rows = self.call("GET", "/api/account/sessions", cookie=cookie)[1]["sessions"]
        return next(row["id"] for row in rows if row["current"])

    def test_ends_the_other_device(self) -> None:
        other = self.session(self.owner.id)
        target = self.current_id(other)
        self.assertEqual(self.revoke(self.owner_cookie, target), (200, {"ok": True}))
        self.assertEqual(self.call("GET", "/api/me", cookie=other)[0], 401)
        self.assertEqual(self.call("GET", "/api/me", cookie=self.owner_cookie)[0], 200)

    def test_needs_the_csrf_token(self) -> None:
        other = self.session(self.owner.id)
        self.assertEqual(self.revoke(self.owner_cookie, self.current_id(other), token="wrong")[0], 403)
        self.assertEqual(self.call("GET", "/api/me", cookie=other)[0], 200)

    def test_a_foreign_id_and_an_unknown_id_look_the_same(self) -> None:
        foreign = self.revoke(self.owner_cookie, self.current_id(self.user_cookie))
        unknown = self.revoke(self.owner_cookie, "no-such-session")
        self.assertEqual(foreign, (404, {"error": "not found"}))
        self.assertEqual(unknown, foreign)
        self.assertEqual(self.call("GET", "/api/me", cookie=self.user_cookie)[0], 200)

    def test_refuses_the_session_in_use(self) -> None:
        current = self.current_id(self.owner_cookie)
        self.assertEqual(self.revoke(self.owner_cookie, current), (400, {"error": "use sign out for this device"}))
        self.assertEqual(self.call("GET", "/api/me", cookie=self.owner_cookie)[0], 200)

    def test_revoke_others_keeps_this_one_and_counts(self) -> None:
        others = [self.session(self.owner.id), self.session(self.owner.id)]
        session_id, _, expired = sessions.new_session(self.owner.id, -10)
        self.store.put(db.BUCKET_SESSIONS, session_id, expired.to_record())
        token = self.csrf(self.owner_cookie)
        path = "/api/account/sessions/revoke-others"
        self.assertEqual(self.call("POST", path, cookie=self.owner_cookie, body={})[0], 403)
        status, body = self.call("POST", path, cookie=self.owner_cookie, body={"csrf_token": token})
        self.assertEqual((status, body), (200, {"ok": True, "removed": 2}))
        for cookie in others:
            self.assertEqual(self.call("GET", "/api/me", cookie=cookie)[0], 401)
        self.assertEqual(self.call("GET", "/api/me", cookie=self.owner_cookie)[0], 200)
        self.assertEqual(self.call("GET", "/api/me", cookie=self.user_cookie)[0], 200)

    def test_both_are_audited(self) -> None:
        other = self.session(self.owner.id)
        target = self.current_id(other)
        token = self.csrf(self.owner_cookie)
        with TestClient(self.app) as client:
            self.call("POST", f"/api/account/sessions/{target}/revoke", cookie=self.owner_cookie, body={"csrf_token": token}, client=client)
            self.call("POST", "/api/account/sessions/revoke-others", cookie=self.owner_cookie, body={"csrf_token": token}, client=client)
        logger = self.app.state.logs
        entries, _ = logger.read_day(logger.today(), logs.Query("kind:auth"))
        messages = [entry.get("msg") for entry in entries]
        self.assertIn("session_revoked", messages)
        self.assertEqual(next(entry for entry in entries if entry.get("msg") == "sessions_revoked")["detail"], {"removed": 0})


class CredentialsStillEndSessionsTest(RoutesTestCase):
    def test_disabling_an_account_empties_its_list(self) -> None:
        credentials.set_user_disabled(self.store, self.user.id, True)
        self.assertEqual(sessions.list_sessions(self.store, self.user.id), [])


if __name__ == "__main__":
    unittest.main()
