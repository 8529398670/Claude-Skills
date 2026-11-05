"""The jobs the template itself runs (app/core/jobs.py): dropping what has
expired, applying the activity log's retention, the scheduled backup, and
clearing out files. An
app adds its own the same way, from main.build_app.
"""
from __future__ import annotations

from typing import Callable

from app.core import backup, config_live, db, logs
from app.core.jobs import Registry
from app.models import api_keys, files, invite_links, login_tokens, sessions, sign_in, webhooks

HOUR_SECONDS = 60 * 60


def purge(store: db.Store) -> tuple[str, str]:
    """Drop expired sessions, expired login tokens, the API keys and invite
    links that have been dead long enough to stop being worth listing, and old
    webhook rows.
    None of it is a correctness issue -- every one of those is already refused
    on read -- so this is only here to keep the file from growing forever.

    Every step runs even when one fails, so a damaged invite link cannot stop
    expired sessions being dropped, and the errors are reported together."""
    steps: tuple[tuple[str, Callable[[db.Store], int]], ...] = (
        ("sessions", sessions.purge_expired_sessions),
        ("login tokens", login_tokens.purge_expired_login_tokens),
        ("api keys", api_keys.purge_dead_api_keys),
        ("invite links", invite_links.purge_dead_invite_links),
        ("sign-in challenges", sign_in.purge_expired_challenges),
        ("webhook rows", webhooks.purge_webhooks),
    )
    removed: list[str] = []
    errors: list[str] = []
    for name, step in steps:
        try:
            count = step(store)
        except Exception as error:  # noqa: BLE001 -- the next step still runs
            errors.append(f"{name}: {error}")
            continue
        if count:
            removed.append(f"{count} {name}")
    note = f"removed {', '.join(removed)}" if removed else "nothing to remove"
    if removed:
        print(f"[purge] {note}", flush=True)
    return note, "; ".join(errors)


def prune_logs(logger: logs.Logger) -> tuple[str, str]:
    """The activity log's retention: compress, expire, recycle."""
    pruned = logger.prune()
    if pruned["deleted"] or pruned["compressed"]:
        print(f"[purge] logs: {pruned['deleted']} days deleted, {pruned['compressed']} compressed", flush=True)
        logger.system(logs.LEVEL_INFO, "logs pruned", {"deleted": pruned["deleted"], "compressed": pruned["compressed"]})
    return f"{pruned['deleted']} days deleted, {pruned['compressed']} compressed", ""


def scheduled_backup(store: db.Store, settings: config_live.Holder, logger: logs.Logger) -> tuple[str, str]:
    """The scheduled backup, when one is on and due. The job is hourly and
    backups.interval_hours decides which of those passes write a file. The
    settings are read on every pass, so switching it on needs no restart."""
    cfg = settings.get()
    if not cfg.backups.enabled:
        return "off", ""
    made, removed = backup.run_scheduled(store, cfg)
    if made is None:
        return "not due", ""
    print(f"[backup] {made.path} ({made.size} bytes), {removed} old backups removed", flush=True)
    logger.system(logs.LEVEL_INFO, "scheduled backup written", {"path": str(made.path), "bytes": made.size, "removed": removed})
    return f"wrote {made.path.name}", ""


def sweep_files(store: db.Store, settings: config_live.Holder) -> tuple[str, str]:
    """Remove what uploads leave behind; see files.sweep_files. It runs with
    files.enabled off as well: switching uploads off is no reason to keep a
    deleted file past its undo window."""
    removed, strays = files.sweep_files(store, settings.get())
    return f"removed {removed} files, {strays} strays", ""


def register(registry: Registry, store: db.Store, settings: config_live.Holder, logger: logs.Logger) -> None:
    registry.register("purge", HOUR_SECONDS, lambda: purge(store))
    if logger.enabled:
        registry.register("prune-logs", HOUR_SECONDS, lambda: prune_logs(logger))
    registry.register("backup", HOUR_SECONDS, lambda: scheduled_backup(store, settings, logger))
    registry.register("files", HOUR_SECONDS, lambda: sweep_files(store, settings))
