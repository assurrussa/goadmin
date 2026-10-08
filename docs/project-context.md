# goadmin Project Context

Last updated: 2026-10-03.

## Current Role

`goadmin` is the standalone embedded admin subsystem for Go hosts. It uses
`github.com/assurrussa/goadmin` as its module path. The current checkout pins
`github.com/assurrussa/goauth v0.5.1` as the canonical auth/RBAC Runtime.
GoAuth and GoNotify resolve published v0.5.1 and v0.6.0 without replacements;
GoUploads resolves published v0.11.0.
GoNotify uses `transport.Transport` and NotifyHub rather than its
removed direct-delivery manager. Core and PostgreSQL Outbox pins are v0.16.0,
including the no-attempt `DeferAt` contract required for authorization outages.
Realtime resolves the
published `gowebsocket v0.2.1` dependency. GoCache resolves v0.2.2. GoInertia
v0.11.0 retains its protocol v2 default, matching the embedded frontend.

The module is intended to be consumed by clean host applications through a small
supported package list. Do not model future host integration as copying
`goadmin` internals into a product repository.

## Stable Public Surface

The machine-readable source of truth is
`reference/externalconsumer.SupportedPackages`.

Stable embedding packages:

- `github.com/assurrussa/goadmin/host`
- `github.com/assurrussa/goadmin/migrations`

Stable test support:

- `github.com/assurrussa/goadmin/hosttest`

Stable reusable feature packages:

- `github.com/assurrussa/goadmin/features/operations`
- `github.com/assurrussa/goadmin/features/access`
- `github.com/assurrussa/goadmin/features/jobs`
- `github.com/assurrussa/goadmin/features/uploads`
- `github.com/assurrussa/goadmin/features/queues`
- `github.com/assurrussa/goadmin/features/notifications`
- `github.com/assurrussa/goadmin/features/realtime`
- `github.com/assurrussa/goadmin/features/authmail`
- `github.com/assurrussa/goadmin/features/users`

Stable host-support toolkits:

- `github.com/assurrussa/goadmin/toolkit/datagrid`
- `github.com/assurrussa/goadmin/toolkit/formvalidator`

When this list changes, update `reference/externalconsumer/packages.go`,
`reference/externalconsumer/imports.go`, `README.md`, `RELEASING.md`, and this
file together.

## Host Facade Contract

`host/` is the supported embedding boundary.

`host.New(ctx, Config, Dependencies, modules...)` is the assembly boundary.
`Config.Admin` uses `AdminConfig`; core security config stays separate from
module configuration. A minimal core needs PostgreSQL, signing/token/journal
keys and CSRF config. Optional `Dependencies.Auth`, sessions and clients are
borrowed. A borrowed auth adapter must use the core pool. Empty modules mean
only auth, membership/current RBAC, sessions/CSRF, secure setup, basic profile
and the UI shell.

Module packages access/jobs/uploads/queues/notifications/realtime/authmail
explicitly mount built-in capabilities. Uploads, queues and notifications
require jobs; authmail uses GoAuth's own encrypted queue. `FeatureModule`
adapts existing users/operations/product features. `DefinedFeatureModule`
declares owned routes and literal `RouteNamespaces` for conflict validation.
Namespaces reserve the path and all descendants for every HTTP method. Built-in
access owns `/admins`, `/roles` and `/permissions`; uploads owns `/files` and
`/uploads`, queues owns `/queues`, notifications owns `/notifications`. Core
owns `/public` and `/test` in every environment. Realtime and authmail use
explicit claims. Parameter and catch-all intersections fail preflight
conservatively; GET claims include HEAD ownership. Claims are relative to the
admin mount; namespaces must be literal paths. Duplicate keys, missing/cyclic
dependencies and client bundle drift fail assembly. No dependent module is
implicitly enabled. `Config.Public` defaults to the ready embedded bundle;
custom bundles are verified against feature requirements automatically.

`Runtime.Run` starts server and owned workers once. Unexpected exits cancel,
drain and join sibling workers. `Close` closes only owned wrappers after joining;
`Readiness` checks running state, PostgreSQL and owned queue readiness.
JobsConfig.Borrowed never starts/drains/closes a host queue; JobsConfig.Owned
explicitly transfers worker supervision to Runtime. AuthMailConfig.BorrowedWorker
keeps already supervised host auth delivery external.

