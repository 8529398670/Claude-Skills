"""The owner's Webhooks screen. Owner only, from a browser session, like the
Logs and Settings screens: an endpoint sends what happens here to somewhere
else, so a leaked key must not be able to add one. There are no MCP tools for
this, for the same reason. See app/core/webhooks.py."""
from __future__ import annotations

from typing import Any

from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.core import webhooks
from app.core.logs import KIND_ADMIN
from app.core.security import actor_from, check_csrf
from app.models import RestoreError, to_iso
from app.models import webhooks as models
from app.routes.helpers import bad_request, conflict, forbidden, get_config, get_store, not_found, read_json, restore_refused, run, send_page
from app.routes.logs import audit


def _row(hook: models.Webhook) -> dict[str, Any]:
    """Picks the fields by hand, so the sealed secret cannot reach a response
    by way of a change to the model."""
    return {
        "id": hook.id,
        "name": hook.name,
        "url": hook.url,
        "events": hook.events,
        "enabled": hook.enabled,
        "created_at": to_iso(hook.created_at),
    }


def _delivery_row(delivery: models.Delivery) -> dict[str, Any]:
    """Leaves out the body: the screen is for "did it arrive"."""
    row: dict[str, Any] = {
        "id": delivery.id,
        "event": delivery.event,
        "status": delivery.status,
        "attempts": delivery.attempts,
        "created_at": to_iso(delivery.created_at),
        "next_attempt_at": to_iso(delivery.next_attempt_at),
    }
    if delivery.last_status:
        row["last_status"] = delivery.last_status
    if delivery.last_error:
        row["last_error"] = delivery.last_error
    if delivery.finished_at is not None:
        row["finished_at"] = to_iso(delivery.finished_at)
    return row


async def list_events(request: Request) -> Response:
    """What the form offers."""
    return JSONResponse({"events": models.EVENTS})


async def list_webhooks(request: Request) -> Response:
    hooks = await run(models.list_webhooks, get_store(request))
    return send_page(request, "webhooks", [_row(hook) for hook in hooks])


async def create_webhook(request: Request) -> Response:
    """Answers with the signing secret, the only time it is shown."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    name, url, events = body.get("name"), body.get("url"), body.get("events")
    if not models.valid_webhook(name, url, events):
        return bad_request(models.BAD_WEBHOOK_MESSAGE)
    cfg = get_config(request)
    refused = webhooks.check_url(url, cfg.webhooks.allow_private_targets)
    if refused:
        return bad_request(refused)
    secret, hook = await run(models.create_webhook, get_store(request), cfg.secret_key, name, url, events, actor_from(request).user.id)
    # The name only: the url can hold a token of its own.
    audit(request, KIND_ADMIN, "webhook created", {"webhook_id": hook.id, "name": hook.name})
    return JSONResponse({**_row(hook), "secret": secret}, status_code=201)


async def set_enabled(request: Request) -> Response:
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    enabled = body.get("enabled")
    if not isinstance(enabled, bool):
        return bad_request("enabled must be true or false")
    webhook_id = request.path_params["webhook_id"]
    if not await run(models.set_enabled, get_store(request), webhook_id, enabled):
        return not_found("no such webhook")
    audit(request, KIND_ADMIN, "webhook enabled set", {"webhook_id": webhook_id, "enabled": enabled})
    return JSONResponse({"ok": True})


async def delete_webhook(request: Request) -> Response:
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    webhook_id = request.path_params["webhook_id"]
    if not await run(models.delete_webhook, get_store(request), webhook_id, actor_from(request).user.id):
        return not_found("no such webhook")
    audit(request, KIND_ADMIN, "webhook deleted", {"webhook_id": webhook_id})
    return JSONResponse({"ok": True})


async def restore_webhook(request: Request) -> Response:
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    webhook_id = request.path_params["webhook_id"]
    actor = actor_from(request).user
    try:
        hook = await run(models.restore_webhook, get_store(request), webhook_id, actor.id, actor.is_admin)
    except RestoreError as refusal:
        return restore_refused(refusal)
    audit(request, KIND_ADMIN, "webhook restored", {"webhook_id": webhook_id})
    return JSONResponse(_row(hook))


async def _live_webhook(request: Request) -> models.Webhook | None:
    """An endpoint the screen can act on."""
    hook = await run(models.get_webhook, get_store(request), request.path_params["webhook_id"])
    return None if hook is None or hook.deleted_at is not None else hook


async def send_test(request: Request) -> Response:
    """Queues a webhook.test event. For an endpoint that is switched off, the
    delivery says so rather than the event going out."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    hook = await _live_webhook(request)
    if hook is None:
        return not_found("no such webhook")
    await run(request.app.state.webhooks.send_test, hook)
    return JSONResponse({"ok": True}, status_code=202)


async def resend_delivery(request: Request) -> Response:
    """Queues a delivery that was given up on."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    hook = await _live_webhook(request)
    if hook is None:
        return not_found("no such webhook")
    delivery_id = request.path_params["delivery_id"]
    try:
        sent = await run(request.app.state.webhooks.resend, hook.id, delivery_id)
    except models.DeliveryNotFailed:
        return conflict("delivery_not_failed")
    if not sent:
        return not_found("no such delivery")
    audit(request, KIND_ADMIN, "webhook delivery resent", {"webhook_id": hook.id, "delivery_id": delivery_id})
    return JSONResponse({"ok": True}, status_code=202)


async def list_deliveries(request: Request) -> Response:
    hook = await _live_webhook(request)
    if hook is None:
        return not_found("no such webhook")
    deliveries = await run(models.list_deliveries, get_store(request), hook.id)
    return send_page(request, "deliveries", [_delivery_row(delivery) for delivery in deliveries])
