# Frontend dependency advisory review

Reviewed on 2026-10-03 for the next release. The current
`npm audit --audit-level=high --json` command completes with exit status 0:
0 high, 0 critical and 37 moderate dependency records. Those records represent
two unique advisories, not 37 independent vulnerabilities. Passing this gate
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
  Symlinks and ambiguous output collisions now fail explicitly instead of
  allowing out-of-root traversal or order-dependent overwrites. Add new images
  under the same source directory; no per-image configuration is required.
- Remove unused `ts-loader` and `vue-loader`. This project builds with Vite's
  Vue plugin and checks types with `vue-tsc`; it has no webpack build pipeline.

`markdown-it` moves from 14.3.0 to 14.3.2 within its declared compatible range,
removing [GHSA-253c-mchw-3w2r](https://github.com/advisories/GHSA-253c-mchw-3w2r).
The other retained package versions are unchanged. No major upgrade,
`npm audit fix --force`, override, advisory ignore or severity reduction is used.

## Remaining moderate advisories

| Advisory | Installed version and exposure | Review and remaining limit |
| --- | --- | --- |
| [GHSA-cp6q-959q-f8rh](https://github.com/advisories/GHSA-cp6q-959q-f8rh) | `@tiptap/core` 2.27.3; 35 records including affected dependants; browser editor runtime | The upstream `mergeAttributes` helper remains affected. Exploitation requires an untrusted own `__proto__` attribute key to reach that helper and then a consumer that renders inherited attributes. The current editor's attribute names are fixed by its ProseMirror schemas; extension HTML options are application constants. Eight regression cases exercise the real image/video/CMS schemas and link marks with imported JSON, content updates, imported HTML and the video insertion command. They preserve the intended nodes while rejecting executable/prototype attributes in serialization. These checks do not certify arbitrary custom host extensions or direct calls to the upstream helper. A future extension that forwards arbitrary attribute maps must validate them or use a patched upstream version. The official patched release begins at 3.30.4; there is no listed 2.x fix. Removing this finding requires a separately reviewed Tiptap 2→3 editor migration. This is production editor code, not development-only tooling. |
| [GHSA-82fw-gwwq-j7x9](https://github.com/advisories/GHSA-82fw-gwwq-j7x9) | `vitest` / `@vitest/mocker` 3.2.7, two records; development test tooling | The unauthenticated path requires registering standalone mocker/interceptor plugins on a reachable development WebSocket. The repository uses `vitest run` with jsdom; its Vite and Vitest configurations do not register these plugins or enable Vitest browser mode. It is outside the embedded admin runtime. Do not introduce or expose standalone mocker endpoints without addressing the advisory; patched upstream versions begin at 4.1.11 and require a major test-tool upgrade from this line. |

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
Tiptap helper or certify arbitrary host extensions. A patched Tiptap major and
a patched Vitest major remain separate migration work, not completed fixes.

The conclusions apply to the reviewed first-party source and lockfile. They do
not constitute an independent security audit or extend to a host's custom
frontend plugins, schemas, or exposed development services.