Notifications borrow a GoNotify `transport.Transport`. The existing
`host.NotificationManager` alias names this contract; its factory now takes
`host.NotificationConfig` (NotifyHub config) and returns `(NotificationManager,
error)`. Construction validates URL/key/security without contacting the gateway
or starting workers. Remote HTTP requires explicit `AllowInsecureHTTP`; default
remote connections use HTTPS. NotifyHub acceptance confirms persistence, not
provider delivery. Authmail remains independent: its sender must not persist
plaintext auth codes/tokens in a downstream queue.

Admin producers generate a dispatch ID once when staging a job and snapshot the
email sender, recipients and rendered title/body in the payload. Per-recipient,
per-channel gateway keys survive retries. Inbox rows use a unique
`(admin_id, dispatch_key)`; replay returns the existing ID without resetting read
state. GoNotify's job classifies permanent failures, quota `Retry-After`,
no-attempt authorization deferral and transient retry. Legacy jobs without a
dispatch ID use the stable outbox job ID supplied by the worker; execution
without either identity fails before persistence. Old external-delivery payloads
without an email sender fail permanently, so drain those candidate jobs before
upgrading. The payload's
metadata remains in the admin inbox and is not forwarded to NotifyHub.

Realtime authenticates `GET /ws` before HTTP 101 through the current browser
session and a typed `WithUserIDExtractor`. Stream identity is the admin projection
UUID. `realtime.New(nil)` creates an independent stream for each runtime;
shutdown drains its handler, closes that stream, then stops HTTP. A supplied
stream is borrowed. `Runtime.Close` also drains realtime when Run never started.
Hosts that stop the server directly use `Server.Shutdown(ctx)` to receive drain
errors; direct Fiber App shutdown invokes a bounded fallback hook.

`ServerConfig.ReadTimeout`, `WriteTimeout` and `IdleTimeout` configure HTTP
deadlines. Zero selects 10s/10s/120s; negative values fail assembly. These do not
replace the WebSocket library's frame, ping/pong and queue limits. The client
uses plain JSON text frames; the exact schemas and close-code contract are in
[`realtime-client.md`](realtime-client.md).

PostgreSQL is the default canonical browser backend and stores both Fiber
sessions and the encrypted owner/version journal. Redis remains explicit.
Session cleanup is bounded and cancellable. No account or module data is reset
when switching backend or disabling modules. GoAuth disabled delivery is explicit:
email/reset routes and controls are absent, password/profile flows retain audit.

The typed SDK exposes canonical Actor, guards, rendering, logger and extensions;
`Command[T]` binds typed commands after current authorization. Business services
belong to feature constructors. Declarative CRUD remains a TODO in the current
implementation notes.

## Optional users feature

The users feature keeps read access as the common route precondition; editing
requires `users/update`, and deletion/restoration require `users/delete`.
Existing email is readonly without authmail, and crafted unavailable email
changes are rejected before canonical or projection writes. Supported user
profile updates join the existing canonical transaction and projection SQL
executor; an unknown commit result is not automatically retried.

CSV export includes matching users independently of visible page size, capped
at 10,000 rows. Search, supported filters and sorting use the same user repository
query as the list. `X-Goadmin-Export-Truncated: true` reports a larger matching
set and the client warns after downloading; exports are not cached. The client
uses a same-origin file download rather than an Inertia page visit, using the
grid's synchronized URL state. Selection does not change export scope. CSV cell
values retain the existing literal policy, including formula-like strings.

## Storage And Migrations

`goauth` owns canonical auth/RBAC storage. `goadmin/migrations.Migrate` invokes
the canonical runner first, then installs goadmin projections, files,
queues, notifications, hash-only first-admin setup tokens, and the users
projection. `Migrate` detects v0.1 and returns
`postgres.ErrLegacySchemaRequiresReset` without deleting data.

`goadmin/migrations.Reset` requires `postgres.ConfirmResetAuthState` and is only
for an explicit development/test reset. It removes canonical auth state plus
admin/user projections but preserves unrelated files and queue tables. The old
`Run`, `RunWithConfig`, and feature-specific users migration facades are not in
the supported public surface.

