# DataGrid backend targeted fixes

Date: 2026-10-02. Baseline audit: `backend-report.md`, production base `7d51c83` and characterization tests `49e8746`.

The baseline report and baseline logs remain historical evidence. This note describes the subsequent approved first-stage implementation. No module boundaries, dependencies, database environment, routes, GET/POST limit policy or custom error callback status policy were changed.

## Fixed

- Every exported JSONB operator now binds all value operands. Key arrays use element bindings and an explicit `text[]` cast, including valid typed empty arrays. Existing scalar PostgreSQL array-literal inputs remain bound as `?::text[]`. Extraction paths and field names are also bound, with explicit `text[]`/`text` casts to avoid overloaded-operator ambiguity.
- Literal question marks in JSONB `?`, `?|`, `?&` and `@?` operators are escaped for Squirrel. `squirrel.Question` returns intermediate escaped SQL; finalize PostgreSQL statements with `squirrel.Dollar`. Question-format output is not claimed to be directly executable PostgreSQL SQL. Field mapping columns and operators remain trusted SQL configuration; the existing adopted-field/sort allowlists remain the trust boundary.
- Pagination URL/link builders share canonical scalar filter encoding. Dates use `YYYY-MM-DD`; every generated pagination URL carries the effective limit, including 10 with a configured default of 25. `Filters.ToQueryParams`/`BuildURL` also preserve explicit 10 correctly while retaining default omission.
- Existing public `map[string]string` filter metadata is preserved. Integer, numeric JSON, boolean and date values now survive as canonical strings, and nonempty global search appears as `_search` in both API and initial page responses. Reserved protocol names cannot overwrite search, sorting, pagination or `_search` during serialization. This does not change the filters passed to repositories.
- `toolkit/datagrid.ErrorData` is now an alias of the callback's core type, so supported-import-only hosts can declare custom error callbacks.
- The default HTML error callback returns a generic 500; repository/decorator details remain in server logging. Explicit custom callbacks still receive the original wrapped error and own response status, including callbacks that choose a non-500 status.

## Regression evidence

`toolkit/datagrid/security_regression_test.go` adds eight test groups:

1. All eleven exported JSONB operators: exact intermediate and PostgreSQL-final SQL, offset placeholder numbering, hostile value/path/field operands, and bound argument assertions.
2. Both JSONB array-key operators: strings, JSON-decoded arrays, integers, nil/empty typed slices, scalar array literals, null, quotes, backslashes, question marks, Unicode and punctuation. These are SQL-construction negative probes, not database execution.
3. Every pagination navigation/numbered link preserves date, false, zero, escaped text/search, sorting and a ten-row selection.
4. Direct URL helpers preserve the effective limit and prevent reserved-field control collisions, including unset/default configuration.
5. POST scalar/search metadata is preserved without changing the existing repository filter contract.
6. A real compiled public-only `ErrorTemplateComponent`/`ErrorData` callback receives the original error and retains its chosen status.
7. Default HTML/API responses redact both preparation and resolution failures.
8. Initial page callback metadata retains typed filter values and search.

The existing characterization tests now assert the corrected behavior for SQL bindings, date/page-size round trips, scalar/search metadata and default HTML redaction. Existing GET/POST asymmetry, callback sequencing and custom-status tests remain intact. A formerly ineffective nonstring-filter test now configures actual filterable typed columns before checking their returned values.

## Verification

Passed using Go 1.27.1 and the existing permitted external caches:

```sh
go test -count=1 ./toolkit/datagrid ./infrastructure/core/datagrid
golangci-lint run --timeout=5m ./toolkit/datagrid ./infrastructure/core/datagrid
```

- Scoped package tests: `backend-fix-test.log`.
- Scoped lint: `backend-fix-lint.log`, zero issues.
- `gofumpt -l`, `gci diff` and `git diff --check` are clean on the changed backend files.
- Aggregate race, build and benchmark results are recorded in `STAGE1-VERIFICATION.md`.
- PostgreSQL runtime/integration semantics were not executed. No database bootstrap or external target was used. Parameterization and placeholder numbering are verified at SQL construction boundaries.

## Deliberately unchanged

The GET 100 versus POST 1000 cap, POST type/field validation and malformed-body fallback, complex collection-valued filter wire policy, pagination overflow/out-of-range policy, selection behavior and action endpoint authorization still require their separate contract work. Metadata compatibility is preserved through canonical strings; this patch does not introduce a new typed-object JSON metadata schema.
