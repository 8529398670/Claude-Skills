# Plan: one `@route(...)` declaration, the same in Go and Python

Follow-up to the route-directives plan (deleted once this replaced it; in git history) (built, all its checks green: Go
check_features 38/38, Go and Python full suites, byte-identical docs). Decided
in an interview on 2026-11-13.

## Results (2026-11-14)

Built as specified. Checked by diffing against a saved baseline, then both
full suites once (Go `go test ./...` all green, Python 431 tests OK), then
`refresh_api_docs.py` for both. Go and Python write byte-identical docs.

- **Route table and route list:** mounts, guards and error lists unchanged;
  only Summary/Description/Title text differs.
- **`mcp.json`:** byte-identical except two tools that gained route words
  folded in (below).
- **`openapi.json` / `api.md`:** 41 routes changed summary or description,
  all of them routes with tools. Nothing else changed.
- **Folded in** (route words the tool lacked, so the tool text changes):
  `admin_set_user_role` adds "Becoming an admin ends the account's sessions
  and live login link."; `files_delete` becomes "Move one of your files to the
  bin. It can be restored for ten minutes; after that it is gone." + note
  "files_restore brings it back.".
- **Not folded:** `label_my_session`'s "This device included." (the tool's
  "one of your devices" covers it).
- **Overrides** (`mcp_title`/`mcp_description`, route keeps its own words):
  `files_start_upload`, `files_download_url`, and also
  `revoke_my_other_sessions` ("Sign out every other device" is right for a
  browser; through a key every device is signed out).
- **Notes are strict suffixes** of today's tool text, split at the first
  sentence naming a tool, `login_url`, `join_url` or "tool", so a few
  sentences that apply over HTTP too (e.g. "Answers with removed: 0 or 1." on
  `admin_cancel_login_link`) moved to the note with it.
- Python's `LOGIN_LINK_ANSWER` is gone (inlined). One-line declarations are
  allowed when a route has no tool and fits in 110 columns; longer ones end
  with `// )` / `)` on their own line.
- Pruning mcp checked once each: both compile, no tool keys left, and the Go
  routes tests pass after `scripts/api_docs.sh`.

## Goal

1. **One text per route.** A route's summary and description are also its
   MCP tool's title and description. No more writing them twice.
2. **One syntax in both templates.** Go declares a route in a comment that
   reads exactly like Python's decorator.

```go
// cancelLoginLink stops an account's live login link working, without
// making a new one. Sessions it already made stay signed in.
//
// @route("POST /api/admin/users/{user_id}/cancel-login-link", summary="Stop an account's login link working",
//     tag="Admin", role=ADMIN, errors=(404,),
//     description="Void an account's live login link without making a new one: a link that went "
//         "somewhere it should not have. Devices it already signed in stay signed in. Answers with removed: 0 or 1.",
//     mcp_note="admin_revoke_access ends those too.",  // feature:mcp
//     mcp="admin_cancel_login_link", destructive=True,  // feature:mcp
// )
func ( handlers *Handlers ) cancelLoginLink( c fiber.Ctx , in *userIDPath ) ( reply *cancelLoginLinkReply , err error ) {
```

```python
@route(
    "POST /api/admin/users/{user_id}/cancel-login-link", summary="Stop an account's login link working",
    tag="Admin", role=ADMIN, errors=(404,),
    description="Void an account's live login link without making a new one: a link that went "
    "somewhere it should not have. Devices it already signed in stay signed in. Answers with removed: 0 or 1.",
    mcp_note="admin_revoke_access ends those too.",  # feature:mcp
    mcp="admin_cancel_login_link", destructive=True,  # feature:mcp
)
def cancel_login_link(request: Request, body: UserIDPath) -> CancelLoginLinkReply:
```

## The declaration

- **Spelled exactly like Python in both:** `ADMIN`, `OWNER`, `PUBLIC`,
  `SESSION_ONLY`, `KEY_ONLY`, `True`/`False`, tuples `errors=(404,)`, lists
  `use=[...]`. A route can be copied between the templates and only the Go
  expressions change.
- **Long text wraps as adjacent string literals,** joined the way Python joins
  them, in both. No constants.
- **Route keys:** the first argument `"METHOD /path/{param}"`, then the Op
  fields as today: `summary`, `description`, `tag`, `access`, `role`,
  `feature`, `mcp_http`, `status`, `errors`, `reply_description`,
  `body_type`, `reply_type`, `no_csrf`, `raw_body`, `no_switch`, `use`.
