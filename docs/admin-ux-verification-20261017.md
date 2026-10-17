# Admin UX verification — 2026-10-17

Base: `28b109703dde8cfecd274e1a4f55eb4a2068e730`.
Scope: recovery UI, local MFA capability/enrollment/errors, read-only role
preview, responsive sidebar and extensible Russian/English interface.

All command execution below used the serial Sol low lane, Go 1.27.2, Node
24.19.0, shared caches and the repository-pinned formatting/lint tools. Review
and implementation stayed within Sol medium. No workflow was dispatched.

## Go checks

The Go prefix of `make check` passed at `66ceaec`: tidy, formatting, vet, zero
lint findings and race/coverage tests across `./...`. Review subsequently added
presentation-only bootstrap/host changes. Their final delta passed
`go test -race ./bootstrap ./host` and
`golangci-lint run ./bootstrap ./host --timeout=3m` (zero findings).
The permission engine and remaining Go inputs were unchanged by that delta.
The default GoAdmin gate does not run tests behind the integration build tag;
canonical PostgreSQL consumer integration is a separate AuthHub gate.

## Client checks

`npm run test:manifest` passed 82 tests. The full unit run was followed by
repairs to English-locale fixtures and targeted reruns: the upload/editor/i18n
suites passed 77 tests, and the detail-action fixture passed two tests. All
reported failing cases were rerun; this is incremental evidence, not a claim
that one uninterrupted `make check` invocation passed. Final source passed
ESLint, Prettier, type/host-type checks and `npm run build-only`, including the
embedded asset verifier. Generated runtime assets are committed separately.

The tests cover unknown outcomes without replay, ephemeral MFA secret cleanup,
conditional enrollment code fields, typed MFA errors, both-direction locale
updates, preservation of consumer validation rules, meaningful collapsed
navigation and fresh selected-role action visibility.

## Review and browser evidence

Independent source review found no unresolved actionable findings at
`2d3f9deddf3fd2a1293d34aa9b17f0f6e5a71461` plus the reviewed source changes.
Source-only dirty manifest SHA-256:
`f9c628a0af47b8b03e27520bc82b4d858bdf796c29ea527e79a8157b05bbecbf`.
A later test-only props-variable change resolved a Vue lint false positive;
its two assertions and ESLint were rerun successfully.

Chromium passed 16 checks against actual Vue components with synthetic
Inertia props: desktop width/persistence, nested keyboard controls, language
persistence, unchanged arbitrary input, mobile close/focus/inert behavior and
horizontal overflow. This UI-only fixture does not prove authentication or
server integration. Reproduce with `resources/scripts/ui-browser-fixture.mjs`
and `resources/scripts/check-ui-browser.py`.

Raw logs and per-input hash manifests are retained in the cloud evidence
bundle `checks-20261017`. The final bundle/source manifest is
`goadmin-final-evidence.json`.

Publication is limited to branches/PRs. Merge, release, deployment and
production acceptance are outside this change.
