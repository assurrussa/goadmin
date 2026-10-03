# DataGrid backend contract audit

> Historical baseline at local commit `49e8746`. The current branch includes approved stage-one fixes; see `FIXES.ru.md` and `STAGE1-VERIFICATION.md` for current results.
Date: 2026-10-02. Base: `7d51c83eb1f1184d3b3b6421833ce22c97979de0`.
Scope: the supported `toolkit/datagrid` facade and its `infrastructure/core/datagrid` implementation. Only characterization tests, compile probes and audit evidence were added. Production code, module boundaries, dependencies and public contracts were not changed. No database, remote endpoint, push, PR, merge or deployment was used.

## Decision summary

The existing row/decorator/action pipeline is deterministic, and the new tests provide a reliable baseline for a later API-design decision. Do not infer a rewrite is needed from these results. Resolve the request/response round-trip defects and SDK export gap before treating the demo as a complete supported-host example. The 100-versus-1000 limit difference needs an explicit product/API decision; this audit does not assume either cap is the intended one.

Two conditional safety findings deserve attention: SQL key operators are unsafe for untrusted values in downstream users of the public helper, and the default HTML error component exposes the underlying error with Fiber's default error handler. Neither finding proves exploitation of a registered GoAdmin endpoint.

## Actual supported configuration

Source of truth: `toolkit/datagrid/datagrid.go:10-31,63-112`, `infrastructure/core/datagrid/types.go:23-45,90-157`, and `handler.go:22-72`.

- `FilterConfig[T]` has route/entity/title/repository, defaults, columns, actions, row decorators, search mode/placeholders, ID key, selectable/creatable/exportable/refreshable flags and page/error components. It has no fluent builder and no `Behaviour`, `UI`, `InlineCreatable`, configurable page-size-options or description field.
- `Repository[T].GetList(context.Context, Filtered) ([]T, int, error)` owns filtering, global search, database sorting, pagination and total-count semantics. The grid forwards filters; it does not implement these operations on returned rows or truncate an oversized repository response.
- `Column` describes key, label, type, format, sortable/filterable flags, filter placeholder/options and badges. Query parsing supports strings for text/badge/select/unknown types, integer numbers, booleans and dates in `2006-01-02` format. It does not parse a multi-sort, range or operator grammar from URL parameters.
- Handler defaults: page size 10, sort `createdAt desc`, server search, ID key `id`; localization defaults are Russian. Defaults are filled only when zero/empty, not generally validated during construction.
- `SearchMode` (`server`, `client`, `none`) is returned as frontend metadata. All three still forward a supplied search string and pagination to the repository. The backend does not load all rows automatically for client search or suppress search for `none`.
- Registration adds GET page, GET data and POST data routes. `RoutePath` separately drives returned URLs; the caller must keep it consistent with the registered base path. Action keys/labels/icons are presentation metadata, not execution handlers or route authorization.
- API shape: `data: [{item, actions, values?}]`, `meta: {title, description, pagination, sorting, filters}`, and `config: {routePath, columns, ui, behaviour}`. The original row is preserved beside computed values; computed values are not merged into the row. Raw callback functions and response-level `Actions` are not serialized. No configured actions yields `actions: null`; zero data rows yields an empty array.

## Prioritized findings

### P1, conditional public-helper risk: JSONB key values enter SQL text

`infrastructure/core/datagrid/helpers.go:136-148,241-253`; public exposure at `toolkit/datagrid/datagrid.go:52-54,87-89`.

`OpJSONBHasKey` embeds the input inside a quoted SQL literal rather than binding it. `OpJSONBHasAnyKey`/`OpJSONBHasAllKeys` format arrays, but `[]any` (the shape produced by JSON decoding) and the fallback do not escape quotes. The passing string-construction probe returns `metadata ? 'x' OR TRUE --'` with no bound arguments. This demonstrates an untrusted value crossing the SQL syntax boundary. It does not execute SQL.

Additionally, Squirrel's PostgreSQL dollar-placeholder conversion rewrites the unescaped key operator `?` into `$1`; the safe input `safe` produces `metadata $1 'safe'` with zero args. This is a correctness failure even without hostile input.

