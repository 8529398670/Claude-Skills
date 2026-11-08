# whole-file feature:files
"""Uploaded files. The record here describes one; the bytes are on disk. Go's
twin is server/models/file.go.

A file arrives in pieces of CHUNK_BYTES, each an ordinary request under the
global body limit, so nothing here raises that limit and both templates speak
the same protocol. It also means a file is encrypted as it arrives and never
sits in a temp directory in the clear.

Each file has its own random key, and only that key is sealed with the secret
key. Moving to a new secret key therefore rewrites one field of the record (see
sealed.SEALED_FIELDS) rather than every file on disk, and needs nothing from
rotate-key or a restore beyond what they already do for a sealed field.

Each piece is sealed on its own, bound to the file's id, its index and the
number of pieces, so it opens only where it belongs. All pieces but the last are
the same size, which puts every piece at an offset that can be computed and
leaves the disk format with nothing else in it.
"""
from __future__ import annotations

import os
import threading
import time
import unicodedata
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from typing import Iterator

from app.core import config_secret, encryption
from app.core.config import FILE_CHUNK_BYTES as CHUNK_BYTES
from app.core.config import Config
from app.core.db import BUCKET_FILES, Record, Store, Transaction
from app.models import RestoreGoneError, deleted_long_ago, from_iso, mark_deleted, to_iso, unmark_deleted, utc_now
from app.models import users

UPLOADING = "uploading"
READY = "ready"

# What sealing adds to a piece: nonce and tag.
CHUNK_OVERHEAD = 24 + 16
NAME_MAX = 255
TYPE_MAX = 127
# How old a file with no record must be before it is removed. Finishing a file
# renames it before the record says so, and a restore can leave either one
# without the other.
STRAY_AGE_SECONDS = 24 * 60 * 60


class BadFileError(Exception):
    def __init__(self) -> None:
        super().__init__("a file needs a name and a size")


class BadFileNameError(ValueError):
    def __init__(self) -> None:
        super().__init__("a file name must be 1-255 bytes")


class FileTooLargeError(Exception):
    """The message is a code, worded in language.yaml."""

    def __init__(self) -> None:
        super().__init__("file_too_large")


class FileQuotaError(Exception):
    """The message is a code, worded in language.yaml."""

    def __init__(self) -> None:
        super().__init__("file_quota")


class ChunkSizeError(Exception):
    def __init__(self) -> None:
        super().__init__("that piece is the wrong size")


class FileMissingError(Exception):
    """A record whose bytes are not there."""


@dataclass
class File:
    id: str
    user_id: int
    name: str
    # What the uploader said it is. It is shown, and never served: a download
    # is always application/octet-stream.
    type: str
    size: int
    chunks: int
    received: int
    state: str
    # This file's own key, sealed with the secret key. It is in SEALED_FIELDS,
    # so it follows a rotation.
    key: str
    created_at: datetime
    # When the last piece arrived.
    updated_at: datetime
    deleted_at: datetime | None = None
    deleted_by: int | None = None

    @property
    def deleted(self) -> bool:
        return self.deleted_at is not None

    def chunk_length(self, index: int) -> int:
        """How many bytes piece index holds."""
        return self.size - index * CHUNK_BYTES if index == self.chunks - 1 else CHUNK_BYTES

    @property
    def sealed_size(self) -> int:
        return self.size + self.chunks * CHUNK_OVERHEAD

    def bound(self, index: int) -> bytes:
        """What a piece is sealed to. Without the count, a file cut off after
        a whole piece would still open."""
        return f"{self.id}|{index}|{self.chunks}".encode()

    def to_record(self) -> Record:
        return {
            "id": self.id,
            "user_id": self.user_id,
            "name": self.name,
            "type": self.type,
            "size": self.size,
            "chunks": self.chunks,
            "received": self.received,
            "state": self.state,
            "key": self.key,
            "created_at": to_iso(self.created_at),
            "updated_at": to_iso(self.updated_at),
            "deleted_at": to_iso(self.deleted_at),
            "deleted_by": self.deleted_by,
        }

    @classmethod
    def from_record(cls, record: Record) -> "File":
        return cls(
            id=record["id"],
            user_id=record["user_id"],
            name=record["name"],
            type=record["type"],
            size=record["size"],
            chunks=record["chunks"],
            received=record["received"],
            state=record["state"],
            key=record["key"],
            created_at=from_iso(record["created_at"]),
            updated_at=from_iso(record["updated_at"]),
            deleted_at=from_iso(record.get("deleted_at")),
            deleted_by=record.get("deleted_by"),
        )


