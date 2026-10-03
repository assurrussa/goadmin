# DataGrid request and URL correctness fixes

Scope: targeted first-stage fixes on the isolated audit branch; no architecture replacement or new bulk/inline/export capability.

## Changes

- Search and filter changes now share one pending debounce. The latest change supplies the debounce delay and the combined current query is sent once. Direct page, sort, refresh, URL and retry requests cancel pending debounce work.
- Existing sequence fencing still rejects obsolete successes/errors immediately when a new debounced intent arrives and after unmount. The existing fetch contract remains unchanged (no AbortSignal was added).
- Ordinary requests snapshot their complete effective page, limit, sorting, search and copied filters for reliable retries. URL-derived requests also retain which transport controls were omitted, so retry cannot resurrect an older page-size/sort choice. Errors preserve previously loaded rows and selection; a successful response still clears selection.
- Reserved query keys (`page`, `limit`, `sortBy`, `sortOrder`, `search`, `_search`) cannot be overwritten by ordinary filters in API requests or synchronized URLs.
- Initial and fetched responses share hydration of pagination (metadata first, top-level fallback), canonical sorting and filter metadata. Canonical `meta.filters._search` rehydrates global search without leaking `_search` into ordinary filters. Missing canonical search in older responses preserves the requested search. Typed canonical filter strings remain strings for UI compatibility.
- The documented `window.AdminDataGrid.dataResponse` wrapper is unwrapped alongside direct objects and JSON strings.
- Mounting without initial data now makes one request using the URL intent. Initial URL-derived requests omit absent transport limit/sort controls, letting the backend select its authoritative defaults rather than inferring defaults from a previous effective response. Existing initial filter state is retained when no ordinary URL filters are supplied.
- Valid positive integer page/limit parsing rejects negative, fractional, unsafe and malformed values. Numeric page actions also require safe integers. Positive URL page numbers still go to the server for authoritative total-page bounds; no GET/POST maximum was unified.
- Synchronized URLs retain effective page size, including 10, and sort direction, including descending, so custom host defaults do not alter a reload. Existing hash and `history.state` survive replacement.
- Popstate requests the new URL state once, resetting omitted search/filters and omitting absent transport page-size/sort controls so the backend supplies authoritative defaults. Response metadata then rehydrates effective state, including when initial URL parameters had explicit overrides. New navigation supersedes pending debounce and old requests. The listener is removed on unmount; disabled synchronization ignores it.
- `apiUrl` is read reactively for subsequent requests. Changing it invalidates old endpoint work without triggering a new request. `initialData` remains mount-only; exposed refresh uses the current endpoint.
- `allSelected` checks visible ID membership rather than equal array lengths. Select-all deduplicates valid IDs and excludes invalid/missing IDs. No cross-page selection behavior was introduced.
- Zero/false filters count as active in empty-state messaging and create-button visibility.

## Regression coverage and verification

The fixed initial-pagination and selection `it.fails` guards are now ordinary passing tests; neighboring defect characterizations assert the corrected behavior. Existing request, error/retry, stale response and lifecycle tests were preserved. Added focused regressions cover canonical metadata, reverse-order debounce, direct intent cancellation, full retry snapshots, invalid numeric values, stale metadata, host query defaults, URL retention, popstate races/unmount and endpoint changes.

Passed from `resources/`:

- Prettier on `useDataGrid.ts`, `DataGrid.vue`, `useDataGrid.contracts.test.ts`, `DataGrid.url.test.ts`.
- ESLint on those four files.
- `vitest run src/js/composables/useDataGrid.contracts.test.ts src/js/composables/useDataGrid.test.ts src/js/components/datagrid/DataGrid.contracts.test.ts src/js/components/datagrid/DataGrid.url.test.ts --reporter=dot`: 4 files, 146 cases passed. This includes the table fixes in the shared component suite.

Aggregate type/build/repository gates and review results are in `STAGE1-VERIFICATION.md`. These mocked-fetch/JSDOM checks are not deployed backend validation or real-browser performance measurements.