Exposure trace: all six production helper users were inspected. Their static field mappings use equality, LIKE and date comparisons, not JSONB operators:

- `infrastructure/pgsql/repositories/filerepo/api.go:25-46,69,96-97`
- `infrastructure/pgsql/repositories/adminrepo/api.go:43-63,83,127-128`
- `infrastructure/pgsql/repositories/userrepo/api.go:28-47,60,106-107`
- `domain/outbox/repositories/jobsbatchesrepo/api.go:28-34,44,55-56`
- `domain/outbox/repositories/jobsfailedrepo/api.go:23-31,69,80-81`
- `domain/outbox/repositories/jobsrepo/api.go:25-41,80,91-92`

Repository-wide production-source search found JSONB operator references only in the operator definitions, implementation and public facade. No current built-in route reaching the vulnerable key-operator branch was identified. This is a confirmed public toolkit hazard for hosts opting into those mappings, not a claim of an exploited built-in route. Follow-up: bind values and correctly escape operator question marks; test the resulting statements against PostgreSQL before advertising those operators for host use.

Evidence: `TestAuditPublicJSONBKeyOperatorsInterpolateUntrustedValues`; `backend-characterization.log`.

### P2: request paths have different normalization and validation contracts

`filters.go:27-80,157-205`; `handler.go:149-174`.

GET accepts requested limits only in 1..100 and silently falls back for malformed/negative/excessive values. POST accepts 1..1000 after only zero defaults. POST bypasses column filtering, type normalization, sort-field allowlisting and direction normalization: arbitrary fields, unfilterable keys, unknown sort keys and unknown directions reach the repository. JSON numbers arrive as `float64`; GET numbers arrive as `int`. Invalid JSON is silently retried using query filters.

This is a characterized inconsistency, not evidence that 100 or 1000 is the correct cap. A default `PageSize` above 100 can also remain effective for GET because the 100 cap is only applied to the query override. Repository SQL helpers retain independent sort/field allowlists, so this discrepancy alone is not proof of SQL injection in those repositories.

Follow-up decision: define one transport-independent normalized filter contract, including whether malformed requests fail or fall back, then apply it consistently. Preserve any intentional difference explicitly.

Evidence: `TestAuditPublicGETAndPOSTFilterContracts`, `TestAuditPublicValidationStopsRepositoryButHTMLResetsAllFilters`, `TestAuditPublicSQLHelpersKeepAllowlistBoundary`.

### P2: filter response metadata does not reproduce applied filters

`handler.go:182-194,211-212,320-332`; `types.go:375`.

Only string fields are returned in `meta.filters`. Applied integer, boolean and date values disappear. Global search is put into a local `appliedFilters` map, which is never used in the returned API response, so `_search` also disappears. A request applying name, count, enabled, date and search returns only the name in `meta.filters`.

Follow-up: choose a lossless wire representation and verify actual request → repository → response → UI rehydration, not just JSON shape.

Evidence: `TestAuditPublicFilterMetadataAndDatePaginationRoundTrip`.

### P2: generated pagination links alter filters and page size

`helpers.go:276-284`; `filters.go:74-76,118-120`; `handler.go:296-315`.

A parsed date becomes `2026-10-02 00:00:00 +0000 UTC` in pagination URLs; the parser accepts only `2006-01-02`, so following the next-page URL drops the date filter. Users/admins currently configure date filters (`http/handlers/users/handler.go:179-185`, `http/handlers/administrations/handler.go:192-198`). Also, `limit=10` is omitted unconditionally from generated links. With the common configured default 25, selecting 10 then following the next URL changes the limit to 25. Both failures are reproduced through registered GET data handlers.

Evidence: `TestAuditPublicFilterMetadataAndDatePaginationRoundTrip`, `TestAuditPublicTenRowPaginationLosesCustomPageSize`.

### P2: default HTML errors expose details; custom error callbacks own status

`handler.go:51-54,128-132,176-180,417-431`.

