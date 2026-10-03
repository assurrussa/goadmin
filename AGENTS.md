# Repository Guidelines

## Purpose

`goadmin` is the standalone embedded admin library published as
`github.com/assurrussa/goadmin`. Treat it as a reusable admin subsystem with a
narrow public host integration surface, not as a source folder to copy into a
host application.

The compact project overlay is in `docs/project-context.md`. Read it before
changing public contracts, release gates, migrations, feature packages, or host
integration code.

## Source Order

Local verified files win over older shared notes:

1. `AGENTS.md`, `README.md`, `RELEASING.md`, and `docs/project-context.md`.
2. `reference/externalconsumer`, public-surface tests, `host/`,
   `migrations/`, `features/*`, `toolkit/*`, and command helpers.
3. Go tests, resource build config, generated contracts, and host import-policy
   checks.
4. Shared agent-context wiki pages listed below.

If shared wiki content conflicts with current repo docs or code, treat the wiki
as stale and update the matching platform page only after local verification.

## Project Map

- `host/`: stable embedding facade, host config mapping, extension API, runtime
  factories, public assets/templates accessors, and install validation.
- `hosttest/`: stable host-facing test harness.
- `migrations/`: core goadmin storage migrations.
- `features/operations`: reusable operations admin feature.
- `features/users`: optional users-management feature.
- `toolkit/datagrid`, `toolkit/formvalidator`: stable host-support packages.
- `resources/`: embedded Vue admin UI source and build tooling.
- `public/`, `views/`, `templates/`: embedded runtime assets and templates.
- `bootstrap/`, `adminapp/`, `di/`, `domain/`, `http/`, `infrastructure/`,
  `outbox/`, `seeders/`, `services/`, `shared/`: goadmin-owned internals, not
  host SDK.
- `reference/externalconsumer`: compile-checked supported package manifest.
- `cmd/externalconsumerprobe`, `cmd/importpolicy`: release and boundary helpers.

## Public Surface Rules

Use `reference/externalconsumer.SupportedPackages` as the source of truth for
published support. Host projects should integrate only through:

- `github.com/assurrussa/goadmin/host`
- `github.com/assurrussa/goadmin/migrations`
- `github.com/assurrussa/goadmin/hosttest`
- `github.com/assurrussa/goadmin/features/operations`
- `github.com/assurrussa/goadmin/features/access`
- `github.com/assurrussa/goadmin/features/jobs`
- `github.com/assurrussa/goadmin/features/uploads`
- `github.com/assurrussa/goadmin/features/queues`
- `github.com/assurrussa/goadmin/features/notifications`
- `github.com/assurrussa/goadmin/features/realtime`
- `github.com/assurrussa/goadmin/features/authmail`
- `github.com/assurrussa/goadmin/features/users`
- `github.com/assurrussa/goadmin/toolkit/datagrid`
- `github.com/assurrussa/goadmin/toolkit/formvalidator`

Do not promote exported internal packages to stable SDK just because they are
importable. Do not move built-in admin auth, sessions, handlers, seeders, roles,
permissions, templates, assets, or runtime wiring into a host project unless the
external-consumer manifest explicitly says that surface is supported.

## Host Integration Rules

`host.NewAuthAdapter` owns the goauth Runtime, admin membership gate, realms,
and RBAC wiring. `host.New` assembles the core and explicitly requested modules.
Hosts supply typed core config and PostgreSQL; module constructors receive
optional infrastructure. Runtime owns its workers; borrowed clients/workers
retain host lifecycle.

Use `host.New` with typed `Config`, `Dependencies`, and explicit modules; its
preflight rejects missing module dependencies and conflicting route claims.
The default canonical admin session backend is PostgreSQL; Redis is explicit.
Keep backend config, cookie policy and owner/version fencing aligned.

Use `goadmin/migrations.Migrate`; it installs canonical goauth v0.2 storage
before the complete goadmin host schema. Legacy v0.1 state requires the typed,
explicit development/test reset path.

