# Verification ledger

> Historical baseline at local commit `49e8746`. The current branch includes approved stage-one fixes; see `FIXES.ru.md` and `STAGE1-VERIFICATION.md` for current results.
Baseline: `7d51c83eb1f1184d3b3b6421833ce22c97979de0`. Final tracked production diff: empty.

## Passed

- Go: `go mod tidy -diff`, gofumpt/gci, `go vet ./...`, full golangci-lint and `go test -race -cover -covermode=atomic -count=1 ./...` through the single `make check` attempt. See `make-check.log`.
- Backend-focused characterization (13 new top-level groups plus subcases), race/coverage and scoped lint. Core 93.3%, public wrapper 53.8%; statement coverage is not behavioral completeness.
- Public generic-method compile/run probe on actual Go 1.27.1; generic-interface negative probe fails as intended.
- Resource manifest tests: 28 passed.
- Final resource tests: 178 cases / 20 files (169 ordinary passes plus 9 expected-failure guards). See `final-functional-check.log`. Nine explicitly expected-failure guards remain; the green suite is not a claim that these nine behaviors work.
- Frontend ESLint, Prettier, application typecheck.
- Production resource build and `verify-client-assets`: passed in `resources-final-check.log`. Only generated CSS changed during that build; it was restored to the exact baseline bytes to keep this audit free of production changes. The main component/handler code was never edited.
- Isolated demo ESLint, typecheck and build: `demo-final-check.log`.
- Dedicated demo smoke test: real DataGrid plus mock host; create validation/save, 403, retry and mount/unmount. See `demo-integration-test.log`.

## Corrected verification attempts

- Initial npm install failed because its default cache path was unavailable. Reinstalled from the locked registry dependencies using a writable cache outside the checkout.
- `make check` stopped at Prettier while new frontend tests were being formatted. Those tests were formatted, and the complete frontend gates were then rerun. Do not describe the original single command as a clean pass; the Go gates and final frontend gates are separate recorded passes.
- An intermediate demo build caught invalid multiline inline-handler expressions after formatting. Replaced them with named demo-only functions; the final build and integration test pass. No production component was changed.
- The first demo integration attempt used a frozen fake timestamp with nested click-capture listeners. Its test clock now advances before clicks; this is a harness-only adjustment, not a production fix.

## Incomplete / not run

- Test-browser local preview navigation: blocked with `net::ERR_BLOCKED_BY_CLIENT`. Real-browser visual, mobile/responsive, focus traversal, paint/scroll and script execution checks: NOT RUN.
- Repeated JSDOM 100/1000-row benchmark: incomplete, externally stopped at 180 seconds. One 1000-row mount completed before the next repetition; isolated repeat reached a 75-second budget. Do not equate this synthetic result with browser latency or promise 1000-row browser responsiveness.
- PostgreSQL JSONB operator semantics and actual SQL execution: NOT RUN. Security evidence is constructed SQL and bound-argument inspection, not exploitation of a deployed endpoint.
- Full host end-to-end auth/CRUD/database/persistence and real export downloads: NOT RUN. The demo's form/CRUD/export flows are explicit synthetic host callbacks.
- No remote push, PR, merge, deployment or module extraction.

## The nine desired contracts that currently fail

1. Accessible, operable selection checkboxes.
2. Visible numeric zero.
3. Escaped richtext text nodes.
4. Rejection of javascript richtext links.
5. Empty action-cell preservation for actionless rows.
6. Top-level initial pagination accepted consistently with fetched responses.
7. allSelected requiring membership of every visible row ID.
8. Zero/false preservation in the separate, currently unused CSV helper.
9. CSV doubled-quote escaping in that helper.

Current-defect characterizations and these desired guards must be changed together when approved fixes land.
