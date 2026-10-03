# DataGrid frontend characterization report

> Historical baseline report, recorded before the approved fixes. Its defect
> descriptions and expected-failure counts refer to `7d51c83` plus test-only
> characterization commit `49e8746`, not the repaired working tree. See
> [frontend-cell-fixes.md](frontend-cell-fixes.md) for the table/security fixes.

Baseline: `7d51c83eb1f1184d3b3b6421833ce22c97979de0`, branch `audit/datagrid-contracts-local`.
Scope: unchanged Vue DataGrid, its composable, and the separate unused Pinia helper. This work added tests only; production components, composable, store and configuration were not edited.

## Result and reproducibility

125 new cases pass the focused Vitest command:

- `DataGrid.contracts.test.ts`: 63 cases using real components, real UI primitives and real DOM event paths.
- `useDataGrid.contracts.test.ts`: 50 cases.
- `dataGridStore.contracts.test.ts`: 12 cases, explicitly independent of DataGrid UI.
- Nine of the 125 use `it.fails`, in clearly named **desired regression guards: expected failures on unchanged baseline** groups. They assert desirable behavior that is currently broken. Thus a green audit suite is **not** a claim that those nine behaviors work. If a fix makes one pass, Vitest reports an unexpected pass until the marker is removed.
- The other 116 assert existing intended contracts or explicitly labeled current limitations. Limitations are not promoted to desirable future guarantees.

Commands, from `resources/`:

```sh
./node_modules/.bin/prettier --write \
  src/js/components/datagrid/DataGrid.contracts.test.ts \
  src/js/components/datagrid/dataGridStore.contracts.test.ts \
  src/js/composables/useDataGrid.contracts.test.ts
./node_modules/.bin/eslint \
  src/js/components/datagrid/DataGrid.contracts.test.ts \
  src/js/components/datagrid/dataGridStore.contracts.test.ts \
  src/js/composables/useDataGrid.contracts.test.ts
./node_modules/.bin/vitest run \
  src/js/components/datagrid/DataGrid.contracts.test.ts \
  src/js/components/datagrid/dataGridStore.contracts.test.ts \
  src/js/composables/useDataGrid.contracts.test.ts --reporter=dot
```

All three commands passed after the final test edit. Final focused run: 3 files, 125 cases, 3.81 seconds wall duration in this shared environment. This is test runtime, not browser/UI performance. Full repository, typecheck and demo/browser results are recorded separately in `VERIFICATION.md`.

## Verified callback and interaction contracts

- Header refresh and export each emit exactly one `action-table` callback with arguments `('refresh', null)` or `('export', null)`. They do not fetch data themselves. Declared `action-refresh` and `action-export` emits are unused (`DataGrid.vue:119–120,260–266`).
- Header and empty-state create emit exactly one `action-create` with no arguments. On an empty creatable grid there are two create buttons, one in the header and one in the empty state.
- `edit`, `show`, `view`, `delete`, and a keyboard-opened custom dropdown action emit exactly one callback with the action key and the raw row object. Computed display values do not replace the action payload.
- Exposed `refreshData()` fetches once using current grid state. Refreshing the header and invoking the exposed method are distinct contracts.
- Pagination's enabled numbered/mobile-next controls emit the exact URL once; disabled previous/active-page controls and ellipsis emit nothing. A full-grid enabled page click performs one fetch plus one public `page-change` callback.
- Non-sortable/unknown columns do not fetch. A new sortable key starts ascending, toggles descending, and retains page/search/filters.
- Actual text/number input events emit one-key filter patches, including numeric zero. A real select opened by keyboard emits the selected value once. Clear-all emits empty strings for configured filter columns.
- Row DOM elements are retained across reordering and value updates with unique stable `ui.idKey` values. This verifies DOM identity, not just label equality.
- Real dropdown-containing grids mount/unmount repeatedly in the standard suite (three iterations of ten rows) without detached menus. No large performance workload was added to ordinary unit tests.

## Confirmed defects and mismatches

### Richtext HTML injection: high-priority security boundary

`DataGridTable.vue:100,381–404` builds HTML from ProseMirror JSON and assigns it to `v-html`. Text is not escaped; link attribute strings are interpolated without escaping or scheme validation.

The DOM tests use harmless synthetic input only:

1. A text node containing `<b data-audit-injection="text">harmless marker</b>` becomes a real matching `b` element, rather than literal text.
2. A link retains `href="javascript:void(0)"`.
3. A link href of `#" data-audit-link="injected` creates a separate `data-audit-link` attribute on the resulting `a`.

This is a reproduced HTML/attribute injection sink, not merely source speculation. The tests do not execute JavaScript, click unsafe links, transmit data, trigger network loads, or prove an end-to-end stored-XSS exploit under a particular host CSP. Browser script execution and host policy impact remain separate questions. Plain text columns and malformed/non-doc richtext fallback are escaped correctly. Safe basic richtext marks/headings render, and unsupported image nodes are omitted.

### Selection UI is not wired

