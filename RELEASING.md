# Releasing goadmin

This module is published as `github.com/assurrussa/goadmin`.

## Current Baseline

- Go 1.27 is the minimum. Modular host assembly is a breaking source API:
  migrate hosts to `host.New`, explicit modules and `host.AdminConfig`.
- GoAuth v0.5.1 supplies the shared-transaction/delivery-policy API. The root
  module resolves the published tag without a sibling replacement.
- GoNotify v0.5.0 changes `host.NotificationManager` to
  `transport.Transport`; `host.NewNotificationManager` now takes
  `host.NotificationConfig` and returns `(NotificationManager, error)`. Its
  published tag replaces the former local override. Outbox core and
  PostgreSQL backend are aligned at v0.16.0, retaining the API
  supporting no-attempt authorization deferral.
- Apply the additive notification dispatch-key migration before starting jobs.
  Drain legacy external-delivery jobs that lack an email sender; these payloads
  fail permanently under the retry-safe transport contract. Legacy dispatch
  identity falls back to the worker's stable outbox job ID.

- The clean-history baseline was published as v0.7.0.
  Old history, PRs and tags remain in the private history archive; do not move
  or rewrite them. Verify the new remote tag and public module independently.
- Current `go.mod` pins GoAuth v0.5.1 and GoNotify v0.6.0. GoInertia v0.11.0
  retains protocol v2 by default, matching the embedded v2 frontend. GoCache
  resolves v0.2.2. GoDI and GoLogger retain their existing compatible pins.
- `VERSION` is intentionally unset by default. Pass the exact planned or
  published tag for release gates; do not rewrite existing tags.
- Resource verification requires Node.js 22.13+ on the 22.x line or Node.js
  24+.
- Release readiness requires reachable PostgreSQL and Redis services for the
  `integration`-tagged test suite. Configure the test connection environment
  before running the release gate; it fails when either service is unavailable.
- Release readiness also requires `govulncheck` on `PATH`. Install
  `golang.org/x/vuln/cmd/govulncheck` separately before running the gate.
- Keep module path unchanged: `module github.com/assurrussa/goadmin`.
- The root dependency graph has no local replacements. GoUploads is pinned to
  published v0.11.0 and GoWebSocket to v0.2.1. The nested starter keeps explicit
  source-mode overrides; its published Docker build removes them all.
  Published verification rejects every replacement and legacy shared/Redis module.
- The frontend advisory gate and its remaining moderate findings are reviewed
  in [docs/dependency-advisory-review.md](docs/dependency-advisory-review.md).

## Custom upload strategy acceptance

The additive `host.UploadsConfig.Strategies` map and its
`features/uploads.Config` alias must compile in the clean consumer probe.
Verify reserved/invalid names and nil/typed-nil strategies fail assembly, map
mutation cannot alter the module, and bootstrap retains all four built-in
strategies. Client tests must prove audio metadata uses canonical TUS completion
and task status. Built-in policies remain unchanged; full MP3/WAV acceptance
requires the published GoUploads v0.11.0+ dependency and `original_only` mode.
Do not use a candidate replacement as published-release evidence.

## Upload migration upgrades

Starting with v0.9.1, the public `migrations.Migrate` runner installs the
published GoUploads canonical migration filesystem after GoAdmin core, using
`goadmin_uploads_goose_db_version`. It excludes only the base `files` migration
already owned by GoAdmin. No upstream SQL is copied or rewritten. The separate
ledger is required because upstream upload migration versions collide with
unrelated GoAdmin migration versions.

For a normal existing GoAdmin-only installation, pause upload writes/workers,
run the public migration command, and resume traffic only after success. The
canonical SQL creates upload sessions, durable finalization tombstones and
deletion plans, adds lifecycle indexes and a single-active-primary-file index,
and backfills unclassified legacy image/video/document types. Existing IDs,
files, queues and explicit file types (including audio) are retained. Plan for
ordinary PostgreSQL DDL/backfill locks on populated tables.

Duplicate active primary files for one object intentionally fail migration;
resolve the host's intended primary-file assignment explicitly, then retry.
The failing lifecycle migration rolls back its own DDL/backfill; a preceding
session migration may already be committed. Never delete or choose user data
automatically to satisfy this check.

If a host independently installed lifecycle SQL under a different migration
ledger, review and reconcile that history explicitly before adopting this
runner. Object existence is not proof of an equivalent schema, so the runner
does not guess or mark those migrations applied. Public `Reset` remains a
confirmed development/test auth reset and preserves all upload state and its
ledger, allowing a subsequent `Migrate` without replaying lifecycle SQL.

Required regressions cover a fresh public facade, a previous-GoAdmin schema,
repeat migration, reset preservation, duplicate-primary rollback/retry, and
native TUS create/PATCH/complete (including retry) through `NewLocalUploads`
with a custom audio policy. A reader-only upload test cannot establish durable
TUS schema readiness.

