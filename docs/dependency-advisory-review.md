# Frontend dependency advisory review

Reviewed on 2026-10-10 for the next release. The current
`npm audit --audit-level=high --json` command completes with exit status 0:
0 high, 0 critical and 35 moderate dependency records. Those records represent
one unique advisory, not 35 independent vulnerabilities. Passing this gate
does not mean that the dependency tree is vulnerability-free.

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
No editor or frontend framework major, `npm audit fix --force`, override,
advisory ignore, dependency exclusion or severity reduction is used.

## Remaining moderate advisories

| Advisory | Installed version and exposure | Review and remaining limit |
| --- | --- | --- |
| [GHSA-cp6q-959q-f8rh](https://github.com/advisories/GHSA-cp6q-959q-f8rh) | `@tiptap/core` 2.27.3; 35 records including affected dependants; browser editor runtime | The upstream `mergeAttributes` helper remains affected. Exploitation requires an untrusted own `__proto__` attribute key to reach that helper and then a consumer that renders inherited attributes. The current editor's attribute names are fixed by its ProseMirror schemas; extension HTML options are application constants. Eight regression cases exercise the real image/video/CMS schemas and link marks with imported JSON, content updates, imported HTML and the video insertion command. They preserve the intended nodes while rejecting executable/prototype attributes in serialization. These checks do not certify arbitrary custom host extensions or direct calls to the upstream helper. A future extension that forwards arbitrary attribute maps must validate them or use a patched upstream version. The official patched release begins at 3.30.4; there is no listed 2.x fix. Removing this finding requires a separately reviewed Tiptap 2→3 editor migration. This is production editor code, not development-only tooling. |

## Verification scope

Eight representative effective ESLint configurations (Vue, TS, MTS, TSX,
JavaScript, unit tests and browser tests) were serialized before and after the
wrapper replacement and matched exactly. New `node --test` regressions exercise
lint policy, nested image aliases, output bytes, configured output roots,
collisions, symlinks, encoded traversal, Vite deny rules, GET/HEAD and conditional
requests, source additions/changes/deletions, and recovery after invalid source
files are removed. These tests run in the ordinary `test:manifest` gate.

The existing eight `mediaAttributeSecurity.test.ts` cases protect the first-party
editor's fixed attribute boundary. They do not repair the vulnerable upstream
Tiptap helper or certify arbitrary host extensions. A patched Tiptap major remains separate migration work. The Vitest 4.1.11
migration passes the complete resource gate: 48 manifest tests, 40 unit files
and 519 unit tests, followed by lint, formatting, type and build checks. The
DataGrid contract retains its 15-second deadline and assertions. The earlier
full-suite timeout remains historical failed evidence; this success does not
establish its cause or convert the earlier publish-readiness wrapper to PASS.

The conclusions apply to the reviewed first-party source and lockfile. They do
not constitute an independent security audit or extend to a host's custom
frontend plugins, schemas, or exposed development services.
