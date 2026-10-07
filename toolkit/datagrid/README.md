# DataGrid public toolkit

Import `github.com/assurrussa/goadmin/toolkit/datagrid`. This is the supported
host-facing package; `infrastructure/core/datagrid` is its internal implementation.
DataGrid is part of the goadmin module, not a separate module or a standalone CRUD
framework. See the root [installation instructions](../../README.md).

## Runnable example

[example_test.go](example_test.go) is an external-package Go example using only
this public goadmin import, Fiber, the logger, and the standard library (the
companion regression test also uses Testify assertions). Run from
this checkout with the Go version required by `go.mod`:

```sh
go test ./toolkit/datagrid -run '^ExampleNewHandler$' -v
```

It requires no database or running server. Its checked output demonstrates a
fully typed repository, pagination/search, action visibility, one prepare pass
for a page of rows, per-row computed values, route registration, a page callback,
and a custom page-error callback. `app.Test` sends real requests through Fiber.
Use `app.Listen` in your application when you want a listening server.

## Integration contract

- Implement `GetList(context.Context, datagrid.Filtered) ([]T, int, error)`.
  Return the requested page and the **total matching rows before pagination**.
  Apply search, field filters, sorting and pagination in your repository; the
  handler does not run SQL for you. The example deliberately supports only
  search and pagination.
- Configure `FilterConfig[T]` directly: `Columns`, `Actions`, `RowDecorators`,
  `PageComponent`, `ErrorComponent`, and flat flags such as `Creatable`,
  `Exportable`, `Refreshable` and `Selectable`. There is no `Behaviour`, `UI`
  or `InlineCreatable` configuration.
- `RegisterRoutes(router, "/users")` registers `GET /users`, `GET /users/data`
  and `POST /users/data`. Use the same externally visible path in `RoutePath`
  for generated pagination URLs, accounting for any router-group prefix.
- Mount routes behind your host's authentication/authorization middleware.
  `Action.CanView` only controls which action metadata a row exposes. It does
  not authorize or implement an edit/delete endpoint. UI flags likewise do
  not create CRUD, export, bulk-operation or inline-create handlers.
- `NewRowDecorator(prepare, resolve)` runs preparation once per nonempty loaded page,
  then resolution per row. Batch-fetch related data in preparation rather than
  issuing one query per row. Keep enrichment request-owned: the example updates
  a fresh repository slice, never shared handler state. Do not mutate shared
  cached repository rows. `Values` holds computed presentation fields separately
  from `Item`; later decorators overwrite duplicate value keys.
- `PageComponent` receives `Response[T]`; supply a renderer for an actual HTML
  page. The example sends text. The default is diagnostic JSON, not an admin UI.
- `ErrorComponent` handles page-load failures and owns its HTTP status/body.
  It receives the original error for server-side diagnosis and sanitized
  `ErrorData`. Never send the original error to clients. Data-route load errors
  return a generic HTTP 500 without invoking this page callback. Filter
  validation on data routes returns HTTP 400; invalid page filters reset to
  defaults. Your Fiber error handler can customize API error presentation.

## Request and response details

GET accepts `page`, `limit`, `search`, `sortBy`, `sortOrder` and configured
filterable column keys. GET limits outside 1–100 fall back to the configured
page size. POST accepts a JSON `Filters` object (dynamic values inside `fields`),
validates limits up to 1000, and falls back to query parsing if body binding
fails. These transports are intentionally documented as they behave today:
POST does not apply GET's field/sort allowlists or typed query conversion.
Always allowlist SQL identifiers and bind values in the repository, including
when using `SQLBuilderx`, `SQLSortx` and `SQLWherex`; do not concatenate request
values into SQL. Trusted mappings must come from application code.

`/users/data` and `Response.ToAPIResponse()` expose `data`, `meta`, `config`.
Each data row has `item`, `actions`, and optional `values`. Pagination uses
camelCase keys such as `currentPage` and `perPage`; scalar filter metadata is
string-valued and search is represented by `_search`. `Response.ToJSON()`
serializes that same API envelope. There is no `ToJSONRaw()` or
`success/message/errors` wrapper.

## Verification

The executable example runs with normal Go tests. Public behavior and security
regressions are covered by this package's tests; supported package boundaries
are checked by `reference/externalconsumer` and `make externalconsumer-local`.
For repository-wide verification use `make check` as described in
[AGENTS.md](../../AGENTS.md). Measure coverage from the current test run rather
than relying on a fixed documentation percentage.

### Initial data and query provenance

Handler-generated GET page/data responses include optional `meta.requestQuery`.
It identifies the request query that produced the response; effective state stays
in `meta.pagination`, `meta.sorting` and `meta.filters`. A known empty query is
serialized as `""`; absent provenance must not be treated as a query match.
`Response.RequestQuery` is optional for manual responses; the GET handler sets it
automatically. POST body responses do not claim URL query provenance.

The Vue DataGrid can hydrate matching initial data without another request,
including server default/cap fallbacks. Legacy responses continue to work and
load once when their applicability to a nonempty URL cannot be established.
`initialData` can be replaced reactively. Generic grids use native browser
navigation by default; Inertia-owned pages set `navigationMode="inertia"` and
receive authoritative rows through restored/replaced page props.
