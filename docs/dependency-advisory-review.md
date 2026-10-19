# Frontend dependency advisory review

Reviewed on 2026-10-19 for the Tiptap migration candidate. The baseline
`npm audit --json` reported 35 moderate dependency records, representing one
unique advisory (GHSA-cp6q-959q-f8rh), with no low/high/critical findings.
After migrating the coherent Tiptap graph from 2.27.3 to 3.32.0, the same
command reports **0 vulnerabilities at every severity**. No suppression,
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
The bounded test-tool migration now selects Vitest 4.1.11 and its compatible
transitive graph. Vue runtime is 3.5.42; compiler packages may resolve compatible
3.5.43 patches. The lockfile selects shell-quote 1.12.0 (within its existing
1.x range), source-map-js 1.2.2 and postcss-selector-parser 7.1.6. These remove
[Vue SSR](https://github.com/advisories/GHSA-g2v6-rqmx-r4w6),
[shell-quote](https://github.com/advisories/GHSA-pqg4-j6r4-53mv),
[source-map-js](https://github.com/advisories/GHSA-68fv-2mgg-jv7q) and
[selector parsing](https://github.com/advisories/GHSA-rj75-hqrm-r3gf) advisory paths.
Vitest 4 removes Tinypool rather than overriding its incompatible major.
Vitest/mocker 4.1.11 addresses
[GHSA-82fw-gwwq-j7x9](https://github.com/advisories/GHSA-82fw-gwwq-j7x9).
That earlier cleanup used no editor or frontend framework major, `npm audit fix --force`, override,
advisory ignore, dependency exclusion or severity reduction is used.

## Tiptap advisory removed

The [official advisory](https://github.com/advisories/GHSA-cp6q-959q-f8rh)
lists versions below 3.30.4 as affected and 3.30.4 as patched. The candidate
pins every directly declared Tiptap package to 3.32.0, including an explicit
core dependency for our custom extensions. The lockfile contains one coherent
3.32.0 graph and no Tiptap 2 packages. The existing fixed-schema media tests
remain useful defense in depth; the upstream upgrade removes the vulnerable
helper, rather than claiming those tests fixed it.

See [the migration contract](tiptap3-migration.md) for API/default changes,
host extension compatibility and validation scope.

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