The embedded profile uploader owns `DELETE /auth/profile/avatar`. The endpoint
requests deletion through the host-supplied gouploads service and schedules
the internal `admin_preview_detach` job. Its database update uses the observed
version and preview ID so a late detach cannot clear a different preview. The
default local uploader soft-deletes the file and enqueues the job in one
database transaction, then unlinks the physical file after commit. Preview binding rechecks owner, finalization and the observed binding under a
row lock in the shared transaction. Only linkage fields change; uploader/preset
metadata is preserved. Self profile edits write canonical profile only.

## Custom canonical upload strategies

`host.UploadsConfig.Strategies` adds named host policies to the existing full
`/files` handler; `features/uploads.Config` exposes the same field. Assembly
validates names against `[a-z][a-z0-9_-]{0,63}`, rejects nil/typed-nil strategies
and reserves `avatar`, `rich-text`, `default`, `image-uploader`. The module
snapshots the map and makes a fresh map for each assembly; the actual strategy
instances are borrowed and must support concurrent calls. Bootstrap validates
again before registering built-ins and the custom contexts.

The full `uploadFiles` client already accepts an open string category; `audio`
is forwarded without narrowing that contract. HTTP responses retain GoUploads'
numeric-string `fileType` (`"7"` for audio); the client preserves it. MP3/WAV
requires a custom context
with domain authorization and explicit MIME/extension/size constraints, plus
GoUploads v0.11.0+ in `ProcessingOriginalOnly` mode. Built-in upload policies
retain their old allowlists. The normal TUS complete, task status, finalization,
replacement and deletion routes are reused. Isolated `WithUploadTransport` and
`uploadPendingWithTus` remain quarantine/domain-finalizer tools, not an audio
pipeline. No media editor/player contract changes are included.

## Admin client bundle

The sidebar footer shows GoAdmin's version through the shared `adminInfo.version`
prop. It reads the GoAdmin module's Go build metadata, independently of the
host application's version. Local source replacements and builds without a
module version report `dev` (displayed as `разработка`); versioned replacements
report their actual selected version. Only the version is sent to the browser,
without build settings or local replacement paths. Older hosts that omit the
prop display `неизвестна`.

New UI modules publish source manifests and pass their paths through
`VITE_ADMIN_EXTENSION_MANIFESTS`. The builder materializes a deterministic
generated import tree, rejects duplicate extension/page keys, missing
or cyclic dependencies, symlinks, and source fingerprint drift, then emits a
path-free runtime manifest. Resolved source roots are also registered as
Tailwind v4 sources; a host does not maintain a separate extension CSS
safelist. `VITE_ADMIN_EXT_ROOT` is compatibility-only for one legacy host root.

The host integration and page lifecycle are described in
[`embedding-client.md`](embedding-client.md). The bundle validator checks the
entry JavaScript, stylesheet, and favicon in addition to extension fingerprints;
the resource build checks every asset referenced by the embedded templates.

The Vite entry and CSS filenames remain stable because existing server
templates address them directly. Hashed secondary chunks and the runtime
extension manifest are already emitted; changing the entry filename requires a
separate template asset-resolution migration.

The browser entrypoint uses the client `createInertiaApp` contract and mounts
the Vue application directly. An SSR `render` callback belongs only in a
separate server entrypoint and is intentionally absent from
`resources/src/js/app.ts`. Host development proxies must route the generated
`/.admin-extensions/` module path to the Vite service; it is part of the client
module graph rather than an admin backend route.

## First-admin setup

First-admin registration is fail-closed. The login screen publishes a
registration link only when a host-configured static setup token exists and no
administrator row exists. The form accepts that token explicitly; the host
facade validates at least 32 bytes and retains only its SHA-256 hash. Static
token consumption takes a PostgreSQL transaction advisory lock and rechecks
the database state while that lock remains held, so concurrent nodes cannot
create two first administrators. Canonical account, RBAC, setup-token consumption, admin projections and audit
join one GoAuth-managed database/sql transaction. Projection engines reject a
different pool. Unknown commit outcomes are typed and require reconciliation;
there is no automatic retry or compensating account deletion.