## Supported External Surface

The supported embedding surface is intentionally narrow:

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

`reference/externalconsumer` is the machine-readable source of truth. Future
feature or toolkit packages must be added there explicitly before they are
stable public SDK.

## Full Run

From the repository root:

```sh
make
```

This runs the mutating preparation phase and then verification:

- `go mod tidy`
- `go generate ./...`
- `go fmt`, `gofumpt`, and `gci`
- `golangci-lint run --fix`
- `go mod tidy -diff`
- non-mutating `gofumpt` and `gci` checks
- `go vet ./...`
- `golangci-lint run`
- one `go test -race -cover -count=1 ./...` pass
- `resources` admin UI check: client-bootstrap and extension-manifest tests,
  Vitest component tests, non-mutating ESLint and Prettier checks, `npm ci`,
  `npm run type-check`, and `npm run build-only`

Consumer validation is explicit. `make externalconsumer-local` uses only the
current GoAdmin replacement and the declared dependencies;
`make externalconsumer-candidates` requires selected dependency paths for
development. Both run with `GOWORK=off` and empty `GOFLAGS`,
so a developer workspace or ambient Go flags cannot satisfy its imports.
The normal `make check` does not require live integration services or online
vulnerability lookups.

Commit the generated `public/dist` output together with admin UI source changes.
Published consumers embed these files and do not run Vite during a Go build.
Use `make resources` to rebuild and verify the client before committing it.

Repeated race stress and HTML coverage artifacts are explicit diagnostics:
`make test-race` and `make cover-html`. Do not stack them onto a successful
`make check` unless a release investigation specifically requires them.

For a non-mutating verification pass after preparation, run:

```sh
make check
```

## Release Readiness

Repository source-policy scans ignore generated caches and dependency trees.
The v0.5 auth and migration boundary was breaking; all published tags remain
immutable. The local starter under `examples/starter` is a source-checkout
smoke path, not a published-consumer check.

Before tagging, run the complete local publish gate:

```sh
make publish-readiness VERSION=<next-goadmin-tag>
```

This release-only gate also requires `externalconsumer-public-deps-local` and
runs the full `integration`-tagged suite, focused
negative secret-handling tests, `govulncheck ./...`, and
`npm audit --audit-level=high`. The integration suite requires reachable test
PostgreSQL and Redis services. The vulnerability and npm advisory checks need
network access. Local verification is the default. The manual-only hosted
workflow can run the canonical checks, live PostgreSQL/Redis integration suite,
and anonymous public-dependency consumer on demand; push and pull-request
events do not launch it. Release security and published-tag checks remain
explicit release gates. See [local checks and manual CI](docs/ci.md).

After that exact tag is published and resolves without local replacements, run:

```sh
make release-readiness VERSION=<tag>
```

If you need to validate a sibling `goauth` checkout before publishing it, use:

```sh
make externalconsumer-candidates GOAUTH_LOCAL_PATH=../goauth GONOTIFY_LOCAL_PATH=../gonotify
```

## Host Consumer Validation

To scan a site checkout as a host consumer from this repo:

```sh
make import-policy-site SITE_REPO="$SITE_REPO"
```

After publishing, host repos should remove local replaces and run their own
published-module gates. The expected site gate is:

```sh
cd "$SITE_REPO"
task platform:published-check GOAUTH_VERSION=<published-goauth-tag> GOADMIN_VERSION=<published-goadmin-tag>
```

## Release-Visible Behavior

- `goadmin/host` is the stable embedding and extension facade.
- `host.NewAuthAdapter` is the only supported auth DI entrypoint. Hosts pass it
  through `Dependencies.Auth`; no host constructs goauth repositories directly.
- `host.WithRolePresets` delegates canonical idempotent role policy seeding to
  `goauth`; feature packages keep ownership of their concrete presets.
- `goadmin/migrations.Migrate` installs canonical goauth storage, the
  complete goadmin host schema, and canonical GoUploads lifecycle storage in order. It never resets a v0.1 schema.
- `goadmin/migrations.Reset` accepts only the typed goauth reset confirmation
  and is restricted to explicit development/test state reset.
- `goadmin/features/users` is optional and must be registered by the host.
- Built-in admin auth, sessions, roles, permissions, handlers, UI assets,
  seeders, and runtime wiring stay goadmin-owned.
- `host.App.CurrentActor` is the narrow request actor accessor; it must not grow
  into a session, role, or permission dump.
- `host.NewSubjectPermissionChecker` is the stable transport-neutral RBAC
  bridge. Reuse it through `Dependencies.SubjectPermissions`; do not
  rebuild goauth role repositories inside external features.
