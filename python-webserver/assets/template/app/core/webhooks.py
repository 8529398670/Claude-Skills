"""Telling other systems that something happened. Go's twin is server/webhooks.

emit writes one delivery per interested endpoint to the outbox and returns;
the webhooks job sends them. So a slow or dead receiver never holds up a
request, and a restart loses nothing. Delivery is at least once: a crash
between the send and writing down that it worked sends it again, so a receiver
dedupes on the delivery id.

This is the only code in the template that calls an address someone typed,
which makes it the one place a server-side request forgery could start. The
address is checked when it is saved and when it is used, and the check that
counts is on the address actually dialled, after DNS: a name that resolves to
something public today and 169.254.169.254 tomorrow gets nowhere.

http.client rather than a new dependency. It follows no redirects, reads no
proxy settings, and its _create_connection is the one place to hook: TLS is
wrapped around the socket afterwards, so certificate and hostname checks still
run against the name in the URL. A private name, but it has been there since
Python 3.4 and test_webhooks.py fails if it ever goes.
"""
from __future__ import annotations

import asyncio
import http.client
import ipaddress
import socket
import ssl
import threading
import time
from collections.abc import Callable
from typing import Any
from urllib.parse import urlsplit

from app.core import encryption, jobs
from app.core.config import Config, WebhooksConfig
from app.core.config_live import Holder
from app.core.db import Store
from app.models import utc_now
from app.models import webhooks as models

JOB_NAME = "webhooks"
# How often the job looks for retries. A new event does not wait for this:
# emit asks for a run straight away.
JOB_INTERVAL_SECONDS = 60

# Caps one run, so a backlog against a dead receiver cannot hold the job for
# hours. What is left goes in the next run.
SENDS_PER_RUN = 50

BAD_URL_MESSAGE = "the url must be a full https address with no username or password"
PRIVATE_TARGET_MESSAGE = "that address is private, and webhooks.allow_private_targets is off"

# Everywhere a webhook may not go: this machine, private networks, link-local
# (cloud metadata lives there) and the rest of the ranges that are not the
# public internet. The Go template has the same list, and webhook_cases.json
# holds both to it.
BLOCKED_NETWORKS = tuple(
    ipaddress.ip_network(value)
    for value in (
        "0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
        "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15",
        "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
        "::/128", "::1/128", "64:ff9b::/96", "100::/64", "2001:db8::/32", "2002::/16",
        "fc00::/7", "fe80::/10", "ff00::/8",
    )
)


class PrivateTargetError(OSError):
    def __init__(self) -> None:
        super().__init__(PRIVATE_TARGET_MESSAGE)


def blocked(value: str) -> bool:
    """Whether an address is off limits. An IPv4 address written as IPv6
    (::ffff:10.0.0.1) is judged as the IPv4 address it is."""
    address: Any = ipaddress.ip_address(value.split("%", 1)[0])
    if address.version == 6 and address.ipv4_mapped is not None:
        address = address.ipv4_mapped
    return any(address in network for network in BLOCKED_NETWORKS)


def check_url(raw: str, allow_private: bool) -> str:
    """Why an address cannot be used, or "" if it can. Runs on save, for a
    clear answer, and again before each send, since allow_private_targets can
    change. A hostname passes here: what it resolves to is the dialler's
    check."""
    try:
        parts = urlsplit(raw)
        hostname = parts.hostname
        parts.port  # noqa: B018 -- raises on a port that is not a number
    except ValueError:
        return BAD_URL_MESSAGE
    if not hostname or parts.username is not None or parts.password is not None:
        return BAD_URL_MESSAGE
    if parts.scheme != "https" and not (allow_private and parts.scheme == "http"):
        return BAD_URL_MESSAGE
    if allow_private:
        return ""
    if hostname == "localhost" or hostname.endswith(".localhost"):
        return PRIVATE_TARGET_MESSAGE
    try:
        return PRIVATE_TARGET_MESSAGE if blocked(hostname) else ""
    except ValueError:
        return ""  # a name, not an address


def sign(secret: str, timestamp: int, body: str) -> str:
    """The signature a receiver checks: HMAC-SHA256 of "<timestamp>.<body>"
    with the endpoint's secret. The timestamp is in the signed text so a
    captured request cannot be replayed later as new."""
    return f"t={timestamp},v1={encryption.hmac_sha256_hex(secret, f'{timestamp}.{body}')}"


def _guarded_dialler(allow_private: bool) -> Callable[..., socket.socket]:
    """socket.create_connection, with the check on each address DNS gives."""

    def dial(address: tuple[str, int], timeout: float | None = None, source_address: Any = None) -> socket.socket:
        host, port = address
        failure: OSError = OSError("the address could not be resolved")
        for family, kind, protocol, _, target in socket.getaddrinfo(host, port, type=socket.SOCK_STREAM):
            if not allow_private and blocked(target[0]):
                failure = PrivateTargetError()
                continue
            connection = socket.socket(family, kind, protocol)
            try:
                connection.settimeout(timeout)
                connection.connect(target)
                return connection
            except OSError as error:
                connection.close()
                failure = error
        raise failure

    return dial


