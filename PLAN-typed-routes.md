# Plan: one typed declaration per route, no hand-written API docs

Paste the prompt below into a fresh session once the security fixes
(`PLAN-security-fixes.md`, phase 1) are committed. It touches the same route
files, so it must not run alongside that work.

---

## Prompt

In `~/.claude/skills`, the go-webserver and python-webserver templates describe
every route twice. I want each route described once, in typed code beside the
handler, with the API docs, the MCP tool and the request validation all coming
from that one place. Nothing about a route should be written down twice, and
no separate per-route docs files should be left.

### What is there now

- **Python.** Handlers read raw dicts (`body = await read_json(request)`, then
  `body.get("user_id")`), so the code says nothing about what a route accepts
  or returns. Each `app/routes/X.py` has a twin `app/routes/X_api.py`, about
  2,900 lines across both, holding `doc(handler, summary=..., access=...,
  role=..., body=SomeTypedDict, reply=..., errors=...)` plus the `TypedDict`
  classes. `app/apidocs/` turns those into `config/openapi.json`,
  `config/api.md` and `config/mcp.json`.
- **Go.** Same shape: 23 `server/routes/*_api.go` files, about 2,700 lines,
  each with `apidoc.Doc( (*Handlers).X , apidoc.Op{ ... } )` and `...BodyDoc`,
  `...ParamsDoc` and `...ReplyDoc` structs. Those structs copy the request
  structs the handlers already bind into (`createUserRequest` and so on). The
  same is true of query parsing and of the reply maps (`fiber.Map{...}`).
- **MCP tools** (`mcp_tools*.go`, `mcp_tools*.py`) hand-write an
  `InputSchema` for each tool. That is a third copy of the same fields.
- **What keeps the copies in line today:** tests. `check_reply` in
  `route_support.py` (and Go's `checkReplies`) checks each success reply
  against its documented schema. `api_docs.sh` fails on a route with no docs,
  and the MCP parity tests fail on a key route with no tool. Keep these
  guarantees, or make them true by construction.

Read `references/architecture.md` in both skills, `server/apidoc/` and
`app/apidocs/`, a few `*_api` files, `mcp_tools_accounts.*`, and the parity
tests before you design anything.

### What I want

1. **One declaration per route, beside the handler.** Python: a decorator on
   the handler. Go: whatever is idiomatic, such as a typed handler generic over
   its request and reply types, registered with its metadata in one call. The
   declaration carries:
   - the summary, tag, access, role, feature and errors;
   - the request type (body, path and query params);
   - the reply type.
2. **The types are the code, not documentation.**
   - The framework parses and validates the request against the declared
     type, and hands the handler a typed value.
   - **Python: use msgspec.** Requests and replies are `msgspec.Struct`
     types, with constraints as `Annotated[..., msgspec.Meta(...)]`. One type
     decodes and validates the request, encodes the reply, and gives the JSON
     Schema (`msgspec.json.schema`) for the docs and the MCP tools. Check
     first that it has prebuilt wheels for every platform
     `app/requirements.txt` promises (musllinux, glibc Linux, macOS,
     Windows). Then pin it there, the same way as the other packages. Do not
     add pydantic.
   - **Go: use Huma v2** (`github.com/danielgtaylor/huma/v2`). Handlers
     become `func(ctx, *Input) (*Output, error)`, where the input and output
     structs define the parsing, the validation and the OpenAPI spec. First
     check that its Fiber adapter (`humafiber`) works with the Fiber v3 the
     template uses. Also check that it fits the guards (session, CSRF,
     owner-only), `refusal`, the error body shape, feature pruning and the
     single-binary build.
   - **Go fallback: invopop/jsonschema**, if Huma does not fit. Keep the
     template's own typed handler wrapper, a generic over the request and
     reply structs. Use invopop/jsonschema only to turn those structs into
     JSON Schema. Say which way you went and why, in the design.
   - Either way, the existing request structs (`createUserRequest` and so
     on) become the one declaration, and the `...BodyDoc` copies go.
   - The packages' generators should replace as much of `server/apidoc/` and
     `app/apidocs/` as they can. Keep only what still has a job, such as
     `x-feature`, MCP and `api.md`.
   - The handler returns the declared reply type, and the framework encodes
     it. No more ad hoc `fiber.Map` or dict replies on documented routes.
   - Because handlers cannot run without the types, the docs cannot drift, and
     the reply-check tests become a safety net rather than the only guard.
3. **MCP tools come from the same declaration.** Their input schema is the
   route's request type. Writing a schema by hand for a tool that wraps a route
   should be impossible.
4. **Delete every `*_api.py` and `*_api.go` file.** The generated
   `openapi.json`, `api.md` and `mcp.json` must come out the same as today, or
   differ only in ways you list and justify. Diff them before and after.
5. **Keep what works:**
   - feature markers (`// feature:x`, `# >>> feature:x`) and pruning. A
     declaration must sit inside the same marker as its handler, so
     `scripts/prune.py` still cuts cleanly;
   - the `refusal` / `Refused` pattern that both the HTTP and MCP doors share;
   - CSRF, session-only and owner-only guards;
   - the Python and Go templates staying twins: same routes, same docs, same
     behavior.

### How to work

- Check the packages first: msgspec's wheels, and whether Huma works with
  Fiber v3. Pick Huma or the invopop/jsonschema fallback from what you find.
  Don't decide from memory: read the current releases and docs.
- Write the design first, as a short section at the top of this file covering
  both languages. Show what one route looks like before and after, using
  `CreateUser` / `create_user` and one route with query params. Stop and show
  me before converting everything.
- Then convert one feature area end to end (accounts/admin is a good test,
  because it has MCP tools), run both suites, and diff the generated docs.
  Then the rest.
- Every existing test must pass. Add tests for the new validation: a missing
  field, a wrong type, out-of-range values and unknown fields each give the
  same 400 the routes give today.
- Update `SKILL.md` and `references/architecture.md` in both skills, so a new
  route is taught the new way, and remove all guidance about `*_api` files.
- Add one `UPGRADING.md` entry per skill so `webserver-upgrade` can carry this
  into the apps. Say plainly that an app's own routes need converting too, and
  how.
- Rules: work on `master` only, with no branches or worktrees. Leave commits
  and pushes to `gitPushUpdate.sh`. Change only `~/.claude/skills`; rolling
  this out to the apps is a separate step after this is committed.

### Done means

- No `*_api.py` or `*_api.go` files remain, and no hand-written MCP
  `InputSchema` remains.
- Each route's fields, constraints and reply shape are written exactly once.
- Both test suites are green, and the generated docs match today's, apart
  from listed differences.
- Adding a route, as the updated SKILL.md describes it, takes one declaration.
