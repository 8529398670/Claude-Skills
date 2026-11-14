"""The web server. This module only wires things together -- load config,
resolve assets, open the database, build the app, start and stop cleanly.
Every decision of substance lives in a module under app/core/, app/models/,
or app/routes/. If you find yourself adding business logic here, it belongs
in a route module or a new core module instead.

Started by `python -m app` (see app/__main__.py), never by pointing uvicorn
at it from the command line: host, port, and every other setting come from
config.yaml, and a second place to pass them would be a second source of
truth.
"""
from __future__ import annotations

import asyncio
from contextlib import asynccontextmanager
from typing import AsyncIterator

from starlette.applications import Starlette
from starlette.concurrency import run_in_threadpool
from starlette.middleware import Middleware
from starlette.middleware.body_limit import RequestBodyLimitMiddleware

from app.core import assets, bootstrap, config, config_live, db, graceful, housekeeping, jobs, language, logs, server_lock
from app.core import metrics  # feature:metrics
from app.core import sse  # feature:logs
from app.core import webhooks  # feature:webhooks
from app.core.maintenance import Maintenance  # feature:maintenance
from app.core.middleware import EXCEPTION_HANDLERS, RateLimit, SecurityHeaders
from app.core.request_id import RequestID
from app.core.request_log import RequestLog  # feature:logs
from app.core.security import LoadActor
from app.core.static_files import StaticServer
from app.models import webhooks as webhook_models  # feature:webhooks
from app.routes import build_routes

# How often config.yaml is checked for edits made outside the Settings screen.
CONFIG_POLL_SECONDS = 2


async def config_watch(settings: config_live.Holder) -> None:
    """Apply edits to config.yaml made by hand, or by `config set`, while the
    server runs. See config_live.Holder.poll."""
    while True:
        await asyncio.sleep(CONFIG_POLL_SECONDS)
        try:
            await run_in_threadpool(settings.poll)
        except Exception as error:  # noqa: BLE001 -- a bad poll must never take the server down
            print(f"[config] check failed: {error}", flush=True)


def build_app(
    settings: config_live.Holder, store: db.Store, lang: language.Language, static_server: StaticServer, version: str = "dev"
) -> Starlette:
    # The configuration the server started with: what is set up once (the
    # body limit, the log's routes and directory). Everything read per
    # request comes from settings, which follows edits to config.yaml.
    cfg = settings.boot()
    # >>> feature:logs
    # The activity log writes from its own thread; closing it, last, lets the
    # final requests make it to disk.
    logger = logs.Logger(cfg.logs, cfg.secret_key, cfg.encrypt_at_rest)

    # A change to config.yaml reaches the activity log's own settings, and is
    # reported on stdout and in the log itself.
    settings.on_change(lambda old, new: logger.apply(new.logs))
    # <<< feature:logs
    logger = logs.Logger()  # feature:!logs

    def report(level: str, message: str, detail: dict) -> None:
        print(f"[config] {message}", flush=True)
        logger.system(level, message, detail)

    settings.report = report

    registry = jobs.Registry(store, logger)
    housekeeping.register(registry, store, settings, logger)
    # >>> feature:webhooks
    dispatcher = webhooks.Dispatcher(store, settings)
    dispatcher.registry = registry
    registry.register(webhooks.JOB_NAME, webhooks.JOB_INTERVAL_SECONDS, dispatcher.run)

    def job_failed(name: str) -> None:
        # Not its own failure: that would queue a delivery about being unable
        # to deliver.
        if name != webhooks.JOB_NAME:
            dispatcher.emit(webhook_models.EVENT_JOB_FAILED, {"job": name})

    registry.on_failure = job_failed
    # <<< feature:webhooks
    log_hub = sse.Hub()  # feature:logs
    measured = metrics.Metrics()  # feature:metrics

    @asynccontextmanager
    async def lifespan(app: Starlette) -> AsyncIterator[None]:
        loop = asyncio.get_running_loop()  # feature:logs|webhooks
        # >>> feature:logs
        logger.on_flushed = lambda: sse.publish_from_thread(loop, log_hub, sse.CHANGED)
        # <<< feature:logs
        dispatcher.loop = loop  # feature:webhooks
        logger.start()  # feature:logs
        logger.system(logs.LEVEL_INFO, "server started", {"version": version})
        if cfg.secret_migrated:
            logger.system(logs.LEVEL_WARN, "secret key moved to secret.key", {"config": str(cfg.config_file), "secret": str(cfg.secret_file)})
        watcher = asyncio.create_task(config_watch(settings))
        registry.start()
        try:
            yield
        finally:
            # By now requests have finished and streams have ended (serve's
            # on_stop). Jobs next, since they write to the log; the log last.
            watcher.cancel()
            dispatcher.loop = None  # feature:webhooks
            await registry.stop()
            log_hub.close()  # feature:logs
            logger.on_flushed = None  # feature:logs
            logger.system(logs.LEVEL_INFO, "server stopped")
            await run_in_threadpool(logger.close)  # feature:logs

    app = Starlette(
        routes=build_routes(static_server),
        # Outermost first. Every request gets its reference and is counted,
        # whatever becomes of it; headers go on everything; the body limit is
        # here rather than Starlette's max_body_size= because that one sits
        # outside all of this, and its 413 would have neither; the rate limit
        # runs before anything touches the database, so a flood is cheap to
        # refuse; then the caller is resolved once for every route behind it;
        # then each request is written to the activity log, knowing who made
        # it; and only then can maintenance mode refuse it, knowing who asked.
        middleware=[
            Middleware(RequestID),
            Middleware(metrics.MeasureRequests, metrics=measured),  # feature:metrics
            Middleware(SecurityHeaders, settings=settings),
            Middleware(RequestBodyLimitMiddleware, max_body_size=cfg.body_limit_bytes),
            Middleware(RateLimit, settings=settings),
            Middleware(LoadActor, store=store, settings=settings),
            Middleware(RequestLog, settings=settings, logger=logger),  # feature:logs
            Middleware(Maintenance, store=store, settings=settings),  # feature:maintenance
        ],
        exception_handlers=EXCEPTION_HANDLERS,
        lifespan=lifespan,
    )
    app.state.settings = settings
    app.state.store = store
    app.state.logs = logger
    app.state.jobs = registry
    app.state.webhooks = dispatcher  # feature:webhooks
    app.state.log_hub = log_hub  # feature:logs
    app.state.metrics = measured  # feature:metrics
    app.state.language = lang
    # Reported to MCP clients as serverInfo.version.
    app.state.version = version
    return app