def send(hook: models.Webhook, secret: str, delivery: models.Delivery, settings: WebhooksConfig, context: ssl.SSLContext | None = None) -> tuple[int, str]:
    """One attempt: (status, failure), failure being what to show the owner
    and empty if it worked. A socket timeout is per read, so a receiver
    dripping a byte at a time would never trip it; the timer is what makes
    timeout_seconds the limit on the whole attempt. The reply's body is never
    read, for the same reason."""
    parts = urlsplit(hook.url)
    target = (parts.path or "/") + (f"?{parts.query}" if parts.query else "")
    timeout = settings.timeout_seconds
    connection: http.client.HTTPConnection
    if parts.scheme == "https":
        connection = http.client.HTTPSConnection(parts.hostname, parts.port, timeout=timeout, context=context or ssl.create_default_context())
    else:
        connection = http.client.HTTPConnection(parts.hostname, parts.port, timeout=timeout)
    connection._create_connection = _guarded_dialler(settings.allow_private_targets)

    def hang_up() -> None:
        if connection.sock is not None:
            try:
                connection.sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass

    deadline = threading.Timer(timeout, hang_up)
    deadline.daemon = True
    deadline.start()
    headers = {
        "Content-Type": "application/json",
        "User-Agent": "webhooks",
        "X-Webhook-Event": delivery.event,
        "X-Webhook-Delivery": delivery.id,
        "X-Webhook-Signature": sign(secret, int(time.time()), delivery.body),
    }
    try:
        connection.request("POST", target, body=delivery.body.encode("utf-8"), headers=headers)
        status = connection.getresponse().status
    except (OSError, http.client.HTTPException) as error:
        return 0, str(error) or type(error).__name__
    finally:
        deadline.cancel()
        connection.close()
    return status, "" if 200 <= status <= 299 else f"status {status}"


def _attempt(cfg: Config, hook: models.Webhook | None, delivery: models.Delivery) -> tuple[int, str, bool]:
    """Decides whether a delivery can go at all, then sends it. The last value
    is False for a failure that waiting will not fix."""
    if hook is None or hook.deleted_at is not None or not hook.enabled:
        return 0, "the endpoint is off", False
    refused = check_url(hook.url, cfg.webhooks.allow_private_targets)
    if refused:
        return 0, refused, False
    try:
        secret = models.open_secret(hook, cfg.secret_key)
    except Exception:  # noqa: BLE001 -- whatever went wrong, it will not send
        return 0, "the signing secret could not be read", False
    status, failure = send(hook, secret, delivery, cfg.webhooks)
    return status, failure, True


class Dispatcher:
    """The outbox's two ends. emit is synchronous and writes to the database,
    so a handler reaches it through await run(...) (helpers.emit), like any
    other model call. It works from any thread."""

    def __init__(self, store: Store, settings: Holder) -> None:
        self.store = store
        self.settings = settings
        # Set by main: the registry to nudge, and the loop it lives on while
        # the server runs. Without them events wait for the next tick.
        self.registry: jobs.Registry | None = None
        self.loop: asyncio.AbstractEventLoop | None = None

    def emit(self, event: str, data: dict[str, Any] | None) -> None:
        """Queue an event for every endpoint that wants it. data is ids and
        names only: it leaves the server. A failure here is reported and goes
        no further, since whatever just happened did happen, and the caller's
        work is done."""
        if not self.settings.get().webhooks.enabled:
            return
        try:
            queued = models.enqueue_event(self.store, event, data)
        except Exception as error:  # noqa: BLE001 -- see above
            print(f"[webhooks] could not queue {event}: {error!r}", flush=True)
            return
        if queued:
            self.nudge()

    def send_test(self, hook: models.Webhook) -> None:
        models.enqueue_test(self.store, hook)
        self.nudge()

    def resend(self, webhook_id: str, delivery_id: str) -> bool:
        """Queue a failed delivery again. See models.resend_delivery."""
        sent = models.resend_delivery(self.store, webhook_id, delivery_id)
        if sent:
            self.nudge()
        return sent

    def nudge(self) -> None:
        """Ask for a run now. A run already going leaves this to the next one;
        a loop already closed means the server is on its way out."""
        if self.loop is None or self.registry is None:
            return
        try:
            self.loop.call_soon_threadsafe(self._run_now)
        except RuntimeError:
            pass

    def _run_now(self) -> None:
        try:
            if self.registry is not None:
                self.registry.run_now(JOB_NAME)
        except (jobs.JobRunningError, jobs.UnknownJobError):
            pass

    def run(self) -> tuple[str, str]:
        """The job: send what is due and write down how each went. Only
        trouble with the database fails the run. A receiver being down is what
        the retries are for, and shows on the delivery."""
        cfg = self.settings.get()
        if not cfg.webhooks.enabled:
            return "switched off", ""
        due = models.due_deliveries(self.store, utc_now(), SENDS_PER_RUN)
        if not due:
            return "nothing to send", ""
        hooks: dict[str, models.Webhook | None] = {}
        sent = failed = 0
        for delivery in due:
            if delivery.webhook_id not in hooks:
                hooks[delivery.webhook_id] = models.get_webhook(self.store, delivery.webhook_id)
            status, failure, retry = _attempt(cfg, hooks[delivery.webhook_id], delivery)
            if failure:
                failed += 1
            else:
                sent += 1
            models.record_attempt(self.store, delivery.id, status, failure, retry)
        return f"sent {sent}, failed {failed}", ""