API repository/decorator failures become a generic 500. The default page error component wraps the original error; Fiber's default handler writes that wrapped message to the 500 response. Custom error components receive `ErrorData.Status=500`, but the grid does not set the actual response status before calling them; a callback merely sending text returns 200. Hosts with custom application error handling may redact or change this behavior; no claim is made that every built-in production response leaks details.

Evidence: `TestAuditPublicRepositoryErrorsAreHiddenForAPIButDefaultHTMLLeaksMessage`, `TestAuditCustomErrorComponentOwnsHTTPStatus`.

### P2: supported facade omits a required custom-error callback type

`toolkit/datagrid/datagrid.go:10-31`; `types.go:94,134-135`; `handler.go:409-414`.

The supported facade exports `ErrorTemplateComponent`, whose signature requires core `ErrorData`, but does not export `ErrorData`. A normal typed custom callback using only the supported `toolkit/datagrid` import cannot name that parameter. The compile probe fails with `undefined: datagrid.ErrorData`; a consumer must currently cross into the unsupported internal surface (or avoid this customization).

Evidence: `testdata/public-surface/error_callback.go.txt`, `public-surface-probe.log`.

### P3: internal usage documentation is stale

`infrastructure/core/datagrid/README.md:6-8,20,39-85` advertises 97.7% coverage and shows a repository signature plus `Action`, `Behaviour`/`UI` and inline-create fields that do not match the current implementation. The current coverage from this audit is listed below; neither number proves semantic completeness. Update examples alongside the later contract decision, without promoting internal packages into the host SDK.

## Callback contract, permissions and values

`handler.go:201-215,248-262,335-396`; `types.go:138-157`.

1. Repository is called once with the same Fiber request context and normalized/as-bound `Filtered` value.
2. For a nonempty returned slice, every non-nil prepare callback is called once, in decorator registration order, with the entire slice and that request context. The slice is not copied; preparation may intentionally update its elements. Empty results skip all prepares and all row callbacks.
3. For each row in repository order, every non-nil `Action.CanView` runs once in action order. Nil predicates make the action visible. These receive the row after preparation, but before computed values are resolved. Allowed action order is preserved. The grid does not deduplicate or memoize permission checks and does not hide/filter the row itself.
4. Then every non-nil resolver runs once for that row, in decorator order, with the same context and row. Map results are shallow-merged; later decorators win duplicate keys. The original row remains separate. Empty maps/nil results contribute nothing and leave `values` omitted when all are empty.
5. Repository failure stops everything. First preparation failure stops later prepares and all row work. First resolution failure stops later resolves and later rows; that row's action checks already ran. Errors preserve wrapping (`errors.Is`) and return no partial response. Predicate panics and callback side effects have no local rollback/recovery contract.

For N rows, A non-nil action predicates, P non-nil prepares and R non-nil resolvers: repository calls = 1, preparation calls = P if N > 0, action calls = N×A, resolve calls = N×R, absent errors. No global caching is performed.

Consumer implication: cache only request-invariant permission facts for that request, retaining per-row conditions and endpoint authorization. Current roles edit checks call `AdminGuardCheck` per row (`http/handlers/roles/handler.go:188-200`); the benchmark does not measure that service. Administrations show the intended batched prepare mechanism, with a per-row fallback for repositories lacking the batch capability (`http/handlers/administrations/handler.go:221-253`). Do not assume either path implies a database query count without tracing that service/repository.

Evidence: `TestAuditCallbackOrderArgumentsAndMerge`, `TestAuditCallbackErrorsStopWorkWithoutPartialResponse`, `TestAuditEmptyRowsSkipAllCallbacksAndNilActionsSerializeNull`, `TestAuditPublicActionsAndValuesWireShape`.

## Go 1.27 generic-method claim

Verified twice: official current documentation and the exact available Go 1.27.1 toolchain.

