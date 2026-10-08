# Implementation Notes

## Atomic admin role mutations (2026-10-05)

Attach, detach and explicit replacement acquire canonical actor/target subject
locks in sorted order before reading roles or checking protected administrators.
The existing GoAuth v0.5.1 managed transaction joins projection reads and RBAC
writes; no new GoAuth API, schema or dependency version is required. Projection
subject mappings are rechecked after locking. HTTP success and informational
responses are emitted only after commit; failed or uncertain transactions are
not retried automatically.
A host-supplied subject checker remains the route/action authorization guard;
transactional canonical-state checks use the AuthAdapter database handle rather
than passing its transaction to a separately opened checker handle.

Attach/detach keep their existing idempotent behavior, response forms and
self-super-admin restriction. Explicit replacement remains a full replacement,
while its protected-target checks now share the same transaction. This does not
introduce a global last-administrator policy, session rotation or new audit
events. No real account permissions are changed by installing this code.

## Opt-in canonical audio consumers (2026-10-04)

Add `host.UploadsConfig.Strategies` through module assembly and bootstrap to the
existing canonical handler. Validate ASCII names, reserved built-ins and
nil/typed-nil strategy values; snapshot the caller map and isolate repeated
assemblies. Strategies remain borrowed concurrency-safe host objects. The full
client already accepts string categories, so preserve that API and cover audio
metadata plus `/complete` in regression tests. No production client logic or
pending-TUS quarantine contract changes are required.

Built-in policies, auth/CSRF, manager scoping, task status, replacement/deletion,
finalization and upload routes are unchanged. A feature owns its named audio
strategy and entity authorization; it selects explicit MP3/WAV MIME/size limits.
Audio requires GoUploads v0.11.0+ original-only processing. Dependency publication
and final verification are tracked separately; no validation claims are made by
these source notes.

## Identity-only public assets (2026-10-04)

The `/public` GET/HEAD namespace bypasses the global compressor when an owned
public filesystem is mounted. Its static handler keeps compression disabled.
This prevents fasthttp v1.74.0 from asynchronously reading an embedded file while
client disconnect cleanup resets/seeks/pools that same reader. Dependencies,
public API, UI source and embedded asset bytes are unchanged. No response body
materialization, fake encoding, request-header override or compression cache was
introduced.

Compression selection uses Fiber's effective path and case setting with a full
segment boundary; `/publicity` stays dynamic. Host middleware rewriting another
namespace into `/public` after selection is outside this policy. Existing static
routing (including filename aliases), fixed-entrypoint cache rules and auth/error
fallthrough remain intact. Existing byte-range-disabled behavior remains full 200.
`Vary: Accept-Encoding` is added after static processing, including 304, so upstream
response resets cannot remove it. Existing `Vary: *` remains intact.

Only a successfully found representation (200/206/304) can become an empty 406
when identity is explicitly refused. Exact identity and wildcard coding tokens
are recognized across repeated fields, explicit identity overrides the wildcard,
and any zero-weight duplicate excludes that coding. Parsing is tolerant: media-type parameter syntax and finite numeric weights
between zero and one are accepted; unparseable/out-of-range entries are ignored. Representation headers are cleared, the original synchronous stream is
closed and Content-Length is set to zero; unrelated response headers remain.
Ordinary gzip-only offers implicitly accept identity. Redirects and errors retain
their existing handling, but GET/HEAD fallthrough within `/public` also uses
identity. POST and dynamic compression outside this namespace are unaffected.

The real loopback HTTP regression deliberately closes 100 responses after headers
without draining: default gzip negotiation plus gzip/br/deflate/zstd offers. Each
batch is followed by an exact complete embedded app.js byte comparison. Tests also
cover MIME, cache/validators, HEAD, range requests, no-transform, status/error
handling, path boundaries/aliases, and dynamic compression isolation. This changes
static wire representation, may increase transfer size, and makes no bandwidth or
full wire-compatibility claim. Reverse-proxy compression is independently optional.

## WebSocket v0.2.0 integration (2026-09-30)

Scope: published gowebsocket v0.2.0, trusted pre-handshake admin identity,
bounded Fiber HTTP deadlines, handler-before-stream shutdown and a documented
plain JSON client contract. Preserve existing upload notification field names,
the /ws route, and ownership of supplied streams. No publication or deployment.

Implemented: removed the gowebsocket replacement and ordinary consumer-probe
override; configured a typed extractor and explicit JSON wire format; exposed
10s/10s/120s default HTTP timeouts with positive overrides; added explicit server
drain with returned errors and a Fiber fallback hook. Nil realtime modules create
independent owned streams per assembly; Close and failed construction release
owned resources. The upload event bridge restores concrete models after the
stream's JSON snapshots. Frontend parsing already uses JSON and retains its
schemas. Shared teardown stops HTTP even when an individual caller's drain
deadline expires; direct Fiber and Server shutdown share the same connection drain.

Verification:

- Real Fiber WebSocket tests cover checked admin session identity, ambiguous
  cookies, plain JSON, invalid frames and codes 1003/1008/1009/1013. The 1013
  test terminates the subscription; it does not simulate a physical slow client.
- `make -k tidy-check fmt-check vet test-full externalconsumer-local` passed,
  including the full Go race/coverage run and consumer resolution with the
  published websocket dependency. Targeted race tests and independent lifecycle
  review also passed after correcting the deadline shutdown path.
