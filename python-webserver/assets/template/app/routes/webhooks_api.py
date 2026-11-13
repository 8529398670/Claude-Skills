# whole-file feature:webhooks
"""The API docs for webhooks.py's routes: config/openapi.json and config/api.md are
written from these by scripts/api_docs.sh (see app/apidocs)."""
from __future__ import annotations

from typing import Annotated, NotRequired, TypedDict

from app.apidocs import Field, OWNER, SESSION_ONLY, doc
from app.routes import webhooks
from app.routes.schemas_api import WebhookDeliverySchema
from app.routes.schemas_api import WebhookSchema


class ListWebhooksParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]
    q: Annotated[str, Field(query="q", doc="Keep webhooks whose name or address contains this, in any case.")]
    sort: Annotated[str, Field(query="sort", enum=("name", "url", "enabled"), doc="Field to sort the whole list by. Rows without it go last.")]
    dir: Annotated[str, Field(query="dir", enum=("asc", "desc"), doc="Default asc.")]


class ListWebhooksReply(TypedDict):
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    total: int
    webhooks: list[WebhookSchema]


doc(
    webhooks.list_webhooks,
    summary="Webhooks",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    params=ListWebhooksParams,
    reply=ListWebhooksReply,
    errors=(400, 404),
)


class CreateWebhookBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    events: list[str]
    name: str
    url: str


class CreateWebhookReply(WebhookSchema):
    secret: Annotated[str, Field(doc="Shown in this response only.")]


doc(
    webhooks.create_webhook,
    summary="Create a webhook",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    body=CreateWebhookBody,
    status=201,
    reply=CreateWebhookReply,
    errors=(400, 403, 404),
)


class ListEventsReply(TypedDict):
    events: list[str]


doc(
    webhooks.list_events,
    summary="Events a webhook can ask for",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    reply=ListEventsReply,
    errors=(404,),
)


class DeleteWebhookBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class DeleteWebhookReply(TypedDict):
    ok: bool


doc(
    webhooks.delete_webhook,
    summary="Delete a webhook",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    body=DeleteWebhookBody,
    reply=DeleteWebhookReply,
    errors=(400, 403, 404),
)


class ListDeliveriesParams(TypedDict):
    limit: Annotated[int, Field(query="limit", minimum=1, maximum=1000, doc="Rows to return. Default 200.")]
    offset: Annotated[int, Field(query="offset", minimum=0)]


class ListDeliveriesReply(TypedDict):
    deliveries: list[WebhookDeliverySchema]
    next: Annotated[int | None, Field(doc="Offset of the next page, or null on the last one.")]
    total: int


doc(
    webhooks.list_deliveries,
    summary="Recent deliveries",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    params=ListDeliveriesParams,
    reply=ListDeliveriesReply,
    errors=(400, 404),
)


class ResendDeliveryBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class ResendDeliveryReply(TypedDict):
    ok: bool


doc(
    webhooks.resend_delivery,
    summary="Send a failed delivery again",
    description="A new delivery with the same body. The failed one stays listed.",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    body=ResendDeliveryBody,
    status=202,
    reply=ResendDeliveryReply,
    errors=(400, 401, 403, 404, 409),
)


class SetEnabledBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    enabled: bool


class SetEnabledReply(TypedDict):
    ok: bool


doc(
    webhooks.set_enabled,
    summary="Switch a webhook on or off",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    body=SetEnabledBody,
    reply=SetEnabledReply,
    errors=(400, 403, 404),
)


class RenameWebhookBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    expect: NotRequired[Annotated[str, Field(doc="The name you think it has. Answers 409 if it has another.")]]
    name: Annotated[str, Field(doc="1-80 characters.")]


class RenameWebhookReply(TypedDict):
    name: str
    ok: bool


doc(
    webhooks.rename_webhook,
    summary="Rename a webhook",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    body=RenameWebhookBody,
    reply=RenameWebhookReply,
    errors=(400, 403, 404, 409),
)


class RestoreWebhookBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


doc(
    webhooks.restore_webhook,
    summary="Undo deleting a webhook",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    body=RestoreWebhookBody,
    reply=WebhookSchema,
    errors=(400, 403, 404, 409),
)


class RotateWebhookSecretBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class RotateWebhookSecretReply(TypedDict):
    secret: Annotated[str, Field(doc="Shown only here. The old one has stopped working.")]


doc(
    webhooks.rotate_webhook_secret,
    summary="Replace a webhook's signing secret",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    body=RotateWebhookSecretBody,
    reply=RotateWebhookSecretReply,
    errors=(400, 403, 404),
)


class SetWebhookTargetBodyExpect(TypedDict):
    events: list[str]
    url: str


class SetWebhookTargetBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]
    events: list[str]
    expect: NotRequired[Annotated[SetWebhookTargetBodyExpect, Field(doc="What you think it has now. Answers 409 if it has something else.")]]
    url: Annotated[str, Field(doc="Checked like a new webhook's.")]


class SetWebhookTargetReply(TypedDict):
    ok: bool


doc(
    webhooks.set_webhook_target,
    summary="Change a webhook's address and events",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    body=SetWebhookTargetBody,
    reply=SetWebhookTargetReply,
    errors=(400, 403, 404, 409),
)


class SendTestBody(TypedDict):
    csrf_token: NotRequired[Annotated[str, Field(doc="From GET /api/me. Browser sessions only.")]]


class SendTestReply(TypedDict):
    ok: bool


doc(
    webhooks.send_test,
    summary="Send a test event",
    tag="Webhooks",
    access=SESSION_ONLY,
    role=OWNER,
    feature="webhooks",
    body=SendTestBody,
    status=202,
    reply=SendTestReply,
    errors=(400, 403, 404),
)