def chunk_count(size: int) -> int:
    return (size + CHUNK_BYTES - 1) // CHUNK_BYTES


def sealed_offset(index: int) -> int:
    """Where piece index starts on disk."""
    return index * (CHUNK_BYTES + CHUNK_OVERHEAD)


def upload_path(cfg: Config, file_id: str) -> Path:
    return cfg.files.uploads_dir / file_id


def finished_path(cfg: Config, file_id: str) -> Path:
    return cfg.files.dir / file_id


def clean_name(name: str) -> str:
    """The last part of whatever path a client sent, without control
    characters. It is only ever shown and offered as a download name: paths on
    disk use the id."""
    name = name.replace("\\", "/").rsplit("/", 1)[-1]
    return "".join(character for character in name if unicodedata.category(character) != "Cc" and character != "�").strip()


def _used_bytes(transaction: Transaction, user_id: int) -> int:
    """What an account holds against its quota. Uploads still arriving count
    at their full size, or several at once could pass the check together."""
    records = (record for _, record in transaction.all(BUCKET_FILES))
    return sum(record["size"] for record in records if record["user_id"] == user_id and record.get("deleted_at") is None)


def storage_used(store: Store, user_id: int | None = None) -> int:
    """_used_bytes outside a transaction, for showing it: one account, or every
    account for None."""
    records = (record for _, record in store.all(BUCKET_FILES))
    return sum(
        record["size"]
        for record in records
        if (user_id is None or record["user_id"] == user_id) and record.get("deleted_at") is None
    )


def create_file(store: Store, cfg: Config, user_id: int, name: str, declared_type: str, size: int) -> File:
    """Start an upload. The quota is checked in the same transaction that adds
    the record, so two uploads cannot both take the last of it."""
    name = clean_name(name)
    if not name or len(name.encode()) > NAME_MAX or size < 0 or len(declared_type.encode()) > TYPE_MAX:
        raise BadFileError()
    if size > cfg.files.max_file_bytes:
        raise FileTooLargeError()
    now = utc_now()
    file = File(
        id=encryption.url_token(12),
        user_id=user_id,
        name=name,
        type=declared_type,
        size=size,
        chunks=chunk_count(size),
        received=0,
        state=UPLOADING,
        key=encryption.xchacha_encrypt_string(cfg.secret_key, config_secret.generate_secret()),
        created_at=now,
        updated_at=now,
    )
    with store.transaction() as transaction:
        if _used_bytes(transaction, user_id) + size > cfg.files.quota_bytes_per_user:
            raise FileQuotaError()
        transaction.put(BUCKET_FILES, file.id, file.to_record())
    return file


def get_file(store: Store, file_id: str) -> File | None:
    """A file, deleted or not."""
    record = store.get(BUCKET_FILES, file_id)
    return None if record is None else File.from_record(record)


def list_files(store: Store, user_id: int | None = None) -> list[File]:
    """The finished files, newest first: one account's, or everyone's."""
    files = [File.from_record(record) for _, record in store.all(BUCKET_FILES)]
    files = [file for file in files if file.state == READY and not file.deleted and user_id in (None, file.user_id)]
    files.sort(key=lambda file: file.created_at, reverse=True)
    return files