An existing canonical local account may become the first administrator under
the same email. Registration verifies its current password, preserves its
canonical subject, profile, credentials, and security version, then provisions only the
`administrations` membership and canonical Super Admin role. Accounts without
local credentials and password mismatches fail closed. Nullable optional
canonical profile fields remain nullable in `goauth` storage and are adapted to
empty display strings only when read through the legacy admin projection.

The stricter generated setup-link flow remains supported. Its interactive
token travels in a URL fragment, which Vue removes from the address bar before
POST; it is therefore absent from the initial HTTP request and ordinary access
logs.

The issue command validates `/dev/tty`, a `0600` regular file, or a protected
pipe/descriptor before creating a token. Non-interactive execution without a
secret channel stops first. Only the SHA-256 hash is stored, prior active tokens
are revoked, expiry is short, and any administrator row permanently closes
future issuance. The legacy admin seeder accepts only an explicit account and
has no built-in credentials.

The users projection table is part of the single core migration set; mounting
`features/users.NewFeature()` remains optional.

## Reusable Features

`features/operations` contributes the operations admin screen, menu entry, and
permissions. It requires host-owned adapters for route regeneration, sitemap
regeneration, frontend state, and frontend rebuild actions.

`features/users` contributes optional user-management screens, menu entry, and
permissions. It depends on the goadmin runtime's users projection repository and
canonical subject store. The core admin host should not mount `/users` unless
the feature is explicitly registered.

Feature packages are stable only when listed in `reference/externalconsumer`.
Do not treat `features/*` as a wildcard public namespace.

## Internal Ownership Boundary

These directories are goadmin-owned implementation details even when individual
packages are exported:

- `adminapp/`
- `bootstrap/`
- `di/`
- `domain/`
- `http/`
- `infrastructure/`
- `outbox/`
- `seeders/`
- `services/`
- `shared/`

Host applications own process lifecycle, project config loading, concrete
infrastructure instances, project-specific admin features, public-site auth/API,
and product domain code. They should not import goadmin internals to customize
built-in auth, roles, handlers, seeders, templates, or sessions.

## Verification Gates

Use the Makefile from the repository root:

- `make prepare`: mutating prep (`go mod tidy`, generation, formatting,
  lint-fix).
- `make check`: non-mutating verification after prep, including one combined
  Go race+coverage pass and resources build
  with non-mutating ESLint and Prettier checks.
- `make integration-check`: run all `integration`-tagged Go tests; requires
  reachable test PostgreSQL and Redis services and fails when unavailable.
- `make release-security-check`: focused negative secret-handling tests,
  `govulncheck ./...`, and the online npm audit; this is required by both local
  publish and post-publication release readiness, not by `make check`.
- `make test-race`: explicit five-run race stress diagnostic.
- `make cover-html`: explicit HTML coverage artifact.
- `make resources-audit`: online high-severity npm advisory gate, required by
  publish and release readiness but not repeated in the normal development
  loop.
- `make`: full preparation plus verification.
- `make externalconsumer-local`: shared-cache consumer replacing only GoAdmin;
  dependencies come from its declared graph, without sibling/version overrides.
- `make externalconsumer-candidates GOAUTH_LOCAL_PATH=../goauth GONOTIFY_LOCAL_PATH=../gonotify`:
  dev-only consumer using explicitly selected sibling candidates. Candidate
  paths have no defaults and at least one must be selected. `make check` excludes
  this probe; the root go.mod has no replacements.
- `make externalconsumer-public-deps-local`: anonymous pre-tag probe replacing
  only the current GoAdmin source, with no dependency overrides; mandatory in
  `publish-readiness`.
- `make externalconsumer-published VERSION=<tag>`:
  published clean-consumer probe after tags exist.
- `make release-readiness VERSION=<tag>`: full release gate
  plus published external-consumer validation.
- `make import-policy-site SITE_REPO=../site`: scan the current site repo as a
  host consumer and reject unsupported goadmin imports.

The Makefile reuses shared Go build/module/lint caches under
`$(HOME)/dev/projects/.cache/go`, with explicit overrides. The default full run
includes
`resources`; its clean install, deterministic extension-manifest and Vitest
tests, ESLint and Prettier checks, type checking, and Vite production build stay
local and reproducible. External-consumer probes disable `go.work` and ambient
`GOFLAGS` so they resolve the declared module surface independently.
The external consumer probe rejects selected module versions that differ from
the requested versions after Go module resolution. Registry-backed
`npm audit` remains a distinct mandatory release check.

