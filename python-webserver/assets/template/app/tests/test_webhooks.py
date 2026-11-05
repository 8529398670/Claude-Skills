"""Webhooks: the outbox, the sender and its address guard, and the routes."""
from __future__ import annotations

import http.client
import json
import socket
import tempfile
import threading
import time
import unittest
from dataclasses import replace
from datetime import timedelta
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from typing import Any
from unittest import mock

from app.core import db, encryption, webhooks
from app.core.config import WebhooksConfig
from app.models import to_iso, two_factor, utc_now
from app.models import webhooks as models
from app.tests.route_support import RoutesTestCase

KEY = "ab" * 32
CASES = json.loads((Path(__file__).parent / "webhook_cases.json").read_text(encoding="utf-8"))


class SharedCaseTests(unittest.TestCase):
    """The file both templates share, so the two cannot drift apart."""

    def test_signatures(self) -> None:
        for item in CASES["signatures"]:
            self.assertEqual(webhooks.sign(item["secret"], item["timestamp"], item["body"]), item["header"])

    def test_blocked_addresses(self) -> None:
        for value in CASES["blocked"]:
            self.assertTrue(webhooks.blocked(value), value)
        for value in CASES["allowed"]:
            self.assertFalse(webhooks.blocked(value), value)

    def test_urls(self) -> None:
        for item in CASES["urls"]:
            refused = webhooks.check_url(item["url"], item["allow_private"])
            self.assertEqual(refused == "", item["ok"], item)


class HookIsStillThere(unittest.TestCase):
    def test_http_client_still_dials_through_the_hook(self) -> None:
        """The guard hangs off a private attribute. If a Python release moves
        it, this fails rather than the check quietly no longer running."""
        connection = http.client.HTTPConnection("example.com", 80)
        self.assertTrue(hasattr(connection, "_create_connection"))
        called: list[Any] = []

        def dial(address: Any, timeout: Any = None, source_address: Any = None) -> Any:
            called.append(address)
            raise OSError("stop here")

        connection._create_connection = dial
        with self.assertRaises(OSError):
            connection.connect()
        self.assertEqual(called, [("example.com", 80)])


class Receiver:
    """A local endpoint that answers with status and remembers what it got."""

    def __init__(self, test: unittest.TestCase) -> None:
        self.status = 200
        self.requests: list[tuple[dict[str, str], str]] = []
        outer = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                body = self.rfile.read(int(self.headers["content-length"])).decode("utf-8")
                outer.requests.append((dict(self.headers), body))
                self.send_response(outer.status)
                if outer.status == 302:
                    self.send_header("location", "/elsewhere")
                self.send_header("content-length", "0")
                self.end_headers()

            def log_message(self, *args: Any) -> None:
                pass

        self.server = HTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.server.server_port}/in?token=topsecret"
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        test.addCleanup(self.close)

    def close(self) -> None:
        self.server.shutdown()
        self.server.server_close()


class FakeHolder:
    def __init__(self, cfg: Any) -> None:
        self.cfg = cfg

    def get(self) -> Any:
        return self.cfg


class SendCase(unittest.TestCase):
    allow_private = True

    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.store = db.open_store(Path(directory.name) / "test.db", KEY, True)
        self.cfg = mock.Mock(secret_key=KEY, webhooks=WebhooksConfig(timeout_seconds=2, allow_private_targets=self.allow_private))
        self.dispatcher = webhooks.Dispatcher(self.store, FakeHolder(self.cfg))
        self.receiver = Receiver(self)
        self.secret, self.hook = models.create_webhook(self.store, KEY, "Receiver", self.receiver.url, [models.ALL_EVENTS], 1)

    def rows(self) -> list[models.Delivery]:
        return models.list_deliveries(self.store, self.hook.id)


