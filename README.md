# goadmin

`goadmin` is a standalone embedded admin subsystem for Go applications. Host
projects integrate it through the stable `github.com/assurrussa/goadmin/host`
facade instead of copying admin internals.

Install the public release with `go get github.com/assurrussa/goadmin@v0.9.1`.
Its dependency graph uses GoAuth v0.5.1, GoNotify v0.6.0, GoUploads v0.11.0 and
GoWebSocket v0.2.1 without sibling replacements.

## Try the admin locally

The [standalone starter](examples/starter/README.md) runs the embedded admin,
PostgreSQL with Docker Compose. Redis is an optional profile; mail modules use
NotifyHub when explicitly enabled. Copy
`examples/starter/.env.example` to `examples/starter/.env`, fill every required
value, then run `docker compose up --build` from `examples/starter`. Open
`http://localhost:8080`; the login page offers first-admin registration until
an administrator exists. No administrator password is bundled. The starter
builds this checkout and its embedded UI together; it is a local example, not
evidence that an unpublished module version resolves remotely.
The explicit Docker source mode uses sibling checkouts. Run
`make externalconsumer-candidates GOAUTH_LOCAL_PATH=../goauth GONOTIFY_LOCAL_PATH=../gonotify`
to validate the selected source candidates; the [starter instructions](examples/starter/README.md)
describe the prerequisites. This local build does not prove that a clean public
consumer can resolve those dependencies.

The built-in interface is currently Russian-only. Navigation and the home
screen use the signed-in administrator's permissions; optional host features
appear only when registered and permitted.

Custom application sections with an omitted/zero `Section.Order` appear after Home and
before the built-in Access/System sections. Their effective order is 15;
Home, Access and System use 10, 20 and 40. Set a nonzero order to place a
section explicitly (including after the system sections). Equal orders retain
their current relative order, repeated contributions deduplicate links, and adding
items to a built-in section does not change that section's existing order.
Reserved keys `main`, `access` and `system` keep their default positions even
when a feature is their first contributor and the corresponding core module is
disabled (for example, Operations without Queues). Nonzero overrides still win.

Host UI extensions share the shell's manual light/dark selection. Tailwind
`dark:` utilities follow `.dark` and `[data-theme-mode='dark']`; they do not
independently override a saved selection with the OS preference.

## Runtime profiles

Go 1.27 is the minimum. The default core needs PostgreSQL and security keys;
Redis, uploads, queues, notification delivery and WebSockets are optional.
`host.New(ctx, Config, Dependencies, modules...)` validates the requested module
graph and bundle, then constructs an inert runtime. `Run` supervises the HTTP
server and owned workers; `Readiness` checks running state and dependencies;
`Close` cancels and joins owned work. Host clients and borrowed workers keep
their host lifecycle. An empty module list mounts login/logout, secure first-admin
setup, sessions, CSRF, current membership/RBAC checks, basic profile and the shell.

Explicit modules are `access`, `jobs`, `uploads`, `queues`, `notifications`,
`realtime`, `authmail`, plus `users`, `operations` and host features. Uploads,
queues and notifications require an explicitly mounted jobs module. Authmail
uses GoAuth's encrypted native notification queue; it has no generic jobs
requirement. Missing dependencies, duplicate keys, cycles, declared route
conflicts and client fingerprints fail assembly. Disabled capabilities have no
module services/routes and are hidden in the client. Uploads without realtime
use protected status polling.

Built-in modules reserve complete route namespaces. Core reserves `/public`
and `/test` in every environment; uploads reserves both `/files` and `/uploads`.
Host features declare
`ModuleDescriptor.Routes` (for example `GET /reports/:id`) or literal
`RouteNamespaces` (for example `/reports`, reserving all descendants and methods)
through `DefinedFeatureModule`. Preflight rejects possible parameter and wildcard
intersections conservatively. GET claims also reserve HEAD.

PostgreSQL stores browser sessions and the encrypted owner/version auth journal.
Redis is an explicit alternative. Security keys for the journal remain required
even when mail is disabled. Run `migrations.Migrate` before accepting traffic;
existing migration history and module data are preserved when modules are disabled.
Production hosts configure PostgreSQL transport security and durable upload
storage. The built-in upload delivery is private to the current administrator;
public-site media needs a separately authorized host route. The default in-memory
stream is local to one process.