def serve(version: str) -> None:
    cfg = config.load()
    if cfg.rotation_pending:
        raise RuntimeError(config.ROTATION_PENDING_MESSAGE)
    if cfg.rotation_finished:
        print(f"[secret] finished a key rotation that had been cut short; the new key is in {cfg.secret_file}", flush=True)
    # Held until the process ends. See app/core/server_lock.py.
    lock = server_lock.acquire(cfg.app_dir / config.SERVER_LOCK_NAME, timeout_seconds=5)
    if lock is None:
        raise RuntimeError(f"another server is already running on {cfg.app_dir}")
    # Everything in static_dir is public, so it may not be, hold, or sit
    # inside the config or state directory. See app/core/assets.py.
    assets.check_static_dir(cfg.static_dir, cfg.config_file.parent, cfg.app_dir, cfg.secret_file, cfg.database_path)
    bundle = assets.resolve(cfg.static_dir, cfg.language_file)
    store = db.open_store(cfg.database_path, cfg.secret_key, cfg.encrypt_at_rest)
    lang = language.parse(bundle.language_yaml)
    static_server = StaticServer(bundle.static_dir)

    bootstrap.ensure_first_admin(store, cfg, "Admin")

    app = build_app(config_live.Holder(cfg), store, lang, static_server, version)

    print(f"[start] {cfg.project_name} {version} listening on {cfg.listen_address} (secure cookies: {cfg.secure_cookies})")
    print(f"[url]    {cfg.url}  (from {cfg.url_source})")
    if cfg.config_created:
        print(f"[config] {cfg.config_file} (created; edits apply while the server runs)")
    else:
        print(f"[config] {cfg.config_file} (edits apply while the server runs)")
    if cfg.secret_migrated:
        print(f"[secret] {cfg.secret_file}")
        print("[secret] ================================================================")
        print(f"[secret] MOVED server.secret_key out of {cfg.config_file}")
        print(f"[secret] into {cfg.secret_file} (mode 0600). config.yaml holds no secret now.")
        print("[secret] Back secret.key up with the database: nothing else can read it.")
        print("[secret] ================================================================")
    elif cfg.secret_created:
        print(f"[secret] {cfg.secret_file} (created, with a new key -- back it up)")
    else:
        print(f"[secret] {cfg.secret_file}")
    if cfg.config_warning:
        print(f"[config] warning: {cfg.config_warning}")
    print(f"[state]  {cfg.app_dir}")
    # >>> feature:logs
    if cfg.logs.enabled:
        print(f"[logs]   {cfg.logs.dir} (keeping {cfg.logs.retention_days} days, up to {cfg.logs.max_total_bytes // 1024 // 1024} MB)")
    # <<< feature:logs
    print(f"[assets] {bundle.source}", flush=True)

    graceful.run(
        app,
        # >>> feature:logs
        # Before uvicorn waits for connections: it would otherwise wait the
        # full 10 seconds for a stream that is never going to finish.
        on_stop=app.state.log_hub.close,
        # <<< feature:logs
        on_stop=lambda: None,  # feature:!logs
        host=cfg.host,
        port=cfg.port,
        # An empty Server header keeps the response from advertising which
        # server and version is running -- free reconnaissance otherwise.
        server_header=False,
        # Proxy trust is server.trust_proxy, applied by the rate limiter's
        # client_ip(); uvicorn's own X-Forwarded-For handling would be a
        # second, differently configured answer to the same question.
        proxy_headers=False,
        # Idle keep-alive connections are dropped rather than held until the
        # process runs out of sockets; in-flight requests get 10s to finish
        # on shutdown so the database is never left mid-write.
        timeout_keep_alive=75,
        timeout_graceful_shutdown=10,
        lifespan="on",
        log_level="info",
        # Every request is in the activity log already, and with logs.echo
        # on, a one-line summary goes to stdout from there (with the real
        # client address and who it was); uvicorn's own line would repeat it.
        access_log=False,
    )
