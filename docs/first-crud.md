# Your first CRUD resource

Goal: run a real, host-owned Products section, create and change a record, and
understand which parts GoAdmin supplies. This tutorial uses the existing
[PostgreSQL starter implementation](../examples/starter/products.go), not a
second toy implementation. It needs no custom Vue, Redis, uploads or mail service.

## 1. Start the published example

Follow the [starter prerequisites and secret setup](../examples/starter/README.md#start-with-the-published-library)
to clone the v0.32.1 tag. In that checkout's private `.env`, select the example:

```dotenv
GOADMIN_VERSION=v0.32.1
ADMIN_MODULES=products
```

From `examples/starter`, run:

```sh
GOADMIN_VERSION=v0.32.1 docker compose up --build
```

Open `http://localhost:8080`. On a fresh database, register the first administrator
using your configured setup token, then sign in. There is no bundled password.
The Compose profile runs core migrations and the separate Products ledger before
serving. Compose stores PostgreSQL data in a named volume; stopping a container
is not a database reset. Startup readiness is a separate concern: see the
[restart and recovery caveat](../examples/starter/README.md#restart-and-recovery).
Disabling Products hides its routes but does not drop its tables. Do not use
production data for this tutorial.

Core includes authentication, membership/RBAC, sessions, CSRF, setup, basic profile
and the shell. Products adds list, detail, create/edit, delete, CSV and guarded
Publish/Unpublish actions using the embedded renderer. `features/access` is an
optional management UI, not the source of core security. Add `access` explicitly
only when you want to manage administrators, roles and permissions in this demo.

## 2. Verify what you can see and save

1. Open **Products** (`/products`). Create a record with a name, stock, active flag,
   valid date and optional category. Save, then reload: the stored values remain.
2. Change the record. **Save** keeps the editor open; **Save and return to list**
   restores the originating list context. Open the detail page to inspect it.
3. Publish, cancel one confirmation, then confirm a transition. Publication Status
   is not a normal editable field. Publish and Unpublish have separate permissions.
4. Open the same record in two tabs. Save competing edits; the other save must
   expose a revision conflict rather than silently overwrite the first tab.
5. Filter the list and export CSV. Only authorized, scoped records may appear.
6. Delete your sample record after confirmation. When checking persistence after
   a stop/start, follow the [restart and recovery caveat](../examples/starter/README.md#restart-and-recovery)
   and verify both application readiness and the remaining records. Do not treat
   a successful container command as application readiness. `docker compose down`
   stops the stack without deleting its named volumes; do not add `-v` unless you
   intend to discard the demo data.

Menu visibility is not authorization. A missing menu/action can mean the module
is omitted or the administrator lacks a permission. Registering a resource's
permission definitions does **not** grant them to every administrator.

## 3. Follow the implementation into your host

The code below follows the current source. To try it, use the
[starter's source mode](../examples/starter/README.md#run-the-current-source)
and select `ADMIN_MODULES=products,product-events`. The Products form shows the
200-code-point name limit and a minimum stock of zero. Publish a product, open
Unpublish from its list or detail page, enter a reason and confirm. In Product
events, search for its number and open the Unpublished event to read the reason.
The event remains available after deleting the product.

Use the named functions in [products.go](../examples/starter/products.go) as the
complete runnable source, and [main.go](../examples/starter/main.go) for assembly.
Do not copy GoAdmin internals into your application.

| Step | Existing code | Your responsibility |
| --- | --- | --- |
| Define the data | `productData`, `productInput` | Flat typed DTOs with explicit JSON tags. Keep secrets out of readable fields; omit Status from ordinary writes. |
| Define the resource | `productsModule`, `crud.New`, `crud.Definition` | Stable key/path, fields, columns and separate read/create/update/delete permissions. |
| Read scoped records | `productRepo.list`, `get`, `LoadSelected` callback | Apply the same owner scope to rows, totals and exports, not just visible buttons. |
| Write safely | `create`, `update`, `command` | Validate domain rules, compare revision atomically, and commit audit with the mutation. |
| Add domain actions | `Actions`, `Authorize`, `ActionWithParams` | Guard each transition and validate the typed unpublish reason before committing it with the status change. |
| Own the schema | `migrateProducts` | Versioned host migrations; no automatic schema inference from DTOs. |
| Mount the module | `main.go` Products branch | Pass the returned module to `host.New`; keep borrowed database lifecycle with the host. |

The constructors perform no database I/O. GoAdmin supplies the standard pages,
route/permission checks and DTO/field transport; the host owns its business data,
repository, migrations and transactions. Read the [CRUD contract](crud.md) before
changing field types, write-only semantics, custom actions or error handling.

For a compact illustration of just the supported Go types, see the
[external-consumer compile test](../reference/externalconsumer/crud_test.go).
Its callbacks are stubs, not a persistence or authorization implementation.

### The resource assembly, directly from the runnable starter

The following is the complete `productsModule` function, synchronized from
[products.go](../examples/starter/products.go). In the same file, `productNameField`
and `productStockField` define the form and write constraints. `productCategoryField`
supplies the options used by the form, list filter and repository. The current
source uses `ListColumns`; the tagged starter in step 1 uses explicit `Columns`.
`unpublishReason`
defines the required, trimmed reason and its 500-code-point limit for
`UnpublishInput`. The function uses the file's DTOs and owner-scoped repository
methods, plus `context`, `database/sql`, `errors`,
`features/crud` and `host`. It is **not a standalone program**:
run the starter above or keep the surrounding repository/migration code when
adapting it. In particular, the small `Authorize` callback does not replace the
owner checks in the repository reads and writes.

<!-- source: examples/starter/products.go#productsModule -->
```go
func productsModule(db *sql.DB) (host.Module, error) {
	r := productRepo{db}
	permission := func(action host.PermissionAction) host.PermissionKey {
		return host.NewPermissionKey("products", action)
	}
	fields := []crud.Field{
		productNameField,
		{Key: "description", Label: "Description", Type: "textarea", Read: true, Create: true, Update: true},
		productStockField,
		{Key: "active", Label: "Active", Type: "boolean", Read: true, Create: true, Update: true},
		{Key: "available", Label: "Available", Type: "date", Read: true, Create: true, Update: true},
		productCategoryField,
		{
			Key: "status", Label: "Status", Type: "select", Read: true,
			Options: []crud.Option{{Value: "draft", Label: "Draft"}, {Value: "published", Label: "Published"}},
		},
	}
	return crud.New(crud.Definition[productData, productInput, productInput]{
		Key: "products", BasePath: "/products", Title: "Products", Description: "Manage your catalog and review publication changes in Product activity.", Fields: fields, PageSize: 20, DefaultSort: "name", DefaultOrder: "asc",
		ListColumns: []crud.ListColumn{
			{Key: "name", Sortable: true},
			{Key: "stock", Sortable: true, Filterable: true},
			{Key: "active", Filterable: true},
			{Key: "available", Sortable: true, Filterable: true},
			{Key: "category", Filterable: true},
			{Key: "status", Filterable: true},
		},
		Permissions: crud.Permissions{Read: permission("read"), Create: permission("create"), Update: permission("update"), Delete: permission("delete")},
		Menu:        host.Menu{Sections: []host.Section{{Key: "products", Title: "Products", Items: []host.Item{{Name: "Products", Href: "/products", Icon: "users", PermissionKey: permission("read")}}}}},
		List:        r.list, Get: r.get, LoadSelected: func(ctx context.Context, a host.Actor, ids []string) ([]crud.Record[productData], error) {
			records := []crud.Record[productData]{}
			for _, id := range ids {
				record, err := r.get(ctx, a, id)
				if errors.Is(err, crud.ErrNotFound) {
					continue
				}
				if err != nil {
					return nil, err
				}
				records = append(records, record)
			}
			return records, nil
		},
		Create: r.create, Update: r.update,
		Delete: func(ctx context.Context, a host.Actor, id, revision string) error {
			return r.command(ctx, a, id, revision, "deleted")
		},
		Authorize: func(_ context.Context, _ host.Actor, op crud.Operation, record crud.Record[productData]) error {
			if op == "publish" && record.Data.Status != "draft" {
				return crud.ErrDenied
			}
			if op == "unpublish" && record.Data.Status != "published" {
				return crud.ErrDenied
			}
			return nil
		},
		Actions: []crud.Action[productData]{
			{Key: "publish", Label: "Опубликовать", Confirmation: "Опубликовать этот товар?", Permission: permission("publish"), Run: func(ctx context.Context, a host.Actor, id, revision string) error {
				return r.command(ctx, a, id, revision, "publish")
			}},
			crud.ActionWithParams[productData, UnpublishInput](
				crud.Action[productData]{Key: "unpublish", Label: "Снять с публикации", Description: "The product will become a draft. This action and its reason are recorded in Product activity.", Confirmation: "Снять этот товар с публикации?", Permission: permission("unpublish")},
				[]crud.ActionParam{unpublishReason}, r.unpublish,
			),
		},
	})
}
```
<!-- /source: examples/starter/products.go#productsModule -->

`make check` compares this excerpt with the Go function. After changing
`productsModule`, run `python3 scripts/check-doc-snippets.py --write` and review
the updated example.

## 4. Verify denied paths too

On an isolated demo with `access` explicitly selected, create a second
administrator with only the intended Products permissions. A read-only actor must
not be able to mutate through a direct request, even if it knows the URL. An actor
with Products rights still must not read another owner's records through detail,
list, totals or export. Test revoked permissions again on an already-open page.
The Products example deliberately scopes records to each administrator; it is not
a shared catalogue.

Existing maintainer checks cover these boundaries; reuse them rather than treating
successful compilation as end-to-end acceptance:

- `go test ./features/crud ./reference/externalconsumer` from the repository root.
- In the current checkout, run `make externalconsumer-local`. In the v0.32.1
  checkout from step 1, run `make externalconsumer-published VERSION=v0.32.1`
  to check the published package with that release's consumer.
- From `examples/starter`, run `PRODUCTS_TEST_DSN='<isolated PostgreSQL URL>' go test -tags=integration -run '^TestProductsPostgresCASIsolationAndAuditRollback$' .`.
  This requires create-database privileges and creates/drops its own test database.
- `make starter-smoke` runs the existing source-candidate browser acceptance,
  including Products, in both presentation packs. Follow its
  [prerequisites and evidence contract](../examples/starter/README.md#compare-presentation-packs-on-a-real-backend).
  It is not a check of the published Docker build.

## If something does not work

| Symptom | Check / next action |
| --- | --- |
| Startup rejects missing configuration | Fill every required `.env` value; use three distinct 32-byte base64 keys. Do not weaken validation. |
| Products is absent | Confirm `ADMIN_MODULES=products`, apply the configuration with `docker compose up -d --build`, and check the current actor's permissions. Keep the same Compose project name. |
| App exited during startup while PostgreSQL was restarting | Inspect `docker compose ps -a` and `docker compose logs admin postgres`; use `docker compose up -d` with the same project/configuration to recover, then verify readiness and records. See the [restart caveat](../examples/starter/README.md#restart-and-recovery). |
| Relation does not exist | Confirm the selected Products migration ran against the same database. Do not reset existing data as a workaround. |
| A record is not found for another account | Owner scoping is expected; do not remove it to make the demo visible. |
| Save reports conflict | Review current versus draft values and explicitly save the reconciled edit. Do not retry stale writes automatically. |
| Save outcome is unknown | Check the stored result before retrying. An error after commit may not mean the write failed. |
| Custom bundle rejected | Ship the matching server, generated assets, manifest and templates. Standard Products does not need a custom bundle. |
| Writes fail after a custom frontend build | Check CSRF cookie-name alignment and current auth; never disable CSRF. See [client embedding](embedding-client.md). |

Next: [choose a custom page only if needed](consumer-guide.md#choose-the-smallest-extension),
or use the [detailed CRUD reference](crud.md) for the existing resource contract.
