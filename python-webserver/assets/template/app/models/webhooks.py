"""Webhooks: endpoints that want to hear about things, and the outbox of what
is on its way to them. app/core/webhooks.py does the sending. Go's twin is
server/models/webhook.go.

The event names are a promise to whoever receives them, so a name never
changes once it ships; the activity log's messages are free text and make no
such promise. An app adds its own by appending to EVENTS and calling emit
where the thing happens. An event missing from EVENTS still reaches "*"
subscribers.

A webhook's secret signs what is sent to it, so unlike a key's secret it has
to come back: it is sealed with the secret key on its own, the way a TOTP
secret is, because records may be stored unencrypted. two_factor.rekey_record
carries it across a rotation.
"""
from __future__ import annotations

import json
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Any

from app.core import encryption
from app.core.db import BUCKET_WEBHOOK_DELIVERIES, BUCKET_WEBHOOKS, Record, Store
from app.models import RestoreGoneError, deleted_long_ago, from_iso, mark_deleted, to_iso, unmark_deleted, utc_now

EVENT_USER_CREATED = "user.created"
EVENT_USER_DISABLED = "user.disabled"
EVENT_USER_DELETED = "user.deleted"
EVENT_API_KEY_CREATED = "api_key.created"
EVENT_API_KEY_REVOKED = "api_key.revoked"
EVENT_INVITE_LINK_CREATED = "invite_link.created"
EVENT_INVITE_LINK_REVOKED = "invite_link.revoked"
EVENT_JOB_FAILED = "job.failed"
# Only ever goes to the one endpoint being tested.
EVENT_WEBHOOK_TEST = "webhook.test"

# Subscribes an endpoint to everything but the test.
ALL_EVENTS = "*"

EVENTS = [
    EVENT_USER_CREATED,
    EVENT_USER_DISABLED,
    EVENT_USER_DELETED,
    EVENT_API_KEY_CREATED,
    EVENT_API_KEY_REVOKED,
    EVENT_INVITE_LINK_CREATED,
    EVENT_INVITE_LINK_REVOKED,
    EVENT_JOB_FAILED,
]

PENDING = "pending"
DELIVERED = "delivered"
FAILED = "failed"

# How long a finished delivery stays listed.
DELIVERY_RETENTION = timedelta(days=7)
NAME_MAX = 80
URL_MAX = 2000

# The wait after each failed attempt. One more failure after the last of these
# and the delivery is given up on: six attempts over about fifteen hours, long
# enough to ride out a receiver's bad night.
BACKOFF = (timedelta(minutes=1), timedelta(minutes=5), timedelta(minutes=30), timedelta(hours=2), timedelta(hours=12))

BAD_WEBHOOK_MESSAGE = "name must be 1-80 characters, the url at most 2000, and at least one known event picked"


class BadWebhookError(ValueError):
    def __init__(self) -> None:
        super().__init__(BAD_WEBHOOK_MESSAGE)


@dataclass
class Webhook:
    id: str
    name: str
    url: str
    secret: str
    created_by: int
    created_at: datetime
    events: list[str] = field(default_factory=list)
    enabled: bool = True
    deleted_at: datetime | None = None
    deleted_by: int | None = None

    def wants(self, event: str) -> bool:
        return self.enabled and self.deleted_at is None and (event in self.events or ALL_EVENTS in self.events)

    def to_record(self) -> Record:
        return {
            "id": self.id,
            "name": self.name,
            "url": self.url,
            "events": list(self.events),
            "secret": self.secret,
            "enabled": self.enabled,
            "created_by": self.created_by,
            "created_at": to_iso(self.created_at),
            "deleted_at": to_iso(self.deleted_at),
            "deleted_by": self.deleted_by,
        }

    @classmethod
    def from_record(cls, record: Record) -> "Webhook":
        return cls(
            id=record["id"],
            name=record["name"],
            url=record["url"],
            events=list(record["events"]),
            secret=record["secret"],
            enabled=record["enabled"],
            created_by=record["created_by"],
            created_at=from_iso(record["created_at"]),
            deleted_at=from_iso(record.get("deleted_at")),
            deleted_by=record.get("deleted_by"),
        )