def write_chunk(store: Store, cfg: Config, file: File, index: int, plain: bytes) -> None:
    """Seal one piece and put it at its place in the upload. It goes to a
    computed offset, and whatever follows is cut off, so a retry after a crash
    between the write and the record's update simply writes it again. The
    caller holds the UploadLocks entry and has checked index is the next one."""
    if len(plain) != file.chunk_length(index):
        raise ChunkSizeError()
    file_key = encryption.xchacha_decrypt_string(cfg.secret_key, file.key)
    sealed = encryption.xchacha_encrypt_bound(file_key, plain, file.bound(index))

    cfg.files.uploads_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    flags = os.O_WRONLY | (os.O_CREAT if index == 0 else 0)
    try:
        descriptor = os.open(upload_path(cfg, file.id), flags, 0o600)
    except FileNotFoundError:
        raise FileMissingError() from None
    try:
        offset = sealed_offset(index)
        # Short of where this piece goes means the pieces before it are gone,
        # as after a restore. Writing anyway would leave a hole.
        if os.fstat(descriptor).st_size < offset:
            raise FileMissingError()
        os.pwrite(descriptor, sealed, offset)
        os.ftruncate(descriptor, offset + len(sealed))
        # Before the record, which is durable as soon as it is written, says
        # the piece is here.
        os.fsync(descriptor)
    finally:
        os.close(descriptor)

    def mutate(record: Record) -> None:
        record["received"] = index + 1
        record["updated_at"] = to_iso(utc_now())

    store.update_value(BUCKET_FILES, file.id, mutate)


def finish_file(store: Store, cfg: Config, file: File) -> File:
    """Move a complete upload into place and mark it ready. Asked again after a
    crash between the two, it finds the file already moved and goes on."""
    # Both, here rather than at startup: an empty file has written no piece,
    # and a restore replaces storage/ with whatever the backup held.
    for directory in (cfg.files.dir, cfg.files.uploads_dir):
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    source = upload_path(cfg, file.id)
    target = finished_path(cfg, file.id)
    if file.chunks == 0:
        source.touch(mode=0o600)
    try:
        if source.stat().st_size != file.sealed_size:
            raise FileMissingError()
        source.rename(target)
    except FileNotFoundError:
        if not target.exists() or target.stat().st_size != file.sealed_size:
            raise FileMissingError() from None

    def mutate(record: Record) -> None:
        record["state"] = READY
        record["updated_at"] = to_iso(utc_now())

    record = store.update_value(BUCKET_FILES, file.id, mutate)
    if record is None:
        raise FileMissingError()
    return File.from_record(record)


def rename_file(store: Store, file_id: str, name: str, expect: str | None = None) -> str | None:
    """Change the name a file is shown and downloaded under, and answer it as
    cleaned. Only the record changes: paths on disk use the id, and a piece is
    sealed to the id too. None when there is no such file, it is deleted, or it
    is still uploading.

    expect, when given, is the name the caller believes is stored, as in
    users.rename_user: RenameConflictError if it is not."""
    clean = clean_name(name)
    if not clean or len(clean.encode()) > NAME_MAX:
        raise BadFileNameError()
    found = False

    def mutate(record: Record) -> None:
        nonlocal found
        if record.get("deleted_at") is not None or record["state"] != READY:
            return
        found = True
        if expect is not None and record["name"] != expect:
            raise users.RenameConflictError(record["name"])
        record["name"] = clean

    if store.update_value(BUCKET_FILES, file_id, mutate) is None or not found:
        return None
    return clean


def delete_file(store: Store, file_id: str, actor_id: int) -> bool:
    """Mark it, for the usual undo window. The bytes stay until the sweep.
    False when there is none or it is already deleted."""
    found = False

    def mutate(record: Record) -> None:
        nonlocal found
        found = record.get("deleted_at") is None
        mark_deleted(record, actor_id)

    return store.update_value(BUCKET_FILES, file_id, mutate) is not None and found


