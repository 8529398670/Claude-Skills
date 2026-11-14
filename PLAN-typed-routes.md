# Plan: one typed declaration per route, no hand-written API docs

## Design (agreed 2026-11-13)

User decisions: unknown fields are rejected with a 400 on both doors; a field that fails validation answers with today's message for that field (declared on the field); a wrong type or bad JSON stays `"malformed request body"`.

### Package check (done 2026-11-13, from current releases)

- **msgspec 0.22.0** (2026-09-29) has wheels for cp310–cp315 on:
  - musllinux x86_64/aarch64;
  - manylinux x86_64/aarch64;
  - macOS arm64/x86_64;
  - win_amd64.

  That covers `python:3.14-alpine` and everything `requirements.txt` promises. Pin it as `msgspec==0.22.0`.
- **Huma v2.40.0** (2026-10-27): `adapters/humafiber` imports `gofiber/fiber/v3` (v3.6.1, the template's version) and needs Go 1.26, which the template meets. So it *works*, but I'm **not using it**. See the next section.
- **invopop/jsonschema v0.14.0** (2026-04): it is already in the module cache. It generates schemas only and does no validation.

### Design

#### Go: own typed wrapper + invopop/jsonschema (the fallback), and why

I'm taking the fallback over Huma for four reasons:
1. **Huma owns the whole OpenAPI document.** It brings its own schema names, `$schema` fields, `application/problem+json` errors and 422s. The templates must write byte-identical `openapi.json` / `mcp.json` / `api.md` across Go and Python, and Python can't run Huma. We would be post-processing most of Huma's output away and teaching Python to mimic the rest.
2. **Its handlers get a `context.Context`, not `fiber.Ctx`.** `Guard`, `CheckCSRF`, `refusal.send`, audit, webhooks and `serverError` all take `fiber.Ctx`.
3. **Errors would need a global `huma.NewError` override.** That override is needed to keep `{"error": msg}`, and there is also an undocumented remap of 422 to 400.
4. **A much larger dependency for a single-binary build.** That buys little once (1)–(3) are worked around.

The fallback keeps the doc assembly we control (security, `x-feature`, errors, `api.md`). It uses invopop only to turn structs into schemas, and validates against that same generated schema, so there is still one source.

#### Go: CreateUser, before and after

**Before** (`admin.go` + `admin_api.go` + `mcp_tools_accounts.go`):
- three structs: `createUserRequest`, `createUserBodyDoc`, `createUserReplyDoc`;
- a hand-written `InputSchema`;
- a handler that binds, checks CSRF, calls `createUser`, and replies with `fiber.Map`.

**After**, all in `admin.go`, inside the same feature markers as before:
```go
type createUserRequest struct {
	DisplayName string `json:"display_name" jsonschema:"minLength=1,maxLength=80" error:"display name must be 1-80 characters"`
	Role        string `json:"role" jsonschema:"enum=admin,enum=user" error:"role must be admin or user"`
	models.LoginLinkRequest // MaxUses/ExpiresInMinutes carry their own jsonschema + error tags
}
type createUserReply struct {
	UserID int `json:"user_id"`
	loginLinkReply // login_path, expires_in_seconds, expires_at, max_uses, unlimited
}

// CreateUser creates an account and its login link.
func ( handlers *Handlers ) CreateUser() apidoc.Route {
	return apidoc.Handle( apidoc.Op{ Summary: "Create an account and its login link", Tag: "Admin",
		Role: apidoc.Admin, Status: 201, Errors: []int{ 400 , 401 , 403 } },
		func( c fiber.Ctx , in *createUserRequest ) ( *createUserReply , error ) {
			return handlers.createUser( c , in ) // returns (*createUserReply, *refusal); refusal implements error
		} )
}
```
`routes.go` keeps the method, path and guard groups: `api.Post( admins , "/users" , handlers.CreateUser() )`.

`apidoc.Handle[In, Out]` does the following:
1. Reads path params (`path:"user_id"`), query params (`query:"q"`) and the JSON body into `In`.
2. Strips `csrf_token` and checks CSRF for session callers on non-GET requests. That removes the 66 hand checks, and `csrf_token` is documented on every body automatically.
3. Rejects unknown fields.
4. Validates `In` against its invopop schema, using `apidoc/check.go`, which today checks only replies and gains min/max/length/enum/pattern. On failure it answers 400 with the field's `error:` tag, or `"malformed request body"`.
5. Calls the handler, sends a refusal through `refusal.send`, and encodes `Out` with `Status`.

`api.Post` / `api.Get` / `api.Delete` mount the route and record method + path → `Route` in the doc registry. This replaces the `runtime.FuncForPC` lookup. A route mounted any other way is reported as undocumented, so the existing test keeps that guarantee.

**GET with query params, `ListUsers`:**
```go
type listUsersRequest struct {
	Q string `query:"q" jsonschema:"description=..."`
	Sort string `query:"sort" jsonschema:"enum=display_name,enum=role,enum=disabled,enum=created_at" error:"sort must be one of: display_name, role, disabled, created_at"`
	pageRequest // limit (1-1000, default 200), offset (>=0), dir (asc|desc) with today's messages
}
type listUsersReply struct { Users []userRow `json:"users"`; Next *int `json:"next"`; Total int `json:"total"` }
```
`sendSorted` / `sendPage` become typed helpers that take the already-validated page values. The hand parsing in `pagination.go` goes away.

#### Python: create_user, before and after

**After**, in `admin.py`, inside the same `# >>> feature:x` block as the handler:
```python
class CreateUser(Request):                       # Request = msgspec.Struct, forbid_unknown_fields=True
    display_name: Annotated[str, Meta(min_length=1, max_length=80, extra={"error": "display name must be 1-80 characters"})]
    role: Annotated[Literal["admin", "user"], Meta(extra={"error": "role must be admin or user"})]
    max_uses: MaxUses = UNSET                    # shared Annotated aliases with their own messages
    expires_in_minutes: ExpiresInMinutes = UNSET

class CreateUserReply(LoginLinkReply):
    user_id: int

@route(summary="Create an account and its login link", tag="Admin", role=ADMIN, status=201, errors=(400, 401, 403))
async def create_user(request, body: CreateUser) -> CreateUserReply:
    return await run(create_user_checked, request, body)
```
`@route` reads the handler's annotations: one parameter carries the request type, and the return type is the reply. Then it does the following:
1. Decodes the body (or path params + query, via `msgspec.convert(..., strict=False)`) into the Struct.
2. Strips and checks `csrf_token`.
3. Maps a `msgspec.ValidationError` to 400 with the failing field's `extra["error"]`, using the `$.field` path in the message, or else `"malformed request body"`.
4. Catches `Refused` and encodes the reply with `msgspec.json.encode`.

Path and query fields are marked with `Annotated[int, PATH]` / `QUERY`. `ListUsers` mirrors Go: `q`, `sort: Literal[...]`, and a shared `Page` base. The `__init__.py` route table and guard groups stay as they are, and the doc registry keeps keying by handler.

#### Docs generation (both)

- The schemas come from the packages:
  - Go: invopop `Reflector`, with `$defs` turned into `components/schemas`.
  - Python: `msgspec.json.schema_components(..., ref_template="#/components/schemas/{name}")`.
- A small **normalizer** maps both outputs to today's exact shape:
  - key order;
  - `["type","null"]` vs `oneOf`;
  - dropping msgspec's `title`;
  - named schemas.
- The rest stays in place: `Op`, security, `x-required-role`, `x-feature`, `x-mcp-http`, error responses, `markdown`, `encode`, and the reply `check`.
- Deleted: Go `apidoc/schema.go` and Python `apidocs/schema.py`, along with every `*_api.go` / `*_api.py` (including `schemas_api`; the named schemas move to their models/helpers).

#### MCP (both)

- **Inputs:** `Tool.InputSchema` / `input_schema` is removed. A tool names its route, and its schema is that route's request type: path + query + body fields, with `csrf_token` left out. Arguments decode into the same type with the same validator, so `call.Bind` with hand specs goes away.
- **List tools:** these gain `q` / `sort` / `dir`, because their routes have them.
- **Replies:** tools that add reply fields (`login_url`, `chunk_url`) embed the route's reply type plus the extra field.

#### Expected doc differences (to be confirmed by diff, and listed in the pilot)

- Request bodies get `additionalProperties: false`.
- `role` on create-user becomes required. The code wins here: it already rejects a missing role.
- Constraints that today live only in code now appear in the docs, e.g. `display_name` 1-80 and the MCP login-link maximums.
- MCP list tools gain `q` / `sort` / `dir`.
- The MCP descriptions of fields become the route's field descriptions, wherever they differed.

## Pilot findings (accounts/admin, 2026-11-13)

Converted: the 10 admin routes and their 10 MCP tools, in both templates.
`admin_api.go` / `admin_api.py` are gone. Go and Python write byte-identical
`openapi.json`, `mcp.json` and `api.md`. New validation tests:
`server/routes/typed_routes_test.go`, `app/tests/test_typed_routes.py`.

How the design landed:

- **Go:** `apidoc.Handle[In, Out](op, fn)` returns an `*apidoc.Endpoint`; a
  handler method returns it (`func (h *Handlers) CreateUser() *apidoc.Endpoint[...]`)
  and `apidoc.Post(group, path, handlers.CreateUser, middleware...)` mounts it
  and names the route `apidoc:<handler>`, which is how `Build` finds its
  declaration. Schemas come from invopop/jsonschema; `apidoc/reflect.go`
  normalizes them to the docs' shape. The old `schema.go` stays until the
  last `*_api.go` is gone.
- **Python:** `@route(**op)` on a plain function (run in a worker thread, so
  the MCP tool can call it too) or a coroutine; the second parameter's type is
  the request, the return annotation the reply. Schemas come from
  `msgspec.json.schema_components`; `apidocs/reflect.py` normalizes them the
  same way. msgspec sorts `Literal` values, so the normalizer puts them back
  in written order.
- **MCP:** `mcp.ForRoute(handlers.X, mcp.Tool{...}, call)` /
  `Tool.for_route(admin.x, call=..., ...)`. No call means the route's own
  handler answers.
- **Checks, in this order, in both:** JSON object, CSRF (session, non-GET,
  non-public), URL values readable as their type, unknown fields (first
  alphabetically), then each field in declaration order: missing, wrong JSON
  type (body: "malformed request body"; URL: the field's message), limits
  (the field's message; an enum without one says "x must be one of: ...").
  MCP: a malformed or unknown argument is JSON-RPC invalid params; a field's
  own message is a tool error, as the helpers' refusals were.

Doc differences so far (all expected, all from the code now being the docs):

1. Every JSON request body gets `additionalProperties: false`.
2. `POST /api/admin/users`: `role` is required (the code always required it).
3. `display_name` (create, rename): `minLength: 1`, `maxLength: 80`.
4. `max_uses`, `expires_in_minutes`, `expect`: may be null (they always could).
5. The `user_id` path parameter: an integer with a description, `minimum: 1`,
   where it was an undescribed string. `?limit` gets `default: 200`;
   `?offset` gets the description the MCP schema already had.
6. `User`: every field is required (the route always sends them all, false or
   "" included); `login_link` is required and nullable. `Revoked`: every field
   required.
7. MCP: tools take their route's whole request, so `admin_list_users` gains
   `q`, `sort` and `dir`, and the login-link tools gain the maximums. A field's
   description is the route's (e.g. `user_id`'s now names
   `GET /api/admin/users`), and `required` lists are sorted.

Behavior changes:

- An unknown body field is a 400 (was ignored over HTTP).
- `user_id` 0 is 400 "invalid user id" (was 404 "no such user").
- The MCP admin tools now also write the audit log and fire the webhooks
  their routes do (both doors run the same handler).


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