@dataclass
class Delivery:
    """One event on its way to one endpoint: the outbox row. emit writes it
    and the webhooks job sends it, so a slow receiver never holds up a request
    and a restart loses nothing. body is exactly what is sent, so a retry
    signs and sends the same bytes."""

    id: str
    webhook_id: str
    event: str
    body: str
    created_at: datetime
    next_attempt_at: datetime
    status: str = PENDING
    attempts: int = 0
    last_status: int = 0
    last_error: str = ""
    finished_at: datetime | None = None

    def to_record(self) -> Record:
        return {
            "id": self.id,
            "webhook_id": self.webhook_id,
            "event": self.event,
            "body": self.body,
            "status": self.status,
            "attempts": self.attempts,
            "next_attempt_at": to_iso(self.next_attempt_at),
            "last_status": self.last_status,
            "last_error": self.last_error,
            "created_at": to_iso(self.created_at),
            "finished_at": to_iso(self.finished_at),
        }

    @classmethod
    def from_record(cls, record: Record) -> "Delivery":
        return cls(
            id=record["id"],
            webhook_id=record["webhook_id"],
            event=record["event"],
            body=record["body"],
            status=record["status"],
            attempts=record["attempts"],
            next_attempt_at=from_iso(record["next_attempt_at"]),
            last_status=record.get("last_status", 0),
            last_error=record.get("last_error", ""),
            created_at=from_iso(record["created_at"]),
            finished_at=from_iso(record.get("finished_at")),
        )


def valid_webhook(name: object, url: object, events: object) -> bool:
    """Everything but the address, which is app.core.webhooks.check_url's job:
    that depends on the running config."""
    if not isinstance(name, str) or not isinstance(url, str) or not isinstance(events, list):
        return False
    if not 1 <= len(name.strip()) <= NAME_MAX or not 1 <= len(url) <= URL_MAX or not events:
        return False
    return all(event == ALL_EVENTS or event in EVENTS for event in events)


def create_webhook(store: Store, secret_key: str, name: str, url: str, events: list[str], created_by: int) -> tuple[str, Webhook]:
    """Returns the signing secret exactly once, like a key: it can be
    recovered by the server, but no route ever shows it again."""
    if not valid_webhook(name, url, events):
        raise BadWebhookError()
    secret = "whsec_" + encryption.url_token(24)
    hook = Webhook(
        id=encryption.url_token(12),
        name=name.strip(),
        url=url,
        events=list(events),
        secret=encryption.xchacha_encrypt_string(secret_key, secret),
        created_by=created_by,
        created_at=utc_now(),
    )
    store.put(BUCKET_WEBHOOKS, hook.id, hook.to_record())
    return secret, hook


def open_secret(hook: Webhook, secret_key: str) -> str:
    """The signing secret in the clear, for the sender."""
    return encryption.xchacha_decrypt_string(secret_key, hook.secret)


def get_webhook(store: Store, webhook_id: str) -> Webhook | None:
    """An endpoint, deleted or not."""
    record = store.get(BUCKET_WEBHOOKS, webhook_id)
    return None if record is None else Webhook.from_record(record)


def list_webhooks(store: Store) -> list[Webhook]:
    """Every endpoint, newest first, leaving out deleted ones."""
    hooks = [Webhook.from_record(record) for _, record in store.all(BUCKET_WEBHOOKS)]
    hooks = [hook for hook in hooks if hook.deleted_at is None]
    hooks.sort(key=lambda hook: hook.created_at, reverse=True)
    return hooks


def set_enabled(store: Store, webhook_id: str, enabled: bool) -> bool:
    """False when there is no such endpoint or it is deleted."""
    found = False

    def mutate(record: Record) -> None:
        nonlocal found
        found = record.get("deleted_at") is None
        if found:
            record["enabled"] = enabled

    return store.update_value(BUCKET_WEBHOOKS, webhook_id, mutate) is not None and found


def delete_webhook(store: Store, webhook_id: str, actor_id: int) -> bool:
    """Mark an endpoint deleted, restorable for RESTORE_WINDOW. False when
    there is none or it is already deleted."""
    found = False

    def mutate(record: Record) -> None:
        nonlocal found
        found = record.get("deleted_at") is None
        mark_deleted(record, actor_id)

    return store.update_value(BUCKET_WEBHOOKS, webhook_id, mutate) is not None and found