## Boundary Tests To Notice

- `reference/externalconsumer/manifest_test.go` keeps the supported package list
  and compile manifest aligned; the generated clean-consumer probe executes the
  AuthAdapter and migration validation contract.
- `public_surface_docs_test.go` checks supported packages are documented and
  unstable internals are not advertised in public-surface sections.
- `compatibility_imports_test.go` prevents runtime regressions back to legacy
  goadmin auth packages and protects the goauth integration boundary.
- `host/import_boundary_test.go` protects the stable host facade from depending
  on unsupported host-facing imports.

When changing public packages, host integration behavior, migrations, or
goauth/goadmin boundaries, run targeted tests first and then the relevant
Makefile gate.

## Shared Context Status

On 2026-06-04, the shared wiki pages for `platforms/goadmin` and
`platforms/goauth` matched the current local direction at a high level:
published modules, clean-consumer readiness, `goadmin/host` as the admin facade,
and `goauth` as canonical auth/RBAC. The local repository remains the source of
truth for exact commands, package lists, env keys, migrations, and release
gates.

## Dependency ownership

Admin projection UUIDs are owned internally and exposed to audit consumers as
`host.AdminUUID`; uploads `host.UserID` and canonical auth subject IDs retain
separate types. JSON/Text and SQL UUID representations are unchanged. CSRF uses the admin identity directly; the WebSocket boundary explicitly
converts to its library types.
The public package list remains unchanged.

The Redis facade uses go-redis/v9 directly, retains address-keyed Ring hashing,
DB/timeout/auth defaults and scoped session reset. Hosts own client lifetime.
Incompatible gob browser sessions delete only the affected session key and
become fresh anonymous sessions; Redis read/delete errors still fail. Valid
payloads incur a preflight gob decode before Fiber's decode. Account data,
canonical auth and encrypted browser authority are not reset.

Source policy includes test/generated imports and rejects goshared/goredis and assurrussa/gofiber.
`make externalconsumer-anonymous-local` verifies declared dependencies with
fresh caches and no credentials, allowing only the explicit local candidate
set (GoAdmin plus the dependency paths explicitly supplied by the caller).
The public-deps-local mode allows
only GoAdmin and deliberately fails while published dependencies are unavailable
or incompatible. Published consumer verification allows no replacements and
does not force
a newer goauth to repair a broken dependency graph.

The HTTP runtime now belongs to goadmin; Fiber v3 remains the underlying server.
`host.Server` exposes App/Run/ReadShutdown and `host.ServerHandler` is the route
registration contract. `host.CSRFService` signs and checks proofs, with public
`host.CSRFRequest`, `host.CSRFToken` and `host.CSRFClaims` types. It has no generic
cookie middleware: bootstrap owns Origin checks, duplicate-input rejection and
opaque-session binding. Configure cookies through admin config/SessionStore;
CSRFConfig no longer accepts IsProd, CookieNameCSRFToken, CookieNameRefreshToken
or ExcludePaths. These are source contract changes requiring host adaptation.

## Canonical upload migrations

The public migration facade installs GoAuth, GoAdmin core, then the pinned
GoUploads `host.MigrationsFS` using the independent
`goadmin_uploads_goose_db_version` ledger. Only the already-owned base `files`
migration is excluded. Upload versions collide with unrelated admin versions,
so sharing a ledger would silently omit required tables. The Goose provider
disables host global Go migration registration and uses a PostgreSQL session
lock. `Reset` preserves upload data and its ledger. Existing independently
managed lifecycle schemas require reviewed ledger reconciliation; no schema
adoption is inferred from table existence. See `RELEASING.md` for duplicate
primary-file handling and migration rollout requirements.

## Browser upload request lifecycle

