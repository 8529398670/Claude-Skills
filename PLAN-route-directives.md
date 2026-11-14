# Plan: route directives in Go, declared routes in Python

Follow-up to `PLAN-typed-routes.md`. Read its "Design" and "Pilot findings"
sections first. Decisions below were settled in an interview on 2026-11-13.

## Where things stand (start here after /compact)

- The typed-routes work is done in both templates: every route goes through
  `apidoc.Handle` (Go) or `@route` (Python). There are no `*_api` files and no
  hand-written MCP schemas. Go and Python write byte-identical
  `openapi.json` / `mcp.json` / `api.md`. Both full suites pass.
- Two bugs found late are fixed: Go `go.mod`/`go.sum` now list invopop's
  dependencies, and `msgspec==0.22.0` is pinned in `app/requirements.txt`.
- **Before starting this plan:**
  1. **Go `check_features.py` was stopped on purpose,** at 30 of 38 sets,
     because this plan rewrites Go routing anyway. Its only failures were the
     `code` field on `/api/account/password`, which was gated on two_factor
     and is now fixed (`only-passwords` passes on a rerun). Step 8 runs the
     full Go check.
  2. Python `check_features.py` may still be running (log in the
     scratchpad, `cf-py.log`). Look at its result before Python step 5.
  3. Then the user checkpoints with `gitPushUpdate.sh`. Claude does not
     commit.
  4. Then `/compact`.
- Scratchpad helpers from the last session (may be gone): `dev.sh`, `vet.sh`,
  `pydocs.sh`, `gydiff.sh` (Go-vs-Python doc diff), `jdiff.py`,
  `CONVENTIONS.md`, and the batch notes `notes-{go,py}{1,2}.md`.

## Goal

Go has no decorators. Today every Go endpoint is an exported constructor
returning `apidoc.Handle( apidoc.Op{...} , func(...) {...} )`: a closure inside
a return, with the metadata buried in a long literal. Each handler becomes a
**flat, unexported method on `*Handlers`**, with its metadata in comment
directives directly above it. A generator reads them and writes the route
table.

At the same time, both templates stop writing a route's guards twice (once as
route-group middleware, once as `access`/`role` in the docs). **The guards are
derived from the declaration.**

## Go: what a route looks like

```go
// setPasswordAccess is the owner's per-account switch.
// {allowed: false} stops this account's password working and stops it setting
// one.
//
//apidoc:route POST /api/owner/users/{user_id}/password-access
//apidoc:op summary="Allow or block an account's password" tag="Owner" access=SessionOnly role=Owner feature=passwords
//apidoc:op errors=404
//apidoc:mcp name=owner_set_password_access title="..." description="..." destructive
func ( handlers *Handlers ) setPasswordAccess( c fiber.Ctx , in *passwordAccessRequest ) ( reply *passwordAccessReply , err error ) {
	...
}
```

### Directives

- `//apidoc:route METHOD /path/{param}` (one per method). Paths are written in
  **spec style**, `{user_id}`, as in the docs, MCP and Python. The generator
  translates them to Fiber's `:user_id`, and fails unless every `{param}` is a
  `path:"..."` field of the request type, and every path field appears in the
  path.
- `//apidoc:op key=value ...`
  - Several lines merge. A long `description="..."` gets a line of its own.
  - Values with spaces are `"double quoted"` with Go string escapes. Lists are
    comma-separated (`errors=404,409`).
  - Keys are the `apidoc.Op` fields: `summary`, `description`, `tag`,
    `access` (`Signed`, the default, or `Public`, `SessionOnly`, `KeyOnly`),
    `role` (`Admin`, `Owner`), `feature`, `mcp_http`, `status`, `errors`,
    `reply_description`, `body_type`, `reply_type`, `no_csrf`, and
    `raw_body` (a type name).
  - An unknown key, a bad enum value or a bad status is a generate-time error
    naming file and line.