class DeliveryTests(SendCase):
    def dead_receiver(self) -> None:
        """Nothing listens on the address any more."""
        self.receiver.server.server_close()

    def test_a_sent_event_is_signed_and_marked_delivered(self) -> None:
        self.dispatcher.emit(models.EVENT_USER_CREATED, {"user_id": 7})
        self.assertEqual(self.dispatcher.run(), ("sent 1, failed 0", ""))

        headers, body = self.receiver.requests[0]
        payload = json.loads(body)
        self.assertEqual((payload["event"], payload["data"]), (models.EVENT_USER_CREATED, {"user_id": 7}))
        self.assertEqual(headers["X-Webhook-Event"], payload["event"])
        self.assertEqual(headers["X-Webhook-Delivery"], payload["id"])

        # What a receiver does with the header.
        stamp, signature = headers["X-Webhook-Signature"].split(",", 1)
        stamp = stamp.removeprefix("t=")
        self.assertLess(abs(time.time() - int(stamp)), 60)
        self.assertEqual(signature.removeprefix("v1="), encryption.hmac_sha256_hex(self.secret, f"{stamp}.{body}"))

        row = self.rows()[0]
        self.assertEqual((row.status, row.attempts, row.last_status), (models.DELIVERED, 1, 200))
        self.assertEqual(self.dispatcher.run()[0], "nothing to send")

    def test_a_failure_waits_its_backoff_and_is_eventually_given_up_on(self) -> None:
        self.receiver.status = 500
        self.dispatcher.emit(models.EVENT_USER_DELETED, {"user_id": 7})
        self.dispatcher.run()
        row = self.rows()[0]
        self.assertEqual((row.status, row.attempts, row.last_error), (models.PENDING, 1, "status 500"))
        wait = row.next_attempt_at - utc_now()
        self.assertTrue(timedelta(seconds=50) < wait <= timedelta(minutes=1), wait)
        self.assertEqual(self.dispatcher.run()[0], "nothing to send")

        for _ in range(5):
            models.record_attempt(self.store, row.id, 500, "status 500", True)
        row = self.rows()[0]
        self.assertEqual((row.status, row.attempts), (models.FAILED, 6))
        self.assertIsNotNone(row.finished_at)

    def test_a_recovered_receiver_gets_the_same_body_and_id(self) -> None:
        self.receiver.status = 503
        self.dispatcher.emit(models.EVENT_USER_DELETED, {"user_id": 7})
        self.dispatcher.run()
        row = self.rows()[0]
        self.store.update_value(db.BUCKET_WEBHOOK_DELIVERIES, row.id, lambda record: record.update(next_attempt_at=to_iso(utc_now() - timedelta(seconds=1))))
        self.receiver.status = 200
        self.dispatcher.run()
        (first_headers, first), (second_headers, second) = self.receiver.requests
        self.assertEqual(first, second)
        self.assertEqual(first_headers["X-Webhook-Delivery"], second_headers["X-Webhook-Delivery"])
        self.assertEqual((self.rows()[0].status, self.rows()[0].last_error), (models.DELIVERED, ""))

    def test_a_redirect_is_a_failure_and_is_not_followed(self) -> None:
        self.receiver.status = 302
        self.dispatcher.emit(models.EVENT_USER_DELETED, None)
        self.dispatcher.run()
        self.assertEqual(len(self.receiver.requests), 1)
        self.assertEqual(self.rows()[0].last_error, "status 302")

    def test_a_failure_never_quotes_the_url(self) -> None:
        self.dead_receiver()
        self.dispatcher.emit(models.EVENT_USER_DELETED, None)
        self.dispatcher.run()
        row = self.rows()[0]
        self.assertTrue(row.last_error)
        self.assertNotIn("topsecret", row.last_error)

    def test_only_wanted_events_are_queued(self) -> None:
        models.create_webhook(self.store, KEY, "Picky", "https://example.com/in", [models.EVENT_USER_DELETED], 1)
        self.assertEqual(models.enqueue_event(self.store, models.EVENT_USER_CREATED, None), 1)
        self.assertEqual(models.enqueue_event(self.store, models.EVENT_USER_DELETED, None), 2)
        models.set_enabled(self.store, self.hook.id, False)
        self.assertEqual(models.enqueue_event(self.store, models.EVENT_USER_DELETED, None), 1)

    def test_an_endpoint_switched_off_after_queueing_stops_its_deliveries(self) -> None:
        self.dispatcher.emit(models.EVENT_USER_DELETED, None)
        models.set_enabled(self.store, self.hook.id, False)
        self.dispatcher.run()
        self.assertEqual(self.rows()[0].status, models.FAILED)
        self.assertEqual(self.receiver.requests, [])

    def test_a_slow_drip_cannot_outlast_the_timeout(self) -> None:
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        self.addCleanup(listener.close)

        def drip() -> None:
            connection, _ = listener.accept()
            try:
                connection.recv(65536)
                for _ in range(20):
                    connection.sendall(b"H")
                    time.sleep(0.2)
            except OSError:
                pass
            finally:
                connection.close()

        threading.Thread(target=drip, daemon=True).start()
        hook = models.Webhook(id="x", name="x", url=f"http://127.0.0.1:{listener.getsockname()[1]}/", secret="", created_by=1, created_at=utc_now())
        delivery = models._new_delivery("x", models.EVENT_USER_DELETED, None)
        started = time.monotonic()
        status, failure = webhooks.send(hook, "s", delivery, WebhooksConfig(timeout_seconds=1, allow_private_targets=True))
        self.assertLess(time.monotonic() - started, 2.5)
        self.assertEqual(status, 0)
        self.assertTrue(failure)