def restore_file(store: Store, cfg: Config, file_id: str, actor_id: int, actor_is_admin: bool) -> File:
    """Undo delete_file. Raises a RestoreError when it may not happen. It
    counts against the quota again, so that is checked here the same way as for
    a new upload."""
    with store.transaction() as transaction:
        record = transaction.get(BUCKET_FILES, file_id)
        if record is None:
            raise RestoreGoneError()
        unmark_deleted(record, actor_id, actor_is_admin)
        if _used_bytes(transaction, record["user_id"]) + record["size"] > cfg.files.quota_bytes_per_user:
            raise FileQuotaError()
        transaction.put(BUCKET_FILES, file_id, record)
    return File.from_record(record)


class FileReader:
    """A finished file's plain bytes, a piece at a time, so memory stays flat
    whatever the size. A piece that does not open raises, and nothing from it is
    handed out."""

    def __init__(self, cfg: Config, file: File) -> None:
        self.file = file
        self.key = encryption.xchacha_decrypt_string(cfg.secret_key, file.key)
        try:
            self.source = finished_path(cfg, file.id).open("rb")
        except FileNotFoundError:
            raise FileMissingError() from None
        if os.fstat(self.source.fileno()).st_size != file.sealed_size:
            self.source.close()
            raise FileMissingError()

    def __iter__(self) -> Iterator[bytes]:
        """Closes the file when it ends, however it ends."""
        try:
            for index in range(self.file.chunks):
                sealed = self.source.read(self.file.chunk_length(index) + CHUNK_OVERHEAD)
                yield encryption.xchacha_decrypt_bound(self.key, sealed, self.file.bound(index))
        finally:
            self.source.close()

    def close(self) -> None:
        self.source.close()


def sweep_files(store: Store, cfg: Config) -> tuple[int, int]:
    """The cleanup job's work. A deleted file past its undo window, an upload
    nobody has touched for files.abandoned_upload_hours and a file whose account
    has gone lose their record and then their bytes, so a crash between the two
    leaves a stray rather than a record pointing at nothing. Strays are the last
    step. Answers how many of each went."""
    accounts = {user.id for user in users.list_users(store)}
    cutoff = utc_now() - timedelta(hours=cfg.files.abandoned_upload_hours)
    doomed: list[str] = []

    def gone(key: object, record: Record) -> bool:
        abandoned = record["state"] == UPLOADING and from_iso(record["updated_at"]) < cutoff
        if deleted_long_ago(record) or abandoned or record["user_id"] not in accounts:
            doomed.append(str(key))
            return True
        return False

    removed = store.delete_where(BUCKET_FILES, gone)
    for file_id in doomed:
        upload_path(cfg, file_id).unlink(missing_ok=True)
        finished_path(cfg, file_id).unlink(missing_ok=True)
    return removed, _remove_strays(store, cfg)


def _remove_strays(store: Store, cfg: Config) -> int:
    """Reads the record ids after the directories, so a file arriving in
    between has its record seen. Its age would keep it safe anyway."""
    old = time.time() - STRAY_AGE_SECONDS
    candidates: list[Path] = []
    for directory in (cfg.files.uploads_dir, cfg.files.dir):
        if directory.is_dir():
            candidates += [path for path in directory.iterdir() if path.is_file() and not path.is_symlink() and path.stat().st_mtime < old]
    if not candidates:
        return 0
    known = {str(key) for key, _ in store.all(BUCKET_FILES)}
    strays = [path for path in candidates if path.name not in known]
    for path in strays:
        path.unlink(missing_ok=True)
    return len(strays)


class UploadLocks:
    """Keeps two requests from writing one upload at once. It lives in the
    process, like the rate limiter, because this template is one process."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._held: set[str] = set()

    def hold(self, file_id: str) -> bool:
        """Claim file_id; False means another request has it."""
        with self._lock:
            if file_id in self._held:
                return False
            self._held.add(file_id)
            return True

    def release(self, file_id: str) -> None:
        with self._lock:
            self._held.discard(file_id)