- `host.WithUploadTransport` may use a transport-specific `TusStore`; nil keeps
  the canonical admin store. No transport may inherit generic delete routes.
- The exported rich-text form contract must keep allowlists, typed media
  selection and invalid-content handling source-compatible.
- Tailwind semantic utilities used by embedded features must resolve to the
  active light/dark palette; tablet widths must not hide the rich-text toolbar.
- `host.Extension.WithPublicRegister` is only a mount hook. The registered
  internal route must still perform its own authentication.
- Multi-root admin UI builds must pass source-manifest tests and emit
  `dist/admin-extensions.manifest.json`. Hosts with structured client
  requirements are validated automatically by `host.New` against `Config.Public`.
  Production CSS must include utility classes that occur only in extension
  source roots; the materializer-generated Tailwind sources are part of this
  gate.
- The locked admin dependency graph must pass `npm audit --audit-level=high`
  through the publish/release readiness gates; release readiness does not accept
  known high or critical findings.
- First-admin setup may use an opaque host-configured static token or the
  hash-only generated one-time token flow. Static tokens retain no raw value
  and generated tokens require a secure `/dev/tty`, `0600` file, or protected
  descriptor. An existing canonical local account is reused only after its
  current password is verified; its credentials and security version remain
  unchanged. Nullable optional canonical profile fields must remain valid when
  read through the string-valued admin projection. Release tests must prove raw
  tokens are absent from stdout, stderr, and application logs.
- `NewAdminSeed` requires an explicit `AdminSeedAccount`; no release may restore
  built-in email/password credentials.
- The browser `createInertiaApp` entrypoint is client-only. `renderToString`
  belongs in a separate SSR server entrypoint and must not be configured in
  `resources/src/js/app.ts`.
- A host dev proxy must route `/.admin-extensions/` to the Vite service; a
  backend redirect for that generated module prevents the client graph from
  reaching `app.mount`.

## Notes

- Do not treat `bootstrap`, `adminapp`, `di`, `http/*`, `infrastructure/*`,
  `shared`, or `seeders` as stable public SDK for new host projects.
- Do not treat `features/*` or `toolkit/*` as wildcard support. Only packages
  listed in `reference/externalconsumer` are stable.
- If module publication is not available, stop after publish-readiness and keep
  host consumer switching pending.
- Publish and validate the compatible goauth tag before the goadmin release. Remove
  every local `replace` before committing or tagging. Publication is not a
  production deployment.

## Anonymous dependency acceptance

Use
`make externalconsumer-anonymous-local GOAUTH_LOCAL_PATH=../goauth GONOTIFY_LOCAL_PATH=../gonotify`
for explicitly selected candidate validation. This creates fresh
HOME/module/build caches, disables authentication/workspaces/direct VCS and
uses the public Go proxy. Only GoAdmin and the dependency paths explicitly
passed by the caller are replaced; no sibling paths are selected by default.
The complete declared graph is downloaded and
checked before and after the consumer test; goshared and goredis are forbidden.

Before tagging, `make externalconsumer-public-deps-local` uses the current
GoAdmin checkout as its only replacement. It does not inject a newer GoAuth or
read sibling paths, and checks the natural declared dependency graph. This is
mandatory in `publish-readiness`; a candidate-only graph cannot pass it.
Neither publish nor post-publication release readiness runs the candidate
consumer. Removing development replacements and updating the dependency pins
therefore leaves no implicit sibling-checkout requirement in either gate.

After tags resolve, `make externalconsumer-published VERSION=<tag>` uses the
same isolation with no source replacements and no injected goauth requirement.
The optional probe CLI goauth-version flag is a compatibility diagnostic, not
the publication gate. A blocked public resolution is a failed gate, never a
reason to reuse developer credentials/cache. Publication and visibility changes
are separate maintainer actions after checks pass.

The admin-owned HTTP cutover removes assurrussa/gofiber entirely. Host code
must use host.Server/ServerHandler and construct the signer with
host.NewCSRFService. Its Request/Token/Claims types are available from host.
Generic CSRF middleware and its IsProd/CookieNameCSRFToken/
CookieNameRefreshToken/ExcludePaths config fields are removed; admin cookie
configuration and Origin policy remain in the installed bootstrap. Adapt host
source before publishing the corresponding breaking contract change.

### Hosts that already own upload migrations

`migrations.MigrateWithHostManagedUploads(ctx, cfg, db)` is the explicit facade
for an established host-owned upload schema and ledger. It installs canonical
auth and GoAdmin core, but does not execute, adopt, or stamp GoUploads migrations.
Before calling it, the host must verify and apply every migration from its pinned
GoUploads dependency, including future additions, and serialize its full pack.
The default `migrations.Migrate` remains unchanged and manages uploads itself.
Do not switch an existing canonical upload ledger to host management without a
separate reviewed ownership transition. This API itself moves no data or history.
