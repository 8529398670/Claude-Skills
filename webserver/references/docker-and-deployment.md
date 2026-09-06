# Docker & deployment reference

## Dockerfile hardening choices

- **Multi-stage, Alpine base.** The builder stage installs into a venv;
  the runtime stage copies only the venv + app code, so pip, build tools,
  and cached wheels never end up in the shipped image.
- **Pure-Python dependencies.** `requirements.txt` deliberately uses plain
  `uvicorn` (not `uvicorn[standard]`) so nothing needs a C compiler on
  Alpine (musl) at build time. This trades a little request-handling speed
  for a much smaller, simpler, gcc-free image. If a project genuinely
  needs `uvloop`/`httptools` for throughput, that's a deliberate opt-in
  the person building the app should make -- add `gcc musl-dev` to a
  builder-stage `apk add` and switch to `uvicorn[standard]`.
- **Non-root user.** `addgroup`/`adduser` create an unprivileged `app`
  user; `USER app` drops root before `CMD` ever runs. Only `/app/data`
  (the sqlite file's home) is chowned to that user -- everything else in
  the image can stay owned by root, which matters if `dockerRun.sh`'s
  `--read-only` flag is in use, since a compromised process still can't
  write anywhere but the one mounted volume.
- **Healthcheck** hits `/` so orchestration (or just `docker ps`) can tell
  the app is actually serving, not just that the process is alive.

## dockerRun.sh choices (and why there's no docker-compose)

A single `docker run` invocation is easier to read top-to-bottom and audit
than a compose file, and this project intentionally has exactly one
container -- compose's value (multi-service orchestration) doesn't apply.
`dockerRun.sh`:

- Builds the image, then replaces any existing container of the same name
  (idempotent re-runs).
- Publishes only to `127.0.0.1:<port>` -- **this app does not terminate
  TLS**. Put nginx/Caddy/Traefik/etc. in front for anything reachable from
  the internet, and let that reverse proxy hold the real certificate.
  `SECURE_COOKIES` should stay `true` (the default) whenever a proxy is
  terminating HTTPS in front of it, and should only be set to `false` for
  local plain-`http://localhost` development.
- Mounts `./data` as the one writable volume (the sqlite db lives there)
  and generates `.secret_key` once, persisting it on the host so it
  survives container rebuilds -- if it changed on every restart, nothing
  would actually break (`SECRET_KEY` only signs the CSRF... nothing else
  currently), but keeping it stable is one less thing to reason about.
- Runs with `--read-only`, a `tmpfs` for `/tmp`, `--cap-drop ALL`, and
  `--security-opt no-new-privileges:true` -- the container can't write to
  its own filesystem (other than the mounted volume + tmpfs), can't gain
  new Linux capabilities, and can't escalate privileges even if a bug
  lets an attacker run code inside it.
- Sets `--memory` and `--pids-limit` as basic guardrails against a runaway
  process consuming the whole host; adjust these for the actual expected
  load rather than leaving the placeholder values in a production
  deployment.

## Bootstrapping the first admin

There's no user in the database until one is created. After the container
is up:

```bash
docker exec -it <container-name> python -m scripts.manage bootstrap-admin --name "Your Name"
```

This prints a one-time `/login/<token>` path. Prepend the real scheme +
host (e.g. `https://yourapp.example.com/login/<token>`) and open it in a
browser to log in as that admin for the first time.

## Adjusting timeouts and other knobs

Everything tunable lives in `app/core/config.py` as environment-variable
reads with sane defaults (`SESSION_TTL_SECONDS`, `LOGIN_TOKEN_TTL_SECONDS`,
`SECURE_COOKIES`, `PORT`, etc.). Pass overrides via `-e` flags in
`dockerRun.sh` rather than hardcoding new defaults into `config.py`, so the
same image stays reusable across environments.