- [Go 1.27 release notes](https://go.dev/doc/go1.27#language): concrete methods may declare their own type parameters; interfaces may not declare type-parameterized methods or be implemented by generic methods.
- [Go generic-methods explanation](https://go.dev/blog/generic-methods): rationale and interface limitation.
- `testdata/generic-methods/main.go` defines a non-generic `Builder` with `With[T any](T) Typed[T]`. It compiles and runs with both int and string instantiations.
- `testdata/generic-methods/interface.go.txt` declares the corresponding generic interface method. The compile probe exits 2 with `interface method must have no type parameters`, as expected.

Thus a future concrete generic builder/method design is feasible on the module's Go 1.27 floor. The older blanket claim that Go cannot have generic methods is false here. This does not make generic interface methods available or by itself justify replacing the existing API.

Evidence: `go127-probe.log`; no go.mod edit, downgraded toolchain or extracted module was used.

## Tests and bounded performance

- Both scoped packages pass, including 13 added top-level test groups and subcases: `backend-test.log`, `backend-characterization.log`.
- One scoped race+coverage run passes: core 93.3%, public wrapper 53.8%, combined instrumented statements 92.3%. The wrapper's alias-imported methods run as core code; wrapper-only coverage is not feature coverage. Full data: `backend-race-coverage.log`, `backend-coverage.out`, `backend-coverage-functions.log`.
- Scoped lint passes with 0 issues; gofumpt and gci checks report no differences. Evidence: `backend-lint.log`.
- The aggregate repository check is owned by the overall audit; these scoped results alone are not a full-repository release gate.

`BenchmarkAuditGridResponse` performs one in-memory repository call, two prepares, three per-row predicates, two per-row map resolvers and full response JSON encoding. Fixed work: `-benchtime=50x -count=3`, only 100 and 1000 rows. No database, HTTP transport, real permission service or browser rendering is included. No throughput SLA is inferred.

| Rows | Time range | Bytes/op range | Allocations/op | Checks/resolves/prepares |
|---|---:|---:|---:|---:|
| 100 | 0.189–0.228 ms | 161,746–163,590 | 807 | 300 / 200 / 2 |
| 1000 | 1.612–1.791 ms | 1,693,867–1,732,142 | 9,501–9,503 | 3000 / 2000 / 2 |

These are three samples from the audit environment, useful as a bounded characterization, not stable production capacity. The 1000-row test exercises the transformation path directly; GET query parsing does not accept `limit=1000`. Callback counters are asserted by the benchmark, not estimated. Raw output: `backend-benchmark.log`.

Reproduction from repository root with Go 1.27.1 on PATH and existing permitted external Go caches:

```sh
GOTOOLCHAIN=local go test -count=1 ./toolkit/datagrid ./infrastructure/core/datagrid
GOTOOLCHAIN=local go test -race -coverprofile=audit/datagrid/backend-coverage.out -count=1 ./toolkit/datagrid ./infrastructure/core/datagrid
GOTOOLCHAIN=local go test -run '^$' -bench '^BenchmarkAuditGridResponse$' -benchtime=50x -count=3 -benchmem ./infrastructure/core/datagrid
GOTOOLCHAIN=local go run audit/datagrid/testdata/generic-methods/main.go
```

The audit reused an existing external module/build cache because the nominal shared-cache mount was read-only; it did not copy caches into the checkout or change dependencies. Negative probes use copies of `.go.txt` fixtures in a temporary directory so they do not break ordinary package builds.

## High-value remaining tests

1. Once intended behavior is decided, replace characterization assertions for the GET/POST and pagination defects with desired-contract regressions, including browser rehydration of typed filters/search.
2. PostgreSQL integration for every exported JSONB operator, both placeholder formats, malicious quotes, arrays and empty arrays; assert bound args as well as successful query semantics.
3. End-to-end authorized/unauthorized host routes proving hidden actions remain denied on their action endpoints. Grid `CanView` is presentation filtering only.
4. Multi-request/concurrent tests for stateful decorator closures and batched permission facts; the current deterministic tests cover one request and the race pass does not manufacture shared mutable closures.
5. Pagination empty/out-of-range/overflow policy, a caller-supplied RoutePath different from registered base, and custom PageSize values. Current `CalculateFromTo` can report from greater than to for out-of-range pages (`helpers.go:319-330`).
6. Public-only compile fixture for every callback customization and constructor, including the missing error-data alias, plus supported examples rather than the stale internal README.
7. Cancellation/deadlines and serialization failures from computed values; nil logger/repository/configuration validation and panic semantics if those are meant to be supported contracts.
