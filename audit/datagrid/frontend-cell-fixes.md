# DataGrid table safety and UI fixes

Date: 2026-10-02. Historical baseline: `7d51c83`; characterization/demo commit:
`49e8746`. This report describes the approved working-tree repairs, not a release
or deployed result. The original `frontend-report.md` remains historical evidence.

## Production changes

- `DataGridTable.vue` imports the actual Checkbox primitive and binds its
  `modelValue` / `update:modelValue` API. This was verified against the installed
  Reka UI v2 `CheckboxRoot.vue`; merely importing the primitive while preserving
  the old `checked` event would not fix selection.
- Standard cells preserve `0` and `false`; only null, undefined and empty string
  render empty.
- When any row has actions, every row has an action cell. Actionless cells stay
  empty, including after live action changes. Existing row action dispatch and
  raw-row payloads are unchanged.
- Sortable headings contain native, focusable `type="button"` controls with
  visible focus styling. The currently sorted column has `aria-sort` set to
  `ascending` or `descending`; non-sortable headings do not become buttons.
  There are no duplicate synthetic keyboard handlers.
- Richtext rendering moves to the adjacent `richTextPreview.ts`, preserving the
  existing supported formatting and fixed tag set. Text and accepted href values
  are escaped before reaching `v-html`. Malformed JSON, malformed structures,
  and unexpected types cannot fall back to raw HTML.
- Heading levels must be numeric integers 1–6, otherwise the fixed tag is `h1`.
  Unsupported image nodes remain omitted. No new richtext features were added.

## Link policy

The preview keeps relative paths, query strings and fragments, explicit valid
HTTP/HTTPS URLs, and non-empty mailto/tel destinations. Unsupported schemes,
control characters, backslashes, protocol-relative references, ambiguous
HTTP(S) forms, and colons (including percent-encoded colons) in the initial
segment of an otherwise relative path are rejected. An unsafe link loses its
anchor but retains its safely formatted label. URL checks and HTML attribute
escaping are separate protections; an allowed URL with quotes or ampersands
still cannot add an attribute.

## Verification

From `resources/`:

```sh
./node_modules/.bin/prettier --write \
  src/js/components/datagrid/DataGridTable.vue \
  src/js/components/datagrid/richTextPreview.ts \
  src/js/components/datagrid/DataGridTable.safety.test.ts \
  src/js/components/datagrid/DataGrid.contracts.test.ts
./node_modules/.bin/eslint \
  src/js/components/datagrid/DataGridTable.vue \
  src/js/components/datagrid/richTextPreview.ts \
  src/js/components/datagrid/DataGridTable.safety.test.ts \
  src/js/components/datagrid/DataGrid.contracts.test.ts
./node_modules/.bin/vitest run \
  src/js/components/datagrid/DataGridTable.safety.test.ts \
  src/js/components/datagrid/DataGrid.contracts.test.ts --reporter=dot
```

Formatting and targeted lint pass. Final focused run: **150/150 cases pass** in
two files (65 existing/extended contracts, 85 focused safety/UI cases). The five
table expected-failure guards are normal assertions now, and all paired defect
characterizations were updated to assert the repaired behavior. None were
deleted or weakened. Request-state coverage is described in `frontend-state-fixes.md`.

Coverage includes:

- Harmless literal HTML-shaped text, quote/entity attribute breakout negatives,
  mixed-case/control-character URL schemes, supported links, malformed document
  shapes, malformed headings, and safe fallback output.
- Real Reka controls, exact callback counts/payloads, controlled checked-state
  changes, custom string/zero IDs and full-grid selection with no network request.
- Actual sort-button clicks through the full grid, one request per activation,
  ascending/descending state and `aria-sort` updates, focus and native semantics.
- Real Reka Enter suppression for checkbox controls (per its ARIA behavior).
- Stable row and checkbox DOM identities through reordering/value changes, live
  action-column alignment, and three repeated mount/interact/unmount cycles of
  ten selectable rows. No 1,000-row JSDOM workload was added to unit tests.

Payload tests only parse/assert harmless DOM; they do not activate links, execute
scripts, or initiate network loads. Independent review also ran 21 supplementary
Node/JSDOM security probes with no security blocker found.

## Explicit limits

- Native browser Enter/Space activation and visual/assistive-technology behavior
  are **NOT RUN** here. JSDOM `.click()` and focus assertions are not represented
  as native keyboard proof. Test-browser navigation was blocked; no alternate
  browser route was used to bypass that block.
- Full repository gates, aggregate type checking/build and bounded performance
  results are recorded in `STAGE1-VERIFICATION.md`.
