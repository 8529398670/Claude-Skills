# whole-file feature:webhooks
"""The owner's Webhooks screen. Owner only, from a browser session, like the
Logs and Settings screens: an endpoint sends what happens here to somewhere
else, so a leaked key must not be able to add one. There are no MCP tools for
this, for the same reason. See app/core/webhooks.py."""
from __future__ import annotations

from typing import Annotated, ClassVar, Literal

import msgspec
from msgspec import UNSET, Meta, UnsetType
from starlette.requests import Request

from app.apidocs import OWNER, PATH, QUERY, SESSION_ONLY, Reply, route
from app.apidocs import Request as RequestType
from app.core import webhooks
from app.core.logs import KIND_ADMIN
from app.core.security import actor_from
from app.models import RestoreError, to_iso
from app.models import users
from app.models import webhooks as models
from app.routes.admin import OkReply
from app.routes.helpers import PAGE_DEFAULT, DateTime, Dir, Limit, Offset, Page, Paged, Refused, cut_page, get_config, get_store, match_rows, restore_refusal, sorted_page
from app.routes.logs import audit


class Webhook(Reply):
    """An endpoint as the screen shows it. The signing secret is not in it:
    that is in two responses, create and rotate, and never again."""

    api_name: ClassVar[str] = "Webhook"
    id: str
    name: str
    url: str
    events: list[str]
    enabled: bool
    created_at: DateTime


class WebhookDelivery(Reply, kw_only=True):
    api_name: ClassVar[str] = "WebhookDelivery"
    id: str
    event: str
    status: str
    attempts: int
    created_at: DateTime
    next_attempt_at: DateTime
    last_status: int | UnsetType = UNSET
    last_error: str | UnsetType = UNSET
    finished_at: DateTime | UnsetType = UNSET


def _row(hook: models.Webhook) -> Webhook:
    """Picks the fields by hand, so the sealed secret cannot reach a response
    by way of a change to the model."""
    return Webhook(
        id=hook.id,
        name=hook.name,
        url=hook.url,
        events=hook.events,
        enabled=hook.enabled,
        created_at=to_iso(hook.created_at),
    )


def _delivery_row(delivery: models.Delivery) -> WebhookDelivery:
    """Leaves out the body: the screen is for "did it arrive"."""
    row = WebhookDelivery(
        id=delivery.id,
        event=delivery.event,
        status=delivery.status,
        attempts=delivery.attempts,
        created_at=to_iso(delivery.created_at),
        next_attempt_at=to_iso(delivery.next_attempt_at),
    )
    if delivery.last_status:
        row.last_status = delivery.last_status
    if delivery.last_error:
        row.last_error = delivery.last_error
    if delivery.finished_at is not None:
        row.finished_at = to_iso(delivery.finished_at)
    return row


class WebhookIDPath(RequestType):
    """The endpoint a route under /api/owner/webhooks/{webhook_id} acts on."""

    webhook_id: Annotated[str, PATH, Meta(description="The webhook's id, from GET /api/owner/webhooks.")]


class EventsReply(Reply):
    events: list[str]