- The resource gate passed: manifest tests, 38 Vitest tests, ESLint/Prettier,
  Vue type checking and production asset build.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...` passed: no reachable
  vulnerabilities; three module-level advisories have no imported affected packages.
- `make check` remains blocked by seven lint findings in the existing profile
  test changes (`http/handlers/auth/profile_settings_test.go` and
  `services/adminservice/email_reauthentication_test.go`). WebSocket changes have
  no remaining lint findings. Gopls retained stale test buffers; compiler/vet
  and executed tests verify the current files.

Limits: handshake authentication does not add live socket revocation on
logout/expiry; callbacks that ignore cancellation cannot be forcibly terminated.
No production/device smoke, publication or deployment was performed.

## Modular runtime (2026-09-30)

Accepted scope: Go 1.27, one module/version, PostgreSQL-only minimal core,
explicit access/jobs/uploads/queues/notifications/realtime/authmail modules,
typed feature SDK, coordinated GoAuth transactions, and migration of site and
tsop. Keep existing data and migration history. No publication or deployment.

The core owns auth, membership, current RBAC checks, CSRF, first-admin setup,
basic profile and the UI shell. Modules declare dependencies; assembly rejects
missing dependencies rather than mounting them implicitly. Construction starts
no workers. Runtime supervises and joins owned workers; borrowed clients and
host workers keep their host lifecycle.

Implemented: GoAuth explicit disabled mail delivery and shared SQL executor;
PostgreSQL Fiber sessions; module registry/runtime and conditional bootstrap;
atomic provisioning/profile/preview updates; capability-driven client; supported
consumer manifests; the minimal starter; and site/tsop adoption. The detailed
acceptance evidence and review corrections are recorded in the final modular
core section below. No release or deployment was performed.

TODO after the typed SDK: declarative CRUD built on the same actor, guards,
rendering and module contracts. It is deliberately outside this implementation.

## 2026-09-27 Public admin readiness

Scope: make the current embedded admin usable by both host developers and
signed-in administrators without changing the supported package list. The
built-in UI remains Russian-only. A new independent local starter demonstrates
the complete host wiring while production integrations remain free to supply
their own infrastructure and notification transport.

Decisions:

- Preserve `host.NewOutbox` and add `host.NewOutboxRuntime` for process lifecycle
  supervision; do not expose internal worker packages to hosts.
- Build the home from the permission-filtered menu, group access administration
  separately from system queues, and fall back to the first permitted route
  after login. Keep demo screens in local/development environments only.
- Keep the Node manifest suite and Vitest component suite separate; both run in
  the local resource gate. Probe the *selected* Go dependency version after
  external-consumer resolution, not just whether a requested version exists.
- Document the MIT license and private vulnerability reporting. Enabling the
  GitHub repository setting remains separate from adding `SECURITY.md`.
- Align `host.NewCSRFService` and the starter's CSRF cookie name with the
  embedded client's `csrf_token` default. The first browser smoke exposed a
  419 on registration when the starter used `csrf_`; matching the names made
  the flow complete.

Verification:

- `make check` passed: Go tidy, formatting, vet, lint, race/coverage, local
  external-consumer probe, UI manifest tests, 13 Vitest tests, Vue type check,
  and production asset build.
- `npm audit --audit-level=high` passed after compatible lockfile updates;
  moderate advisories remain below the release gate's threshold.
- The local Docker Compose starter built a fresh UI and server, applied
  migrations, and returned 204 from `/healthz`. Browser smoke created the first
  administrator, signed in, reached the permission-filtered home, loaded the
  administrator list, and confirmed the setup link disappeared afterward.
- The starter's own tests passed with `go test -race ./...` in a temporary Go
  workspace using this checkout. Its `go.mod` and `go.sum` were tidied against
  the verified remote `v0.6.1` tag without persistent local replacements.
- Clean public-consumer resolution remains unverified while pinned private
  modules and the goauth pseudo-version are unavailable to external users. A
  direct `GOWORK=off go test ./...` of the standalone starter cannot compile
  until a new goadmin tag contains `host.NewOutboxRuntime` and its module pin is
  updated. No tag was created, repository visibility changed, or deployment
  performed.

## 2026-07-19 Nullable existing-account admin projection

Observed behavior:

- First-admin registration correctly reused an existing canonical public-site
  account with the same email and verified its local password.
- That account legitimately had `NULL` in optional `auth_subjects.username`.
  After creating the `administrations` membership, `adminrepo.GetByID` scanned
  canonical fields into the legacy string-valued `models.Admin` projection and
  failed before the Super Admin role could be assigned.

Decisions and tradeoffs:

- Preserve the documented existing-account contract: do not rewrite the
  canonical subject, profile, credentials, password version, public ID, or
  subject kind during admin provisioning.
- Normalize nullable canonical `username`, `name`, `last_name`, and optional
  local `password_hash` to empty strings only in admin projection SELECTs.
  Canonical storage remains nullable and owned by `goauth`.
- Cover both the generated SQL contract and a real PostgreSQL row whose
  optional canonical profile fields and local credential are absent.

Verification:

- `go test ./infrastructure/pgsql/repositories/adminrepo -count=1` passed.
- `go test -race ./infrastructure/pgsql/repositories/adminrepo -count=1`
  passed.
- The existing-account first-admin handler regression test passed.
- The targeted PostgreSQL integration test with a nullable canonical profile
  passed against the site integration database.
- `go vet ./infrastructure/pgsql/repositories/adminrepo` and `git diff --check`
  passed.
- Targeted `golangci-lint` completed with zero issues; it reported only the
  repository's existing `gomodguard` deprecation warning.
- `gopls` diagnostics could not load the sibling module from the active `site`
  root workspace; package compilation, vet, race, and integration checks were
  run directly from the `goadmin` module instead.

## 2026-07-19 GoLand module-cache exclusion

Observed behavior:

- The Makefile intentionally places `GOCACHE`, `GOMODCACHE`, `GOPATH`, npm,
  and linter caches under the ignored repository-local `.go-cache` directory.
- Go module downloads inside `gomodcache` are owned by the current user but are
  read-only by design. GoLand discovered the cached published
  `goadmin@v0.3.0-alpha.1` as a nested module and attempted to update that
  module's read-only `go.sum` during its setup `go list` call.

Decision:

- Keep the repository-local caches used by the Makefile, but exclude
  `.go-cache` from the local GoLand module content root in the ignored
  `.idea/goadmin.iml` file.
- Do not recursively change module-cache permissions and do not enable
  `-modcacherw`: both approaches would let the IDE mutate downloaded module
  contents while leaving the incorrect nested-module discovery in place.

Verification:

- `xmllint --noout .idea/goadmin.iml` passed and the `.go-cache` exclusion is
  present in the module content root.
- `go list -m -json all` passed from the real repository root.
- Ownership inspection confirmed `amir:staff`; the cached module's `go.sum`
  remains intentionally read-only (`0444`). GoLand may need one project-model
  reload to apply the changed local `.iml` file to the already open session.

## 2026-07-19 Clean-install migration squash

Request: make `db/migrations` describe only the final production schema because
the first production deployment will use a clean database and does not need to
replay historical compatibility transitions.

Decisions and tradeoffs:

- Kept one creation migration per final goadmin-owned table instead of folding
  every table into one large SQL file. This preserves readable dependency order
  and focused `goose reset` behavior without preserving obsolete schema states.
- Folded the final projection shape directly into `administrations` and
  `users`: canonical identity and credential columns remain owned by `goauth`.
- Removed legacy auth table creation, its later cleanup migration, the old
  `admin_roles` bridge, and ownership-transfer no-op migrations. A clean install
  runs canonical `goauth` migrations first, so duplicating those tables in the
  goadmin pack is incorrect.
- Added the current hash-only first-admin setup-token table to the monolithic
  pack; it was already part of the supported core migration pack but missing
  from this integration-test/legacy runner path.
- Retained Down sections for tables that exist in the final schema because the
  repository integration harness uses `goose reset` during cleanup.

Verification:

- `make check` passed under Node.js 24:
  tidy check, vet, 73-linter policy, all unit tests, five race runs, coverage,
  clean external-consumer probe, zero-vulnerability npm audit, client manifest
  tests, Vue type-check, and production Vite build.
- A real PostgreSQL clean-install run applied canonical `goauth` first and then
  all eight `db/migrations` files through version `20260710120000`; `goose
  reset` removed the goadmin pack cleanly afterward.
- PostgreSQL integration suites for files and the three queue repositories
  passed against the squashed pack.
- The broader integration invocation also reached the final schema but exposed
  unrelated existing fixture failures in `adminrepo` (duplicate canonical
  `subject_id`) and `adminseed` (missing expected seeded role assignment); no
  migration statement failed.
- `git diff --check` passed.

## 2026-07-19 Existing account first-admin bootstrap

Request: allow a public-site account to become the first administrator under
the same canonical email, then merge the verified change into `master`.

Decisions and tradeoffs:

- Reuse the existing canonical subject instead of weakening the global email
  uniqueness invariant or treating a duplicate-key error as an upsert signal.
- Require the existing local password to match before granting the first admin
  membership. A missing local credential and a mismatch both fail closed.
- Preserve canonical profile fields, credentials, password version, public ID,
  and subject `kind`; admin access comes from the `administrations` membership
  plus the canonical Super Admin role.
- Keep token consumption, subject lookup/password verification, projection
  provisioning, and role assignment inside the existing transaction and
  advisory-lock boundary. Failure therefore rolls back setup-token consumption.
- Add a dedicated projection method to the internal admin auth adapter so the
  handler does not synthesize canonical writes or catch unrelated PostgreSQL
  uniqueness failures.

Verification completed:

- Targeted unit tests for the auth handler and canonical admin adapter passed.
- Targeted race tests for both changed packages passed.
- `make check GOAUTH_VERSION=v0.1.6` passed with Node.js 24.5.0: tidy
  check, vet, 73-linter policy, all Go tests, five race runs, coverage output,
  clean external-consumer probe, zero-vulnerability npm audit, manifest tests,
  Vue type-check, and production Vite build.
- `git diff --check` passed. The existing Vite chunk-size warning remains
  unchanged and is outside this bootstrap change.

## 2026-07-17 Host-neutral role guidance

- Replaced CMS-specific role guidance on the reusable roles page with generic
  permission-composition wording.
- Kept role behavior unchanged: permissions still accumulate across assigned
  roles, while the super-administrator retains the complete permission catalog.
- The host must rebuild its embedded admin bundle after adopting the corrected
  goadmin version; generated `dist` assets are not edited directly.

## 2026-07-15 CMS role preset facade

- Added a type-safe `host.RolePreset` alias and `host.WithRolePresets` option.
- The host facade contains no CMS policy and no duplicate persistence logic; it
  delegates to the supported `goauth/integration/roles` boundary.
- This is an additive API inside the already supported `goadmin/host` package.

## 2026-07-14 Stable v0.3.0 Promotion

- Promoted the verified `v0.3.0-alpha.2` candidate to stable `v0.3.0` without
  expanding the machine-readable external consumer surface.
- Kept `goauth v0.1.4` as the exact stable authorization baseline and retained
  the published clean-consumer and admin UI build gates.
- Historical alpha notes below remain as release provenance; active release
  instructions now target the stable tag.

## 2026-06-04 Project Initialization

Request: initialize the project context, analyze the repository, fill
`AGENTS.md`, and create focused project docs if useful.

Decisions and tradeoffs:

- Kept `AGENTS.md` as the short operational entrypoint for future agents.
  Detailed package, host facade, migration, feature, and verification context
  now lives in `docs/project-context.md`.
- Created `docs/` because the repository had no local docs directory and the
  public surface is too nuanced for `AGENTS.md` alone.
- Preserved existing dirty worktree changes in `Makefile`, `README.md`,
  `RELEASING.md`, `go.mod`, and `go.sum`. Those changes predated this pass and
  were used as current local context rather than reverted.
- Treated `reference/externalconsumer.SupportedPackages` as the source of truth
  for stable public imports, then aligned the new docs around that manifest.
- Read shared agent-context pages for `goadmin` and `goauth`; no direct conflict
  with local verified docs/code was found, so no shared wiki update was made.
- Did not run the full `make` gate because this was a docs-only initialization
  pass and the repository already had unrelated code/module changes.

Verification:

- `git diff --check` passed after the documentation edits.

## 2026-07-10 CMS embedding prerequisites

Request: implement the first dependency stage of the reusable CMS platform
plan in goadmin.

Decisions and tradeoffs:

- Added a narrow `host.App.CurrentActor` result containing only admin ID and
  canonical subject ID. Authorization remains an explicit RBAC check; actor
  identity is not a trusted permissions container.
- Replaced the single-root UI discovery assumption with deterministic
  multi-root source manifests. Static generated imports keep Vite analyzable;
  duplicate page/extension keys, invalid dependency graphs, symlinks, and
  fingerprint drift fail the build and runtime validation.
- Kept `VITE_ADMIN_EXT_ROOT` as a compatibility input. New module work should
  use `VITE_ADMIN_EXTENSION_MANIFESTS` and structured server-side
  `ClientExtensionRequirement` values.
- Kept stable `app.js` and `app.css` entry names because current templates load
  those paths directly. Secondary chunks remain hashed. A fully hashed entry
  needs a separate template manifest-resolution change and was not hidden in
  this boundary update.
- Made first-admin registration fail closed and goadmin-owned. The token is
  hash-only, expiring, one-use, and consumed inside the admin creation
  transaction under a PostgreSQL advisory lock. Expiry and consumption use the
  database clock so web replicas do not disagree because of host clock skew.
- Put interactive setup secrets in a URL fragment instead of a query string so
  the initial request and access logs cannot contain the token. Vue extracts
  and clears the fragment before submitting the form body.
- Required `/dev/tty` or a `0600` file/descriptor before token issuance.
  Non-interactive execution without a secure channel exits before touching the
  token store; output failures revoke the newly issued token.
- Added a transport-neutral subject permission checker over the canonical
  goauth role guard. Hosts can construct it before the feature registry, pass
  it to CMS authorization, and reuse the same checker during goadmin install,
  avoiding a circular composition dependency and duplicate role caches.
- Refreshed only lockfile versions allowed by existing semver ranges to remove
  all 40 npm audit findings (including three critical) without forced or major
  dependency upgrades. Added the high-severity audit to the standard resources
  gate; type-check and Vite production build remain green on the remediated
  graph.
- Preserved the admin seeder API shape with a variadic explicit account, but
  removed implicit credentials. Existing source still compiles; invoking the
  seeder without one explicit account now fails safely.

Verification completed during implementation:

- `make prepare` stages (`tidy`, generation, Go formatting, and lint-fix)
- `make check` with project-local Go, module, linter, and npm caches: vet,
  lint, unit tests, five race-test runs, coverage artifact, local
  external-consumer probe, `npm ci`, `vue-tsc`, and Vite production build
- `go test ./...`
- `npm run build` in `resources/`, including manifest tests, `vue-tsc`,
  Prettier, ESLint, and the Vite production build
- PostgreSQL integration test for hash-only storage, rollback-safe consumption,
  one-use semantics, and permanent closure after an admin exists

Known follow-up signals:

- Vite reports the existing main chunk above 500 kB; entry hashing/code-splitting
  remains the explicit asset-resolution follow-up described above.
- Published clean-consumer validation remains impossible until this change has
  a new semver tag.

Go validation scope follow-up:

- Restricted formatting, linting, vetting, and tests to packages owned by the
  module. A populated `resources/node_modules` tree may contain Go source, but
  third-party npm implementation files are not part of goadmin's Go validation
  surface.
- Enforced the documented Node.js 22.12 minimum before installing or building
  admin resources. Engine warnings on an older runtime can no longer pass the
  aggregate prerelease gate.

Additional client-extension compatibility fix:

- Relaxed only the generated extension-page module cast boundary so Vue pages
  with typed required props remain compatible with the common Inertia page
  map. The builder still validates extension keys, dependencies, source
  fingerprints, duplicate page keys, and the emitted runtime manifest.

Dependency advisory follow-up:

- Removed the unused `@vue/test-utils` and `shadcn-vue` development packages;
  neither was referenced by resource scripts or source imports, while both
  retained vulnerable transitive tooling.
- Upgraded the compatible ESLint/Vue TypeScript toolchain to the first current
  releases whose dependency graph contains the patched glob expansion path.
  ESLint 10 raises the resource-tooling minimum from Node.js 22.12 to 22.13 on
  the 22.x line; Node.js 24+ is also supported.
- Keep online audit lookup in publish/release readiness. The normal `check`
  remains deterministic and source-local.

Prerelease gate follow-up:

- Split pre-tag `publish-readiness` from post-tag `release-readiness`. Both run
  the complete local module, external-consumer, race, coverage, and admin asset
  checks; only the latter requires the exact published module to resolve.
- Declared `v0.3.0-alpha.0` as the expected prerelease for the new host facade,
  typed extension pages, and DB-backed first-admin token surface. The existing
  `v0.2.3` baseline remains unchanged until publication actually occurs.

## 2026-07-13 Browser admin bootstrap correction

Observed behavior:

- The login route returned a valid Inertia page payload for `auth/LoginPage`,
  and Vite served both `/@vite/client` and `/src/js/app.ts` with HTTP 200, but
  the browser left `#app` empty without reporting an application exception.