- `//apidoc:use expr, expr` holds extra middleware as **Go expressions, copied
  as-is** into the table, so the compiler checks them. Examples:
  `sameOrigin( loginElsewhere )`, `handlers.Guard.RequireSession`.
- `//apidoc:mcp name=... title=... description=... [readonly] [destructive] [admin] [enabled=expr] [call=expr]`
  makes the route's MCP tool. It replaces the registrations in
  `mcp_tools*.go`.
  - `admin` is `AdminOnly`, and defaults to on when `role=Admin`.
  - `call=withLoginLinkURL` names the function for a tool that adds to the
    reply.
  - `enabled=` is a Go expression of type `func() bool`.
- **Stream routes.** The method signature is `( c fiber.Ctx , in *T ) error`
  and `reply_type=` is set.
- **The method itself.** The generator checks that its signature is
  `( c fiber.Ctx , in *In ) ( *Out , error )` (or the stream form). A
  directive on anything else is an error.
- **Doc comments.** The ordinary Go doc comment is for developers, and never
  goes into the API docs.

### Guards, derived

Guards come from `access`, `role` and `feature`, in this order:

1. **Feature switch.** `feature=passwords` gives `RequirePasswords`, from a
   hand-written `featureGuards` map in `routes.go`: `invite_links`,
   `passwords`, `two_factor`, `files`, `webhooks`, `api_keys`, `mcp`,
   `metrics`, and the new `logs`. A feature with no switch, such as
   `sessions` or `appearance`, adds nothing. A feature switched off answers
   404 before anything else, as today.
2. **Who may call.**
   - The default `access` gives `RequireLogin`. `role=Admin` gives
     `RequireAdmin`, and `role=Owner` gives `RequireOwner`.
   - `KeyOnly` gives `RequireAPIKey`, and `Public` gives nothing.
3. `SessionOnly` adds `RequireSession` (not with `role=Owner`, which is
   session-only already).
4. **Extras.** Whatever `//apidoc:use` lists comes last.

Before converting, check each route's guards against what it has today, and
list every difference. The parity test (`TestTheSpecSaysWhoMayCallEachRoute`
plus the MCP coverage check) **stays as a safety net**.

### Startup conditions become request-time switches

The log routes are mounted only when `handlers.Logs.Enabled()` is true. That
condition goes. Instead they are always mounted, behind a new `RequireLogs`
switch guard that answers 404 while logs are off, like `RequireWebhooks`.
`/mcp` (GET, POST and DELETE) is declared like any route. The static and API
catch-alls, `/login/*` and `/join/*` stay hand-written in `routes.go`.

### The generated table

- **File.** `server/routes/routes_generated.go`, with the standard
  `// Code generated by apidoc/gen. DO NOT EDIT.` header. Never name a file or
  identifier `zz_...`.
- **Generator.** A Go program at `server/apidoc/gen/main.go`, using `go/ast`.
  A `//go:generate go run ../apidoc/gen` line in `routes.go` runs it, and
  `scripts/api_docs.sh` runs `go generate ./...` before the docs.
- **What it writes.** One `func ( handlers *Handlers ) mountDeclared( app *fiber.App )`
  that mounts every declared route with its derived guards. It names each
  route (for `apidoc.Build`) and records its `apidoc.Op`, request and reply
  types. It also writes one `registerDeclaredTools( server *mcp.Server )` for
  the `//apidoc:mcp` tools.
- **Order.** Most specific first: a literal segment sorts before a `{param}`
  at the same position, then by path, then by method. The generator fails on
  two routes that could match the same URL with the same method.
- **Feature markers.** Each entry carries its feature marker
  (`// feature:passwords`, or a `>>>` / `<<<` block), so `scripts/prune.py`
  cuts it with its handler. A test fails while regenerating would change the
  file, the way `TestAPIDocsAreCurrent` does for the docs. `prune.py` must
  leave a table that still compiles; check it with `check_features.py`.
