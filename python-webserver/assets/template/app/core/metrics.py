"""What GET /metrics reports, in Prometheus's text format, written by hand: a
few counters do not need a client library.

The names, labels and buckets are the same as the Go template's, and carry a
fixed app_ prefix rather than the project's name, so one dashboard fits every
app made from either. There are no path labels, and methods come from a fixed
list: a label anyone can invent by sending a request is an unbounded series
count. Hence "OTHER".

Its own middleware, because RequestLog is skipped when the log is off and on
the paths it never logs. Everything here runs on the event loop, so none of it
needs a lock.
"""
from __future__ import annotations

import time
from datetime import datetime
from typing import Mapping

from starlette.datastructures import Headers
from starlette.types import ASGIApp, Message, Receive, Scope, Send

from app.models.jobs import JobRecord

CONTENT_TYPE = "text/plain; version=0.0.4; charset=utf-8"
METHODS = frozenset({"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
BUCKETS = (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)
EVENT_STREAM = "text/event-stream"


def status_class(status: int) -> str:
    return f"{status // 100}xx"


def _label(value: str) -> str:
    return value.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")


def _family(lines: list[str], name: str, kind: str, help_text: str) -> None:
    lines.append(f"# HELP {name} {help_text}")
    lines.append(f"# TYPE {name} {kind}")


def _timestamp(moment: datetime) -> str:
    return f"{moment.timestamp():.3f}"


class Metrics:
    def __init__(self) -> None:
        self.started = time.monotonic()
        self.requests: dict[tuple[str, str], int] = {}
        self.in_flight = 0
        # One count per bucket, not yet cumulative; the last is +Inf.
        self.buckets = [0] * (len(BUCKETS) + 1)
        self.duration_sum = 0.0
        self.duration_count = 0

    def count(self, method: str, status: int) -> None:
        key = (method if method in METHODS else "OTHER", status_class(status))
        self.requests[key] = self.requests.get(key, 0) + 1

    def observe(self, seconds: float) -> None:
        self.duration_sum += seconds
        self.duration_count += 1
        for index, bound in enumerate(BUCKETS):
            if seconds <= bound:
                self.buckets[index] += 1
                return
        self.buckets[-1] += 1

    def render(self, logs_dropped: int, jobs: Mapping[str, JobRecord]) -> str:
        lines: list[str] = []

        _family(lines, "app_http_requests_total", "counter", "HTTP requests answered, by method and status class.")
        for (method, klass), count in sorted(self.requests.items()):
            lines.append(f'app_http_requests_total{{method="{method}",status_class="{klass}"}} {count}')

        name = "app_http_request_duration_seconds"
        _family(lines, name, "histogram", "How long requests took, live streams excepted.")
        total = 0
        for bound, count in zip(BUCKETS, self.buckets):
            total += count
            lines.append(f'{name}_bucket{{le="{bound:g}"}} {total}')
        lines.append(f'{name}_bucket{{le="+Inf"}} {self.duration_count}')
        lines.append(f"{name}_sum {self.duration_sum:.6f}")
        lines.append(f"{name}_count {self.duration_count}")

        _family(lines, "app_http_requests_in_flight", "gauge", "Requests being answered now, live streams excepted.")
        lines.append(f"app_http_requests_in_flight {self.in_flight}")

        _family(lines, "app_uptime_seconds", "gauge", "Seconds since this process started.")
        lines.append(f"app_uptime_seconds {time.monotonic() - self.started:.3f}")

        _family(lines, "app_logs_dropped_total", "counter", "Activity log entries dropped because the writer fell behind.")
        lines.append(f"app_logs_dropped_total {logs_dropped}")

        name = "app_job_last_success_timestamp_seconds"
        _family(lines, name, "gauge", "When each background job last succeeded.")
        for job, record in sorted(jobs.items()):
            if record.last_success is not None:
                lines.append(f'{name}{{job="{_label(job)}"}} {_timestamp(record.last_success)}')

        name = "app_job_last_failure_timestamp_seconds"
        _family(lines, name, "gauge", "When each background job last failed.")
        for job, record in sorted(jobs.items()):
            if record.last_failure is not None:
                lines.append(f'{name}{{job="{_label(job)}"}} {_timestamp(record.last_failure)}')

        return "\n".join(lines) + "\n"


class MeasureRequests:
    """A stream is open for as long as someone watches, which says nothing
    about how the server is doing, so it leaves in_flight the moment it turns
    out to be one and never reaches the histogram. It is still counted when it
    ends."""

    def __init__(self, app: ASGIApp, metrics: Metrics) -> None:
        self.app = app
        self.metrics = metrics

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        metrics = self.metrics
        started = time.perf_counter()
        # What a crash before any response comes to: server_error's 500 is
        # sent outside this middleware.
        status = 500
        streaming = False
        metrics.in_flight += 1

        async def measure(message: Message) -> None:
            nonlocal status, streaming
            if message["type"] == "http.response.start":
                status = message["status"]
                if Headers(raw=message["headers"]).get("content-type", "").startswith(EVENT_STREAM):
                    streaming = True
                    metrics.in_flight -= 1
            await send(message)

        try:
            await self.app(scope, receive, measure)
        finally:
            metrics.count(scope.get("method", ""), status)
            if not streaming:
                metrics.in_flight -= 1
                metrics.observe(time.perf_counter() - started)