- The next browser import, `/.admin-extensions/generated.ts`, received a host
  backend `302` redirect to `/auth/login`. The host's Traefik development rule
  did not route that package-generated Vite module, so the browser stopped
  evaluating the graph before the client entrypoint could mount Vue.
- The published `v0.3.0-alpha.0` browser entrypoint also carried an unnecessary
  `renderToString` option and direct `@vue/server-renderer` dependency. Inertia
  ignores the render callback on its browser branch, so this was not the blank
  page root cause, but it incorrectly coupled the client entrypoint to an SSR
  implementation.

Decisions and tradeoffs:

- Removed the SSR renderer import and `render` option from the browser
  entrypoint while preserving the existing client-side `createApp` mount.
- Removed the unused direct `@vue/server-renderer` dependency. Vue may retain
  it transitively, but goadmin no longer declares or imports it as a browser
  runtime dependency.
- Added a source-level client-bootstrap regression test to the existing
  `test:manifest` gate so the browser entry cannot silently regain an SSR
  renderer dependency or lose its explicit mount and rejection handler.
- Attached an explicit rejection handler to the asynchronous Inertia bootstrap
  so any future pre-mount failure is visible in the browser console instead of
  degrading to an unexplained empty `#app` element.
- The generated module URL is part of goadmin's public development integration
  contract, while edge ownership stays with the host. The `site` consumer adds
  `/.admin-extensions/` to its admin Vite router and owns the corresponding
  production-contract assertion.
- The immutable `v0.3.0-alpha.0` tag remains unchanged. The correction is a new
  prerelease candidate, `v0.3.0-alpha.1`.

## 2026-07-13 Published-consumer source scan correction

Observed behavior:

- A host `platform:published-check` populated goadmin's repository-local
  `.go-cache/gomodcache` and then ran the goadmin test suite.
- Nine repository source-policy walkers recursively inspected that cache. The
  downloaded published copy of goadmin contains compatibility fixtures by
  design, so the scanners falsely reported its imports as violations in the
  current checkout.

Decisions and tradeoffs:

- All repository source-policy walkers now prune cache, VCS, dependency, and
  temporary trees by directory name before reading Go files.
- Kept real repository source directories in scope and added a regression test
  for the exclusion boundary. This does not weaken the supported-import policy;
  it stops applying that policy to generated and third-party trees.
- The immutable `v0.3.0-alpha.1` tag remains unchanged. The scanner correction
  is the new prerelease candidate, `v0.3.0-alpha.2`.

## 2026-07-15 Published gouploads checksum follow-up

Observed behavior:

- The merged `v0.4.0-alpha.1` candidate already required
  `github.com/assurrussa/gouploads v0.10.0-alpha.1`, but that prerequisite did
  not exist while the feature PR was being validated through a local workspace.
- After the prerequisite tag was published, `make publish-readiness` correctly
  failed because `go.sum` still contained the previous `gouploads v0.8.2`
  checksums.

Decision:

- Refresh only the module checksums from the published prerequisite before
  tagging goadmin. No runtime or public-contract change is included in this
  release follow-up.
- The real release gate also exposed cognitive complexity in the merged
  `bootstrap.initServer`. Extracted extension collection, upload-transport
  validation, and route registration into focused helpers without changing
  registration order or middleware boundaries; added focused validation tests
  instead of weakening the repository lint policy.

## 2026-07-15 Extension Tailwind content boundary

Observed behavior:

- Vue pages materialized under `.admin-extensions` rendered with common utility
  classes but lost extension-only utilities. The shared CMS media dialog
  therefore lacked its full-screen overlay, centering, width, and z-index even
  though its component markup was correct.

Decision:

- Generate Tailwind v4 `@source` directives from the resolved manifest source
  roots and import that generated file from the canonical stylesheet. Scanning
  the symlinked `.admin-extensions` tree through `tailwind.config.js` was not a
  reliable v4 boundary and could leave extension-only utilities absent.
- Keep extension styling host-owned without copying extension sources or
  replacing reusable components with host-specific markup. Direct source roots
  also preserve dev-mode Tailwind watching.
- Extend the materialization regression test to cover the generated source
  directives. A real production Vite build and the active site Vite endpoint
  confirmed `z-[100]`, `max-w-5xl`, and `bg-black/55` are generated.

## CMS admin follow-up defects (2026-07-16)