class SwitchTests(SendCase):
    def test_off_nothing_is_queued_or_sent(self) -> None:
        self.dispatcher.emit(models.EVENT_USER_CREATED, {"user_id": 7})
        on = self.cfg.webhooks
        self.cfg.webhooks = replace(on, enabled=False)
        self.dispatcher.emit(models.EVENT_USER_DELETED, {"user_id": 7})
        self.assertEqual(len(self.rows()), 1)
        self.assertEqual(self.dispatcher.run(), ("switched off", ""))
        self.assertEqual(self.receiver.requests, [])

        self.cfg.webhooks = on
        self.assertEqual(self.dispatcher.run()[0], "sent 1, failed 0")


class GuardTests(SendCase):
    allow_private = False

    def test_the_dialler_refuses_a_private_address(self) -> None:
        with self.assertRaises(webhooks.PrivateTargetError):
            webhooks._guarded_dialler(False)(("127.0.0.1", self.receiver.server.server_port), 1)

    def test_a_name_that_resolves_private_is_refused(self) -> None:
        """DNS rebinding: the name looks public, the address is not."""
        answer = [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("127.0.0.1", self.receiver.server.server_port))]
        with mock.patch("socket.getaddrinfo", return_value=answer):
            with self.assertRaises(webhooks.PrivateTargetError):
                webhooks._guarded_dialler(False)(("hooks.example.com", 443), 1)
        self.assertEqual(self.receiver.requests, [])

    def test_a_saved_private_address_is_refused_at_send_time(self) -> None:
        """It passed when allow_private_targets was on, then that went off."""
        self.dispatcher.emit(models.EVENT_USER_DELETED, None)
        self.dispatcher.run()
        self.assertEqual(self.rows()[0].status, models.FAILED)
        self.assertEqual(self.receiver.requests, [])


class ModelTests(unittest.TestCase):
    def test_a_secret_survives_a_rekey(self) -> None:
        new_key = "cd" * 32
        hook = models.Webhook(id="a", name="x", url="https://a.example", secret=encryption.xchacha_encrypt_string(KEY, "whsec_x"), created_by=1, created_at=utc_now())
        record = hook.to_record()
        two_factor.rekey_record(KEY, new_key)(db.BUCKET_WEBHOOKS, record)
        rekeyed = models.Webhook.from_record(record)
        self.assertEqual(models.open_secret(rekeyed, new_key), "whsec_x")
        with self.assertRaises(Exception):
            models.open_secret(rekeyed, KEY)

    def test_valid_webhook(self) -> None:
        self.assertTrue(models.valid_webhook("x", "https://a.example", [models.EVENT_USER_CREATED]))
        self.assertTrue(models.valid_webhook("x", "https://a.example", ["*"]))
        self.assertFalse(models.valid_webhook("", "https://a.example", ["*"]))
        self.assertFalse(models.valid_webhook("x", "", ["*"]))
        self.assertFalse(models.valid_webhook("x", "https://a.example", []))
        self.assertFalse(models.valid_webhook("x", "https://a.example", ["made.up"]))
        self.assertFalse(models.valid_webhook("x", "https://a.example", [models.EVENT_WEBHOOK_TEST]))