def restore_webhook(store: Store, webhook_id: str, actor_id: int, actor_is_admin: bool) -> Webhook:
    """Undo delete_webhook. Raises a RestoreError when it may not happen."""

    def mutate(record: Record) -> None:
        unmark_deleted(record, actor_id, actor_is_admin)

    record = store.update_value(BUCKET_WEBHOOKS, webhook_id, mutate)
    if record is None:
        raise RestoreGoneError()
    return Webhook.from_record(record)


def _new_delivery(webhook_id: str, event: str, data: dict[str, Any] | None) -> Delivery:
    """data carries ids and names only, the same rule as a log entry's detail:
    this leaves the server."""
    now = utc_now()
    delivery_id = encryption.url_token(12)
    body = json.dumps({"id": delivery_id, "event": event, "created_at": to_iso(now), "data": data}, ensure_ascii=False, separators=(",", ":"))
    return Delivery(id=delivery_id, webhook_id=webhook_id, event=event, body=body, created_at=now, next_attempt_at=now)


def enqueue_event(store: Store, event: str, data: dict[str, Any] | None) -> int:
    """One delivery per endpoint that wants the event, and how many."""
    queued = 0
    for hook in list_webhooks(store):
        if hook.wants(event):
            delivery = _new_delivery(hook.id, event, data)
            store.put(BUCKET_WEBHOOK_DELIVERIES, delivery.id, delivery.to_record())
            queued += 1
    return queued


def enqueue_test(store: Store, hook: Webhook) -> None:
    """A test event for one endpoint, whatever it subscribes to."""
    delivery = _new_delivery(hook.id, EVENT_WEBHOOK_TEST, {"webhook_id": hook.id})
    store.put(BUCKET_WEBHOOK_DELIVERIES, delivery.id, delivery.to_record())


def due_deliveries(store: Store, now: datetime, limit: int) -> list[Delivery]:
    """What the job should send now, oldest first, at most limit of them."""
    rows = [Delivery.from_record(record) for _, record in store.all(BUCKET_WEBHOOK_DELIVERIES)]
    due = [row for row in rows if row.status == PENDING and row.next_attempt_at <= now]
    due.sort(key=lambda row: row.created_at)
    return due[:limit]


def record_attempt(store: Store, delivery_id: str, status: int, failure: str, retry: bool) -> None:
    """How a send went. failure empty means delivered; otherwise the delivery
    waits its backoff, or is given up on when retry is false or the backoff
    has run out."""

    def mutate(record: Record) -> None:
        now = utc_now()
        record["attempts"] += 1
        record["last_status"] = status
        record["last_error"] = failure
        if not failure:
            record["status"] = DELIVERED
            record["finished_at"] = to_iso(now)
        elif retry and record["attempts"] <= len(BACKOFF):
            record["next_attempt_at"] = to_iso(now + BACKOFF[record["attempts"] - 1])
        else:
            record["status"] = FAILED
            record["finished_at"] = to_iso(now)

    store.update_value(BUCKET_WEBHOOK_DELIVERIES, delivery_id, mutate)


def list_deliveries(store: Store, webhook_id: str) -> list[Delivery]:
    """One endpoint's deliveries, newest first."""
    rows = [Delivery.from_record(record) for _, record in store.all(BUCKET_WEBHOOK_DELIVERIES)]
    rows = [row for row in rows if row.webhook_id == webhook_id]
    rows.sort(key=lambda row: row.created_at, reverse=True)
    return rows


def purge_webhooks(store: Store) -> int:
    """Drop deleted endpoints past RESTORE_WINDOW and deliveries that finished
    longer than DELIVERY_RETENTION ago."""
    cutoff = utc_now() - DELIVERY_RETENTION

    def finished_long_ago(_key: object, record: Record) -> bool:
        finished = from_iso(record.get("finished_at"))
        return finished is not None and finished < cutoff

    removed = store.delete_where(BUCKET_WEBHOOKS, lambda _key, record: deleted_long_ago(record))
    return removed + store.delete_where(BUCKET_WEBHOOK_DELIVERIES, finished_long_ago)