- Explicit TUS transport prefixes now win over the generic `/files` default.
  This keeps CMS uploads on `/admin/api/cms/v1/uploads/tus`, where pending CMS
  metadata is expected, and leaves `/files/tus` as the default only when no
  endpoint was supplied.
- Added native `color-scheme` and semantic form-control text/placeholder/
  disabled colors at the shared admin shell boundary so extension-owned raw
  inputs remain readable in dark mode.
- Kept host operations separate from CMS pages and documented their
  relationship in the UI. Added role scope labels without changing permission
  semantics: CMS roles are CMS-only, role permissions are additive, and Super
  Admin remains the merged-catalog role.
- The authenticated live review found that the programmatically opened
  rich-text and image upload inputs had no stable form identity. The
  `v0.4.0-alpha.5` candidate adds hydration-safe Vue `useId()` values, stable
  names, accessible labels, and a source regression test without changing the
  upload contract.
- The `v0.4.0-alpha.6` release changes no goadmin behavior; it advances the
  exact auth dependency to published `goauth v0.1.6` so clean consumers receive
  the corrected built-in content-admin scope description.

## 2026-07-18 Static first-admin bootstrap token

Decisions and tradeoffs:

- The stable `goadmin/host` facade accepts an opaque first-admin setup value;
  the raw host secret is validated and hashed at construction time and is not
  retained by goadmin.
- The static token opens the registration link only while no administrator
  exists. The existing expiring one-time setup-link flow remains supported.
- Static-token consumption takes the same PostgreSQL transaction advisory lock
  as generated tokens. The lock remains held while the handler creates the
  administrator, closing the cross-node double-registration race.
- The existing mock generator cannot currently resolve the embedded
  `ProfileProvisioner` interface in the auth handler. The additive
  `StaticConfigured` mock method was synchronized manually; no generator or
  public interface workaround was introduced.

## 2026-07-20 Profile avatar deletion

- Restored the goadmin-owned `DELETE /auth/profile/avatar` JSON endpoint used by
  the embedded profile uploader. The missing route previously fell through to
  the Inertia redirect handler and produced a repeated `302 /auth/profile`
  loop that Axios surfaced only as `Network Error`.
- Deletion is queued through the existing gouploads service with an
  `admin_preview_detach` after-job carrying the current session ID. The file
  record and all immutable preset objects are removed by the standard delete
  flow, then the administrator projection and active session are detached.
- The endpoint is idempotent when the current session has no preview and keeps
  the change inside goadmin internals; the stable host facade is unchanged.

## 2026-07-31 Development gate efficiency

- Preserved the existing preparation, verification, resource, consumer, and
  release target names.
- Made formatting verification explicit and replaced three Go package
  traversals in `make check` with one race+coverage pass.
- Retained the five-run race stress loop and HTML coverage generation as
  opt-in diagnostics rather than charging every local verification.
- Moved registry-backed `npm audit` out of normal `make check`; it remains
  mandatory in both publish and release readiness. The 2026-07-31 audit found
  current high-severity advisories in the existing lockfile, which requires a
  separate reviewed dependency update rather than a silent workflow refactor.

## 2026-08-30 Outbox v0.12 administrative compatibility

- Updated the runtime Outbox and PostgreSQL backend to `v0.12.0`; job handlers,
  the stable host facade, and the standard runtime repository wiring are
  unchanged.
- Removed obsolete worker reservation and creation methods from the internal
  administrative adapters. Exact counts and explicit operator deletion remain
  goadmin-owned SQL operations, so the admin UI does not emulate lease-aware
  worker acknowledgement.
- Added the same idempotent, forward-only schema migration to the core and
  monolithic packs. Existing jobs default to schema v1 and an empty lease token;
  capability/deduplication indexes and the idempotency registry are created when
  absent.
- Refreshed only the transitive `nanoid` resolutions in the admin resource
  lockfile, clearing the high-severity advisory enforced by the release audit.

## 2026-08-30 v0.5 canonical auth release

- Merged the current administrative maintenance line into the v0.5 auth
  architecture while keeping `host.AuthAdapter`, canonical RBAC, realm-bound
  sessions, first-admin setup, and explicit auth reset as the supported model.
- Aligned the release with `goauth v0.2.1`, `gonotify v0.3.11`, and Outbox
  core/PostgreSQL `v0.12.0`; the supported package manifest is unchanged.
- Promoted the fully verified v0.5 line directly from the existing alpha train
  to stable `v0.5.0`; earlier prerelease and v0.3/v0.4 tags stay immutable.

## 2026-08-30 v0.5.1 first-admin transaction ordering

- A real host smoke exposed a PostgreSQL self-block during first-admin setup:
  the outer pgx transaction inserted `administrations`, then canonical RBAC
  opened its own `database/sql` transaction and waited for `auth_subjects FOR
  UPDATE` behind the membership foreign-key lock held by the outer request.
- Keep the cross-node advisory lock and supported facade unchanged. Provision
  the verified canonical account, ensure and assign the Super Admin role, and
  only then insert the host membership. No goauth operation runs after that
  membership insert inside the outer transaction.
- Added an order-sensitive handler regression test. The release must also pass
  a real local first-admin registration/login smoke before host publication.

## 2026-09-27 public-admin review follow-up

Review source: https://chatgpt.com/s/t_6ab90d943c208191a1740ae1f409bc61
The list contains verified defects, release checks, and architecture proposals;
the numbers are review references, not proof that every item is a vulnerability.

- The immediate security and correctness batch separates upload action guards,
  bounds streaming uploads, keeps uploaded active content as attachments,
  protects `/uploads` with admin auth, `uploads.read`, file ownership and a live
  path lookup, removes `/tmp` serving, validates reset
  confirmation and redirects, and fixes literal roles routing and fake admin
  export. Redis browser sessions use a private namespace and do not close the
  host-owned client. Deleted admin memberships no longer disable the shared
  canonical subject. DLQ retry requires one transactional delete before it
  enqueues, and queue grids redact job payloads.
- Generated first-admin tokens acquire the PostgreSQL advisory lock in a
  separate statement before reading setup state. Admin projection writes use
  the observed version to reject stale whole-JSON updates. Preview assignment
  rejects foreign, deleted, and active-content files. Local upload metadata,
  replacement deletion, and after-jobs share one database transaction; a
  replacement must have the same manager and object binding. A failed reader
  upload transaction removes its new local file. The default local file repo
  filters admin lists in SQL by manager before pagination; custom host repos
  without an ownership query use page scans and should add a path/manager
  lookup for large installations.
- Preview attach/detach jobs now fence the observed admin version and expected
  preview ID, including late detach after a replacement. A local file delete
  enqueues its after-job in the same database transaction. Two concurrent
  uploads starting from the same preview state are still first-commit-wins:
  if A attaches before B, B is discarded even when B was the later upload.
  This remains an open correctness issue, not a last-action guarantee.
  `gouploads` creates after-jobs before file persistence, so reserving an
  intent in the upload strategy could also discard successful A when B fails.
  A fix needs an intent generated atomically with file/outbox persistence,
  plus invalidation by manual preview selection and deletion; custom host
  uploaders need the same contract. Redis session preview writes still have no
  distributed compare-and-set and can temporarily disagree with the database
  until a subsequent session refresh.
- Release gates now include integration-tagged PostgreSQL/Redis tests,
  negative secret-handling tests, `govulncheck`, and npm advisory lookup. The
  regular local check remains separate from network-dependent release gates.
- The default administrator list now loads canonical role names for the whole
  page in one query after its count and page queries. A custom admin repository
  without the optional batch method still uses per-row role reads. The default
  local file repository scopes pagination in SQL; custom file repositories
  still need an ownership-aware list contract to avoid a full fallback scan.
  A failed role lookup now fails the administrator list instead of displaying
  an incorrect empty role summary.
  Negative HTTP tests cover revoked ownership, soft deletion, missing sessions,
  and GET/HEAD/Range file responses. Existing WebSocket connections are not
  reauthorized after upgrade by the current transport dependency.
- A new append-only PostgreSQL action journal records admin soft-delete and
  queue delete/retry in the same pgx transaction as each mutation. Failure to
  write the event aborts those actions; events contain IDs and action names,
  never job payloads. This is partial coverage: goauth RBAC and canonical
  account mutations have separate `database/sql` transactions and cannot be
  included atomically by the current goadmin API.
- Two cross-component contracts remain unresolved. goauth PostgreSQL writes
  use independent `database/sql` transactions while goadmin uses pgx; ordering
  avoids a deadlock but cannot make first-admin/create/update atomic. Fiber
  saves whole browser sessions without a distributed compare-and-set, so
  concurrent refresh and preview writers can overwrite one another. These
  require coordinated goauth/session-storage changes and real concurrent
  PostgreSQL/Redis acceptance before claiming multi-node correctness.
- Disk writes are not atomic with the database transaction. Replacement file
  cleanup and recovery after a failed physical unlink still require a durable
  cleanup protocol. The current `host.Install` keeps uploads, outbox, event
  stream, and notifier mandatory; an optional core-only installer would be a
  separate host contract and must not be implied by optional UI features.
