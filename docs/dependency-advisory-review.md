# Frontend dependency advisory review

This review accompanies the v0.7.0 release preparation. The actual
`npm audit --audit-level=high --json` command completed with exit status 0:
0 high, 0 critical and 38 moderate dependency records. Those records represent
three unique advisories, not 38 independent vulnerabilities. Passing that gate
does not mean that the dependency tree is vulnerability-free.

| Advisory | Installed version and exposure | Review and remaining limit |
| --- | --- | --- |
| [GHSA-cp6q-959q-f8rh](https://github.com/advisories/GHSA-cp6q-959q-f8rh) | `@tiptap/core` 2.27.3; 35 records including affected dependants; browser editor runtime | The upstream `mergeAttributes` helper remains affected. Exploitation requires an untrusted own `__proto__` attribute key to reach that helper and then a consumer that renders inherited attributes. The current editor's attribute names are fixed by its ProseMirror schemas; extension HTML options are application constants. Eight regression cases exercise the real image/video/CMS schemas and link marks with imported JSON, content updates, imported HTML and the video insertion command. They preserve the intended nodes while rejecting executable/prototype attributes in serialization. These checks do not certify arbitrary custom host extensions or direct calls to the upstream helper. A future extension that forwards arbitrary attribute maps must validate them or use a patched upstream version. |
| [GHSA-253c-mchw-3w2r](https://github.com/advisories/GHSA-253c-mchw-3w2r) | `markdown-it` 14.3.0, one record; installed through `@tiptap/pm` → `prosemirror-markdown` | The issue requires `linkify: true` and sufficiently large crafted Markdown. First-party editor source consumes HTML/JSON and does not import the Markdown parser. The dependency's default Markdown parser uses the commonmark preset without enabling linkify. No affected first-party call was found. This is a source reachability review, not a load-test claim or a guarantee for host extensions that enable Markdown linkification. Upstream patch 14.3.1 is available for a future focused dependency update. |
| [GHSA-82fw-gwwq-j7x9](https://github.com/advisories/GHSA-82fw-gwwq-j7x9) | `vitest` / `@vitest/mocker` 3.2.7, two records; development test tooling | The unauthenticated path requires registering standalone mocker/interceptor plugins on a reachable development WebSocket. The repository uses `vitest run` with jsdom; its Vite and Vitest configurations do not register these plugins or enable Vitest browser mode. It is outside the embedded admin runtime. Do not introduce or expose standalone mocker endpoints without addressing the advisory; patched upstream versions begin at 4.1.11 and require a major test-tool upgrade from this line. |

Validation: `npm run test:unit --
src/js/components/form/extensions/mediaAttributeSecurity.test.ts` passed all
eight tests, with no skips. No bulk lockfile update or advisory suppression was
used. Runtime code and the embedded frontend assets are unchanged by this
review; the new regression test protects the fixed attribute boundary.

The conclusions apply to the reviewed first-party source and lockfile. They do
not constitute an independent security audit or extend to a host's custom
frontend plugins, schemas, or exposed development services.