- **Tool keys** (meaningful only with `mcp=`), each line ending in
  `// feature:mcp` / `# feature:mcp` in the template, so pruning mcp cuts
  exactly those lines:
  - `mcp="tool_name"`: the tool's name. Names never change (agents call
    them).
  - `read_only=True`, `destructive=True`, `admin_only=False` (an override:
    on by default for `role=ADMIN`), `enabled=...`, `call=...`.
  - `mcp_note="..."`: MCP-only words added after the shared description
    (tool names, `login_url`, ...). Only the tool shows it.
  - `mcp_title=` / `mcp_description=`: whole overrides, for the few tools
    whose words must differ from their route (`files_download_url`,
    `files_start_upload`).
- **The tool's title** is `mcp_title`, or else the summary, with ` (admin)`
  appended automatically for an admin-only tool. Its description is
  `mcp_description`, or else the description, then the note.
- **Go expressions stay Go** inside the Go comment: `use=[sameOrigin( loginElsewhere )]`,
  `call=withLoginLinkURL`, `enabled=handlers.filesOn`. They are copied as
  they are into `routes_generated.go`, so the compiler checks them.
- **Go layout:** `// @route(` starts the declaration, it is the last thing in
  the doc comment, and `// )` ends it on its own line. The developer doc
  comment above it never reaches the docs.
- **Python:** `@mcp_tool` and its imports go. Everything is on `@route`.
- **The old `//apidoc:` directives are refused,** with file and line. One
  combined UPGRADING entry per skill, straight to `@route`.
- The generator stays as strict as now: an unknown key, a bad value, a
  `{param}` that is not a path field, or two routes that can match one
  request stops it, by file and line.

## Merging the texts

- **Tool titles win.** For a route with a tool, the summary becomes the
  tool's title minus ` (admin)`. OpenAPI summaries change. MCP titles come out
  as before.
- **Tool text wins** for the description too. Check that nothing in an
  existing route description is lost, and fold it in if it is.
- **MCP-only wording moves to `mcp_note`:** mentions of tool names,
  `login_url`, `join_url`, and "Answers with ..." about fields only the tool
  adds. Split at sentence boundaries, so the tool's description (description,
  space, note) stays **word for word what it is today**.
- Routes without a tool keep their summary and description unchanged.

## How it is checked (no new tests, no feature-check runs)

The work is checked by **diffing what gets generated against what we have
now**, then fixing until every difference is one we meant:

1. Before starting, save today's `routes_generated.go`, `openapi.json`,
   `mcp.json`, `api.md`, and Python's `declared_routes()` (method, path,
   guards, errors) as a baseline.
2. Convert both templates with a scratchpad script (not shipped).
3. Generate and diff:
   - **`routes_generated.go`:** mounts, guards and Errors identical; only the
     Op's Summary/Description text may change.
   - **Python's route list:** identical to the Go table (method, path,
     guards, errors).
   - **`mcp.json`:** expected **byte-identical** to today. Any difference is a
     mistake in the merge.
   - **`openapi.json` / `api.md`:** only summaries and descriptions of routes
     with tools may change. List them.
   - **Go vs Python:** byte-identical docs.
4. Fix and repeat until the diffs are only the expected ones.
5. Then run each template's full test suite **once** (Go `go test ./...`,
   Python's unittest discover). The 38-set `check_features.py` runs are
   skipped for this change. The existing generator unit tests stay and are
   updated to the new syntax.
6. `refresh_api_docs.py` for both, so the templates' committed docs match.

## Steps

1. Baseline (above).
2. Parser for `@route(...)` in `server/apidoc/routegen`: Python-like call
   syntax, adjacent strings, tuples and lists, Go expressions in `use`/
   `call`/`enabled`, trailing feature markers ignored. Update its unit tests.
3. Python: `@route` takes the tool keys. Remove `@mcp_tool`. The title,
   `(admin)` and note rules go in `build_registry`, the same as Go's
   generator.
4. Conversion script for both templates: directives → `@route(...)`,
   `@route` + `@mcp_tool` → one `@route`, with the text merge above.
5. Diff loop (above), then one full suite each, then `refresh_api_docs.py`.
6. Docs: rewrite the combined "After 3731d9a" UPGRADING entry in both
   skills, and update SKILL.md, `references/architecture.md` and
   `references/frontend.md` (the route recipe, the MCP section), routegen's
   package doc and `app/apidocs/route.py`'s.

## Rules

- `master` only, no branches. The user commits with `gitPushUpdate.sh`.
- Change only `~/.claude/skills`.
- Never name anything `zz_...`.
