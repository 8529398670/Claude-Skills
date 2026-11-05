"""Uploads through the real routes: who may reach whose file, what a download
says about itself, and a client resuming. Go's twin is
server/routes/files_test.go."""
from __future__ import annotations

from dataclasses import replace
from typing import Any

from app.core.config import FILE_CHUNK_BYTES as CHUNK
from app.models import api_keys, users
from app.routes.files import content_disposition
from app.tests.route_support import RoutesTestCase


class FilesRoutes(RoutesTestCase):
    def post(self, path: str, cookie: str, **body: Any) -> tuple[int, Any]:
        return self.call("POST", path, cookie=cookie, body={**body, "csrf_token": self.csrf(cookie)})

    def piece(self, file_id: str, index: int, content: bytes, cookie: str = "", key: str = "", csrf: bool = True) -> tuple[int, Any]:
        headers = {"content-type": "application/octet-stream"}
        if cookie:
            headers["cookie"] = cookie
            if csrf:
                headers["x-csrf-token"] = self.csrf(cookie)
        if key:
            headers["authorization"] = "Bearer " + key
        response = self.client.post(f"/api/files/{file_id}/chunks/{index}", headers=headers, content=content)
        return response.status_code, response.json()

    def uploaded(self, cookie: str, name: str, content: bytes) -> str:
        """The way files.js does it. Answers the file's id."""
        status, reply = self.post("/api/files", cookie, name=name, size=len(content), type="text/html")
        self.assertEqual(status, 201, reply)
        self.assertEqual(reply["chunk_bytes"], CHUNK)
        for index in range(reply["chunks"]):
            status, sent = self.piece(reply["id"], index, content[index * CHUNK:(index + 1) * CHUNK], cookie=cookie)
            self.assertEqual(status, 200, sent)
        status, finished = self.post(f"/api/files/{reply['id']}/finish", cookie)
        self.assertEqual(status, 200, finished)
        return reply["id"]

    def test_storage_says_what_is_used_and_what_is_left(self) -> None:
        self.uploaded(self.user_cookie, "a.txt", b"x" * 1000)
        self.uploaded(self.owner_cookie, "b.txt", b"x" * 500)
        self.cfg.files.dir.mkdir(parents=True, exist_ok=True)

        status, mine = self.call("GET", "/api/account/storage", cookie=self.user_cookie)
        self.assertEqual((status, mine["used_bytes"]), (200, 1000))
        self.assertEqual(mine["quota_bytes"], self.cfg.files.quota_bytes_per_user)

        status, everyone = self.call("GET", "/api/admin/storage", cookie=self.owner_cookie)
        self.assertEqual((status, everyone["used_bytes"]), (200, 1500))
        self.assertGreater(everyone["disk_free_bytes"], 0)
        self.assertGreater(everyone["disk_total_bytes"], 0)
        self.assertEqual(self.call("GET", "/api/admin/storage", cookie=self.user_cookie)[0], 403)

    def test_an_upload_round_trips_with_safe_headers(self) -> None:
        content = b"<script>alert(1)</script>" * 80000
        file_id = self.uploaded(self.user_cookie, "page.html", content)
        response = self.request("GET", f"/api/files/{file_id}/download", cookie=self.user_cookie)
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.content, content)
        self.assertEqual(response.headers["content-type"], "application/octet-stream")
        self.assertEqual(response.headers["content-disposition"], "attachment; filename=\"page.html\"; filename*=UTF-8''page.html")
        self.assertEqual(response.headers["cache-control"], "private, no-store")
        self.assertEqual(response.headers["content-length"], str(len(content)))
        self.assertEqual(response.headers["x-content-type-options"], "nosniff")

        status, page = self.call("GET", "/api/files", cookie=self.user_cookie)
        self.assertEqual((status, len(page["files"])), (200, 1))
        self.assertNotIn("key", page["files"][0])

    def test_an_awkward_name_cannot_break_the_header(self) -> None:
        self.assertEqual(content_disposition('na"ï;ve.txt'), "attachment; filename=\"na___ve.txt\"; filename*=UTF-8''na%22%C3%AF%3Bve.txt")

    def test_someone_elses_file_is_not_there(self) -> None:
        file_id = self.uploaded(self.user_cookie, "a.txt", b"theirs")
        other = users.create_user(self.store, "Other", users.ROLE_USER)
        cookie = self.session(other.id)
        self.assertEqual(self.call("GET", f"/api/files/{file_id}", cookie=cookie)[0], 404)
        self.assertEqual(self.request("GET", f"/api/files/{file_id}/download", cookie=cookie).status_code, 404)
        self.assertEqual(self.post(f"/api/files/{file_id}/delete", cookie)[0], 404)
        self.assertEqual(self.piece(file_id, 0, b"x", cookie=cookie)[0], 404)
        self.assertEqual(self.call("GET", "/api/admin/files", cookie=cookie)[0], 403)
        self.assertEqual(self.request("GET", f"/api/admin/files/{file_id}/download", cookie=cookie).status_code, 403)
        # Nor can anyone get in without signing in.
        self.assertEqual(self.request("GET", f"/api/files/{file_id}/download").status_code, 401)

    def test_an_admin_can_open_and_remove_any_file(self) -> None:
        file_id = self.uploaded(self.user_cookie, "a.txt", b"theirs")
        status, page = self.call("GET", "/api/admin/files", cookie=self.admin_cookie)
        self.assertEqual((status, page["files"][0]["user_name"]), (200, "User"))
        response = self.request("GET", f"/api/admin/files/{file_id}/download", cookie=self.admin_cookie)
        self.assertEqual((response.status_code, response.content), (200, b"theirs"))

        self.assertEqual(self.post(f"/api/admin/files/{file_id}/delete", self.admin_cookie)[0], 200)
        self.assertEqual(self.call("GET", f"/api/files/{file_id}", cookie=self.user_cookie)[0], 404)
        self.assertEqual(self.post(f"/api/admin/files/{file_id}/restore", self.admin_cookie)[0], 200)
        self.assertEqual(self.call("GET", f"/api/files/{file_id}", cookie=self.user_cookie)[0], 200)

    def test_a_key_reaches_its_own_files_but_an_admin_key_cannot_open_others(self) -> None:
        file_id = self.uploaded(self.user_cookie, "a.txt", b"theirs")
        key, _ = api_keys.issue_api_key(self.store, self.user.id, "script", users.ROLE_USER, 3600)
        response = self.request("GET", f"/api/files/{file_id}/download", key=key)
        self.assertEqual((response.status_code, response.content), (200, b"theirs"))

        # A key needs no csrf token, for a piece or anything else.
        status, created = self.call("POST", "/api/files", key=key, body={"name": "b", "size": 1})
        self.assertEqual(status, 201, created)
        self.assertEqual(self.piece(created["id"], 0, b"x", key=key)[0], 200)

        self.assertEqual(self.call("GET", "/api/admin/files", key=self.admin_key)[0], 200)
        self.assertEqual(self.request("GET", f"/api/admin/files/{file_id}/download", key=self.admin_key).status_code, 403)

    def test_a_piece_needs_its_csrf_token(self) -> None:
        _, created = self.post("/api/files", self.user_cookie, name="a", size=1)
        self.assertEqual(self.piece(created["id"], 0, b"x", cookie=self.user_cookie, csrf=False)[0], 403)

    def test_a_client_resumes_from_what_the_server_reports(self) -> None:
        cookie = self.user_cookie
        content = b"z" * (CHUNK + 10)
        _, created = self.post("/api/files", cookie, name="a", size=len(content))
        file_id = created["id"]

        status, reply = self.piece(file_id, 1, content[CHUNK:], cookie=cookie)
        self.assertEqual((status, reply["received"]), (409, 0))
        self.assertEqual(self.piece(file_id, 0, content[:10], cookie=cookie)[0], 400)
        self.assertEqual(self.piece(file_id, 0, content[:CHUNK], cookie=cookie)[0], 200)
        # The reply was lost, and the same piece comes again.
        status, reply = self.piece(file_id, 0, content[:CHUNK], cookie=cookie)
        self.assertEqual((status, reply["received"]), (409, 1))

        self.assertEqual(self.post(f"/api/files/{file_id}/finish", cookie)[0], 409)
        status, state = self.call("GET", f"/api/files/{file_id}", cookie=cookie)
        self.assertEqual((status, state["received"]), (200, 1))
        self.assertEqual(self.request("GET", f"/api/files/{file_id}/download", cookie=cookie).status_code, 404)

        self.assertEqual(self.piece(file_id, 1, content[CHUNK:], cookie=cookie)[0], 200)
        self.assertEqual(self.piece(file_id, 2, b"x", cookie=cookie)[0], 400)
        self.assertEqual(self.post(f"/api/files/{file_id}/finish", cookie)[0], 200)
        self.assertEqual(self.post(f"/api/files/{file_id}/finish", cookie)[0], 200)
        self.assertEqual(self.request("GET", f"/api/files/{file_id}/download", cookie=cookie).content, content)

    def test_an_empty_file_works(self) -> None:
        file_id = self.uploaded(self.user_cookie, "empty", b"")
        response = self.request("GET", f"/api/files/{file_id}/download", cookie=self.user_cookie)
        self.assertEqual((response.status_code, response.content), (200, b""))

    def test_limits_answer_with_codes(self) -> None:
        files = self.holder.get().files
        self.set_config(files=replace(files, max_file_bytes=10))
        status, reply = self.post("/api/files", self.user_cookie, name="a", size=11)
        self.assertEqual((status, reply["error"]), (409, "file_too_large"))
        self.set_config(files=replace(files, max_file_bytes=100, quota_bytes_per_user=10))
        status, reply = self.post("/api/files", self.user_cookie, name="a", size=11)
        self.assertEqual((status, reply["error"]), (409, "file_quota"))

    def test_a_bad_request_is_refused(self) -> None:
        for body in ({"name": "a", "size": "1"}, {"name": 1, "size": 1}, {"name": "a", "size": True}, {"name": "a"}):
            self.assertEqual(self.post("/api/files", self.user_cookie, **body)[0], 400, body)

    def test_switched_off_every_route_is_gone(self) -> None:
        file_id = self.uploaded(self.user_cookie, "a", b"x")
        self.set_config(files=replace(self.holder.get().files, enabled=False))
        self.assertEqual(self.call("GET", "/api/files", cookie=self.user_cookie)[0], 404)
        self.assertEqual(self.request("GET", f"/api/files/{file_id}/download", cookie=self.user_cookie).status_code, 404)
        self.assertEqual(self.call("GET", "/api/admin/files", cookie=self.admin_cookie)[0], 404)
        self.assertIs(self.call("GET", "/api/me", cookie=self.user_cookie)[1]["files_enabled"], False)

    def test_delete_and_undo(self) -> None:
        file_id = self.uploaded(self.user_cookie, "a", b"x")
        self.assertEqual(self.post(f"/api/files/{file_id}/delete", self.user_cookie)[0], 200)
        self.assertEqual(self.request("GET", f"/api/files/{file_id}/download", cookie=self.user_cookie).status_code, 404)
        self.assertEqual(self.post(f"/api/files/{file_id}/restore", self.user_cookie)[0], 200)
        self.assertEqual(self.request("GET", f"/api/files/{file_id}/download", cookie=self.user_cookie).status_code, 200)
