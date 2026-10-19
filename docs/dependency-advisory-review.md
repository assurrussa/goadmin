# Frontend dependency advisory review

Reviewed on 2026-10-19 for the next release. The current
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

## Remaining audit records: verified Tiptap 2 backport

The unchanged lockfile selects `@tiptap/core` **2.27.3**, which already contains
the upstream fix for
[GHSA-cp6q-959q-f8rh](https://github.com/advisories/GHSA-cp6q-959q-f8rh).
The previous statement that this installed helper remained vulnerable and
required a major upgrade was incorrect.

Upstream merged the [v2 backport #8301](https://github.com/ueberdosis/tiptap/pull/8301)
on 2026-09-03 and included it in [the 2.27.3 release PR #8310](https://github.com/ueberdosis/tiptap/pull/8310).
The immutable [2.27.3 changelog](https://github.com/ueberdosis/tiptap/blob/0ed4a30a6cda23799a01881f67d01ad218a2ac6d/packages/core/CHANGELOG.md#L3-L7)
and [helper implementation](https://github.com/ueberdosis/tiptap/blob/0ed4a30a6cda23799a01881f67d01ad218a2ac6d/packages/core/src/utilities/mergeAttributes.ts#L7-L18)
show the fix: an own JSON `__proto__` key is copied with `Object.defineProperty`
as an ordinary data property, preserving `Object.prototype` rather than invoking
its legacy setter. It can remain an inert own key in serialized attributes;
the helper is not a general sanitizer of explicitly supplied event handlers.

A fresh official npm tarball for 2.27.3 matches the unchanged lockfile integrity:

```text
sha512-a5LfRbLpfaGI3hbL/LPHUYHI0I+FQHdSHsy8L4YnVIuu3hXcm3QZgkWbpEGf8hCz8krk6zEiu0+iFOjTySU2FA==
```

The advisory metadata lists only the v3 patched floor (3.30.4), omitting this
v2 backport; [upstream issue #8339](https://github.com/ueberdosis/tiptap/issues/8339)
tracks that discrepancy. The unsuppressed `npm audit --audit-level=high --json`
still reports **35 moderate records representing this one advisory**, with zero
high/critical findings. Those records remain visible; neither their severity
nor audit configuration has been changed. This package/source verification
addresses the specific prototype-mutation flaw, not a claim that the graph is
globally vulnerability-free or that arbitrary host extensions are safe.

Retain Tiptap 2.27.3 for this correction. [GoAdmin PR #54](https://github.com/assurrussa/goadmin/pull/54)
is an optional, unmerged major upgrade with separate compatibility implications;
it is not required solely to obtain this backport.

## Verification scope

Eight representative effective ESLint configurations (Vue, TS, MTS, TSX,
JavaScript, unit tests and browser tests) were serialized before and after the
wrapper replacement and matched exactly. New `node --test` regressions exercise
lint policy, nested image aliases, output bytes, configured output roots,
collisions, symlinks, encoded traversal, Vite deny rules, GET/HEAD and conditional
requests, source additions/changes/deletions, and recovery after invalid source
files are removed. These tests run in the ordinary `test:manifest` gate.

The existing eight `mediaAttributeSecurity.test.ts` cases protect first-party
fixed schemas; they do not exercise arbitrary attribute maps directly. Six
`mergeAttributesSecurity.test.ts` regressions now complement them on unchanged
2.27.3. They merge hostile JSON maps in first/middle/last positions, verify the
own data-property descriptor and unchanged prototype through repeated merges,
and serialize a host-like custom Node through ProseMirror DOMSerializer and
editor HTML output. Inherited `onerror`, `onclick`, attacker `src` and canary
attributes must not escape; legitimate attributes and custom JSON content remain.
This verifies the already-shipped upstream fix rather than repairing it locally.

For this documentation/test-only correction, the focused 14 tests (six direct
helper/custom-extension and eight media-schema cases), focused ESLint/Prettier,
vue-tsc and typed-host checks pass. Dependencies, runtime and generated assets
are unchanged; the unchanged broad build/backend suites are not rerun.

The earlier Vitest 4.1.11 migration passed the complete resource gate: 48
manifest tests, 40 unit files and 519 unit tests, followed by lint, formatting,
type and build checks. The DataGrid contract retains its 15-second deadline
and assertions. The earlier full-suite timeout remains historical failed
evidence; that success does not establish its cause or convert the earlier
publish-readiness wrapper to PASS.

The conclusions apply to the reviewed first-party source and lockfile. They do
not constitute an independent security audit or extend to a host's custom
frontend plugins, schemas, or exposed development services.
