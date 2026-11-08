# whole-file feature:jobs
"""The Jobs screen's routes (app/routes/jobs.py). TestClient as a context
manager, so there is one event loop for a started job to run on."""
from __future__ import annotations

import threading
import time
import unittest

from app.core import logs  # feature:logs
from app.models import utc_now
from app.models.jobs import mark_finished, mark_started
from app.tests.route_support import RoutesTestCase, TestClient


class JobsRoutesTest(RoutesTestCase):
    def register_not_due(self, name: str, fn) -> None:
        """A job that has just run: the app starting does not start it, so
        only the test does."""
        self.app.state.jobs.register(name, 3600, fn)
        mark_started(self.store, name, utc_now())
        mark_finished(self.store, name, utc_now(), "", "")

    def test_owner_only(self) -> None:
        self.assertEqual(self.call("GET", "/api/owner/jobs")[0], 404)
        self.assertEqual(self.call("GET", "/api/owner/jobs", cookie=self.admin_cookie)[0], 404)
        self.assertEqual(self.call("GET", "/api/owner/jobs", key=self.admin_key)[0], 404)  # feature:api_keys
        status, body = self.call("GET", "/api/owner/jobs", cookie=self.owner_cookie)
        self.assertEqual(status, 200)
        names = ["purge"]
        names.append("backup")  # feature:backups
        names.append("files")  # feature:files
        names.append("prune-logs")  # feature:logs
        names.append("webhooks")  # feature:webhooks
        self.assertEqual([row["name"] for row in body["jobs"]], sorted(names))
        self.assertEqual(body["jobs"][0]["interval_seconds"], 3600)

        # >>> feature:api_keys
        status, body = self.call("POST", "/api/owner/jobs/purge/run", key=self.admin_key, body={})
        self.assertEqual(status, 404)
        # <<< feature:api_keys

    def test_run(self) -> None:
        release = threading.Event()
        self.addCleanup(release.set)
        registry = self.app.state.jobs
        self.register_not_due("slow", lambda: (release.wait(5), ("slept", ""))[1])
        token = self.csrf(self.owner_cookie)
        with TestClient(self.app) as client:
            self.assertEqual(
                self.call("POST", "/api/owner/jobs/slow/run", cookie=self.owner_cookie, body={}, client=client)[0], 403
            )
            body = {"csrf_token": token}
            self.assertEqual(
                self.call("POST", "/api/owner/jobs/slow/run", cookie=self.owner_cookie, body=body, client=client),
                (202, {"ok": True}),
            )
            self.assertEqual(
                self.call("POST", "/api/owner/jobs/slow/run", cookie=self.owner_cookie, body=body, client=client),
                (409, {"error": "job already running"}),
            )
            self.assertEqual(
                self.call("POST", "/api/owner/jobs/nope/run", cookie=self.owner_cookie, body=body, client=client),
                (404, {"error": "not found"}),
            )
            status, listing = self.call("GET", "/api/owner/jobs", cookie=self.owner_cookie, client=client)
            self.assertEqual(status, 200)
            self.assertTrue(next(row for row in listing["jobs"] if row["name"] == "slow")["running"])
            release.set()
        # Leaving the block stopped the app, which waited for the run.
        row = next(row for row in registry.rows() if row["name"] == "slow")
        self.assertFalse(row["running"])
        self.assertEqual(row["last_note"], "slept")

    # >>> feature:logs
    def test_a_run_is_audited(self) -> None:
        self.register_not_due("quick", lambda: ("", ""))
        token = self.csrf(self.owner_cookie)
        with TestClient(self.app) as client:
            self.call("POST", "/api/owner/jobs/quick/run", cookie=self.owner_cookie, body={"csrf_token": token}, client=client)
        logger = self.app.state.logs
        entries, _ = logger.read_day(logger.today(), logs.Query("kind:admin"))
        record = next(entry for entry in entries if entry.get("msg") == "job_run")
        self.assertEqual(record["detail"], {"name": "quick"})

    # <<< feature:logs
    def test_housekeeping_runs_shortly_after_boot(self) -> None:
        def rows() -> dict[str, dict]:
            return {row["name"]: row for row in self.app.state.jobs.rows()}

        with TestClient(self.app):
            deadline = time.monotonic() + 5
            watched = "purge"
            watched = "backup"  # feature:backups
            while not rows()[watched].get("last_note") and time.monotonic() < deadline:
                time.sleep(0.01)
        self.assertEqual(rows()["purge"]["runs"], 1)
        self.assertEqual(rows()["backup"]["last_note"], "off")  # feature:backups


if __name__ == "__main__":
    unittest.main()
