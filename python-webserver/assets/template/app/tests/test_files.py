"""Uploaded files, below the routes: a file comes back exactly, nothing is on
disk in the clear, a piece opens only in its own place, and the sweep removes
what it should and nothing else. Go's twin is server/models/file_test.go."""
from __future__ import annotations

import os
import tempfile
import threading
import unittest
from dataclasses import replace
from datetime import timedelta
from pathlib import Path
from types import SimpleNamespace

from app.core import db, encryption
from app.core.config import FilesConfig
from app.models import RESTORE_WINDOW, files, to_iso, users, utc_now
from app.models.files import CHUNK_BYTES, CHUNK_OVERHEAD


def pattern(size: int) -> bytes:
    """Not all one byte, so a piece out of place would show."""
    return bytes((index * 31 + index // 251) % 256 for index in range(size))


class FilesCase(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        root = Path(directory.name)
        self.store = db.open_store(root / "app.db", "ab" * 32, True)
        # Only what these functions read: a whole Config needs a config.yaml.
        self.cfg = SimpleNamespace(secret_key="ab" * 32, files=FilesConfig(
            max_file_bytes=64 << 20, quota_bytes_per_user=64 << 20, abandoned_upload_hours=1,
            dir=root / "storage" / "files", uploads_dir=root / "uploads",
        ))

    def limit(self, **changes: int) -> None:
        self.cfg.files = replace(self.cfg.files, **changes)

    def stored(self, user_id: int, content: bytes) -> files.File:
        file = files.create_file(self.store, self.cfg, user_id, "report.pdf", "application/pdf", len(content))
        for index in range(file.chunks):
            files.write_chunk(self.store, self.cfg, file, index, content[index * CHUNK_BYTES:(index + 1) * CHUNK_BYTES])
        return files.finish_file(self.store, self.cfg, files.get_file(self.store, file.id))

    def read(self, file: files.File) -> bytes:
        return b"".join(files.FileReader(self.cfg, file))

    def test_a_file_comes_back_exactly(self) -> None:
        for size in (0, 1, CHUNK_BYTES - 1, CHUNK_BYTES, CHUNK_BYTES + 1, 3 * CHUNK_BYTES):
            content = os.urandom(size)
            file = self.stored(1, content)
            self.assertEqual(file.state, files.READY)
            self.assertEqual(self.read(file), content, size)

    def test_nothing_on_disk_is_in_the_clear(self) -> None:
        marker = b"the plain text of this file"
        file = self.stored(1, marker * 1000)
        self.assertNotIn(marker, files.finished_path(self.cfg, file.id).read_bytes())

    def test_a_piece_only_opens_in_its_own_place(self) -> None:
        content = pattern(3 * CHUNK_BYTES)
        piece = CHUNK_BYTES + CHUNK_OVERHEAD

        swapped = self.stored(1, content)
        path = files.finished_path(self.cfg, swapped.id)
        raw = path.read_bytes()
        path.write_bytes(raw[piece:2 * piece] + raw[:piece] + raw[2 * piece:])
        with self.assertRaises(encryption.CipherTextError):
            self.read(swapped)

        other, foreign = self.stored(1, content), self.stored(1, content)
        files.finished_path(self.cfg, other.id).write_bytes(files.finished_path(self.cfg, foreign.id).read_bytes())
        with self.assertRaises(encryption.CipherTextError):
            self.read(other)

        changed = self.stored(1, content)
        path = files.finished_path(self.cfg, changed.id)
        raw = bytearray(path.read_bytes())
        raw[100] ^= 1
        path.write_bytes(bytes(raw))
        with self.assertRaises(encryption.CipherTextError):
            self.read(changed)

        # A whole piece cut off is the wrong size, and the count in what each
        # piece is sealed to would refuse it anyway.
        cut = self.stored(1, content)
        path = files.finished_path(self.cfg, cut.id)
        path.write_bytes(path.read_bytes()[:2 * piece])
        with self.assertRaises(files.FileMissingError):
            files.FileReader(self.cfg, cut)
        with self.assertRaises(encryption.CipherTextError):
            self.read(replace(cut, chunks=2, size=2 * CHUNK_BYTES))

    def test_chunk_writes_survive_a_retry_and_refuse_gaps(self) -> None:
        content = pattern(2 * CHUNK_BYTES)
        file = files.create_file(self.store, self.cfg, 1, "a", "", len(content))
        first, second = content[:CHUNK_BYTES], content[CHUNK_BYTES:]
        with self.assertRaises(files.ChunkSizeError):
            files.write_chunk(self.store, self.cfg, file, 0, first[:10])
        # The second piece with the first missing would leave a hole.
        with self.assertRaises(files.FileMissingError):
            files.write_chunk(self.store, self.cfg, file, 1, second)
        # A crash after the write and before the record leaves the same piece
        # to send again.
        for _ in range(2):
            files.write_chunk(self.store, self.cfg, file, 0, first)
        files.write_chunk(self.store, self.cfg, file, 1, second)
        finished = files.finish_file(self.store, self.cfg, files.get_file(self.store, file.id))
        self.assertEqual(self.read(finished), content)

    def test_finish_goes_on_after_a_crash_before_the_record(self) -> None:
        file = files.create_file(self.store, self.cfg, 1, "a", "", 10)
        files.write_chunk(self.store, self.cfg, file, 0, pattern(10))
        file = files.get_file(self.store, file.id)
        self.cfg.files.dir.mkdir(parents=True)
        files.upload_path(self.cfg, file.id).rename(files.finished_path(self.cfg, file.id))
        self.assertEqual(files.finish_file(self.store, self.cfg, file).state, files.READY)

    def test_limits_are_checked(self) -> None:
        self.limit(max_file_bytes=100)
        with self.assertRaises(files.FileTooLargeError):
            files.create_file(self.store, self.cfg, 1, "a", "", 101)
        for name in ("", "  ", "dir/", "x" * 256):
            with self.assertRaises(files.BadFileError):
                files.create_file(self.store, self.cfg, 1, name, "", 1)
        with self.assertRaises(files.BadFileError):
            files.create_file(self.store, self.cfg, 1, "a", "", -1)

    def test_a_name_loses_its_path(self) -> None:
        for sent, wanted in {
            "../../etc/passwd": "passwd",
            "C:\\Users\\a\\b.txt": "b.txt",
            "a\x00b\r\n.txt": "ab.txt",
            "  naïve résumé.pdf": "naïve résumé.pdf",
        }.items():
            self.assertEqual(files.clean_name(sent), wanted)

    def test_parallel_uploads_cannot_pass_the_quota(self) -> None:
        self.limit(quota_bytes_per_user=100)
        made: list[files.File] = []

        def attempt() -> None:
            try:
                made.append(files.create_file(self.store, self.cfg, 1, "a", "", 30))
            except files.FileQuotaError:
                pass

        threads = [threading.Thread(target=attempt) for _ in range(20)]
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join()
        self.assertEqual(len(made), 3)
        # Someone else's quota is their own.
        files.create_file(self.store, self.cfg, 2, "a", "", 100)

    def test_deleting_frees_the_quota_and_restoring_takes_it_back(self) -> None:
        self.limit(quota_bytes_per_user=100)
        file = files.create_file(self.store, self.cfg, 1, "a", "", 80)
        self.assertTrue(files.delete_file(self.store, file.id, 1))
        self.assertFalse(files.delete_file(self.store, file.id, 1))
        second = files.create_file(self.store, self.cfg, 1, "b", "", 80)
        with self.assertRaises(files.FileQuotaError):
            files.restore_file(self.store, self.cfg, file.id, 1, False)
        files.delete_file(self.store, second.id, 1)
        files.restore_file(self.store, self.cfg, file.id, 1, False)

    def test_the_sweep(self) -> None:
        user = users.create_user(self.store, "Alice", users.ROLE_USER)
        kept = self.stored(user.id, pattern(10))

        def change(file: files.File, **fields: object) -> None:
            self.store.update_value("files", file.id, lambda record: record.update(fields))

        deleted = self.stored(user.id, pattern(10))
        change(deleted, deleted_at=to_iso(utc_now() - 2 * RESTORE_WINDOW))
        recent = self.stored(user.id, pattern(10))
        files.delete_file(self.store, recent.id, user.id)

        abandoned = files.create_file(self.store, self.cfg, user.id, "a", "", 10)
        files.write_chunk(self.store, self.cfg, abandoned, 0, pattern(10))
        change(abandoned, updated_at=to_iso(utc_now() - timedelta(hours=2)))
        active = files.create_file(self.store, self.cfg, user.id, "a", "", 10)

        orphaned = self.stored(9999, pattern(10))

        old_stray = self.cfg.files.dir / "stray"
        new_stray = self.cfg.files.uploads_dir / "arriving"
        old_stray.write_bytes(b"x")
        new_stray.write_bytes(b"x")
        aged = utc_now().timestamp() - 2 * files.STRAY_AGE_SECONDS
        os.utime(old_stray, (aged, aged))

        self.assertEqual(files.sweep_files(self.store, self.cfg), (3, 1))
        for gone in (deleted, abandoned, orphaned):
            self.assertIsNone(files.get_file(self.store, gone.id))
            self.assertFalse(files.upload_path(self.cfg, gone.id).exists())
            self.assertFalse(files.finished_path(self.cfg, gone.id).exists())
        for alive in (kept, recent, active):
            self.assertIsNotNone(files.get_file(self.store, alive.id))
        self.assertEqual(self.read(kept), pattern(10))
        self.assertFalse(old_stray.exists())
        self.assertTrue(new_stray.exists())

    def test_the_sweep_copes_with_no_directories(self) -> None:
        self.assertEqual(files.sweep_files(self.store, self.cfg), (0, 0))

    def test_upload_locks(self) -> None:
        locks = files.UploadLocks()
        self.assertTrue(locks.hold("a"))
        self.assertFalse(locks.hold("a"))
        self.assertTrue(locks.hold("b"))
        locks.release("a")
        self.assertTrue(locks.hold("a"))
