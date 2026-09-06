// Thin fetch wrapper. Kept separate from app.js so any page can reuse it
// without pulling in the account/admin rendering logic.
const Api = {
  async _json(path, options = {}) {
    const res = await fetch(path, {
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      ...options,
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw Object.assign(new Error(body.error || res.statusText), { status: res.status, body });
    return body;
  },

  me() {
    return this._json("/api/me").catch((err) => (err.status === 401 ? { authenticated: false } : Promise.reject(err)));
  },

  rename(displayName, csrfToken) {
    return this._json("/api/account/rename", {
      method: "POST",
      body: JSON.stringify({ display_name: displayName, csrf_token: csrfToken }),
    });
  },

  logout() {
    return this._json("/api/logout", { method: "POST" });
  },

  listTeamMembers() {
    return this._json("/api/team");
  },

  listUsers() {
    return this._json("/api/admin/users");
  },

  createUser(displayName, role, csrfToken) {
    return this._json("/api/admin/users", {
      method: "POST",
      body: JSON.stringify({ display_name: displayName, role, csrf_token: csrfToken }),
    });
  },

  setDisabled(userId, disabled, csrfToken) {
    return this._json(`/api/admin/users/${userId}/disabled`, {
      method: "POST",
      body: JSON.stringify({ disabled, csrf_token: csrfToken }),
    });
  },

  reissueLogin(userId, csrfToken) {
    return this._json(`/api/admin/users/${userId}/reissue-login`, {
      method: "POST",
      body: JSON.stringify({ csrf_token: csrfToken }),
    });
  },
};
