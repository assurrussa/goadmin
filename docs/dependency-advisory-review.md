# Frontend dependency advisory review

Reviewed on 2026-10-19 for the Tiptap migration candidate and rechecked on
2026-10-22 for the GoAdmin v0.20.0 candidate. The baseline
`npm audit --json` reported 35 moderate dependency records, representing one
unique advisory (GHSA-cp6q-959q-f8rh), with no low/high/critical findings.
After migrating the coherent Tiptap graph from 2.27.3 to 3.32.0, the same
command reports **0 vulnerabilities at every severity** after the Vitest patch
described below. No suppression,
severity reduction, forced upgrade or incompatible override is used.

## Removed advisory paths

[GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm)
affects every published `braces` version through 3.0.3; no patched release was
listed when reviewed. The previous lockfile already used 3.0.3. Its seven high
records came from these development-tool chains:

- `vite-plugin-static-copy` → `chokidar` → `braces`
- `@vue/eslint-config-typescript` → `fast-glob` → `micromatch` → `braces`
- `ts-loader` → `micromatch` → `braces`

The current graph removes these paths rather than suppressing the advisory:

- Use the already-locked `typescript-eslint` and `vue-eslint-parser` directly,
  following the [official Vue ESLint configuration](https://eslint.vuejs.org/user-guide/#example-configuration-with-typescript-eslint-and-prettier).
  The Vue essential/TypeScript recommended presets, parsers, rule severities,
  ignores and test-plugin scopes are preserved. This is syntax-only linting;
  the existing `vue-tsc` gate remains responsible for type checking.
- Replace the static-copy dependency with a local Vite image-asset plugin.
  It discovers ordinary files only below `resources/src/images`, including
  hidden files and all nested/flattened aliases emitted by the former glob.
  `favicon.ico` remains available both at the output root and below `images/`.
  Development aliases delegate serving to Vite, retaining its filesystem
  restrictions, MIME handling, conditional requests and HEAD behavior.
  Symlinks inside the image tree and ambiguous output collisions now fail
  explicitly instead of allowing out-of-root traversal or order-dependent
  overwrites. Add new images
  under the same source directory; no per-image configuration is required.
- Remove unused `ts-loader` and `vue-loader`. This project builds with Vite's
  Vue plugin and checks types with `vue-tsc`; it has no webpack build pipeline.

`markdown-it` moves from 14.3.0 to 14.3.2 within its declared compatible range,
removing [GHSA-253c-mchw-3w2r](https://github.com/advisories/GHSA-253c-mchw-3w2r).
The bounded test-tool migration initially selected Vitest 4.1.11. For the
v0.20.0 candidate, Vitest and its matching development-only @vitest packages
move to 4.1.12. This removes two subsequently reported moderate advisories
in @vitest/mocker,
[GHSA-qxg6-89mg-rqv8](https://github.com/advisories/GHSA-qxg6-89mg-rqv8)
and [GHSA-xp26-3454-mjgp](https://github.com/advisories/GHSA-xp26-3454-mjgp).
The 2026-10-22 full `npm audit --json` reports zero vulnerabilities at every
severity; `npm audit --omit=dev --audit-level=low` also reports zero production
dependency vulnerabilities. Vue runtime is 3.5.42; compiler packages may resolve compatible
3.5.43 patches. The lockfile selects shell-quote 1.12.0 (within its existing
1.x range), source-map-js 1.2.2 and postcss-selector-parser 7.1.6. These remove
[Vue SSR](https://github.com/advisories/GHSA-g2v6-rqmx-r4w6),
[shell-quote](https://github.com/advisories/GHSA-pqg4-j6r4-53mv),
[source-map-js](https://github.com/advisories/GHSA-68fv-2mgg-jv7q) and
[selector parsing](https://github.com/advisories/GHSA-rj75-hqrm-r3gf) advisory paths.
Vitest 4 removes Tinypool rather than overriding its incompatible major.
Vitest/mocker 4.1.11 addressed
[GHSA-82fw-gwwq-j7x9](https://github.com/advisories/GHSA-82fw-gwwq-j7x9).
That earlier cleanup used no editor or frontend framework major, `npm audit fix --force`, override,
advisory ignore, dependency exclusion or severity reduction is used.

## Tiptap audit metadata and chosen v3 migration

The [official advisory](https://github.com/advisories/GHSA-cp6q-959q-f8rh)
lists the v3 patched floor as 3.30.4 but omits the v2 backport. The baseline
2.27.3 helper already contains the fix, merged in
[upstream #8301](https://github.com/ueberdosis/tiptap/pull/8301) and included in
[release #8310](https://github.com/ueberdosis/tiptap/pull/8310).
See the immutable [changelog](https://github.com/ueberdosis/tiptap/blob/0ed4a30a6cda23799a01881f67d01ad218a2ac6d/packages/core/CHANGELOG.md#L3-L7)
and [helper](https://github.com/ueberdosis/tiptap/blob/0ed4a30a6cda23799a01881f67d01ad218a2ac6d/packages/core/src/utilities/mergeAttributes.ts#L7-L18),
plus [metadata issue #8339](https://github.com/ueberdosis/tiptap/issues/8339).
A fresh official 2.27.3 tarball matched the baseline lockfile SHA-512
`a5LfRbLpfaGI3hbL/LPHUYHI0I+FQHdSHsy8L4YnVIuu3hXcm3QZgkWbpEGf8hCz8krk6zEiu0+iFOjTySU2FA==`;
its published helper defines __proto__ as an own data property rather than
mutating the result's prototype. The earlier assertion that the installed
baseline helper remained vulnerable was incorrect.

The project chooses Tiptap 3 as a planned development upgrade, not as a mandatory
repair for this advisory. The candidate pins all directly declared packages to
3.32.0, including explicit core for our extensions, with one coherent graph.
The baseline audit reported 35 moderate records for this single advisory;
the v3 graph reports 0 findings. No audit suppression, severity adjustment,
forced upgrade or override is used. A clean audit is not a blanket security
certification of arbitrary host extensions or the whole application.

See [the migration contract](tiptap3-migration.md) for API/default changes,
normalization, keyboard behavior, host extension compatibility and validation
scope. PR #55 preserves the separate minimal-v2 evidence; this candidate
intentionally follows the selected v3 path.

## Verification scope

Eight representative effective ESLint configurations (Vue, TS, MTS, TSX,
JavaScript, unit tests and browser tests) were serialized before and after the
wrapper replacement and matched exactly. New `node --test` regressions exercise
lint policy, nested image aliases, output bytes, configured output roots,
collisions, symlinks, encoded traversal, Vite deny rules, GET/HEAD and conditional
requests, source additions/changes/deletions, and recovery after invalid source
files are removed. These tests run in the ordinary `test:manifest` gate.

The eight `mediaAttributeSecurity.test.ts` cases still protect the first-party
fixed-schema boundary. Additional Tiptap 3 regressions exercise the patched
helper with own JSON `__proto__`, a custom extension and ProseMirror DOMSerializer,
formatting, links, tables, task lists, all media nodes, JSON round-trip,
controlled updates, invalid-content rejection and undo/redo. The mounted editor
also checks allowlists and duplicate extension names. Historical Vitest 4
migration results above do not substitute for the current candidate gates.

The conclusions apply to the reviewed first-party source and lockfile. They do
not constitute an independent security audit or extend to a host's custom
frontend plugins, schemas, or exposed development services.

## 2026-10-28 reassessment — qs 6.16.1

The earlier zero-finding statements above record their dated audits, not a
permanent claim about the dependency graph. A fresh audit of the merged
session-maintenance tree reported **one moderate finding**, with no
low/high/critical findings:
[GHSA-6pq8-jhm2-2vcw](https://github.com/advisories/GHSA-6pq8-jhm2-2vcw).
It affects `qs` 6.7.0 through 6.16.0: comma-array serialization can throw an
uncaught TypeError for an element with a non-callable own `toString` property.
The first-party client does not select this comma-array mode; this limits the
identified path, but is not a reason to suppress the advisory or postpone an
available compatible patch.

Only the locked transitive `qs` moves from 6.16.0 to **6.16.1**, within
`@inertiajs/core`'s existing `^6.15.0` range. Its version, official registry
tarball URL and SHA-512 integrity are the only lockfile fields changed.
Direct dependencies and Inertia remain unchanged. No override, forced/broad
upgrade, exclusion or audit suppression is used. The official registry already
also offered 6.17.0; that broader update was not selected.

A clean isolated `npm ci` installed the patched lockfile with Node.js 24.19.0
and npm 11.9.0. Fresh full `npm audit --json` and production
`npm audit --omit=dev --json` both report **zero vulnerabilities at every
severity**. A focused check of the advisory's comma-array shape completes
without throwing and ordinary comma-array serialization remains intact.
The embedded production build changes only `public/dist/js/app.js`; asset
names, manifests and CSS remain unchanged. Current candidate frontend gate
results are recorded below. These checks do not establish full release
readiness, host acceptance or deployment, and do not replace final candidate
publish-readiness and exact-tag release-readiness gates.

Final frontend verification on the patched tree passed: 74 unit files / 894
unit tests; 131 source-manifest tests (including default/topbar production
builds); read-only ESLint and Prettier checks; `vue-tsc` plus the complete typed
host positive/negative page and theme checks; production build and embedded
asset verification. Initial isolation attempts lacked parent Go source files
and later overlapping heavy fixture processes exited without a numeric status;
those diagnostics were retained, the isolated checkout context was completed,
and manifest/type checks passed when rerun sequentially. No failed or incomplete
attempt is counted as a pass.
