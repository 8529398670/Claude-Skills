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

import uvicorn
from starlette.applications import Starlette
from starlette.concurrency import run_in_threadpool
from starlette.middleware import Middleware

from app.core import assets, bootstrap, config, config_live, db, language, logs
from app.core.middleware import EXCEPTION_HANDLERS, RateLimit, SecurityHeaders
from app.core.request_log import RequestLog
from app.core.security import LoadActor
from app.core.static_files import StaticServer
from app.models import api_keys, invite_links, login_tokens, sessions, sign_in
from app.routes import build_routes

PURGE_INTERVAL_SECONDS = 60 * 60
# How often config.yaml is checked for edits made outside the Settings screen.
CONFIG_POLL_SECONDS = 2


def purge_once(store: db.Store, logger: logs.Logger | None = None) -> None:
    """Drop expired sessions, expired login tokens, and the API keys and
    invite links that have been dead long enough to stop being worth listing.
    None of it is a correctness issue -- every one of those is already refused
    on read -- so this runs quietly in the background purely to keep the file
    from growing forever. Then apply the activity log's retention: compress,
    expire, recycle."""
    removed_sessions = sessions.purge_expired_sessions(store)
    removed_tokens = login_tokens.purge_expired_login_tokens(store)
    removed_keys = api_keys.purge_dead_api_keys(store)
    removed_invites = invite_links.purge_dead_invite_links(store)
    sign_in.purge_expired_challenges(store)
    if removed_sessions or removed_tokens or removed_keys or removed_invites:
        print(
            f"[purge] removed {removed_sessions} expired sessions, {removed_tokens} expired login tokens, "
            f"{removed_keys} dead api keys, {removed_invites} dead invite links",
            flush=True,
        )
    if logger is not None and logger.enabled:
        pruned = logger.prune()
        if pruned["deleted"] or pruned["compressed"]:
            print(f"[purge] logs: {pruned['deleted']} days deleted, {pruned['compressed']} compressed", flush=True)
            logger.system(logs.LEVEL_INFO, "logs pruned", {"deleted": pruned["deleted"], "compressed": pruned["compressed"]})


async def purge_loop(store: db.Store, logger: logs.Logger) -> None:
    while True:
        await asyncio.sleep(PURGE_INTERVAL_SECONDS)
        try:
            await run_in_threadpool(purge_once, store, logger)
        except Exception as error:  # noqa: BLE001 -- housekeeping must never take the server down
            print(f"[purge] failed: {error}", flush=True)


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
    # The activity log writes from its own thread; closing it, last, lets the
    # final requests make it to disk.
    logger = logs.Logger(cfg.logs, cfg.secret_key, cfg.encrypt_at_rest)

    # A change to config.yaml reaches the activity log's own settings, and is
    # reported on stdout and in the log itself.
    settings.on_change(lambda old, new: logger.apply(new.logs))

    def report(level: str, message: str, detail: dict) -> None:
        print(f"[config] {message}", flush=True)
        logger.system(level, message, detail)

    settings.report = report

    @asynccontextmanager
    async def lifespan(app: Starlette) -> AsyncIterator[None]:
        logger.start()
        logger.system(logs.LEVEL_INFO, "server started", {"version": version})
        if cfg.secret_migrated:
            logger.system(logs.LEVEL_WARN, "secret key moved to secret.key", {"config": str(cfg.config_file), "secret": str(cfg.secret_file)})
        tasks = [asyncio.create_task(purge_loop(store, logger)), asyncio.create_task(config_watch(settings))]
        try:
            yield
        finally:
            for task in tasks:
                task.cancel()
            logger.system(logs.LEVEL_INFO, "server stopped")
            await run_in_threadpool(logger.close)

    app = Starlette(
        routes=build_routes(static_server, cfg),
        # Outermost first. Headers go on everything; the rate limit runs
        # before anything touches the database, so a flood is cheap to refuse;
        # then the caller is resolved once for every route behind it; then
        # each request is written to the activity log, knowing who made it.
        middleware=[
            Middleware(SecurityHeaders, settings=settings),
            Middleware(RateLimit, settings=settings),
            Middleware(LoadActor, store=store, settings=settings),
            Middleware(RequestLog, settings=settings, logger=logger),
        ],
        exception_handlers=EXCEPTION_HANDLERS,
        lifespan=lifespan,
        max_body_size=cfg.body_limit_bytes,
    )
    app.state.settings = settings
    app.state.store = store
    app.state.logs = logger
    app.state.language = lang
    # Reported to MCP clients as serverInfo.version.
    app.state.version = version
    return app


def serve(version: str) -> None:
    cfg = config.load()
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
    if cfg.logs.enabled:
        print(f"[logs]   {cfg.logs.dir} (keeping {cfg.logs.retention_days} days, up to {cfg.logs.max_total_bytes // 1024 // 1024} MB)")
    print(f"[assets] {bundle.source}", flush=True)

    uvicorn.run(
        app,
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
