# Stage-one verification ledger

Production base: `7d51c83eb1f1184d3b3b6421833ce22c97979de0`. Pre-fix characterization commit: `49e8746`. These checks describe the stage-one implementation, not a deployment.

## Final passes

- Go dependency consistency (`go mod tidy -diff`), gofumpt/gci, `go vet ./...`, full golangci-lint (0 issues), and full `go test -race -cover -covermode=atomic -count=1 ./...`: passed in the aggregate attempt. Backend source/tests were frozen for these checks and did not change afterward. Evidence: `stage1-make-check.log`.
- Final Go DataGrid statement coverage from that full race pass: core 92.4%, public wrapper 69.2%. Coverage is not semantic completeness.
- Final frontend: **295 tests / 22 files, all ordinary passes; zero expected-failure tests**. All nine baseline expected failures were converted to real regression tests. Manifest: **28 passed**. Evidence: `stage1-resources-final.log`.
- Final full frontend ESLint, Prettier, application typecheck, production build and embedded client-asset verification: passed in `stage1-resources-final.log`.
- Isolated demo typecheck and production build: passed in that same final log. The demo smoke test is included in the 295-case suite.
- After rebuilding production assets, Go host client-bundle/public-asset tests and the external-consumer manifest tests passed: `stage1-embedded-check.log`. Updated compiled assets are included with the patch, so embedded consumers receive the repaired frontend rather than stale JavaScript.
- Independent read-only review: no unresolved blocker in scope. Additional probes and final URL/default/retry/endpoint checks are described in `independent-review.md`.
- `git diff --check`: passed.

## Aggregate-attempt correction

The original `make check` completed its Go gates, manifest, 293-case intermediate frontend suite, lint and formatting, then stopped at a TypeScript error in the new test fixture (a widened sortOrder type). The fixture now derives exact prop/callback types without broad casts. Independent review also completed the bare-URL default/retry correction. The entire affected frontend sequence, including typecheck/build, was then rerun on the frozen final tree and passed with 295 cases. Do not describe the original single command as a clean pass; the log preserves the failure and the final replacement gate. Full Go race was not wastefully repeated for the test-only/frontend correction.

## Bounded performance evidence

Go 1.27.1; linux/amd64; Intel Xeon Platinum 8573C; Node 24.19.0. Shared virtualized runner, so timing noise is expected.

The same in-memory response/JSON benchmark used in the baseline was run with `-benchtime=50x -count=3`:

- 100 rows: 0.217–0.262 ms/op; 159,900–161,746 B/op; 806–807 allocations/op.
- 1000 rows: 1.635–1.662 ms/op; 1,693,728–1,732,147 B/op; 9,501–9,503 allocations/op.
- Callback assertions remain exactly one repository call, two prepares, `3N` action predicates and `2N` resolvers per request.

Raw evidence: `stage1-backend-benchmark.log`. Database, real permission services, HTTP transport, browser layout/paint and user-observed latency are excluded. This is no production SLA or proof of browser speed. Expensive full-1000-row JSDOM repeats were deliberately not repeated.

## Unverified / unchanged boundaries

- Real-browser visual/mobile/native keyboard activation/scroll/paint: NOT RUN. Prior `ERR_BLOCKED_BY_CLIENT` was respected; no alternate browser or machine was used to bypass it.
- PostgreSQL runtime SQL execution: NOT RUN. Tests verify all exported operator SQL strings, placeholders and bound operands; they do not execute hostile queries.
- No host auth/CRUD/database end-to-end run or real export download. Grid CanView still controls presentation, not endpoint authorization.
- GET/POST cap and validation differences remain explicit; no new cross-page selection policy, bulk action, inline-create or versioned module was introduced.
- CSV formula-like content is retained literally by the currently unused helper; safe spreadsheet-import policy is not claimed.

## Reproduction

Use the repository's documented external caches and installed Go 1.27.1/Node toolchains. Ordinary checks remain `make check`. Demo build: from `resources`, `npx vite build --config audit/datagrid/vite.config.ts`. The delivered archive also includes the self-contained prebuilt demo and source patch.