- Further public-readiness work remains: complete durable audit coverage for
  goauth-backed sensitive actions, WebSocket revocation/rate-limit negative
  tests, custom-repository list/query performance, and process-lifecycle
  acceptance.
  The local source/integration gates do not establish clean published-module
  resolution, multi-node safety, or production deployment readiness.

Verification for this follow-up: final `make check` passed with tidy/format,
vet, lint, one Go race+coverage pass, the local external-consumer probe, UI
manifest tests, 13 Vitest tests, ESLint, Prettier, Vue type check, and the
production build. Final `make integration-check` passed against temporary
PostgreSQL and Redis after fixing the new audit migration's repeat-apply
behavior. Its PostgreSQL tests include concurrent membership provisioning,
preview CAS, batched admin roles, and transactional audit rollback. The
earlier restricted-shell npm attempt had a registry DNS failure; the final
gate ran with network access. `make resources-audit` passed at the
high-severity threshold with moderate advisories still present; `govulncheck`
found no called-symbol vulnerabilities. No public tag, clean
published-consumer probe, or production rollout was performed.

## Auth session candidate (2026-09-28)

- Preserve opaque browser credentials; canonical auth and memberships remain authoritative.
- Add an encrypted owner/version journal behind supported host assembly, Redis and PostgreSQL.
- Native NotificationSender is exposed through AuthAdapterConfig; no supported internal imports.
- Production host cookies are host-only; readable CSRF proof never contains the opaque secret.
- Canonical migration is additive; frontend/backend cut over together and users log in again.
- Targeted race, real Redis/PostgreSQL CAS and real admin lifecycle are recorded in the candidate evidence; publication and deployment are separate.

## 2026-09-29 Opaque admin session candidate

- Preserved the random opaque browser credential and isolated admin realm.
  Access/refresh secrets live in an encrypted Redis Lua or PostgreSQL CAS
  journal, with owner/version fencing and bounded TTL. Stale Fiber snapshots
  cannot overwrite canonical credentials; an unknown owner requires new login.
- Each request checks canonical session, subject, current membership and
  permissions. CSRF binds a purpose-separated digest of the opaque credential;
  exact Origin precedes Referer, and duplicate/conflicting inputs are denied.
- Supported `host.AuthAdapterConfig` accepts managed notification sender/worker
  configuration. Additive migration and rollback/re-login/key retention guidance
  are in `docs/browser-auth-sessions.md`; prior migrations were not rewritten.
- Final `make check` PASS with a temporary candidate goauth modfile: fmt/vet,
  lint 0, race/coverage, local clean consumer and all resource gates/build.
  Real PostgreSQL/Redis integration PASS: concurrent independent owners, stale
  save/version fencing, encrypted state/TTL, opaque rotation and immediate
  permission/membership/logout-all rejection. Independent source/delta review
  has no open finding in this scope.
- Source is uncommitted and unpublished above base `4152e0a`; canonical
  dependency pins are unchanged. No published-consumer or old-binary rollback
  acceptance is inferred from the local candidate. Broader security fault
  matrix remains explicit in the root candidate evidence file.


## Local original uploads integration (2026-09-29)

- Local testing directly replaces sibling goauth, gonotify and gouploads; starter also replaces goadmin. These replacements are temporary and are not publication evidence.
- `host.NewLocalUploads` delegates to gouploads `NewOriginalRuntime`. It returns jobs for host registration; construction starts no workers. Starter registers these jobs before `Install` registers preview jobs, then supervises its existing outbox worker.
- Runtime storage uses `media/v1` and local delivery at `ADMIN_URL/uploads`, with no bucket prefix. Tests use fresh data; there is no legacy file migration.
- File delivery and preview attachment require recorded final main artifacts. Queued, processing, failed, staging, deleted and legacy records fail closed. Both original and media finalizers clear uploader state and persist main relative paths; URL strings are not used as the finalization marker.
- `host.NewUploadEventPublisher` adapts upload UUIDs to the admin stream and preserves original JSON. Admin after-process publishing and WebSocket processors use the same adapter boundary.
- Narrow facade, static guard, UUID/event and preview-job tests and starter tests pass. The added PostgreSQL async-lifecycle test passed against an isolated fresh PostgreSQL after repairing canonical local key preservation in gouploads. The disposable test database and container were removed. Final repository gates are owned by the coordinating task.


## Starter managed auth notification worker (2026-09-29)

The PostgreSQL goauth Runtime rejects custom `Runtime.EventSink` wiring. Starter now supplies its SMTP transport through `host.AuthAdapterConfig.NotificationSender`; it receives already decrypted `NotificationDelivery` values and retains the existing password-reset, reset-success and email-challenge message rendering. Goauth owns encrypted durable enqueueing and retry, so SMTP failures no longer propagate to already committed initiating auth operations. At-least-once SMTP delivery can duplicate messages after uncertain transport acceptance.

Starter runs `auth.Runtime().RunNotifications` alongside the admin server and uploads outbox. Any unexpected loop exit triggers server cancellation and outbox drain; shutdown cancels and joins the notification loop before closing auth/database clients. Context cancellation closes a blocked SMTP connection promptly. Narrow checks passed: starter `go test -race ./...`, and root `go test ./host ./internal/auth` (`internal/auth` has no test files). Regression tests cover notification/server/outbox failure supervision, cancellation joins, rendered SMTP content, invalid notification rejection and blocked SMTP cancellation. Full repository gates and Docker/Mailpit acceptance belong to the coordinating task and were not rerun here.

## Dependency independence follow-up (2026-09-29)

Accepted scope: remove goshared/goredis from goadmin, tests and starter; adopt
cleaned gofiber/gowebsocket via explicit candidate replacements; preserve UUID
wire/storage formats, Redis Ring mapping/TTL and host client ownership. Browser
sessions may require login again; account/database reset is not permitted.
Site's current integration is accepted as the baseline, with affected consumer
checks after this change. Publication and visibility changes are deferred.
The initial tree was clean. Redis implementation is delegated with exclusive
ownership of its infrastructure, connection config and session-storage tests.

Implemented: admin-owned UUID/environment/pointer/delay helpers, direct go-redis
Ring, explicit Fiber/event UUID adapters and a source import guard covering
tests and generated files. Root and starter module graphs no longer contain
goshared or goredis. UUID SQL/JSON representation, address-based shard identity,
database selection, TTL and host client ownership remain covered by tests.
Incompatible gob browser sessions are deleted individually and become anonymous;
valid sessions and Redis failures retain their existing behavior.

The anonymous consumer starts with empty caches and no credentials; local mode
allows only goadmin and its five explicit sibling candidates, while published
mode rejects every replacement. Starter has separate source and published
Docker builds. Source mode snapshots current tracked/non-ignored sibling files
without copying developer caches or Git metadata.

Integration checks also exposed stale PostgreSQL auth test fixtures and an
incomplete explicit development/test Reset. Fixtures now use managed notification
wiring; Reset removes browser auth state before Migrate recreates it. Tests
verify host/file data survives that explicit reset. Normal Migrate remains
additive; no existing application database was reset.

Verified on this candidate:

- `make check`: formatting, vet, lint, race/coverage, local consumer and complete
  resource tests/type-check/build passed.
- `make integration-check`: passed with isolated PostgreSQL 17 and Redis 7.
- `make externalconsumer-anonymous-local`: passed with fresh caches and no
  credentials; declared graph checks exclude both removed modules.
- `make import-policy-site`: passed. Affected site packages tested with a
  temporary modfile reveal a remaining host adaptation: its
  `backend/internal/infrastructure/services/fiber/middlewares/deps.go`
  `MustUserID` and `GetUserID` must explicitly convert the new gofiber UserID
  to the host-owned type. Auth tests passed, but dependent admin/app packages
  cannot compile until that conversion is made. Site files were not changed.
- `govulncheck` v1.7.0 found no reachable or imported-package vulnerabilities;
  seven advisories remain in required modules outside the imported code.
- Fresh Docker starter: first-admin registration, browser login/RBAC, managed
  SMTP delivery to Mailpit, TUS create/patch/complete and background original
  finalization passed. Session and byte-identical original survived an admin
  container restart. This is local acceptance, not production deployment.
- Independent review found no open issue in identity/Redis/session, graph
  isolation, source snapshots or explicit reset changes.

Checked sibling base revisions: goauth `01b2a11`, gofiber `2527f84`, gonotify
`dc86f70`, gouploads `4737b15`, gowebsocket `b55c50e`. Source checks include current
working changes; these revisions alone do not identify a release artifact.
Compatible public tags, removal of candidate replacements and the published
consumer/starter checks remain release work. Nothing was published or deployed.

## Admin-owned HTTP runtime (2026-09-29)

