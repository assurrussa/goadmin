# Declarative CRUD resources

Import `github.com/assurrussa/goadmin/features/crud` to add a host-owned resource
with embedded list, detail, create and edit pages. Pass the module returned by
`crud.New[Row, CreateInput, UpdateInput]` to `host.New`. Standard resources need
no host Vue source or client rebuild. The opt-in `products` module in
[the starter](../examples/starter/products.go) is a complete PostgreSQL example.
Its optional Category select is independent of the read-only publication Status;
only the separately guarded Publish/Unpublish commands change that status.
Unpublish requires `products:unpublish` and returns a published record to draft;
both transitions use owner/revision CAS and transactional audit.

## Resource ownership

`Definition` declares a stable key/base path, title/menu, explicit read/create/
update/delete permissions, fields/columns and typed callbacks. Every callback
receives the current `host.Actor`; every lookup, count and selected export must
apply the same application scope. `Authorize` adds a server-side record check;
it cannot repair an unscoped repository. Creating a form/record uses the create
policy with an empty record. Permissions are registered, not automatically granted.

`Record[T]` contains string `ID`, string `Revision` and typed `Data`. IDs are
nonempty, up to 255 bytes, excluding `.`/`..`, commas, edge whitespace and
CR/LF/tab/NUL characters,
matching the existing selected-export transport. Spaces inside IDs, percent signs,
slashes and Unicode are encoded once without changing their identity. Mutation
callbacks must compare the expected revision atomically with the write and domain
audit in their own transaction. A pre-read comparison is insufficient. Return
`ErrConflict` if the version changed. Transaction commit errors may have unknown
outcomes: return or wrap `crud.ErrUnknown` so the UI explicitly asks the user to
check the result before retrying. No automatic replay is performed. State
transitions that require dedicated permissions must be enforced in every
callback that can perform an equivalent transition. Domain services, migrations,
transactions and audit remain application-owned; SDK construction performs no I/O.

Fields explicitly opt JSON keys into read/create/update. DTOs are flat structs
with explicit unique JSON tags and matching scalar Go types (including aliases
and nullable pointers). Quoted-number JSON tags, objects and arrays are not field
values. Standard dates use calendar-valid YYYY-MM-DD and numbers stay within the
finite JavaScript-safe magnitude. Supported kinds are text, textarea, number,
boolean, date (YYYY-MM-DD), fixed string select and custom scalar fields. Only
readable declared fields are serialized or exported. Undeclared/read-only input
is rejected. Keep secrets out of readable fields, including hidden table columns.
Optional selects accept the blank option; nullable pointer fields also accept
null. The server applies these checks to requests and readable callback output.
Optional write-only update fields (`Read: false`, `Update: true`, `Required: false`)
require pointer fields in the update DTO. Untouched fields are omitted from edit
submissions; omitted input and explicit JSON `null` both decode to nil. The host
update callback must treat nil as "leave the existing value unchanged" and a
nonnil pointer, including an empty string, zero or false, as an explicit
replacement. Required write-only fields must be explicitly edited on every update;
the server rejects an omitted required field instead of accepting an invented
empty/zero default.
Write-only edit controls start without a value. A required select offers a blank
placeholder until an explicit choice, including when only one option is available.
Unreadable values are never returned to populate the edit form.
`FieldErrors` contains safe messages only; other errors are sanitized and logged.
Detail pages preserve record text literally. Only declared option labels and
boolean UI labels are translated. Lists and detail pages display an absent boolean
as an em dash, distinct from false. Standard row actions and their tooltips follow
the current UI language; custom labels without a registered translation stay literal.

Date fields use the shared localized calendar: RU displays `DD.MM.YYYY`, EN
`DD/MM/YYYY`, with Monday-first weeks. Wire values remain `YYYY-MM-DD` without a
timezone conversion. Partial or impossible dates block submission and never reuse
the previous complete value. Admission expiry uses the same control with local
24-hour minute fields and keeps its existing timestamp conversion.

