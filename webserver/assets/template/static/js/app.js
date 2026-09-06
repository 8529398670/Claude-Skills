// Renders the one static page this template ships with. Real apps will
// likely replace most of this file but should keep the pattern: fetch
// /api/me once, then branch on authenticated/role -- don't reinvent the
// session check.
let csrfToken = null;

function show(el, visible) {
  el.hidden = !visible;
}

async function refreshUsersTable() {
  const tbody = document.querySelector("#users-table tbody");
  tbody.innerHTML = "";
  const usersList = await Api.listUsers();
  for (const u of usersList) {
    const tr = document.createElement("tr");

    const status = u.disabled ? "disabled" : "active";
    tr.innerHTML = `<td>${u.display_name}</td><td>${u.role}</td><td>${status}</td>`;

    const actionsTd = document.createElement("td");

    const reissueBtn = document.createElement("button");
    reissueBtn.textContent = "New login link";
    reissueBtn.onclick = async () => {
      const { login_url } = await Api.reissueLogin(u.id, csrfToken);
      showNewLoginLink(login_url);
    };
    actionsTd.appendChild(reissueBtn);

    const toggleBtn = document.createElement("button");
    toggleBtn.textContent = u.disabled ? "Enable" : "Disable";
    toggleBtn.className = "secondary";
    toggleBtn.onclick = async () => {
      await Api.setDisabled(u.id, !u.disabled, csrfToken);
      refreshUsersTable();
    };
    actionsTd.appendChild(toggleBtn);

    tr.appendChild(actionsTd);
    tbody.appendChild(tr);
  }
}

function showNewLoginLink(url) {
  const el = document.getElementById("new-login-link");
  el.textContent = `Send this one-time link to the new user: ${url}`;
  show(el, true);
}

async function init() {
  const me = await Api.me();

  show(document.getElementById("signed-out"), !me.authenticated);
  show(document.getElementById("signed-in"), me.authenticated);
  if (!me.authenticated) return;

  csrfToken = me.csrf_token;
  document.getElementById("who-am-i").textContent = `${me.display_name} (${me.role})`;
  document.getElementById("display-name").value = me.display_name;

  document.getElementById("rename-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const displayName = document.getElementById("display-name").value.trim();
    await Api.rename(displayName, csrfToken);
    document.getElementById("who-am-i").textContent = `${displayName} (${me.role})`;
  });

  document.getElementById("logout-btn").addEventListener("click", async () => {
    await Api.logout();
    window.location.reload();
  });

  if (me.role === "admin") {
    show(document.getElementById("admin-panel"), true);
    document.getElementById("create-user-form").addEventListener("submit", async (e) => {
      e.preventDefault();
      const displayName = document.getElementById("new-name").value.trim();
      const role = document.getElementById("new-role").value;
      const { login_url } = await Api.createUser(displayName, role, csrfToken);
      showNewLoginLink(login_url);
      document.getElementById("new-name").value = "";
      refreshUsersTable();
    });
    refreshUsersTable();
  }
}

init();
