# whole-file feature:logs
"""What the owner calls an address (app/models/ip_names.py): one spelling for each
target, the most specific name wins, and names are searched and shown but never
written to the log. Go's twin is server/models/ip_name_test.go."""
from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from app.core import db, encryption, logs
from app.core.config import LogsConfig
from app.models import ip_names


class TargetTest(unittest.TestCase):
    def test_one_spelling(self) -> None:
        cases = {
            "10.0.0.5": "10.0.0.5",
            " 10.0.0.5 ": "10.0.0.5",
            "::ffff:10.0.0.5": "10.0.0.5",
            "10.0.0.77/24": "10.0.0.0/24",
            "10.0.0.5/32": "10.0.0.5",
            "2001:DB8::1": "2001:db8::1",
            "2001:db8::1/48": "2001:db8::/48",
        }
        for text, want in cases.items():
            self.assertEqual(ip_names.clean_target(text), want, text)
        for text in ("", "laptop", "10.0.0", "10.0.0.0/33", None, 5):
            self.assertIsNone(ip_names.clean_target(text), text)

    def test_the_most_specific_name_wins(self) -> None:
        namer = ip_names.Namer([
            {"target": "10.0.0.0/8", "name": "Company"},
            {"target": "10.1.0.0/16", "name": "Office"},
            {"target": "10.1.2.3", "name": "Printer"},
            {"target": "2001:db8::/32", "name": "Six"},
        ])
        cases = {
            "10.1.2.3": "Printer", "::ffff:10.1.2.3": "Printer", "10.1.2.4": "Office", "10.9.9.9": "Company",
            "2001:db8::9": "Six", "11.0.0.1": "", "not an address": "", "": "",
        }
        for ip, want in cases.items():
            self.assertEqual(namer(ip), want, ip)
            self.assertEqual(namer(ip), want, ip)


class LogNamesTest(unittest.TestCase):
    def test_searched_and_added_but_never_written(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            settings = LogsConfig(dir=Path(directory), retention_days=30, compress_after_days=1)
            logger = logs.Logger(settings, encryption.generate_key_hex(), False)
            self.addCleanup(logger.close)
            logger._write(logs.entry(logs.KIND_SYSTEM, ip="10.0.0.5", msg="one"))
            logger._write(logs.entry(logs.KIND_SYSTEM, ip="10.0.0.6", msg="two"))
            logger.flush()

            def names(ip: str) -> str:
                return "Front Desk" if ip == "10.0.0.5" else ""

            day = logger.today()
            for text in ("ip:front", '"front desk"', "ip:10.0.0.5"):
                records, _ = logger.read_day(day, logs.Query(text, names))
                self.assertEqual([record.get("ip_name") for record in records], ["Front Desk"], text)
            records, _ = logger.read_day(day, logs.Query("-ip:front", names))
            self.assertEqual([record["ip"] for record in records], ["10.0.0.6"])
            lines = [json.loads(line) for line in logger.download(day, logs.Query("", names))]
            self.assertEqual([line.get("ip_name") for line in lines], ["Front Desk", None])
            self.assertEqual(lines[0]["ip"], "10.0.0.5")
            stored = b"".join(path.read_bytes() for path in Path(directory).iterdir())
            self.assertNotIn(b"Front Desk", stored)


class StoreTest(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.store = db.Store(Path(directory.name) / "app.db", encryption.generate_key_hex(), True)
        self.store.initialise()

    def test_a_name_can_be_changed_and_taken_away(self) -> None:
        ip_names.set_name(self.store, "10.0.0.5", "  Old  ")
        ip_names.set_name(self.store, "10.0.0.5", "New")
        self.assertEqual([item["name"] for item in ip_names.list_names(self.store)], ["New"])
        ip_names.delete_name(self.store, "10.0.0.5")
        self.assertEqual(ip_names.list_names(self.store), [])
        self.assertEqual(self.store.all(db.BUCKET_IP_NAMES), [])


if __name__ == "__main__":
    unittest.main()