Starting from clean `5595e79`, remove assurrussa/gofiber while retaining Fiber v3.
Ownership: local HTTP server/middleware/error implementation; internal CSRF proof
signer; local test helpers. Preserve admin request origin/cookie ambiguity checks,
opaque-session digest binding, JWT/UUID wire representation and server shutdown.
The supported package list stays unchanged. New host aliases expose Server and
CSRF Request/Token/Claims without importing internals. Generic wrapper CSRF
middleware is removed: the installed admin bootstrap owns cookie and Origin
policy. Remove its unused IsProd/CookieNameCSRFToken/CookieNameRefreshToken/
ExcludePaths fields from host.CSRFConfig; callers configure actual admin cookies
through admin config and SessionStore. This is a source API migration, not a
schema migration. Standalone source/probe contexts shrink to four siblings.
Checks planned: signer compatibility and bootstrap security tests, server lifecycle
race tests, host/starter consumer tests, make check and anonymous fresh consumer.

Implemented and independently reviewed: unused JWT/OpenAPI proxies removed;
server errors, request ID/logging, Options and shutdown now owned locally.
Shutdown.Close is idempotent; Run synchronizes listener readiness with cancellation
and drains active requests after listener completion. Targeted lifecycle race
checks pass. Prefork delegates to Fiber and retains its existing subprocess
lifecycle limitation; TLS/prefork end-to-end acceptance was not performed.

Signer compatibility, bootstrap CSRF security, host/probe and starter tests pass.
Fresh anonymous consumer passes with four sibling candidates and no wrapper
module. govulncheck v1.7.0 finds no reachable/imported-package vulnerabilities;
three required-module advisories remain outside imported code. Final source gate
and rebuilt starter smoke results follow below.

Site source is outside this change: its API CSRF service can remain independent,
but admin injection must construct host.NewCSRFService instead of passing an
assurrussa/gofiber CSRF service. Externally named server types must likewise use
host.Server. No schema/data migration or tag publication is part of this change.

Final validation: `make check` PASS (tidy/fmt/vet/lint, race+coverage,
local consumer and all resource gates/build); starter module `go test ./...`
PASS. Final Docker source build uses only four sibling contexts. Registration,
login/RBAC, Mailpit notification, TUS 201/204/202 and background finalization
passed on disposable data. Admin process stops with exit code 0; after restart,
the session works and downloaded original has identical SHA-256. Disposable
containers/volumes are removed after validation. Full database integration gate
was not rerun for this HTTP-only change; no migrations were edited.
No tag, commit, publication or deployment was performed.

## PR 10 review fixes (2026-09-30)

Scope: review head `b583813` against `4152e0a`, fix current GitHub comments,
and independently inspect browser auth/session/CSRF/HTTP lifecycle. The initial
working tree was clean. Preserve public host packages and dependency pins.

- Reproduced duplicate cookies blocking all public auth GETs with HTTP 400.
  Recovery now drops ambiguous credentials before session lookup and expires
  legacy Domain/Path variants while retaining a new anonymous cookie. Protected
  requests, login mutations and Authorization are still rejected. Cookie-jar
  regression covers host-only/domain cookies, narrow paths, custom names,
  public pages and anonymous protected mutations.
- `public/dist` was ignored and absent from tracked module sources. Track the
  production build so clean consumers embed the auth error client. A host asset
  test verifies the embedded entry handles the versioned auth error contract;
  the resource gate also checks CSS/favicon and generated manifests.
- Independent source review of the original head found no additional confirmed
  P1/P2 in browserstate, refresh, CSRF or HTTP lifecycle; its seven focused Go
  packages passed. This is bounded source/local evidence, not deployment proof.

Final verification passed: targeted Go packages; focused lint with zero issues;
`make check` (tidy/fmt/vet/lint, one race+coverage pass, local consumer and complete
UI tests/type-check/build); `git diff --check`. UI gates ran 28 Node policy/manifest
tests and 16 Vitest tests. The final source delta also passed independent review
and its four-package Go check. Local consumer validation uses the four declared
sibling candidates; it does not prove published dependency readiness. A fresh
export containing only staged files also passed the embedded-auth-client host
test without rebuilding assets (only its temporary sibling paths were mapped).
Live
PostgreSQL/Redis integration, real-browser acceptance and release gates were not
rerun for these cookie/asset changes. No tag, release or deployment was made.

## Modular core and explicit modules (2026-09-30)

Active scope: Go 1.27, one GoAdmin module/version, `host.New(Config,
Dependencies, modules...)` and unified Runtime lifecycle. Core includes auth,
CSRF, canonical RBAC/membership, firstadmin, profile, shell and extension SDK.
Access/jobs/uploads/queues/notifications/realtime/authmail are explicit modules;
existing users/operations feature constructors mount through FeatureModule.
PostgreSQL browser and auth state are the default; Redis and SMTP are optional.
The old all-dependencies Install contract is removed after site and tsop adopt
New. Declarative generic CRUD remains TODO; the typed command helper is in scope.

Construction is inert. Runtime supervises owned workers, cancels/drains/joins
on exit, and leaves borrowed infrastructure under host ownership. Firstadmin,
admin provisioning, profile/projection/preview binding and action audit share
GoAuth's PostgreSQL transaction; rollback and unknown commit outcomes remain
explicit. Preview binding locks/checks observed ownership and updates only the
binding columns. Database migrations are additive; no host data is reset.
GoAuth's explicit mail-disabled policy rejects unavailable recovery/email commands
before side effects; enabled delivery retains encrypted claims and retries.
Tsop forwards the original sealed native event with stable transport identity.

Review corrections: reject disabled borrowed Auth with authmail and reject
borrowed-worker mode without borrowed Auth. Preserve previews and guard absent
loaders when uploads is disabled. Inertia's session bridge follows the response
cookie after session rotation, avoiding Fiber 3.5.0's stale request-local ID
when a firstadmin success redirect persists flash data. Existing admin email is
readonly without authmail; initial provisioning still accepts its required email.

Additional integration corrections: explicit development/test Reset removes the
new browser-session table. Existing-account bootstrap/provisioning/seeding admits
and verifies credentials before the outer transaction, then revalidates an opaque
runtime-bound proof under the canonical subject row lock. Fresh creation remains
inside the shared transaction. Invitation projection uses the exact public reset
selector returned by the initiating command, without timestamps or latest-row
heuristics; failed association rolls back enqueueing and audit together.

Live browser acceptance also found lazy profile CSS resolving at `/css` instead
of the embedded mount. Vite now emits relative asset references (`base: ./`).
Fixed-name embedded entrypoints ignore `If-Modified-Since` and omit the unreliable
zero `Last-Modified`, preventing an old compiled client from surviving a rebuild
through a false 304. Profile mutations now accept the optional family name and
username shown by the form and already allowed by canonical auth.
The proof deadline is checked again after waiting for the canonical subject lock.

Validation completed:

- GoAdmin `make check`: formatting, vet, lint, Go race/coverage, local clean
  consumer, 28 Node contract tests, 35 Vitest tests, Vue type checking and the
  production asset build. `make integration-check` passed against disposable
  PostgreSQL/Redis; the final optional-profile regression additionally passed
  `go test -race -tags integration ./host -run
  '^TestAssemblyMinimalPostgresLoginProfileAndSessionRestart$' -count=1`.
- GoAuth `make check` and `make integration` passed. After tightening proof
  expiry, targeted PostgreSQL race tests passed, including a real subject-row
  lock wait across the deadline and atomic reset-receipt association.
- Site `task go:verify` and the OpenSpec checks (20/20) passed; the adopted
  change is archived. Tsop `make check`, lint, owned-worker race tests and live
  PostgreSQL invitation-projection tests passed. Both consumers rebuilt their
  admin assets and passed the matching bundle/template contract tests.
- Browser acceptance used the minimal starter with PostgreSQL and no modules:
  firstadmin registration, authorized home/profile, profile mutation with an
  empty family name, session persistence across server restarts and settings
  without email-change controls, then logout and a successful login. Optional
  modules were covered by source,
  contract and integration tests, rather than a full browser matrix.
- Independent security/data/lifecycle review closed the borrowed-auth/mail
  ownership, invitation correlation, proof-expiry and embedded-cache findings.
  Final `git diff --check` passed across the four changed repositories.

Published dependency readiness, deployment and release publication are outside
this implementation. Checkout-local GoAuth and other sibling replacements are
candidate validation, not release evidence. The generic CRUD layer remains the
agreed TODO; user changes to tsop's system load fixture were preserved.

## Modular runtime review corrections (2026-09-30)

Active goal: fix membership-policy composition, module route ownership and the
transaction bridge; adopt published gouploads v0.10.0 and add an anonymous
pre-tag consumer against published dependencies. Preserve the current modular
architecture, data and tags. GoAuth remains a local candidate by explicit user
instruction: its published v0.4.1 lacks the shared transaction/delivery API.
The GoAuth replacement is retained; publication and hosted CI are outside this
change. Full local, integration and release-security checks are the acceptance
evidence; public dependency acceptance may remain blocked independently.

Implemented: upstream membership policy composes with mandatory admin
membership, and custom realms without upstream policy fail closed. Built-in
modules own literal namespaces; explicit route claims reject parameter/catch-all
intersections and implicit HEAD collisions. The transaction bridge exposes only
RowsAffected, which both pgx and database/sql preserve. Root GoUploads requires
v0.10.0 without a replacement; the starter pin matches while its explicitly
selected source-candidate mode retains sibling contexts.