Single-record `Actions` declare a key, permission, optional confirmation and
callback.
They receive ID and expected revision, with no additional input body. `Authorize`
receives the action key as its operation. Action visibility is not authorization;
every command repeats current permission and record checks.

Built-in action confirmations use the shared localized admin dialog, including
operation-specific buttons and destructive styling. Cancellation, navigation,
actor changes and requester disposal never continue a pending action. Browser
unload and unsaved-navigation guards retain their separate existing behavior.
Commands without a confirmation use a neutral action question; only Delete
uses the default deletion question. The client resource type follows the same
optional confirmation contract as the Go action JSON.

## Routes and personal tables

The module reserves its complete base namespace. It owns list `/`, GET/POST
`/data`, `/create`, `/records/:id`, `/records/:id/edit`, POST collection,
PUT/DELETE record and POST `/records/:id/actions/:key`, plus the existing
`/table-preferences` and `/table-export` endpoints. All paths are relative to the
resource. Saves stay on the edit page when permitted; safe `returnTo` list state
survives a save and deletion. The list advertises Create only when the current
permission and resource creation policy allow it. Typed errors retain their
status on both GET and POST `/data`. The module persists redirect flash/errors
explicitly through `host.PersistFlash`,
because extension handlers finish before the core Inertia middleware. Input is
retained on handled Inertia errors. CRUD drafts are
not persisted through a full sign-in transition.
Revision conflicts also emit the existing global flash toast while retaining
the inline error and explicit reload action. One response is consumed once;
a separate response with the same message remains a new notification. No durable
notification inbox event or automatic mutation replay is created.

Personal tables use `crud.<key>` under the current administrator, with existing
CAS, 20-view/64-KiB limits, column reconciliation and URL precedence. Selection
holds up to 100 exact IDs, never persists, and follows existing query/owner reset
rules. CSV exports visible columns, either selected records (100) or filtered
records (10,000 with truncation feedback). Generic bulk actions are not included.

## Custom fields and bundle compatibility

Import `registerCrudField` and `CrudFieldProps` from `@admin-core/crud` in a
client extension. Register `extension.key/field-name` in `AdminExtension.install`.
Both segments start with a lowercase ASCII letter and contain only lowercase
ASCII letters, digits, dots, underscores or hyphens; exactly one slash is allowed.
The component consumes field/modelValue/error/disabled/readonly and emits
`update:modelValue` with a scalar or null. Declare the matching `Component` on
its Go field and the extension fingerprint in `Definition.ClientRequirements`.
Build/serve that extension using the existing embedding guide. Custom fields do
not replace server validation or create optional module dependencies.

Core bundles advertise `goadmin.crud`, API v1, with a generated renderer
fingerprint. Assembly rejects an old or mismatched bundle only when CRUD is
selected. CRUD descriptors use `host.FeatureDescriptor.ClientReferences` for the
core renderer and custom field dependencies. Multiple resources can share an identical
reference; conflicting fingerprints fail assembly. `ClientRequirements` still
declares unique extension ownership, so references cannot hide duplicate owners. Run
`node resources/scripts/crud-contract.cjs` after changing renderer source; the
build derives the same fingerprint. Generated assets and server ship together.

## Verification

Run the package and external-consumer tests, then `make check` and
`make externalconsumer-local`. The starter smoke selects Products explicitly
and tests real CRUD, commands, preferences, exports and revision conflicts in
both presentation packs. A focused rerun uses `node scripts/starter-smoke/crud-check.mjs
output/playwright/starter-smoke/<run-id> [default|topbar]` against an initialized,
owned stack. The starter integration-tag test requires `PRODUCTS_TEST_DSN` for
an isolated PostgreSQL server with create-database privileges; it creates and
drops its own test database and proves CAS, owner scope and audit rollback.
Local checks do not prove publication or deployment.
