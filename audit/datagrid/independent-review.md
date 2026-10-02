# Independent first-stage DataGrid review

Reviewed on 2026-10-02 against production baseline `7d51c83` and characterization/demo commit `49e8746`.

## Result

No unresolved release-blocking issue found in the final reviewed first-stage patch. This is a scoped code/test review, not a claim that PostgreSQL execution, real-browser behavior, or the full release gate has been verified.

## Findings resolved during review

- Reserved backend field names could replace pagination/search/sort controls during URL serialization. The final patch excludes protocol keys from dynamic URL/metadata fields without changing repository filtering or GET/POST limits.
- Browser URL synchronization omitted effective page size 10 and descending order, which lost state with custom host defaults. Both effective values are now serialized.
- Missing controls in initial or older/bare history URLs could force client defaults or retain the previous page size. The bounded URL request mode now omits absent limit/sort controls so the backend chooses authoritative defaults, then hydrates its metadata. Retry preserves these exact omissions.
- An old endpoint response could hydrate after the reactive endpoint changed. The final synchronous endpoint watcher invalidates in-flight/debounced intent; the next explicit refresh uses the new endpoint. It does not claim network-level abort.

## Reviewed boundaries

- Richtext uses fixed tags, escaped text and attributes, shape-checked nodes/marks, bounded heading levels and an explicit URL policy. Malformed input never returns raw HTML into the sink.
- Real Reka checkboxes use their model contract; selection and row-action callbacks retain exact counts and raw-row payloads. Sort controls are native buttons. Mixed action permissions retain aligned cells, and numeric zero/false remain visible.
- JSONB operands, including keys, array elements, extracted paths and extracted fields, are bound. Literal question-mark operators survive Squirrel Dollar finalization. Question-format output is explicitly intermediate SQL. Mapping columns/operators remain trusted host configuration, and unrecognized filter fields remain excluded by the mapping allowlist.
- Public metadata retains its `map[string]string` shape with canonical scalar/date values and `_search`. The public `ErrorData` alias is additive. Default error responses redact implementation details while custom callbacks retain the original error and status ownership.
- Latest-request fencing, debounce cancellation, retry snapshots, response hydration, error row/selection retention, URL history/hash preservation and unmount cleanup were checked. Existing GET 100 versus POST 1000 behavior, action authorization boundaries, page-load selection reset, and absent bulk/inline features were not redesigned.
- The unused CSV helper preserves zero/false/null and standard quoting without changing raw-versus-computed value sourcing or promising spreadsheet-formula sanitization.
- All nine original expected-failure guards are now ordinary passing assertions; the reviewed test changes convert the defects instead of suppressing or weakening their checks.

## Independent verification

- 21 additional Node/JSDOM richtext probes passed: control-character and malformed schemes, entity/encoded schemes, attribute breakout, malformed structures and fallback input. No payload was executed or clicked.
- An independent Node/Vue probe reproduced the stale-endpoint issue before its correction and passed afterward.
- A final independent Node/Vue probe passed for bare-URL host defaults (initial override 10/desc, authoritative host 25/asc), failed-load row/selection preservation, byte-identical retry URL omissions, normal refresh after hydration, stale endpoint fencing and subsequent endpoint targeting.
- Focused Vitest execution passed 175 tests across URL, table safety, composable and CSV suites before the final URL-mode adjustment. After that adjustment, the two affected URL/composable files were rerun independently: 79 tests passed.
- Backend scoped test and lint evidence was inspected (`backend-fix-test.log`, `backend-fix-lint.log`); both passed. The aggregate results are recorded in `STAGE1-VERIFICATION.md`.
- `git diff --check` passed on the final reviewed tree.

Commands run independently from `resources/`:

```sh
./node_modules/.bin/vitest run src/js/components/datagrid/DataGrid.url.test.ts src/js/components/datagrid/DataGridTable.safety.test.ts src/js/composables/useDataGrid.contracts.test.ts src/js/components/datagrid/dataGridStore.contracts.test.ts --reporter=dot
./node_modules/.bin/vitest run src/js/components/datagrid/DataGrid.url.test.ts src/js/composables/useDataGrid.contracts.test.ts --reporter=dot
```

## Verification limits

- No PostgreSQL setup/query execution was performed; SQL review verifies construction, placeholder numbering and bound operands.
- No real-browser visual, responsive, focus traversal or native keyboard activation proof was obtained. The prior `ERR_BLOCKED_BY_CLIENT` was respected; no alternate route was used to bypass it. JSDOM click/focus assertions are not browser layout or native keyboard proof.
- No large 1000-row JSDOM benchmark, production edit by this reviewer, commit, push, PR, merge or deployment was performed.