The public-deps-local consumer replaces only GoAdmin, discards dependency path
and version overrides, and resolves with fresh HOME/module/build caches,
GOAUTH=off, the public proxy/checksum database and direct VCS disabled. It is
mandatory in publish-readiness. Anonymous candidate mode and ordinary local
consumer mode remain separate; normal checks reuse shared Go caches.

Validation passed:

- Targeted host/auth/transaction/consumer regression tests, production-file
  gopls diagnostics, and independent security/compatibility review with a
  follow-up check of the final lint/security delta.
  Gopls could not load metadata for the newly added consumer test; executed Go
  tests and compiler/linter verified it.
- `make check`: tidy/format/vet, zero lint issues, one race/coverage pass, local
  consumer using published GoUploads, 28 Node contract tests, 38 Vitest tests,
  ESLint/Prettier, Vue type checking and production assets build. Existing menu
  test literals were replaced with constants to unblock two lint findings.
- `make integration-check`: all integration-tagged packages against local test
  PostgreSQL/Redis, including shared-transaction rollback, first-admin/profile,
  browser state and upload binding. Tests use disposable databases/schemas.
- `make release-security-check GOVULNCHECK=<temporary-wrapper>`: negative
  security tests, govulncheck v1.7.0 and npm high-severity audit. No reachable Go
  vulnerabilities; three required-module advisories remain outside imported
  affected packages. Axios 1.20.0 and brace-expansion 5.0.12 close the two high
  npm findings within existing semver ranges. The lockfile and embedded bundle
  are updated; 38 moderate npm findings remain.
- Starter `go mod tidy -diff` and `go test ./...` passed after synchronizing
  its dependency metadata; final root/host/auth/transaction/consumer/menu tests
  also passed after documentation and example updates.
- `git diff --check` and edited-doc scans for machine-local paths passed.

Public readiness remains blocked: the executed
`make externalconsumer-public-deps-local` receives HTTP 404 from the public Go
proxy for goauth v0.4.0, gonotify v0.4.0 and gouploads v0.10.0. Availability in the
configured developer registry is not anonymous-public availability. GoAuth
v0.4.1 also lacks the needed shared transaction/optional-delivery API, so its
replacement stays by accepted requirement. No hosted CI, commit, tag, release,
visibility change or deployment was created.


## Static route ownership and explicit consumer modes (2026-10-01)

Active goal: complete built-in ownership for `/uploads`, `/public` and `/test`,
and remove implicit sibling selection from release/ordinary consumer checks.
Preserve GoAuth's explicit root replacement until its compatible release is
ready. Public dependency publication/visibility remains a separate blocker.
This supersedes the previous command contract in which `make check` included
the candidate consumer and local path flags defaulted to sibling directories.

Scope: reserve core public/test namespaces in every environment and both file
API/delivery namespaces for the uploads module. Keep the conservative matcher.
Separate source/UI checks, explicitly selected development consumers and
published dependency consumers; no version overrides may repair the published
graph. No explicit opt-in for hosted CI was received; local gates are used,
and no hosted pipeline is created under ci-cd-cost-guard.

Evidence so far: six new route regressions failed on HEAD 3165df8 because they
reached missing-PostgreSQL validation instead of a route conflict. After the
ownership change, targeted preflight/static-namespace tests and diagnostics
passed; names such as `/publicity`, `/testing` and `/uploads-extra` remain valid.
Final verification of this working tree passed:

- `make check`: tidy/format/vet, zero lint issues, one Go race+coverage pass,
  28 Node contract tests, 38 Vitest tests, ESLint/Prettier, Vue type checking
  and the production build. No generated asset changes resulted.
- `make integration-check`: all integration-tagged tests passed on local test
  PostgreSQL/Redis using disposable databases/schemas.
- `make release-security-check GOVULNCHECK=<temporary-wrapper>`: negative
  security tests, govulncheck v1.7.0 and high-severity npm audit passed. No
  reachable Go vulnerabilities; three required-module advisories are outside
  imported affected packages, and 38 moderate npm advisories remain.
- `make externalconsumer-candidates GOAUTH_LOCAL_PATH=../goauth GONOTIFY_LOCAL_PATH=../gonotify`
  passed. Script/Make tests cover absent and explicit candidates, paths with
  spaces, ignored dependency/version overrides in ordinary/release modes,
  anonymous environment isolation and readiness target selection.
- Independent route/consumer review found one documentation regression: the
  neighbouring site's `task platform:published-check` still requires its own
  GOAUTH_VERSION. The original argument was restored only for that command.
  Other source changes had no substantial findings.
- Focused tests, production-file diagnostics, `git diff --check` and scans
  of edited docs for machine-local paths passed. The CLI test metadata remained
  unavailable to gopls; compiler/tests/lint covered it. Shellcheck is unavailable.

Published dependency readiness is still blocked, deliberately and observably:
`make externalconsumer-local` now uses declared versions and fails compilation
on published GoAuth's missing `postgres.SQLExecutor`; it no longer borrows the
sibling implicitly. `make externalconsumer-public-deps-local` fails with public
proxy HTTP 404 for goauth v0.4.0, gonotify v0.4.0 and gouploads v0.10.0. Root
go.mod and its GoAuth/GoNotify replacements are unchanged. No dependency
publication, visibility change, commit, tag, release or deployment was made.

## Local dependency contract integration (2026-10-01)

Goal: verify GoAdmin with the user's current GoAuth, GoUploads, GoNotify and
NotifyHub candidates, correcting GoAdmin integration without publishing tags.
GoAuth and GoNotify replacements remain explicit; release readiness is separate.
Source snapshots: GoAuth `e902518`, GoUploads `f43d2d3` (v0.10.0), GoNotify
`3dca7c8`, NotifyHub `d518fda`. GoNotify advanced from `4cedf81` during the
initial inspection; final consumer/race integration includes working changes
to its typed-nil validation, options and expiration semantics. Final source
fingerprints (SHA-256 prefixes of Go files/module metadata): GoAuth
`4ea4c45931212e38`, GoNotify `b710b9b2325a4bef`, GoUploads
`1608963c944f8304`, NotifyHub `6738bc6935f64ccb`. GoAuth also had concurrent
documentation/public-manifest edits. These working candidates are not tags.

GoAuth transaction/proof, membership/RBAC and auth worker integration and
GoUploads storage/HTTP ownership matched the current contracts in an independent
read-only review. GoNotify removed its legacy manager; GoAdmin now borrows its
`transport.Transport`. The typed NotifyHub factory validates startup config.
Starter notifications require explicit gateway URL/key; authmail stays on its
own sender contract because downstream queues must not persist plaintext auth
codes/tokens.

Admin notification errors previously disappeared after logging. Jobs now reuse
GoNotify's dispositions and keep a stable dispatch identity across retries.
Email sender/content and recipients are staged in the payload; gateway keys
separate recipients/channels. An additive, duplicated canonical/monolith
migration installs inbox deduplication without modifying historical SQL or
resetting read state. Legacy dispatch identity comes from the stable executing
outbox job ID; direct execution without either identity fails before storage.
Legacy external-delivery payloads without an email sender fail permanently.
Outbox core and
PostgreSQL backend pins move together to v0.15.0 for no-attempt deferral.

Validation passed:

- `make check`: tidy, formatting, vet, zero lint issues, one complete Go
  race/coverage pass, 28 resource contract tests, 38 UI tests, type check and
  verified embedded UI build.
- `make integration-check` against disposable databases on the existing local
  PostgreSQL/Redis services. After the review fix, the affected inbox and
  notification packages also passed `go test -race -tags=integration ...`.
  This included the real outbox worker's legacy identity, one inbox row/read
  state on replay, and actual isolated NotifyHub dry_run acceptance, duplicate
  receipts and conflicting-content rejection.
- `make externalconsumer-candidates GOAUTH_LOCAL_PATH=../goauth GONOTIFY_LOCAL_PATH=../gonotify GOUPLOADS_LOCAL_PATH=../gouploads`
  against current local candidates; final `make tidy-check` also passed.
- `make release-security-check`: no reachable Go vulnerabilities; npm audit
  retained 38 moderate advisories with none at high/critical severity.
- Starter `GOWORK=off go test ./...`, `git diff --check`, identical SQL packs
  and edited-doc scans for local machine paths.

Independent review found the legacy empty-ID retry gap; the worker JobID fallback
and real queue regression closed it. Re-review found no remaining substantial
issues. The source gate also required replacing Outbox's deprecated DefaultJob
alias and spelling out GoUploads' fallback file-type cases, preserving policy.
Gopls retained stale v0.12 metadata for the new core DefaultJob alias; actual
compiler, vet, lint, race and consumer checks verified the v0.15 declaration.

The isolated gateway was stopped after checks. No publication, commit, tag,
hosted CI, deployment or sibling-source changes were made by this task. Published
consumer readiness was not claimed or tested while compatible tags are pending.


## 2026-10-02 users capability and export corrections

