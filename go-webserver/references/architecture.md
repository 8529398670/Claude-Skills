# Architecture reference

Read this when you need to explain *why* the template is built this way, or
when you are changing the template itself rather than building features on it.

## Contents

- [Auth model: links instead of passwords](#auth-model-links-instead-of-passwords)
- [Why credentials are shaped `<id>.<secret>`](#why-credentials-are-shaped-idsecret)
- [Which crypto is used where, and why](#which-crypto-is-used-where-and-why)
- [At-rest encryption and the SECRET_KEY tradeoff](#at-rest-encryption-and-the-secret_key-tradeoff)
- [Default permission level for new features](#default-permission-level-for-new-features)
- [Why auth is middleware, not a helper call](#why-auth-is-middleware-not-a-helper-call)
- [CSRF](#csrf)
- [Why the server holds so little app logic](#why-the-server-holds-so-little-app-logic)
- [Static asset caching policy](#static-asset-caching-policy)
- [Where state lives](#where-state-lives)
- [Where assets come from: disk or embedded](#where-assets-come-from-disk-or-embedded)
- [Known limits](#known-limits)

## Auth model: links instead of passwords

There is no password field anywhere in the schema, and adding one would undo
the design. Identity is proven exactly two ways:

1. **Redeeming a one-time login token** (`server/models/login_token.go`). An
   admin -- or the first-run bootstrap -- mints a high-entropy credential for
   a specific user. Only a bcrypt hash of its secret half is stored. It
   expires, and redemption marks it used inside a single bolt write
   transaction, so two requests racing the same link cannot both succeed.
2. **Presenting a valid session cookie** (`server/models/session.go`). The
   cookie holds an opaque reference, never the user id or role. Revoking a
   session is deleting a row -- no denylist, no token to outlive its welcome.

"Forgot password" does not exist as a concept. It is replaced by "an admin
mints a new link" (or `manage reissue-login` from a shell). That is a real
trade: less code, no password storage, no reset-flow attack surface, at the
cost of needing an admin or shell access when someone loses their session. A
project needing genuine self-service recovery -- email round trip, say -- has
to add it; it is not part of the base model.

The first-run bootstrap prints the initial admin link to the server's own
logs. Anyone who can read those logs already controls the deployment, so this
grants nothing new, and it means a fresh deploy is usable without a separate
setup step. It only fires when no enabled admin exists, so restarts do not
keep minting admins.

## Why credentials are shaped `<id>.<secret>`

Both login tokens and sessions use `<id>.<secret>`, and the reason is worth
understanding before changing it.

Storing only a hash of a credential is standard -- it means a stolen database
cannot be replayed. But a *salted* hash like bcrypt cannot be looked up:
you would have to fetch every candidate row and bcrypt-compare each one,
which hands anyone a trivial way to pin the CPU with garbage tokens.

Splitting the credential fixes that. The id is the bolt key, so lookup is one
O(1) fetch, and exactly one hash comparison follows.

The two halves then use different hashes on purpose:

- **Login tokens use bcrypt.** They are redeemed at most once, so bcrypt's
  deliberate slowness costs a single request and buys real resistance.
- **Sessions use sha256.** This runs on every authenticated request. Against a
  256-bit random secret there is nothing to brute force, so a fast hash is
  correct -- bcrypt here would just be a self-inflicted latency tax.

Comparisons go through `encryption.ConstantTimeEqual` so the check does not
leak, via timing, how many leading characters matched.

## Which crypto is used where, and why

Everything lives in `server/encryption/encryption.go`. Nothing else in the
project imports a crypto primitive directly -- that is what makes "how does
this app handle secrets" a one-file question.

| Primitive | Used for |
|---|---|
| `crypto/rand` | Every credential, key, and nonce |
| `bcrypt` | Hashing one-time login secrets |
| `chacha20poly1305` (XChaCha) | Session cookie payloads, values at rest |
| `sha256` + `crypto/subtle` | Session secret lookup, constant-time compare |
| `nacl/secretbox` | Available for authenticated blobs; unused by the base template |
| `curve25519` | Available for key exchange; unused by the base template |
| `kyber-k2so` | Available for post-quantum KEM; unused by the base template |

The last three ship ready but unwired, and that is deliberate rather than an
oversight. The login flow needs no key exchange at all -- a session cookie is
an opaque database reference. They are there for the features apps built on
this template do grow: handing a browser or a peer service a public key, or
end-to-end encrypting message payloads. Pairing Kyber with X25519 gives the
usual hybrid, where an attacker recording traffic now against a future quantum
computer still has to break both.

Two deliberate differences from the reference implementation the API mirrors:

- **Random bytes come straight from `crypto/rand`.** A hand-rolled PRNG layered
  over a CSPRNG cannot be stronger than the CSPRNG, only harder to review.
- **Anything on an auth path returns an error rather than swallowing it.** A
  decrypt that quietly returns `""` turns a tampered cookie into an
  empty-but-successful value, which is how a subtle bug becomes an auth bypass.

Cookie encryption is not what makes a session unforgeable -- the database
lookup is. It means the cookie is opaque to anyone reading a browser profile
or a proxy log, and that a tampered cookie fails to decrypt before it ever
reaches the lookup.

## At-rest encryption and the SECRET_KEY tradeoff

With `ENCRYPT_AT_REST=true` (the default), every value written to bolt is
XChaCha-encrypted with `SECRET_KEY`. `server/db/db.go` does this in
`encode`/`decode`, so models never think about it and no future model can
forget to.

**The cost is real and worth stating plainly to the user: lose the key and the
database is unreadable.** There is no recovery.

Where the key comes from depends on how the app is run, and the two cases
trade differently -- `configuration.md` covers this in full, but the short
version:

- **Container**: `dockerRun.sh` passes `SECRET_KEY` from `.secret_key` on the
  host, so the key stays outside the mounted volume and a leaked `./data`
  backup is not enough to read it.
- **Portable binary**: there is nobody to set an environment variable, so a
  key is generated once into `secret.key` inside the app directory. That is
  what lets a single file just run -- at the cost that at-rest encryption then
  protects a database file that leaks *on its own*, not someone who takes the
  whole directory.

`config.Load` refuses to start with a malformed key rather than continuing,
because the alternative failure mode only shows up on the next restart, when
every record has already become unreadable.

Turn it off (`ENCRYPT_AT_REST=false`) when operational simplicity beats
at-rest confidentiality -- the data is not sensitive, or disk encryption
already covers it.

## Default permission level for new features

Only two things are admin-gated: managing accounts and minting login links.

When adding features -- projects, documents, comments, whatever the app is
for -- **default to "any signed-in user."** Add an admin check only if the
user's request specifically calls for it. Most small internal tools built on
this are for one team where everyone is a peer once they are let in at all.
Admin here means "can decide who gets in," not "can use more of the app."

If a feature needs to reference other users, use `GET /api/team`. It is
available to any signed-in user and returns only id and display name --
deliberately omitting the role and disabled status that the admin endpoint
exposes.

## Why auth is middleware, not a helper call

`RequireLogin` and `RequireAdmin` are Fiber middleware, applied to route
groups in `server/routes/routes.go`, rather than helpers called at the top of
each handler.

A handler that forgets a helper call is silently public, and nothing about
reading that handler tells you it is wrong. With groups, protection comes from
where a route is registered -- visible in one place, in the URL map itself.
`LoadUser` runs on everything and resolves the cookie at most once, so a
handler and a middleware both asking "who is this" do not cost two reads.

## CSRF

`SameSite=Strict` already blocks the common cross-site POST, but it is a
promise the browser makes: older browsers, odd cross-scheme cases, and
non-browser clients do not all honour it. So state-changing endpoints also
check a per-session token that `/api/me` hands out and the client echoes in
the JSON body. It costs nothing and does not depend on the browser behaving.

`Api.post` in `static/js/api.js` attaches it automatically, so a new endpoint
cannot forget it from the client side.

## Why the server holds so little app logic

The stated goal is that the server does auth, storage, and file serving --
not rendering pages or owning UI state:

- `server/static` serves whatever is in `static/`, unmodified except for gzip.
- `server/routes` exposes the minimum JSON API to make the auth model usable
  from a browser. How the admin panel looks lives in `static/js/`.

When adding features, put new server logic in new files under `server/routes/`
(one concern per file) and keep rendering in `static/`. The layout exists to
prevent a monolith; adding to the wrong file is how you get one anyway.

## Static asset caching policy

`server/static/static.go` implements one rule:

- **Text assets** (`.html`, `.css`, `.js`, `.json`, `.svg`, ...) are read off
  disk and gzipped on every request with `Cache-Control: no-store`. A deploy
  is live on the next refresh -- no content hashing, no `?v=123`, no
  cache-busting build step to keep in sync. Gzip is what makes that
  affordable: the bytes on the wire are a fraction of the file.
- **Images and binaries** get `max-age` of a week and no gzip. Gzip does
  nothing for already-compressed formats, and nobody wants a photo
  re-downloaded on every view.

Path traversal is stopped two ways: any request containing a `..` segment is
rejected outright, and the resolved path is then confirmed to still be under
the root. The second check is the real guarantee -- it verifies where the path
landed rather than trying to blocklist how it got there. The first exists so
probes answer 404 instead of quietly hitting the index fallback, which is
clearer in an access log.

A missing path *with* an extension is a 404. A missing path *without* one
falls back to `index.html` so client-side routing works. Serving HTML for a
missing `.js` would leave the browser reporting a confusing MIME error instead
of the real problem.

If this ever costs too much CPU (very large JS, very high traffic), the fix is
smaller static output, not caching the text assets -- that would reintroduce
exactly the staleness this avoids.

## Where state lives

Everything the app persists -- database, secret key, uploaded files, an
optional `config.yaml` -- lives in one directory, `~/.config/<slug>/` by
default and `/app/data` in a container. Settings resolve as environment
variable, then `config.yaml`, then default.

That convention has its own reference: see `configuration.md`. Read it before
adding a setting or writing a file anywhere, because the two rules it exists
to protect (one backup unit, one place that reads the environment) are easy to
break by accident.

## Where assets come from: disk or embedded

`server/assets.Resolve` decides, at startup, whether the frontend is read from
disk or from the copy compiled into the binary by `embed.go`. Disk wins when
it is present; embedded is the fallback. It logs which it chose:

```
[assets] static=disk /srv/app/static language=embedded
```

That one line answers "why isn't my edit showing up" faster than any amount of
debugging, which is why it is printed on every start.

The rule is *disk wins* rather than a flag or a build tag, because the two
situations it distinguishes are never ambiguous in practice:

- A source checkout has `./static` next to the binary, and someone is editing
  it. Reading from disk is what makes the no-store serving policy useful --
  edit, refresh, done.
- A binary copied somewhere on its own has no `./static`, so it uses what is
  inside it. This is what `build.sh` produces.

A useful third case falls out of treating the static tree and the language
file independently: dropping a `language.yaml` next to a shipped binary
rewords the UI without rebuilding anything, while the frontend stays
embedded. That is the supported way to translate or soften a deployment's copy
without a Go toolchain.

`os.DirFS` is used for the disk case rather than raw file paths, so both
sources are the same `fs.FS` interface and `server/static` has exactly one
code path. It also means `fs.ValidPath` refuses `..` elements for free,
independently of the explicit check in the handler.

### The single binary is also the CLI

`main.go` dispatches `manage` and `version` subcommands before starting the
server. The commands themselves live in `server/manage`, which `cmd/manage`
also wraps -- one implementation, two entry points, no drift.

This matters more than it looks: a portable binary that could not mint a login
link would strand anyone who lost their session on a machine with no Go
toolchain and no container runtime. `myapp manage reissue-login` is what makes
"one file that just runs" actually self-sufficient.

## The control socket

bolt hands its file to exactly one process. That is a real constraint, and the
template's answer is to lean on it rather than work around it: the server owns
the database, and every other process asks the server to act on its behalf
over a unix socket in the app directory.

    ~/.config/myapp/control.sock     mode 0600, created by the running server

`manage` takes whichever route is available -- the socket when a server is
running, the database file when one is not -- so the same command works on a
live deployment and on a fresh install with nothing started yet. It prints
which route it used on stderr.

### Why there is no token on the socket

There is deliberately no password, token, or handshake. The socket lives in
the app directory, which is mode 0700, next to `secret.key` and `app.db`. Any
process that can open the socket can already read the key and the database
directly, so a credential would guard nothing while adding one more secret to
store and rotate. The filesystem permission *is* the authentication.

This is also the reason the socket must never be moved somewhere
world-readable and is never bound to a TCP address. The argument depends
entirely on the socket being exactly as hard to reach as the database file
sitting beside it. A network listener would need a real credential, a second
copy of the auth checks, and an audit story -- which is the cost the socket
exists to avoid.

### What this buys, in security terms

One process holds the secret key. One process applies the model layer's rules.
Disabling an account revokes its sessions the same way whether the change came
from the admin UI, the CLI, or a cron job, because all three run the same code
in the same process. The alternative -- a second process opening the database
directly -- means a second copy of the key in memory, a second implementation
to keep in step, and no single place to audit.

### Adding an operation

Add the endpoint in `server/control/control.go` and the matching method in
`client.go`. If a CLI command needs it, add it to the `backend` interface in
`server/manage/manage.go`; the compiler then requires both the socket
implementation and the direct-database one, which is what stops the two routes
from drifting apart.

### Limits worth knowing

A unix socket path is capped by the kernel at 104 bytes on macOS and the BSDs,
108 on Linux. A deep `APP_DIR` overflows it, and the kernel's own error is a
bare "invalid argument", so `control.Listen` checks the length first and fails
with a message naming the limit and the fix. On Windows the socket works
(AF_UNIX is supported) but the 0600 chmod is a no-op and the directory ACL is
what protects it.

## Known limits

State these to the user rather than letting them be discovered in production:

- **One process per bolt file.** bolt takes an exclusive lock, so a second
  process cannot open the database while the server holds it. The control
  socket (below) is how everything else reaches the data; without it, or with
  `CONTROL_SOCKET=false`, `manage` needs the server stopped. This is why
  `Open` sets a 5s timeout rather than hanging forever.
- **The control socket is a local door, not a remote one.** It is a unix
  socket, so it does not cross machines. Two servers on two hosts sharing one
  database is still not possible and is the point at which this template's
  storage choice has been outgrown.
- **Rate limiting is per-process, in memory.** Correct for the single
  container `dockerRun.sh` builds. With multiple replicas each holds its own
  counters, so the effective limit multiplies -- move to a shared
  `limiter.Config.Storage` at that point rather than assuming it still holds.
- **Losing the secret key loses the data** when at-rest encryption is on. Back
  it up with the database, never instead of it.
- **No TLS.** The app expects a reverse proxy in front. See
  `docker-and-deployment.md`.
- **`language.yaml` is public.** It is served to anyone at `/api/language`,
  signed in or not, because signed-out pages need text too. Never put
  anything sensitive in it.
- **A portable binary has no sandbox.** `build.sh` output is a plain process
  with the user's own permissions -- none of Docker's read-only filesystem,
  dropped capabilities, or pid limits apply. That is fine for an internal box
  or a demo, and not what should face the internet. Say this rather than
  letting someone assume the binary is as protected as the container.