@route(summary="Events a webhook can ask for", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", errors=(404,))
def list_events(request: Request) -> EventsReply:
    """What the form offers."""
    return EventsReply(events=models.EVENTS)


class ListWebhooks(Page):
    q: Annotated[str, QUERY, Meta(description="Keep webhooks whose name or address contains this, in any case.")] = ""
    sort: Annotated[Literal["name", "url", "enabled"], QUERY, Meta(description="Field to sort the whole list by. Rows without it go last.")] = ""  # type: ignore[assignment]
    dir: Dir = ""  # type: ignore[assignment]


class WebhooksReply(Paged):
    webhooks: list[Webhook]


@route(summary="Webhooks", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", errors=(400, 404))
def list_webhooks(request: Request, body: ListWebhooks) -> WebhooksReply:
    rows = match_rows([_row(hook) for hook in models.list_webhooks(get_store(request))], body.q, "name", "url")
    page = sorted_page(rows, body.sort, body.dir, body)
    return WebhooksReply(webhooks=page.pop("rows"), **page)


# The limits valid_webhook checks. It trims the name first, which no limit
# can say, so it still has the last word.
Name = Annotated[str, Meta(min_length=1, max_length=models.NAME_MAX)]
URL = Annotated[str, Meta(min_length=1, max_length=models.URL_MAX)]
Events = Annotated[list[str], Meta(min_length=1)]


class CreateWebhook(RequestType):
    name: Annotated[Name, Meta(extra={"error": models.BAD_WEBHOOK_MESSAGE})]
    url: Annotated[URL, Meta(extra={"error": models.BAD_WEBHOOK_MESSAGE})]
    events: Annotated[Events, Meta(extra={"error": models.BAD_WEBHOOK_MESSAGE})]


class NewWebhook(Webhook):
    api_name: ClassVar[str] = ""
    secret: Annotated[str, Meta(description="Shown in this response only.")]


@route(summary="Create a webhook", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", status=201, errors=(400, 403, 404))
def create_webhook(request: Request, body: CreateWebhook) -> NewWebhook:
    """Answers with the signing secret, the only time it is shown."""
    if not models.valid_webhook(body.name, body.url, body.events):
        raise Refused(400, models.BAD_WEBHOOK_MESSAGE)
    cfg = get_config(request)
    refused = webhooks.check_url(body.url, cfg.webhooks.allow_private_targets)
    if refused:
        raise Refused(400, refused)
    secret, hook = models.create_webhook(get_store(request), cfg.secret_key, body.name, body.url, body.events, actor_from(request).user.id)
    # The name only: the url can hold a token of its own.
    audit(request, KIND_ADMIN, "webhook created", {"webhook_id": hook.id, "name": hook.name})
    return NewWebhook(**msgspec.structs.asdict(_row(hook)), secret=secret)


class SetEnabled(WebhookIDPath):
    enabled: Annotated[bool, Meta(extra={"error": "enabled must be true or false"})]


@route(summary="Switch a webhook on or off", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", errors=(400, 403, 404))
def set_enabled(request: Request, body: SetEnabled) -> OkReply:
    if not models.set_enabled(get_store(request), body.webhook_id, body.enabled):
        raise Refused(404, "no such webhook")
    audit(request, KIND_ADMIN, "webhook enabled set", {"webhook_id": body.webhook_id, "enabled": body.enabled})
    return OkReply(ok=True)


class RenameWebhook(WebhookIDPath):
    name: Annotated[Name, Meta(description="1-80 characters.", extra={"error": models.BAD_NAME_MESSAGE})]
    # The name the caller expects to replace, so an undo cannot overwrite a
    # rename made elsewhere.
    expect: Annotated[str | None, Meta(description="The name you think it has. Answers 409 if it has another.")] = None


class RenameWebhookReply(Reply):
    ok: bool
    name: str


@route(summary="Rename a webhook", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", errors=(400, 403, 404, 409))
def rename_webhook(request: Request, body: RenameWebhook) -> RenameWebhookReply:
    if not models.valid_webhook_name(body.name):
        raise Refused(400, models.BAD_NAME_MESSAGE)
    try:
        renamed = models.rename_webhook(get_store(request), body.webhook_id, body.name, body.expect)
    except users.RenameConflictError as refusal:
        raise Refused(409, str(refusal), name=refusal.current) from None
    if not renamed:
        raise Refused(404, "no such webhook")
    audit(request, KIND_ADMIN, "webhook renamed", {"webhook_id": body.webhook_id, "name": body.name.strip()})
    return RenameWebhookReply(ok=True, name=body.name.strip())


class TargetExpect(msgspec.Struct):
    """What a target change expects to replace. Unknown keys in it are
    ignored, as they always were."""

    url: str
    events: list[str]


class SetWebhookTarget(WebhookIDPath):
    url: Annotated[URL, Meta(description="Checked like a new webhook's.", extra={"error": models.BAD_TARGET_MESSAGE})]
    events: Annotated[Events, Meta(extra={"error": models.BAD_TARGET_MESSAGE})]
    # What the caller expects to replace, as with a rename's.
    expect: Annotated[TargetExpect | None, Meta(description="What you think it has now. Answers 409 if it has something else.")] = None


@route(summary="Change a webhook's address and events", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", errors=(400, 403, 404, 409))
def set_webhook_target(request: Request, body: SetWebhookTarget) -> OkReply:
    """Change the address and the events. The address gets the same check as a
    new endpoint's, every time: an events-only edit of one that has since been
    ruled out is refused too, and the sender would refuse it anyway."""
    if not models.valid_webhook_target(body.url, body.events):
        raise Refused(400, models.BAD_TARGET_MESSAGE)
    refused = webhooks.check_url(body.url, get_config(request).webhooks.allow_private_targets)
    if refused:
        raise Refused(400, refused)
    expect = None if body.expect is None else (body.expect.url, body.expect.events)
    try:
        changed = models.set_webhook_target(get_store(request), body.webhook_id, body.url, body.events, expect)
    except models.WebhookChangedError as refusal:
        raise Refused(409, str(refusal)) from None
    if not changed:
        raise Refused(404, "no such webhook")
    # The events but not the url, which can hold a token of its own.
    audit(request, KIND_ADMIN, "webhook target changed", {"webhook_id": body.webhook_id, "events": body.events})
    return OkReply(ok=True)


class SecretReply(Reply):
    secret: Annotated[str, Meta(description="Shown only here. The old one has stopped working.")]


@route(summary="Replace a webhook's signing secret", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", errors=(400, 403, 404))
def rotate_webhook_secret(request: Request, body: WebhookIDPath) -> SecretReply:
    """Answers with the new signing secret, the only time it is shown."""
    secret = models.rotate_webhook_secret(get_store(request), get_config(request).secret_key, body.webhook_id)
    if secret is None:
        raise Refused(404, "no such webhook")
    audit(request, KIND_ADMIN, "webhook secret rotated", {"webhook_id": body.webhook_id})
    return SecretReply(secret=secret)


@route(summary="Delete a webhook", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", errors=(400, 403, 404))
def delete_webhook(request: Request, body: WebhookIDPath) -> OkReply:
    if not models.delete_webhook(get_store(request), body.webhook_id, actor_from(request).user.id):
        raise Refused(404, "no such webhook")
    audit(request, KIND_ADMIN, "webhook deleted", {"webhook_id": body.webhook_id})
    return OkReply(ok=True)


@route(summary="Undo deleting a webhook", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", errors=(400, 403, 404, 409))
def restore_webhook(request: Request, body: WebhookIDPath) -> Webhook:
    actor = actor_from(request).user
    try:
        hook = models.restore_webhook(get_store(request), body.webhook_id, actor.id, actor.is_admin)
    except RestoreError as refusal:
        raise restore_refusal(refusal) from None
    audit(request, KIND_ADMIN, "webhook restored", {"webhook_id": body.webhook_id})
    return _row(hook)


def _live_webhook(request: Request, webhook_id: str) -> models.Webhook:
    """An endpoint the screen can act on, or a 404."""
    hook = models.get_webhook(get_store(request), webhook_id)
    if hook is None or hook.deleted_at is not None:
        raise Refused(404, "no such webhook")
    return hook


@route(summary="Send a test event", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", status=202, errors=(400, 403, 404))
def send_test(request: Request, body: WebhookIDPath) -> OkReply:
    """Queues a webhook.test event. For an endpoint that is switched off, the
    delivery says so rather than the event going out."""
    request.app.state.webhooks.send_test(_live_webhook(request, body.webhook_id))
    return OkReply(ok=True)


class DeliveryIDPath(WebhookIDPath):
    delivery_id: Annotated[str, PATH, Meta(description="The delivery's id, from GET /api/owner/webhooks/{webhook_id}/deliveries.")]


@route(
    summary="Send a failed delivery again", description="A new delivery with the same body. The failed one stays listed.",
    tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", status=202, errors=(400, 401, 403, 404, 409),
)
def resend_delivery(request: Request, body: DeliveryIDPath) -> OkReply:
    """Queues a delivery that was given up on."""
    hook = _live_webhook(request, body.webhook_id)
    try:
        sent = request.app.state.webhooks.resend(hook.id, body.delivery_id)
    except models.DeliveryNotFailed:
        raise Refused(409, "delivery_not_failed") from None
    if not sent:
        raise Refused(404, "no such delivery")
    audit(request, KIND_ADMIN, "webhook delivery resent", {"webhook_id": hook.id, "delivery_id": body.delivery_id})
    return OkReply(ok=True)


class ListDeliveries(WebhookIDPath):
    limit: Limit = PAGE_DEFAULT
    offset: Offset = 0


class DeliveriesReply(Paged):
    deliveries: list[WebhookDelivery]


@route(summary="Recent deliveries", tag="Webhooks", access=SESSION_ONLY, role=OWNER, feature="webhooks", errors=(400, 404))
def list_deliveries(request: Request, body: ListDeliveries) -> DeliveriesReply:
    hook = _live_webhook(request, body.webhook_id)
    page = cut_page([_delivery_row(delivery) for delivery in models.list_deliveries(get_store(request), hook.id)], body)
    return DeliveriesReply(deliveries=page.pop("rows"), **page)
