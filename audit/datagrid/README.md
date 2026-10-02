# DataGrid contract demo and targeted fixes

> Current results: **295 ordinary frontend tests; zero expected failures**. See `FIXES.ru.md` and `STAGE1-VERIFICATION.md`. Detailed `backend-report.md`, `frontend-report.md`, `REPORT.ru.md` and `VERIFICATION.md` describe the historical pre-fix baseline.

Production baseline: public `assurrussa/goadmin` master `7d51c83eb1f1184d3b3b6421833ce22c97979de0`.
The changes preserve the existing architecture and public API. This directory contains reproducible tests, synthetic demo instructions, findings and verification limits.

## Run the isolated mock demo

From `resources`:

```sh
npm ci
npx vite --config audit/datagrid/vite.config.ts
```

Open `http://127.0.0.1:5184/` in your own development browser. The isolated entry imports the current production DataGrid components directly and does not run the GoAdmin server. It never uses credentials or real account data. All records are synthetic. The fetch adapter only accepts `/audit/data` and rejects other routes. Reset restores the deterministic fixture.

Controls cover 0/100/1000 total records, 10/100/1000 rendered rows, full/read-only/mixed row action sets, configured column types, computed values, search, filters, sorting, links, loading, 401/403/500/retry, out-of-order responses and unmount/remount. Every click, emitted callback and API request/response is recorded with a sequence number, arguments and timestamp. The trace is capped at 300 entries to bound debug overhead. Create/edit/detail/destructive-intent/export handlers are explicit **host simulation**, not built-in DataGrid functionality.

Selection repairs are included. Inline-create UI remains absent. Bulk actions, inline row editing and built-in forms are not implemented features and are not falsely tested as working. `image` and standalone `badge` types currently fall back to text. The harmless HTML probe inserts a local `<b data-audit-injection>` marker; it does not execute JavaScript or make network requests.

The real GET handler caps requested page size at 100. The mock's 1000-row page is intentionally a rendering stress case, **not** a claim that the real endpoint returns 1000 rows or that its cap must change.

Build/typecheck the standalone demo:

```sh
npx vite build --config audit/datagrid/vite.config.ts
npm run admin:paths
npx vue-tsc -p audit/datagrid/tsconfig.json --noEmit
```

## Tests and evidence

- `resources/src/js/composables/useDataGrid.contracts.test.ts`: current composable contracts and repaired regressions.
- `resources/src/js/components/datagrid/`: component contracts and rendering-safety regression tests.
- `resources/src/js/composables/datagridFixture.contracts.test.ts`: the fake host itself, not production backend proof.
- `toolkit/datagrid/audit_contract_test.go`: supported public toolkit tests.
- `infrastructure/core/datagrid/audit_callbacks_test.go`: callback execution/ordering/error and benchmark cases.
- `backend-report.md` / `frontend-report.md`: detailed findings, provenance, coverage and caveats.

The nine historical expected-failure (`it.fails`) tests have become ordinary passing regression assertions. Historical reports retain their original counts and limitations; they do not describe the repaired current tree. Raw logs and the prebuilt demo are supplied in the downloadable evidence bundle, not committed here. The tests and commands in this repository are the reproducible source of truth.

## Browser verification limitation

The test browser attempted to open the running local demo and returned `net::ERR_BLOCKED_BY_CLIENT`. No access workaround, user-computer fallback, remote deployment or headless substitute was used. Visual layout, actual keyboard traversal, responsive/mobile interaction, browser paint/scroll responsiveness and real-browser injection behavior are **NOT RUN**. JSDOM component tests are DOM logic evidence only, not visual/browser performance evidence.

## Performance method

Go benchmarks measure in-memory handler work/callbacks only, excluding database, network, serialization to a socket and browser rendering. The bounded JSDOM benchmark measures actual Vue mount + nextTick with 9 fixture columns, current action menus and 100/1000 rows; it excludes layout and paint. The demo's optional browser timing control uses a warmup plus three samples per size through two animation frames; it is supplied for reproduction but was not run here. Virtualized shared runner results are noisy; no absolute speed or SLA is promised.

### Observed synthetic-render limit

The bounded repeated JSDOM run did not finish within its 180-second external budget. Its log confirms three 100-row mounts (about 1.0–1.5 seconds) and one asserted 1000-row mount (about 84 seconds) before the next repeat. A separate cold 1000-row attempt reached its 75-second external budget. These incomplete, shared-runner JSDOM measurements are **not** a browser-speed result and are not averaged or presented as a production benchmark. Keep the dedicated render benchmark outside the default unit-test gate. Reproduce with an explicit budget, then prioritize real-browser paint/interaction profiling before choosing virtualization or rewriting the component. Raw logs and `environment.json` preserve the limitations rather than claiming a pass.

## Run the prebuilt demo from the delivered bundle

The source/evidence archive also contains `resources/audit/datagrid/dist/`. From the extracted bundle root, a local static server is sufficient:

```sh
python3 -m http.server 5184 --bind 127.0.0.1 --directory resources/audit/datagrid/dist
```

Then open `http://127.0.0.1:5184/`. This prebuilt mock demo is self-contained; no GoAdmin database or installed npm dependencies are needed. Do not treat the demo as an authenticated service or deploy it with real data.

The source overlay requires a checkout of the baseline repository to resolve its production-component imports. The prebuilt bundle is supplied for inspection; the source and tests remain the reviewable deliverable.