The canonical TUS client accepts optional `signal` and `requestTimeoutMs` in
`UploadRequest` and `PendingTusUploadRequest`. The default is 120 seconds of
inactivity for each create, HEAD, PATCH, or completion request; network upload
and download progress reset that timer. This is not a total-file deadline.
Timeout or cancellation aborts the underlying transport and fences late results.
Creation and completion are never automatically retried after uncertain results.
Cancellation cannot undo a completion already accepted by the server. Once a
completion request is dispatched, an ambiguous timeout/cancellation returns
`status: completion_unknown` and `uncertainCompletion` with the original session
and entity/file metadata. `onCompletionSession` lets a host retain that session
before dispatch even when UI cancellation fences the eventual result. Hosts
must block new upload sessions while this state is unresolved. The explicit
`reconcileUploadCompletion` helper replays only the same idempotent completion
endpoint; another uncertain result retains the session. It never creates an
upload or resends file bytes. A definitive completion rejection (including a
canonical upload-validation400, finalization-gone410, or structured error acknowledgment) returns `status: error`
without `uncertainCompletion`; transport/auth/quota failures, missing protocol sessions and malformed
acknowledgments remain uncertain. A replay that never dispatches cannot resolve
the earlier request.

Built-in upload widgets retain completion metadata before dispatch using
`useUploadCompletion`. Its same-tab registry survives Inertia remounts, and
sessionStorage restores metadata after reload. Keys include authenticated admin,
TUS endpoint, entity, upload context and stable field recovery key. File bytes stay only in memory; replay
after reload needs no original File. Unmount aborts transport without discarding
the fence; stale results cannot clear another entity's session or resurrect a
session already resolved. Storage failures fall back to in-memory remount
protection. No automatic expiry unlocks an unresolved completion. Custom host
widgets must explicitly retain/restore their own session and task state, or
adopt the composable; raw `uploadFiles` does not own UI recovery state.

A widget keeps that fence through dispatched/unknown, accepted queued,
processing, and completed-but-not-assigned states. Cancellation, teardown,
scope changes, failed insertion and transient polling errors do not release it.
Only a successful final image assignment or rich-text command mutation into the
matching field, a proven terminal rejection/failure, or explicit dismissal of a
confirmed result releases the exact session. A queued task's file/temporary URL
is not terminal assignment. Rich-text insertion must return success before
assignment events are emitted. Saving a host form is a separate host-owned
commitment: the widget's model update does not claim that the form was saved.

Acknowledged chunk progress uses validated `Upload-Offset` values. PATCH must
return 204 and advance by the exact chunk length. Missing, invalid, out-of-range
or backward offsets fail explicitly; repeated conflicts are bounded. These
checks also reject redirected HTML error pages instead of counting them as
uploaded bytes. Domain-owned pending uploads retain their separate finalization
boundary. Existing `uploadFiles` error-envelope behavior is unchanged.

### Hosts that already own upload migrations

`migrations.MigrateWithHostManagedUploads(ctx, cfg, db)` is the explicit facade
for an established host-owned upload schema and ledger. It installs canonical
auth and GoAdmin core, but does not execute, adopt, or stamp GoUploads migrations.
Before calling it, the host must verify and apply every migration from its pinned
GoUploads dependency, including future additions, and serialize its full pack.
The default `migrations.Migrate` remains unchanged and manages uploads itself.
Do not switch an existing canonical upload ledger to host management without a
separate reviewed ownership transition. This API itself moves no data or history.

## Host menu visibility

`host.Item.AnyPermissionKeys` is an additive, navigation-only OR policy shared by
sidebar filtering, menu-derived breadcrumbs, and menu-based login landing.
Nonempty lists require one nonzero key; nil/empty lists preserve legacy behavior.
When `PermissionKey` is also set, it remains mandatory (AND with the OR list).
Matching top-level items retain each field's first nonempty policy, without
unioning OR lists. Matching nested children keep the first entire item, preserving
legacy child deduplication; declare their complete policy in the first contribution. Registry descriptor/menu snapshots and nonempty menu merges copy the new
permission slices through nested items. No grants or endpoint guards are changed.
Explicit breadcrumb overrides remove disallowed links even when no menu routes
are allowed; plain-text crumbs remain. See the host example in `README.md`.

## Unpublished external-session candidate

The additive candidate host admission/validity API and its dependency/rollback
contract are described in [external-browser-admission.md](external-browser-admission.md). It retains the pinned GoAuth API, adds no public package/import path,
and does not enable a host SSO mode. Release readiness and the real RP consumer
acceptance remain required before publication.