class RouteTests(RoutesTestCase):
    def post(self, path: str, **body: Any) -> tuple[int, Any]:
        return self.call("POST", path, cookie=self.owner_cookie, body={**body, "csrf_token": self.csrf(self.owner_cookie)})

    def created(self) -> dict[str, Any]:
        status, reply = self.post("/api/owner/webhooks", name="Chat", url="https://hooks.example.com/in", events=["*"])
        self.assertEqual(status, 201, reply)
        return reply

    def test_the_owner_manages_webhooks_and_the_secret_shows_once(self) -> None:
        created = self.created()
        self.assertTrue(created["secret"].startswith("whsec_"))
        status, page = self.call("GET", "/api/owner/webhooks", cookie=self.owner_cookie)
        self.assertEqual((status, page["total"]), (200, 1))
        self.assertNotIn("secret", page["webhooks"][0])

        path = "/api/owner/webhooks/" + created["id"]
        self.assertEqual(self.post(path + "/delete")[0], 200)
        self.assertEqual(self.call("GET", "/api/owner/webhooks", cookie=self.owner_cookie)[1]["total"], 0)
        self.assertEqual(self.post(path + "/restore")[0], 200)
        self.assertEqual(self.post(path + "/enabled", enabled=False)[0], 200)
        self.assertFalse(models.get_webhook(self.store, created["id"]).enabled)

    def test_the_routes_are_the_owners_alone(self) -> None:
        self.assertEqual(self.call("GET", "/api/owner/webhooks", cookie=self.admin_cookie)[0], 404)
        self.assertEqual(self.call("GET", "/api/owner/webhooks", key=self.admin_key)[0], 404)

    def test_creating_one_checks_the_address(self) -> None:
        for target in ["http://hooks.example.com/in", "https://127.0.0.1/in", "https://localhost/in", "https://169.254.169.254/"]:
            self.assertEqual(self.post("/api/owner/webhooks", name="x", url=target, events=["*"])[0], 400, target)
        self.assertEqual(self.post("/api/owner/webhooks", name="x", url="https://a.example", events=["made.up"])[0], 400)

    def failed_delivery(self, hook_id: str) -> str:
        """A delivery for hook_id that has been given up on."""
        models.enqueue_test(self.store, models.get_webhook(self.store, hook_id))
        delivery = models.list_deliveries(self.store, hook_id)[0]
        models.record_attempt(self.store, delivery.id, 500, "boom", False)
        return delivery.id

    def test_resending_queues_the_same_event_as_a_new_delivery(self) -> None:
        hook = self.created()["id"]
        failed = self.failed_delivery(hook)
        path = f"/api/owner/webhooks/{hook}/deliveries/"
        self.assertEqual(self.post(f"{path}{failed}/resend")[0], 202)

        rows = {row.id: row for row in models.list_deliveries(self.store, hook)}
        self.assertEqual(len(rows), 2)
        original = rows.pop(failed)
        again = next(iter(rows.values()))
        self.assertEqual(original.status, models.FAILED)
        self.assertEqual((again.status, again.attempts), (models.PENDING, 0))
        self.assertEqual((again.body, again.event), (original.body, original.event))

        self.assertEqual(self.post(f"{path}{again.id}/resend")[0], 409)
        self.assertEqual(self.post(f"{path}nothing/resend")[0], 404)

    def test_a_delivery_cannot_be_resent_through_another_endpoint(self) -> None:
        first, second = self.created()["id"], self.created()["id"]
        failed = self.failed_delivery(first)
        self.assertEqual(self.post(f"/api/owner/webhooks/{second}/deliveries/{failed}/resend")[0], 404)

    def test_a_test_event_queues_a_delivery_and_lists_it(self) -> None:
        created = self.created()
        path = "/api/owner/webhooks/" + created["id"]
        self.assertEqual(self.post(path + "/test")[0], 202)
        status, page = self.call("GET", path + "/deliveries?limit=1", cookie=self.owner_cookie)
        self.assertEqual(status, 200)
        row = page["deliveries"][0]
        self.assertEqual((row["event"], row["status"]), (models.EVENT_WEBHOOK_TEST, models.PENDING))
        self.assertNotIn("body", row)
        self.assertEqual(self.call("GET", "/api/owner/webhooks/nope/deliveries", cookie=self.owner_cookie)[0], 404)

    def test_both_doors_emit(self) -> None:
        created = self.created()
        target = self.user.id
        status, _ = self.call("POST", f"/api/admin/users/{target}/disabled", key=self.admin_key, body={"disabled": True})
        self.assertEqual(status, 200)
        response = self.client.post(
            "/mcp",
            headers={"authorization": "Bearer " + self.admin_key, "accept": "application/json, text/event-stream", "content-type": "application/json"},
            json={"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "admin_set_user_disabled", "arguments": {"user_id": target, "disabled": False}}},
        )
        self.assertNotIn("error", response.json())
        rows = models.list_deliveries(self.store, created["id"])
        self.assertEqual([row.event for row in rows], [models.EVENT_USER_DISABLED, models.EVENT_USER_DISABLED])

    def test_the_job_is_registered(self) -> None:
        names = [row["name"] for row in self.call("GET", "/api/owner/jobs", cookie=self.owner_cookie)[1]["jobs"]]
        self.assertIn(webhooks.JOB_NAME, names)