## Public asset transport

GoAdmin serves its owned `/public` assets with identity content encoding. They
are no longer application-compressed, avoiding a fasthttp streaming-compression
reader-lifetime race when clients abandon a response. Decoded asset bytes, MIME
types, existing routing/authentication boundaries and cache policy are unchanged.
Complete uncached transfers can be larger (the embedded app.js is 361,733 bytes).
An independently configured reverse proxy may compress them; none is assumed.
Dynamic responses outside this namespace and other request methods retain their
existing compression. GET/HEAD fallthrough within `/public` also uses identity,
including authentication and error responses; their status, body and routing stay
unchanged.

Ordinary clients offering gzip, Brotli, deflate or zstd also accept identity by
default. An explicit identity exclusion (`identity;q=0`, or `*;q=0` without an
accepted identity override) receives an empty, non-cacheable 406 for an existing
asset, including HEAD/conditional requests. Missing assets and other errors still
follow the original routes and authentication checks. `Vary: Accept-Encoding`
is retained, as are request `no-transform` semantics. Byte ranges remain disabled
on this mount: range requests receive the full identity representation.

This is a policy for the owned `/public` GET/HEAD namespace, not an upstream
fasthttp repair or a guarantee for host-defined streaming routes. Host middleware
must not rewrite another namespace into `/public` after compression selection.

## Stable public API

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
- `github.com/assurrussa/goadmin/toolkit/datagrid` ([guide and runnable example](toolkit/datagrid/README.md))
- `github.com/assurrussa/goadmin/toolkit/formvalidator`

`reference/externalconsumer` is the machine-readable source of truth for this
surface.

## Ownership Direction

`goadmin` owns the embedded admin subsystem:

- admin auth and session flows;
- admin roles, permissions, and guards;
- built-in admin HTTP/API handlers;
- built-in admin UI/templates/assets;
- bootstrap/runtime wiring;
- one-time first-admin setup and explicit development seeding flows.

Host projects own infrastructure/config integration, process wiring,
project-specific admin features, and public-site auth/API/domain code.

Feature-owning hosts may pass both `host.WithPermissionDefinitions` and
`host.WithRolePresets` to `host.NewRolesSeed`. The feature owns its concrete
policy; the facade delegates canonical persistence and idempotent updates to
`goauth`.

## Auth Runtime and migrations

`host.New` builds an owned GoAuth adapter by default. A host that already owns
one may pass `Dependencies.Auth`; it must use the same PostgreSQL pool. Borrowed
auth workers require explicit lifecycle ownership in `AuthMailConfig`.

```go
admin, err := host.New(ctx, host.Config{
    Admin: host.BuildConfig(adminInput, host.PathDefaults{}),
    Server: host.ServerConfig{Addr: adminAddr, AllowOrigins: []string{adminURL}},
    Auth: authConfig,
    CSRF: host.CSRFConfig{AppDomain: adminDomain, SecretKey: csrfSecret,
        AllowedOrigins: []string{adminURL}},
    FirstAdminSetupToken: setupToken,
}, host.Dependencies{Database: db}, access.New())
if err != nil { return err }
defer admin.Close()
return admin.Run(ctx)
```

Omit `access.New()` for the minimal core. `features/access` supplies that module;
all feature packages use the same GoAdmin version. Existing `host.Config` admin
fields moved to `host.AdminConfig`; the new `host.Config` describes assembly.
Product features mount with `host.FeatureModule(feature, dependencies...)` or
`DefinedFeatureModule` with route claims. `host.Command[T]` combines current
permission checks, typed body binding and a canonical actor. Keep business
services in feature constructors; reuse the generic datagrid toolkit for lists.