- **`apidoc` package.** `Handle`, `Stream` and `Post`/`Get`/`Delete` become
  the internals the generated code calls, with no hand-written callers. Keep
  the request checks exactly as they are.

### Implied error statuses

The docs' error list for each route is derived from its guards and request,
and `errors=` adds only what the handler itself can answer (404 no such
thing, 409 stale, ...):

| Status | When |
|---|---|
| 400 | the request type has any field (a body, path or query value), or the route has a csrf token to check |
| 401 | `RequireLogin` / `RequireAdmin` / `RequireAPIKey` |
| 403 | a role, `RequireSession`, a CSRF check, `sameOrigin` |
| 404 | a feature switch, or `RequireOwner` |
| 429, 503 | every route, as now |

List each route whose documented statuses change.

## Python: the same shape

- **Path in the decorator.** The method and path become `@route`'s first
  argument: `@route("POST /api/admin/users/{user_id}/rename", summary=..., role=ADMIN, ...)`.
- **Route list.** `app/routes/__init__.py` builds its list from every
  declared handler, with the same guard derivation (the same order and the
  same feature → switch map), the same most-specific-first order, and the
  same ambiguity check. The groups go. The catch-alls and the page routes
  stay written out.
- **Extra guards.** `use=[SameOriginLogin, ...]` takes Guard classes, like
  Go's `//apidoc:use`.
- **MCP tools.** `@route(..., mcp=Tool(name=..., title=..., ...))`, or a
  sibling `@mcp_tool(...)` decorator (pick whichever reads better), replaces
  the registrations in `mcp_tools*.py`. `call=` works as now.
- **Startup conditions.** Same as Go: a per-request `RequireLogs`.
  `/mcp` is declared, and the derived error statuses are the same.
- Python's feature markers already prune a handler and its decorator
  together. Check that the built route list never names a pruned module.

## The two must still match

The Go and Python templates must write byte-identical
`openapi.json` / `mcp.json` / `api.md`, and behave the same. The only expected
doc changes come from the derived error statuses (and any guard fixes found
while checking). List each one in this file.

## Steps

1. **Generator.** Write it, with unit tests on directive parsing, errors,
   ordering, ambiguity and feature markers. Use a tiny fixture package.
2. **Go pilot: the admin routes.** Convert `admin.go`, its tools and its
   mounts by hand. Generate, then check `go vet` and `go test ./...`, and
   diff the docs.
3. **Go conversion script.** A one-off script, kept in the scratchpad and not
   shipped. For each `apidoc.Handle( apidoc.Op{...} , func ... )` it finds
   the mount in `routes.go` and the `mcp.ForRoute` registration, and writes
   the flat method with directives. Review the diff, and hand-fix Stream
   routes, `/mcp` and custom MCP calls.
4. **Go cleanup.** Delete the groups and `apidoc.Post` mounts from
   `routes.go`, and the registrations from `mcp_tools*.go`. Keep the
   catch-alls. Add `RequireLogs`.
5. **Python.** Move the path into `@route`, derive the guards, build the
   route list, and move the tools into the declarations. Then the full suite.
6. **Check the twins.** Diff the docs with `gydiff.sh`, then list the
   status-list changes.
7. **Documentation.** Rewrite the one combined **"Typed routes" entry in each
   `UPGRADING.md`** to describe the final design. Apps convert once, straight
   to directives (Go) or path-in-decorator (Python), and never see
   `apidoc.Handle` constructors. Also update `SKILL.md`,
   `references/architecture.md` and `references/frontend.md` in both skills,
   the "a new route" recipe and the MCP section, so a new route is one
   annotated method or function.
8. **Final checks.** Run `refresh_api_docs.py` for both, then
   `check_features.py` for both. Every set must be green.

## Rules

- Work on `master` only, with no branches or worktrees. Commits and pushes
  are the user's, through `gitPushUpdate.sh`.
- Change only `~/.claude/skills`.
- Never name anything `zz_...`.
