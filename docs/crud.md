# Declarative CRUD resources

Import `github.com/assurrussa/goadmin/features/crud` to add a host-owned resource
with embedded list, detail, create and edit pages. Pass the module returned by
`crud.New[Row, CreateInput, UpdateInput]` to `host.New`. Standard resources need
no host Vue source or client rebuild. The opt-in `products` module in
[the starter](../examples/starter/products.go) is a complete PostgreSQL example.
Its optional Category select is independent of the read-only publication Status;
only the separately guarded Publish/Unpublish commands change that status.
Unpublish requires `products:unpublish` and an operator's reason. It returns
the product to draft and records the reason in the same transaction.

## Review records created by the application

GoAdmin v0.33.0 adds review queues and `ListColumns`. Select
`ADMIN_MODULES=requests` to [run the starter queue](../examples/starter/README.md#review-a-request)
from this checkout or from the matching tag once published.

Use `crud.NewWithActions(crud.WithActionsDefinition[Row]{...})` for a shared queue
such as [the starter requests](../examples/starter/requests.go). The application
supplies an actor-scoped `List` (including its total), `Get`, `Authorize`, a
read permission and at least one named action with its own permission. Return
`crud.Record[Row]{ID: id, Revision: version, Data: row}` from the reader.
The embedded UI provides search, personal views, a detail page and the declared
single-record commands. List queries use the same scalar filter types and
100-record limit as `NewReadOnly`. Creation of the business records belongs
to the application.

Set an action's `Placement` to `crud.Detail` for a decision that requires opening
the record, or `crud.Row` for a command in an individual table row. The zero value
and `crud.Both` show the command in both places. The server checks visibility for
the actor and record. Placement affects where an authorized command is offered.

On submission GoAdmin checks permissions and blocks role preview. It loads the
record in the actor's scope and calls `Authorize(ctx, actor, crud.Read, record)`.
For a well-formed request it compares the supplied revision, calls `Authorize`
with the command key, validates any action parameters, then invokes the host.
In `NewWithActions`, confirming a rejection with an empty reason reaches the server.
An operator who still has access receives a conflict for a stale revision and a
field error for a current revision. The operator reviews the new state before any
further submission. The host must compare the revision and business state again
in the transaction that records its decision and audit.

### Register approve and reject commands

The following declaration comes from the PostgreSQL starter. Its `requestData`
row has a subject, status and optional rejection reason. The `rejectRequestInput`
DTO receives a required, trimmed `reason` of up to 500 Unicode code points.
The application supplies the repository and policy functions in
[requests.go](../examples/starter/requests.go) and passes the returned module to
`host.New`. Grant `requests:read` and the relevant decision permission to each
operator role.

<!-- source: examples/starter/requests.go#requestsModule -->
```go
func requestsModule(db *sql.DB) (host.Module, error) {
	repo := requestRepo{db}
	permission := func(action host.PermissionAction) host.PermissionKey {
		return host.NewPermissionKey("requests", action)
	}
	return crud.NewWithActions(crud.WithActionsDefinition[requestData]{
		Key: "requests", BasePath: "/requests", Title: "Заявки", Permission: permission("read"),
		Menu: host.Menu{Sections: []host.Section{{Key: "requests", Title: "Заявки", Items: []host.Item{
			{Name: "Заявки", Href: "/requests", Icon: "content", PermissionKey: permission("read")},
		}}}},
		Fields: []crud.Field{
			{Key: "subject", Label: "Тема", Type: "text", Read: true},
			{Key: "status", Label: "Статус", Type: "select", Read: true, Options: []crud.Option{
				{Value: "pending", Label: "Ожидает решения"}, {Value: "approved", Label: "Одобрена"}, {Value: "rejected", Label: "Отклонена"},
			}},
			{Key: "reason", Label: "Причина отклонения", Type: "textarea", Read: true},
		},
		ListColumns: []crud.ListColumn{{Key: "subject", Sortable: true}, {Key: "status", Filterable: true}},
		List:        repo.list, Get: repo.get, Authorize: authorizeRequest,
		Actions: []crud.Action[requestData]{
			{Key: "approve", Label: "Одобрить", Confirmation: "Одобрить эту заявку?", Permission: permission("approve"), Placement: crud.Detail, Run: repo.approve},
			crud.ActionWithParams(crud.Action[requestData]{Key: "reject", Label: "Отклонить", Confirmation: "Отклонить эту заявку?", Permission: permission("reject"), Placement: crud.Detail},
				[]crud.ActionParam{rejectionReason}, repo.reject),
		},
	})
}
```
<!-- /source: examples/starter/requests.go#requestsModule -->

### Choose list columns from the fields

Supply an ordered `ListColumns` slice in any CRUD definition to reuse the labels,
types and select options of readable fields. Example from the Products catalog:

```go
ListColumns: []crud.ListColumn{
    {Key: "name", Sortable: true},
    {Key: "category", Filterable: true},
},
```

A column may override its label, grid type, format, filter placeholder or badges.
A select cell displays its option label, including when filtering is off.
Set a badge label to override the option label for an individual value.
Filtering a select uses the option value on the client and server. Fields
omitted from the list remain available according to their read policy. To retain
existing personal views, keep the resource key, column keys, sortability, filterability
and option values stable. An existing explicit `Columns` declaration is also
supported. Choose one of the two representations for each resource.

The standard View and Edit links carry the current list query. Opening a record
in another tab or copying its link preserves the return to the same search,
filters, sort and page, including after a decision from the detail page. A return
target must match the resource's exact list path and fit in 4096 bytes after query
encoding. Otherwise the record links back to the base list.

## Connect a read-only section

Use `crud.NewReadOnly` for a journal, directory or other resource with a list
and a detail page. The [starter journal](../examples/starter/product_events.go)
shows the declaration, PostgreSQL queries and record policy together.

1. Define a row DTO and choose a stable record ID. Return
   `crud.ReadOnlyRecord[Row]{ID: id, Data: row}` from your callbacks.
   Declare each displayed field with `Read: true` and a JSON key. Keep internal
   fields such as the owner available to your policy only. The constructor
   rejects `Create`, `Update` and `Required` flags on read-only fields.
2. Build a `crud.ReadOnlyDefinition[Row]` with the key, path, title, read
   permission, menu, fields, columns and callbacks. `List` receives the actor
   and `datagrid.Filtered`. Use the same ownership and search predicates for
   the page and count. Use a stable secondary sort key. `Get` looks up one ID
   within the actor's scope and returns `crud.ErrNotFound` for an absent record.
   `Authorize` checks the returned row for both operations. A rejected row
   fails the whole page. A count obtained without the ownership predicate can
   still disclose other users' activity.
3. Pass the module returned by `crud.NewReadOnly(definition)` to `host.New`.
   Use its read permission on the menu item and grant that permission to the
   intended operator role. The embedded pages provide filters, personal views
   and navigation back to the originating list. The resource exposes neither
   domain writes nor CSV export. Each list request reads at most 100 records.
4. In the starter, enable `ADMIN_MODULES=products,product-events` to generate
   and inspect events. Create and publish a product, then open
   `/product-events` as the same operator with `product_events:read`. Search
   for its exact number, open an event and return to the filtered list. Delete
   the product and search again. A second operator with the read permission
   sees only their own events. The journal can also be enabled on its own.

For example, this declaration displays one field from the starter's event DTO.
The callbacks own the queries and the record policy. Append the returned
module to the modules passed to `host.New`.

```go
module, err := crud.NewReadOnly(crud.ReadOnlyDefinition[productEvent]{
	Key: "product-events", BasePath: "/product-events", Title: "Product activity",
	Permission: host.NewPermissionKey("product_events", "read"),
	Fields: []crud.Field{{Key: "createdAt", Label: "Time (UTC)", Type: "text", Read: true}},
	Columns: []datagrid.Column{{Key: "createdAt", Label: "Time (UTC)", Type: "text", Sortable: true}},
	DefaultSort: "createdAt", DefaultOrder: "desc",
	List: repo.events, Get: repo.event, Authorize: authorizeProductEvent,
})
```

New here? Follow [your first CRUD resource](first-crud.md) for the runnable
setup, code map and browser acceptance. This page is the detailed contract reference.

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
Field keys start with an ASCII letter and contain up to 64 letters, digits or
underscores. `id`, `constructor`, `prototype` and `hasOwnProperty` are reserved;
the last key is intercepted by Vue's reactive model even when it holds a value.
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
callback. An ordinary action takes only the record ID and expected revision.
`Authorize` receives the action key as its operation. The server checks both
the current permission and the scoped record.

Use `crud.ActionWithParams` when the operator must supply additional input.
The starter uses this pattern to drive the action dialog, the HTTP binder
and the typed callback.

```go
type UnpublishInput struct {
    Reason string `json:"reason"`
}

reason := crud.ActionParam{
    Key: "reason", Label: "Reason", Type: "textarea",
    Required: true, TrimSpace: true, MaxLength: new(500),
}
action := crud.ActionWithParams[Product, UnpublishInput](
    crud.Action[Product]{Key: "unpublish", Label: "Unpublish", Permission: permission},
    []crud.ActionParam{reason}, repo.unpublish,
)
```

The callback receives `(context.Context, host.Actor, id, revision, UnpublishInput)`
and returns an error. Each parameter matches an explicit, case-sensitive JSON
tag on a string field of the input struct. The constructor checks this mapping
at startup. An `omitempty` tag still allows the parameter to be required.
For direct Go writes use `reason.Normalize(input.Reason)`. It trims Unicode
`White_Space` when requested, checks required text and counts code points.
Text parameters reject NUL, which PostgreSQL cannot store in a text column.

POST to `/products/records/{id}/actions/unpublish` with JSON such as
`{"_revision":"7","params":{"reason":"Price needs verification"}}`. A successful
parameterized action returns 204. A field error returns 422 with messages in
`errors.reason`. A stale revision returns 409. The dialog keeps the raw draft
for correction or explicit confirmation against the newly read revision.

Revision-only action confirmations use the shared localized admin dialog, including
operation-specific buttons and destructive styling. Cancellation, navigation,
actor changes and requester disposal never continue a pending action. Browser
unload and unsaved-navigation guards retain their separate existing behavior.
Commands without a confirmation use a neutral action question; only Delete
uses the default deletion question. The client resource type follows the same
optional confirmation contract as the Go action JSON.

## Routes and personal tables

Every resource reserves its base path and serves its list, GET/POST `/data`,
GET `/records/:id` and personal `/table-preferences`. All paths below are
relative to that base.

| Constructor | Additional routes |
| --- | --- |
| `NewReadOnly` | The shared read and personal-view routes are the complete set. |
| `NewWithActions` | POST `/records/:id/actions/:key` for each declared command. |
| `New` | GET `/create`, POST to the collection, GET `/records/:id/edit`, PUT/DELETE `/records/:id`, `/table-export` and POST for each declared command. |

On edit, Save and Enter keep the editor open when permitted. Save and
return to list restores the originating search, filters, sort, page size and page
after the server confirms the same edit attempt. Direct edit links return to
the resource list. Deletion also preserves the validated list location.
The list advertises Create only when the current
permission and resource creation policy allow it. Typed errors retain their
status on both GET and POST `/data`. The module persists redirect flash/errors
explicitly through `host.PersistFlash`,
because extension handlers finish before the core Inertia middleware. Input is
retained on handled Inertia errors. CRUD drafts are
not persisted through a full sign-in transition.
On a revision conflict, the editor compares its original values and draft with
the current authorized values. It combines independent edits and asks the
administrator to choose a value for each field changed differently by both
writers. A write-only replacement stays in the tab and requires explicit
confirmation before reuse. Write-only input is excluded from redirect feedback.
The next explicit save submits the reconciled draft against the current
revision. A further concurrent update starts the comparison again. If either
version omits a readable edit field, the editor keeps the draft and offers a
reload because it cannot compare the missing value. Opening or reloading a
record with such a field requires an explicit value before the next save.

Personal tables use `crud.<key>` under the current administrator, with existing
CAS, 20-view/64-KiB limits, column reconciliation and URL precedence. In full
CRUD, row selection is temporary, holds up to 100 exact IDs and resets with the
query or owner. CSV exports visible columns, either selected records (100) or filtered
records (10,000 with truncation feedback). Generic bulk actions are not included.
Filterable columns cannot use `page`, `limit`, `search`, `sortBy`, `sortOrder`,
`scope` or `columns`. These names control pagination, search, sorting and CSV
export. They remain valid for fields and non-filterable columns. Both `ids` and
`returnTo` are valid filter keys. A declared `returnTo` filter stays part of the
list query when an operator opens a record or returns from it.
Number GET filters follow the shared grid's integer-only parsing; CRUD input
fields still support fractional values.

## Write constraints

Declare limits on writable fields to show them in both form languages and
enforce them on the server. The starter declares its name and stock limits on
the fields passed to `crud.New`, and reuses those fields in its repository.

```go
name := crud.Field{
	Key: "name", Label: "Name", Type: "text", Read: true, Create: true, Update: true,
	Required: true, MaxLength: new(200),
}
stock := crud.Field{
	Key: "stock", Label: "Stock", Type: "number", Read: true, Create: true, Update: true,
	Minimum: new(0.0),
}
```

`MaxLength` counts Unicode code points in the submitted text. Spaces and
combining characters count individually. The limit must be positive and the
field must be `text` or `textarea`. `Minimum` is inclusive and supports both
integer and fractional `number` fields. Supply a finite bound in the
JavaScript-safe magnitude supported by CRUD. A nil bound omits the rule.
The constructor rejects write constraints on fields without a write mode and
on resources built with `NewReadOnly` or `NewWithActions`. Declare command
inputs separately with `ActionWithParams`.

POST and PUT collect constraint errors for the fields writable in that
operation before calling the host. For numbers, the submitted decimal and
the value decoded into the host's Go type must both meet the minimum. This
also applies when a `float32` field rounds the input. An optional nil pointer
has no value to compare. An omitted non-pointer number defaults to zero and
must satisfy its minimum. Direct writers can call
`name.ValidateConstraints(input.Name)` and `stock.ValidateConstraints(input.Stock)`
on the same declarations before opening a transaction. Domain rules such as
rejecting an all-whitespace product name still belong to the host.

The form retains text pasted beyond the length limit and displays an inline
error. It also preserves a decimal entry such as `9007199254740990.5` when
JavaScript would send a different value. For an integer DTO field, the server
returns fractional and out-of-range input errors on that field. Reads apply
the existing scalar and access checks, so an operator can open and correct a
legacy value outside a newly declared write bound.

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
build derives the same fingerprint. The shared grid and record navigation code
also contribute to the fingerprint. Generated assets and server ship together.

## Verification

Run the package and external-consumer tests, then `make check` and
`make externalconsumer-local`. In both presentation packs, `make starter-smoke`
exercises Products CRUD, saved views and exports. It also opens two Requests
records from a filtered queue, approves one, rejects the other and verifies
return navigation, stale revisions, missing reasons and denied commands.
A focused Products rerun uses `node scripts/starter-smoke/crud-check.mjs
output/playwright/starter-smoke/<run-id> [default|topbar]` against an initialized,
owned stack. The starter integration-tag test requires `PRODUCTS_TEST_DSN` for
an isolated PostgreSQL server with create-database privileges; it creates and
drops its own test database and proves CAS, owner scope and audit rollback.
Local checks do not prove publication or deployment.

### Connect a second read transport

`NewReadOnlyWithAdapter(definition, ReadOnlyAdapter{BasePath: ..., Build: ...})`
mounts the same resource pages as `NewReadOnly` and reserves the adapter's
route namespace. Its builder receives the current `*host.App` and a
`ReadOnlyReader` for that runtime. Authenticate the caller first, then pass
their context and canonical actor to `List` or `Get`. Both calls use the same
permission, row authorization, owner-scoped repository and field projection as
the browser. The adapter receives projected fields and record identities.
The [Products starter](../examples/starter/README.md#connect-a-local-mcp-client-to-product-activity)
demonstrates session-bound delegation and two read-only MCP tools.

## Explain a resource to operators

Set the optional `Description` on a `crud.Definition` or `crud.ReadOnlyDefinition` to explain what the operator can do in that section. Set it on a `crud.Field` for field help and on a `crud.Action` or `crud.ActionParam` to explain the consequence of a command or the input it requires. The form displays the explanation alongside Go's validation constraints. The Products example tells the operator that the unpublish reason stays in Product activity after deletion.

## Offer read access to an MCP client

Use the public `features/mcp` package for a resource the host explicitly selects. The runnable [Products declaration](../examples/starter/product_agent.go) supplies the resource permission, search filters, individual detail lookup, field projections and operator consent text. `mcp.Adapter` connects that declaration to `crud.NewReadOnlyWithAdapter` so the UI and agent use the same actor-scoped lookup. A custom DataGrid can pass its scoped list and detail callbacks to `mcp.Module` through `mcp.ReadFuncs`. The host owns record visibility, the count and the business data. Declare each output field for each operation; the tool schemas and result projection use that declaration.

Build the standard client with `go build ./cmd/goadmin-mcp` at the release source. Configure the client with `-admin-url <admin-origin> -resource-path <configured-path>`. The program reads its tool descriptions from that resource. A first valid call requests a short-lived pairing code. The operator opens the verification URL, enters the code, and approves reading. Each subsequent operation checks the original login, role and current permission. A local logout, revoke or process restart closes access. Use one server instance for this in-memory grant store. Proxy and quota settings are described in the [starter guide](../examples/starter/README.md#connect-a-local-mcp-client-to-product-activity).

Set `AllowExternalSessions` on individual resources that accept AuthHub consent. An external call requires the same login, canonical identity link and a fresh browser proof. The agent never renews that proof. If it expires, open the admin in the browser and retry after renewal. Consent can continue for the original 15-minute maximum. A central logout takes effect by the existing proof deadline, at most five minutes from issuance. A new login requires new consent.