`migrations.Migrate` installs canonical goauth storage, the goadmin host schema
(including the user projection), then canonical GoUploads lifecycle storage.
GoUploads uses its own `goadmin_uploads_goose_db_version` ledger; core retains
ownership of the shared base `files` table. Apply migrations before accepting
upload traffic or starting upload workers. See [upload migration upgrades](RELEASING.md#upload-migration-upgrades)
for existing-host constraints. A detected v0.1 auth schema
returns `postgres.ErrLegacySchemaRequiresReset` without deleting data. The
typed `migrations.Reset(..., postgres.ConfirmResetAuthState)` path is only for
an explicit development/test reset; it removes auth state and admin/user
projections while preserving files, upload sessions/finalizations/deletion plans,
the upload migration ledger, and queue tables.

## Commands

```sh
make
make prepare
make check
make test-race
make cover-html
make externalconsumer-local
make externalconsumer-candidates GOAUTH_LOCAL_PATH=../goauth GONOTIFY_LOCAL_PATH=../gonotify
make externalconsumer-public-deps-local
make publish-readiness VERSION=<next-goadmin-tag>
make release-readiness VERSION=<published-goadmin-tag>
make import-policy-site SITE_REPO=../site
```

The default full run includes `resources`, so Node manifest tests, Vitest UI
tests, `vue-tsc`, and the Vite admin UI build must pass. Resource tooling
requires Node.js 22.13+ on the 22.x line or Node.js 24+.

Verification is local by default. Hosted CI is manual-only, with no automatic
push or pull-request runs. See [local checks and manual CI](docs/ci.md) for
tool versions, disposable PostgreSQL/Redis setup, and the proven CI baseline.

This checkout pins published `goauth v0.5.1` and `gonotify v0.6.0`, including
the shared transaction and optional-delivery API. Uploads resolve
`gouploads v0.11.0` and WebSockets resolve `gowebsocket v0.2.1`. Outbox core and
its PostgreSQL backend both resolve `v0.16.0`, and GoCache resolves `v0.2.2`.
GoInertia `v0.11.0` keeps protocol v2 as the default for the embedded v2 client.
The root module has no local replacements.

GoNotify v0.5.0 uses durable transports instead of the old direct-delivery
manager. Configure notifications explicitly:

```go
manager, err := host.NewNotificationManager(host.NotificationConfig{
    BaseURL: "https://notify.example.com", ProjectKey: projectKey,
})
if err != nil {
    return err
}
modules = append(modules, host.NotificationsModule(manager))
```

`host.NotificationManager` now implements GoNotify's `transport.Transport`;
the factory takes typed configuration and returns an error. A host may supply
another transport with the same durable acceptance and idempotency guarantees.
HTTP clients remain borrowed. Admin notification jobs preserve transport errors
and retry identities; PostgreSQL inbox deduplication is installed by migrations.
GoAuth authmail keeps its separate sender contract: do not send plaintext auth
codes or tokens into NotifyHub's durable message store.

`make check` runs source and UI verification without a consumer that selects
sibling checkouts. `make externalconsumer-local` replaces only GoAdmin and
resolves its declared dependencies with shared caches; it ignores candidate path
and GoAuth version overrides. `make externalconsumer-candidates` is a separate
dev-only check requiring explicit dependency paths as shown above.
`make externalconsumer-anonymous-local` repeats that explicitly selected
candidate resolution without credentials or existing caches. Candidate
replacements are not publication evidence.
Before tagging, `make externalconsumer-public-deps-local` replaces only this
GoAdmin checkout and resolves dependencies exactly from its declared graph in
the same anonymous environment. `publish-readiness` requires this check; it
remains blocked until compatible dependencies are publicly resolvable.
After release, `make externalconsumer-published VERSION=<tag>` requires only
that goadmin tag and checks its natural dependency graph without replacements
or a forced goauth version. goshared, goredis and the assurrussa/gofiber wrapper are forbidden.

The optional realtime module serves authenticated `GET /ws` using plain JSON
text frames. See the [WebSocket client contract](docs/realtime-client.md) for
event schemas, close codes, HTTP timeouts and shutdown ownership.

`host.AdminUUID` is the admin projection UUID exposed by login audit; it remains
separate from canonical goauth subject IDs and `host.UserID` (uploads).
`host.RedisClient` uses the direct go-redis UniversalClient contract. Redis
instances are owned and closed by the host. The default factory preserves its
address-keyed Ring topology. Incompatible old browser sessions require login
again; account data and database UUIDs are unchanged.

## Host actor and client extensions

`host.App.CurrentActor` exposes only the canonical admin and subject IDs needed
by a host feature. Roles, permissions, session data, credentials, and mutable
admin records remain internal.

Transport-neutral features construct `host.NewSubjectPermissionChecker` from
the same PostgreSQL client and transaction manager used by goadmin. Pass that
checker through `host.Dependencies.SubjectPermissions` so goadmin and embedded
features share one roles cache and cancellation lifecycle. `CheckPermission`
accepts only canonical subject ID plus a typed permission key and fails closed;
it never accepts permissions copied from a session.

Admin UI extensions can be supplied by one or more source manifests through
`VITE_ADMIN_EXTENSION_MANIFESTS` (using the operating-system path delimiter).
Every manifest uses `formatVersion: 1` and declares extensions with a stable
`key`, `apiVersion: 1`, relative `sourceRoot`, SHA-256 source `fingerprint`, and
optional `requires`. The build rejects missing, duplicate, or cyclic
dependencies, duplicate feature or page keys, source symlinks, and fingerprint
drift before Vite starts. It emits `dist/admin-extensions.manifest.json`
without source paths. The materializer also registers every resolved
`sourceRoot` with Tailwind v4, so extension-only utility classes are present in
both development and production CSS.

Server features declare matching `host.ClientExtensionRequirement` values.
Use `host.BuildClientBundleValidation` and pass the result to
`host.Config.Public`; assembly validates the embedded manifest and rejects a missing or drifted admin
bundle. `VITE_ADMIN_EXT_ROOT` remains a compatibility path for a single legacy
host root; new modules should publish source manifests.

For a complete host integration, page lifecycle, asset and CSRF build contract,
see [Embedding and extending the admin client](docs/embedding-client.md).

## Custom upload contexts and audio

`uploads.New(host.UploadsConfig{...})` accepts an optional
`Strategies map[string]host.UploadStrategy`. Register a host-owned strategy such
as `meditation-audio` to add a context to the existing `/files` handler:

```go
uploadConfig.Strategies = map[string]host.UploadStrategy{
    "meditation-audio": audioStrategy,
}
modules = append(modules, uploads.New(uploadConfig))
```

Names must match `[a-z][a-z0-9_-]{0,63}`. Empty/invalid names, nil (including
typed-nil) strategies, and the reserved names `avatar`, `rich-text`, `default`
and `image-uploader` fail assembly. The module snapshots the map; strategy
instances remain host-owned and must be safe for concurrent requests.

The full client helper `uploadFiles` accepts `fileCategory: 'audio'`, a matching
`context: 'meditation-audio'`, and the feature's entity type/ID. It uses the normal
TUS create/chunk/complete flow and `/files/tasks/:id` status endpoint. The host
strategy must authorize that entity in `CanUpload`, constrain `.mp3`/`.wav` and
MIME/size policy in `GetConfig`, and use GoUploads v0.11.0 or newer configured
with `ProcessingOriginalOnly` (`host.NewLocalUploads` already uses this mode).
Completion/finalization, ownership, replacement
and deletion retain the canonical upload pipeline. Audio is not transcoded.

Audio remains opt-in: built-in image, video, avatar and generic policies are
unchanged. `uploadPendingWithTus` remains a separate quarantine-only helper for
a domain-owned finalizer; it is not part of the full admin audio path. No audio
editor/player is added by this API.

## Public rich text and isolated upload transports

The public admin form surface exports `RichTextEditor`, `FormRichTextField` and
their constrained contract types. Embedded features may declare allowed
nodes/marks/features, provide an async typed media picker and receive structured
invalid-content callbacks. CMS media is represented by canonical typed nodes;
the editor does not accept arbitrary HTML as a portable contract.

Set a stable, unique `uploadRecoveryKey` on each `RichTextEditor` or
`ImageUploader` field within an entity, and keep it unchanged across navigation.
`FormRichTextField` and `FormImageUploader` forward `uploadRecoveryKey` or `name`. This key is local
recovery identity, not a server upload strategy/context. Without a key, legacy
widgets retain a shared duplicate-prevention fence, but a remounted or neighboring
widget requires an explicit “Insert into this field” / “Use image in this field”
action after reconciliation instead of guessing the original destination. The
recovery fence remains until explicit insertion/assignment or dismissal; navigating
before that choice restores the same session for another safe check.

`host.WithUploadTransport` mounts an additional permission-guarded TUS surface.
`UploadTransport.TusStore` is optional: nil reuses the canonical admin upload
store, while a supplied store creates an isolated protocol surface for a CMS or
another feature. The transport never exposes generic file-management or delete
routes. This separation lets CMS finalization own quarantine promotion without
changing built-in admin uploads.

`host.Extension.WithPublicRegister` mounts narrowly declared host callbacks
before admin authentication middleware. It is intended for independently
authenticated internal-service routes; callers remain responsible for their
own fail-closed authorization.

## First administrator

A host may configure a stable operator-managed token (for example, from a
deployment secret) through the supported facade:

```go
setupToken, err := host.NewFirstAdminSetupToken(os.Getenv("GOADMIN_FIRST_ADMIN_SETUP_TOKEN"))
if err != nil {
	return err
}

config.FirstAdminSetupToken = setupToken
admin, err := host.New(ctx, config, dependencies, modules...)
```

An empty value disables this path; a configured value must contain at least 32
bytes. The facade retains only its SHA-256 hash. While no administrator exists,
the login screen exposes the registration link and the form accepts the raw
token. The cross-node close check is serialized with the goadmin membership
write by a PostgreSQL advisory lock, so the static token becomes unusable when
the first admin membership commits. Canonical account, roles, setup-token consumption, membership and audit join
one managed SQL transaction. Failed commands roll back together. Unknown commit
outcomes require reconciliation before retry.

If the submitted email already belongs to a canonical local account, the form
verifies that account's current password and provisions only its admin
membership and Super Admin role. The canonical subject, profile, password, and
security version remain unchanged. A missing local password or a password
mismatch fails closed; the flow never creates a duplicate subject or resets
existing credentials. Optional canonical profile fields may remain `NULL`; the
string-valued admin projection reads them as empty display values without
rewriting canonical storage.

The short-lived generated setup link remains available for operators who need
a separately issued one-time secret. Run `goadmin/migrations` before issuing
it. The goadmin-owned command refuses to run after any administrator has
existed:

```sh
GOADMIN_DATABASE_DSN='<postgres-dsn>' \
  go run ./cmd/goadmin-setup-token \
  --setup-url 'https://admin.example.test/auth/register'
```

Interactive mode writes the complete setup URL once to `/dev/tty`. The token is
placed in the URL fragment, so it is not sent in the initial HTTP request or
captured by ordinary access logs. It is never written to stdout or stderr.

For CI or an operator workflow, preconfigure a protected output file or file
descriptor. Without one, non-interactive mode exits before token creation:

```sh
umask 077
GOADMIN_DATABASE_DSN='<postgres-dsn>' \
  go run ./cmd/goadmin-setup-token \
  --non-interactive \
  --secret-file './first-admin.setup-token' \
  --setup-url 'https://admin.example.test/auth/register'
```

The secret channel must have mode `0600`. Normal output contains only the setup
page URL without token, expiry, or request metadata. The database stores only a
SHA-256 token hash. Token consumption, canonical GoAuth account/role writes, goadmin membership
and firstadmin action audit share one managed PostgreSQL transaction.

`host.NewAdminSeed` no longer supplies default credentials. A development
fixture that deliberately uses the seeder must pass one explicit
`host.AdminSeedAccount`; production bootstrap should use the setup-token flow.

See [RELEASING.md](RELEASING.md) for the release checklist.
The source is under the [MIT license](LICENSE). Report security issues using
the private route in [SECURITY.md](SECURITY.md); changes prepared but not yet
released are listed in [CHANGELOG.md](CHANGELOG.md).

The HTTP server and CSRF proof types now belong to goadmin (`host.Server`,
`host.ServerHandler`, `host.CSRFService`, `host.CSRFRequest`, `host.CSRFToken`,
`host.CSRFClaims`). Fiber v3 is retained directly. The signer has no standalone
cookie middleware; install goadmin for its complete CSRF request protection.
Legacy CSRFConfig cookie/middleware fields are removed; configure the admin
cookies through BuildConfig and SessionStore. See RELEASING.md for host migration.