`DataGridTable.vue:11,62` references `Checkbox`, with no corresponding import/registration in the component. Real mounts produce Vue's `Failed to resolve component: Checkbox` warning and literal `checkbox` custom elements. There is no actual checkbox input or `role="checkbox"`, and clicking a row element emits no selection callback. Tests deliberately do not globally register or stub this component, because doing so would hide the problem.

The composable selection methods themselves select/toggle numeric and string IDs, include zero, support `ui.idKey`, clear selection on successful loads and preserve it on failed loads. But `allSelected` compares only array lengths (`useDataGrid.ts:139–141`), so unrelated IDs can mark all rows selected; duplicate row IDs are retained. Missing/null/object IDs are excluded from select-all.

### Rendering and layout

- Standard number/text cells drop `0` and `false` through `if (!value) return ''` (`DataGridTable.vue:281–282`). Boolean cells and select badges with numeric zero have separate branches and work.
- An actions header appears if any row has actions, but actionless rows omit their action cell (`DataGridTable.vue:113`). Mixed-permission rows therefore have different cell counts.
- A zero-valued active filter is treated as inactive by empty-state messaging/create visibility because `DataGrid.vue:64–75` uses `.some(Boolean)`. The filter panel correctly treats zero and false as active (`DataGridFilters.vue:78–82`).

### Initialization and live state

- Initial data accepts pagination only from `meta.pagination` (`useDataGrid.ts:217`); fetched data accepts `meta.pagination` or top-level `pagination` (`:184`). Explicit empty initial arrays are valid loaded data and do not fetch.
- The declared `window.AdminDataGrid: { dataResponse: string }` wrapper (`:107`) is not unwrapped and falls back to fetching. Direct objects and JSON strings work; malformed JSON logs and fetches.
- Search uses a 500 ms debounce and filter changes 300 ms. Independent timers can issue two equivalent requests for one combined change. A pending search still runs after a later numeric page navigation and returns to page 1 (`:229–279,306–318`).
- New debounced intent invalidates older in-flight results immediately. Older successes/errors cannot overwrite the latest state; unmount cancels both debounce timers and ignores late responses. Fetches do not receive an AbortSignal.
- HTTP 401/403/server/network/JSON errors preserve previously loaded rows and selection; retries replay explicit parameters using a copied filter snapshot. First-load failure retains `hasLoaded=false`.
- Non-empty reserved filter names can overwrite built-in URL query keys because filters call `searchParams.set` after standard parameters (`:164–168`).
- Numeric pagination validates page bounds; URL pagination trusts page/sort-order values, defaults omitted limit to 10, and clears omitted sort/search/filters. It parses supplied URL parameters but still requests the configured API endpoint, not the supplied URL host/path.

### URL synchronization and captured props

- URL-only `limit` is ignored; it is excluded from filters and not a load trigger (`DataGrid.vue:209–242`).
- Without initial data, a nonempty URL query causes two mount requests: default initialization and URL initialization.
- There is no popstate response after mounting.
- URL synchronization uses `replaceState({}, '', newUrl)` and drops existing history state and hash (`:199`).
- `initialData` and `apiUrl` are read once when creating the composable (`:155–158`). Changing those props does not reinitialize; exposed `refreshData()` retains the original API URL.

### Separate unused store helper

`resources/src/js/stores/dataGrid.ts` has no non-test consumers in `resources/src/js` (verified by source search). It is not evidence that the current grid implements client export, cross-page selection, bulk operations or caching.

Tests verify per-key cache/selection isolation, strict expiration boundaries, clearing and JSON envelope export. Its CSV helper (`dataGrid.ts:57–64`) loses zero/false, uses JSON backslash escaping rather than CSV doubled quotes, leaves commas in headers unescaped, exports raw rather than computed values, and does not neutralize formula-like strings. The `=1+1` test only checks emitted text; no spreadsheet is opened.

## Nine desired expected-failure guards

1. Render accessible/operable selection checkboxes.
2. Keep numeric zero visible.
3. Escape richtext text nodes.
4. Reject javascript richtext links.
5. Keep an empty action cell for rows without actions.
6. Accept top-level initial pagination consistently with fetched data.
7. Require visible-ID membership for `allSelected`.
8. Preserve zero/false in the separate CSV helper.
9. Use CSV doubled-quote escaping in that helper (isolated from its zero/false defect).

Each has a neighboring passing test documenting the actual defective behavior, reducing the risk of an unrelated setup failure disguising itself as a valid expected failure.

## Capability boundaries and remaining gaps

- The table declares `inline-create`, and DataGrid passes a `show-inline-create` attribute, but there is no implemented inline-create control/template. Tests describe this boundary without labeling an unimplemented feature as a regression.
- No bulk-action UI, inline editor, form submission or persistence contract was invented. Row actions are callback dispatch; the external host decides routing, form, authorization, mutation and refresh behavior.
- JSDOM is not browser layout, keyboard screen-reader accessibility, scroll/sticky behavior, paint or end-to-end host behavior. Cross-browser/mobile observations, large-row rendering benchmarks and demo usability are recorded separately in `VERIFICATION.md`.
- The suite uses a mocked fetch boundary; it does not validate a deployed backend, actual downloads, server authorization or persistence.
- This report records the pre-fix characterization phase.