User email changes now check the authmail capability before writes and join the
existing GoAuth command transaction. User projection reads/updates use the same
`admintx` executor as canonical profile and email enqueue operations. The users
editor mirrors the admin editor's mail-disabled readonly field. This does not
introduce a new transaction coordinator or extend the public SDK.

Export always uses the existing 10,000-row bound, independent of page size,
retains query filters/search/sort, and reports truncation explicitly. The client
uses a same-origin CSV transfer, rejects non-CSV/auth-error responses, deduplicates
repeated clicks, and aborts on unmount. The users repository now applies its
advertised search and sort aliases consistently to list/export queries. Full
URL-state fidelity is supplied by the separately reviewed DataGrid URL fixes.

Focused evidence includes failing-before/passing-after HTTP tests, canonical
in-memory transaction rollback tests, a managed SQL-executor boundary test,
CSV row/cap/filter/quoting tests, and client capability/download/lifecycle tests.
A PostgreSQL rollback regression is included behind the integration build tag;
compilation is not execution and live database/browser acceptance remains
separate. No CSV formula policy or selection policy was changed.

## Published dependency alignment (2026-10-03)

The root and starter now select the completed public dependency releases:
GoAuth v0.5.1, GoNotify v0.6.0, GoInertia v0.11.0, GoUploads v0.10.1,
GoWebSocket v0.2.1, GoCache v0.2.2, and Outbox core/PostgreSQL backend v0.16.0.
GoDI v0.9.3 and GoLogger v0.1.0 remain unchanged. The public root module has no
replacements; the starter retains its explicit source-mode overrides.

GoInertia's protocol v2 default matches the embedded v2 client. No protocol
opt-in, host API, schema or generated asset change accompanies these pins.
GoNotify's validation still rejects remote plaintext HTTP before any request;
the host test follows the released error's uppercase `HTTP` spelling.

Validate the natural public dependency graph independently of the starter's
sibling overrides. Release readiness includes the complete PostgreSQL/Redis
integration suite, security and frontend advisory gates, and an anonymous
consumer with no dependency replacements. The first GoAdmin tag containing
these pins must remain distinct from the immutable v0.7.0 baseline.

The final candidate pins published GoUploads v0.11.0 (verified release commit
02fced8a67f5b07942764d7635aa08b06db3c5a7) without a root replacement. Built-in
strategy switches explicitly retain their existing policy for FileTypeAudio.
Canonical regeneration refreshes only the source/command comments in existing
mock files; their generated behavior is unchanged.

Cloud verification passed on the actual GoUploads v0.11.0 pin: canonical
preparation, `make check` (full Go race/coverage plus every resource gate and
production build), security-negative tests, `govulncheck`, high-severity npm
audit, and the clean local consumer. There are no reachable Go vulnerabilities;
three module-only advisories remain outside imported vulnerable packages. The
37 moderate npm records are the two already-reviewed advisories documented in
`docs/dependency-advisory-review.md`; no force upgrade or advisory suppression
was applied. PostgreSQL/Redis integration, isolated public-dependency consumer
and the eventual published GoAdmin tag are separate required release gates.

## Public upload migration runner correction (v0.9.1)

Real host TUS QA exposed an omitted upstream schema dependency: the public
GoAdmin facade installed core files but never mounted canonical GoUploads
lifecycle migrations, causing `upload_finalizations` lookup failure at
completion. Earlier reader-ingress integration did not exercise the durable
finalization key path. This patch appends the pinned canonical filesystem,
excluding only the base files migration, under an independent ledger. It does
not copy SQL into a host or change the GoUploads pin/API.

Regressions use the public facade and an empty app-migration fixture rather
than manually creating upload tables. Coverage includes fresh, repeat and
previous-GoAdmin migrations, auth-reset preservation, transactional conflict
rollback/retry, and real native TUS audio completion with finalizer execution.
Published release readiness still requires live PostgreSQL/Redis, clean Codex
review and the exact published-tag anonymous consumer gate.

## DataGrid initial response reuse and request lifetime (2026-10-07)

GET DataGrid responses now carry optional `meta.requestQuery`, copied from the
request that produced their rows. The empty string is a known unfiltered request;
absence means unknown provenance (manual/legacy responses and POST body loads).
Effective server defaults, validation fallbacks and page caps remain in the
existing pagination/sorting/filter metadata. Matching the query, including its
absence/presence and duplicates, avoids a second load without assuming that an
arbitrary `initialData` object applies to the current URL. Key order and equivalent
URL encoding do not force reloads. Legacy/mismatched nonempty URL intent still
loads once, preserving missing server-default parameters.

The composable accepts an initialData ref/getter as well as a plain response. It
initializes rows synchronously for Vue server rendering/hydration, starts network
work only on the client, and hydrates replaced server props under preserveState.
The current admin entrypoint is client-rendered from GoInertia's HTML data; this
change does not add a Node SSR server or replace the entrypoint.

Built-in users/admins/roles/permissions grids opt into `navigationMode="inertia"`:
Inertia owns history remount/prop restoration, so they do not start a competing
native popstate fetch. Generic/network-only grids retain `browser` navigation,
which loads Back/Forward URL intent once. Redirected refresh/delete/restore props
are authoritative; successful-visit callbacks no longer refetch the same grid.
Selection retains its existing page-load reset behavior. Export cancellation,
authorization, row actions, filters, sorting, pagination, rich-text rendering and
the public 100-row GET limit remain unchanged.

Each grid fetch has an AbortController. New requests, pending query edits,
endpoint changes, replaced server props and unmount invalidate the sequence and
abort old browser transport. An obsolete response is discarded before status/JSON
work; the post-decode guard remains for bodies already decoding or transports
that ignore abort. Intentional AbortError is quiet across browser realms. This
does not claim cancellation of PostgreSQL work.

Verification and bounded production before/after evidence are recorded in the
DataGrid integration acceptance report; no synthetic dev heap is used as the
production baseline. Independent review belongs to the coordinating task.

Bounded production check on the same 1,000-user synthetic fixture (100-row
public page, Chrome 154, fresh context per sample, one warmup plus five samples):
initial loads and filtered CSR/refresh dropped from two row GETs to one;
initial decoded page/data bodies fell from 296,862 to 185,427 bytes. Median
navigation-to-rows plus two frames was 1,264.4 ms before and 893.2 ms after
(with local outliers); full-page post-GC heap was 14.805 MiB and 14.030 MiB.
Separate API fetch/JSON medians were 12.6 ms and 14.1 ms, so this does not claim
a server-latency gain. Cached Back needed no row request; normalized Forward
needed one because its URL no longer matched cached response provenance.
Real supersession/unmount and delayed success/500/abort checks stayed quiet,
and delete/restore each used one redirected row GET. GET `limit=1000` still
falls back to 20; 1,000 rendered host rows were not forced past the public cap.

Validation passed tidy, scoped format, vet, 73-linter policy, repository
race/coverage tests, client manifest checks, all 510 frontend tests, ESLint,
Prettier, Vue type check and production asset build. Format excludes only the
pre-existing intentionally invalid generic-method audit fixture; no fixture
source or production limit was changed.

Independent review exposed an asynchronous Inertia history gap: between native
`popstate` and decrypted/resolved restored props, old requests or debounce timers
were still current and could rewrite the destination URL. A synchronous
invalidation-only listener now aborts/cancels those operations and pauses URL
synchronization until authoritative props arrive or the grid unmounts. Endpoint
changes preserve that pause; null-state hash-only events do not suspend the grid.
Four deterministic delayed Back/Forward regressions (in-flight request and pending
debounce, same and different path) failed before the fix and pass after it, with
no obsolete JSON parsing, destination URL changes, or duplicate data requests.
Production-host evidence above is CSR through `createApp`; SSR evidence covers
component server rendering and Vue hydration only.

The final history follow-up passed 39 affected lifecycle/URL/cancellation and
component-SSR tests, scoped lint/format, Vue type check and production asset
build. Earlier unchanged backend and bounded production-host results were
reused; no extra host/browser benchmark was run.

The final review also exercised new input during the suspended interval. New
search/sort/page requests and debounce scheduling now return while navigation
is suspended, and URL synchronization explicitly checks the reactive suspension
state. Authoritative props end the boundary. Three real-control interaction
regressions failed before the guard and pass after it; all 42 affected tests
pass, including interaction resuming after restored props.

## Typed host page resolver boundary

The generated extension registry retains each Vue SFC's inferred props. The
heterogeneous resolver accepts Vue `Component` plus the optional persistent
layout contract, rather than bare `DefineComponent` (whose default props reject
SFCs with required props). The only assertion lives at the Inertia 2 resolver
boundary: its declaration requires bare `DefineComponent`, while its runtime
passes the component unchanged to Vue with server-provided page props. No
wrapper, prop conversion, host type suppression, or dependency change is needed.

`npm run test:host-types`, also included in `npm run type-check`, materializes a
minimal host using the normal generator and checks the complete resource build
graph. It then requires compiler errors for incorrect/missing required host
props and a non-component loader payload. These are compile-time client checks;
they do not validate server-provided page props at runtime.