## Commands

- Full local run: `make`
- Mutating preparation only: `make prepare`
- Verification only: `make check` — one Go race+coverage pass plus the admin
  resource gates; candidate consumer validation is explicit.
- Explicit Go stress rerun: `make test-race`.
- Explicit HTML coverage artifact: `make cover-html`.
- Release-only dependency advisory gate: `make resources-audit`.
- Published release readiness:
  `make release-readiness VERSION=<published-tag>`
- Local clean-consumer probe: `make externalconsumer-local`
- Explicit candidate consumer:
  `make externalconsumer-candidates GOAUTH_LOCAL_PATH=../goauth GONOTIFY_LOCAL_PATH=../gonotify`
- Published clean-consumer probe:
  `make externalconsumer-published VERSION=<published-tag>`
- Host import policy against the site repo:
  `make import-policy-site SITE_REPO=../site`

The Makefile uses shared Go build/module/toolchain/lint caches outside the
checkout, rooted at `$(HOME)/dev/projects/.cache/go`. Override
`GO_SHARED_CACHE_ROOT` or individual cache variables for a specific check.
Existing GOPATH and toolchain settings are preserved. Sandbox restrictions
require permitted shared-cache access; do not create per-checkout cache copies.

The default full run includes `resources` because the embedded
admin UI is part of the library runtime. Online npm advisory lookup is kept out
of the normal development loop but remains mandatory in publish/release gates.

During implementation, prefer package-scoped Go tests and the narrow resource
test that matches the changed surface. Run `make check` once after a coherent
batch; do not repeat `test`, `test-race`, or `cover-html` on the same tree.

## Release Notes

Do not rewrite existing tags. If `make` changes generated Go files, admin UI
outputs, `go.mod`, or `go.sum`, commit those changes and publish a new semver
tag. The clean-history baseline is `v0.7.0`; old tags remain in the private
history archive. The root `go.mod` pins GoAuth v0.5.1 and GoNotify v0.6.0 without local
replacements. The nested starter's overrides belong to explicit source mode.
Verify remote tags and dependency availability before claiming public readiness.

External readiness is not proven by local `replace` checks alone. Use the explicit
candidate consumer probe for checkout confidence, then the published probe after
real tags resolve from a clean consumer.

## Documentation Rules

Keep `AGENTS.md` concise. Put durable project details, invariants, and boundary
notes in `docs/project-context.md`. Keep task-specific decisions and tradeoffs
in `docs/implementation-notes.md`.

When changing the supported public package list, update
`reference/externalconsumer`, `README.md`, `RELEASING.md`, and
`docs/project-context.md` together, then run the public-surface tests.

## Shared Agent Context

Use `$project-context-router` when a task needs cross-project context
from a local shared wiki.

Do not hard-code machine-local absolute paths in this public repository.
If a local shared wiki is available, expose its root through
`AGENT_CONTEXT_ROOT` or let `$project-context-router` resolve it for the
current session.

Local docs and code in this repository remain the source of truth for
commands, public APIs, config keys, supported imports, runtime behavior,
and release gates. Read this repo's `AGENTS.md`, `README.md`, `docs/`
or `reference/`, task files, code, tests, and configs before shared wiki
pages.

When shared context is needed, follow `streams/AGENTS.md` and its query route.
Reuse already loaded root rules, PII policy and glossary. Open the known hub
and only the topic relevant to the task:

- `streams/wiki/platforms/goadmin.md`

For integration work, open only the affected neighbour hub:

- `streams/wiki/platforms/goauth.md`

Use `streams/wiki/index.md` only to locate an unknown area or answer an overview
question. This is a task router, not a mandatory list of wiki pages.

If local verified docs/code conflict with the shared wiki, treat the wiki
as stale. When documentation upkeep is in scope, update the relevant
platform page after verification. Do not
copy whole README files into wiki; keep shared pages concise and
contract-focused.
