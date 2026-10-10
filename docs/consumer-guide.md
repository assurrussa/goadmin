# From the starter to your own admin page

GoAdmin is an embedded admin subsystem for a Go host. Start with the existing
starter to see the shell, then add a host-owned feature through the supported
facade. This guide connects the pieces; the linked guides own the detailed
setup and API contracts.

## See the existing interface first

Follow the [standalone starter](../examples/starter/README.md), including its
required secrets and choice of published-release or source-candidate mode.
Open `http://localhost:8080` with the default port. On a fresh database, the
login page offers first-admin registration; use the configured setup token.
There is no bundled administrator password.

With `ADMIN_MODULES` empty, expect login/logout, first-admin setup, basic
profile and the admin shell. The built-in interface is currently Russian-only;
navigation follows the signed-in administrator's permissions. Optional module
screens appear only when enabled and permitted. The starter does not create a
sample business entity for you.

Published mode uses the selected release's UI and dependencies. Source mode
builds the current server and client together with the explicitly selected
sibling checkouts. Follow the starter's commands for the chosen mode; success
with local replacements is not proof that a published consumer resolves.

## Know which parts are required

- **Core runtime:** Go 1.27 or newer, Fiber v3, PostgreSQL, GoAuth-backed
  authentication/RBAC, and the configured signing/token/journal keys and CSRF
  protection. The host supplies its database and configuration; run
  `migrations.Migrate` before serving traffic. PostgreSQL is the default
  browser-session and canonical-session backend.
- **Browser UI:** Vue 3, TypeScript, Inertia v2 with GoInertia on the server,
  Tailwind CSS v4, and Vite. These are the existing admin client's stack, not
  separate services to deploy. A core-only host can use the default embedded
  bundle. Custom Vue pages require rebuilding and embedding a matching bundle.
- **Development tools:** Docker Compose is the starter's run path. Node.js
  22.13+ on the 22.x line or 24+ and npm are needed when rebuilding the client;
  they are not an extra production server. Source mode also needs Git and
  Python 3, as documented by the starter.
- **Optional capabilities:** Redis is an explicitly selected session backend.
  Access management, jobs, uploads, queues, notifications, realtime and authmail
  are explicit modules. Uploads, queues and notifications require jobs;
  authmail uses GoAuth's own encrypted queue. The starter's mail modules require
  an existing NotifyHub gateway, not SMTP configuration in the core.
  Optional users/operations features and your own features are mounted
  separately.

Use only the 14 supported GoAdmin packages listed in the
[public surface](../README.md#stable-public-api) and its
[machine-readable manifest](../reference/externalconsumer/packages.go).
An exported internal package is not a supported host SDK. Keep business
models, repositories, migrations and services in your host.

## Connect one host-owned entity page

The following `/products` names illustrate the contract, not a runnable CRUD
sample. See [Embedding and extending the admin client](embedding-client.md)
for the complete build and page-lifecycle instructions.

1. **Define the feature.** Implement `host.Feature`: its
   `FeatureDescriptor` contributes a stable key, permission definitions, an
   optional menu item with `Href: "/products"`, and
   `ClientRequirements` for its client extension. Inject your product services
   through the feature constructor. Its `Build` returns an
   `ExtensionBuilder`, which receives `*host.App`.
2. **Mount authorized routes.** Use `host.NewExtension().WithRegister(...)`
   or `WithHandlers(...)` to register the feature's HTTP handlers. Mount it
   through `host.DefinedFeatureModule` passed to `host.New`, declaring
   `ModuleDescriptor.Routes` or literal `RouteNamespaces` for conflict
   checks. The menu does not register or authorize a route: guard the page and
   every read/write endpoint with the appropriate permission. Use
   `app.Guard` or the typed `host.Command[T]` contract where appropriate.
3. **Render through the shell.** After authorization and loading your data,
   the GET handler calls
   `app.Render(c, "products/Index", map[string]any{"products": productData})`.
   The host chooses the props and the Vue page consumes that same shape.
   Returning a separate HTML document breaks internal Inertia navigation.
4. **Provide the matching Vue page.** Put `Pages/products/Index.vue` under
   the extension's source root. The name after `Pages/`, without `.vue`,
   must exactly match `products/Index`, including case. Shared components
   belong outside `Pages/`.
5. **Build the extension and core together.** The version-1 source manifest
   declares a unique key, `apiVersion: 1`, relative `sourceRoot` and a
   generated SHA-256 fingerprint. Match that identity in the server's
   `host.ClientExtensionRequirement`. Build the pinned GoAdmin client's
   `resources/` with `VITE_ADMIN_EXTENSION_MANIFESTS` and
   `VITE_ADMIN_PUBLIC_ROOT`; follow the embedding guide's fingerprint and
   build procedure whenever source changes.
6. **Serve the exact built result.** Embed the generated `dist`, including
   `dist/admin-extensions.manifest.json`. Set `host.Config.Public` to that
   built `fs.ReadFileFS` and `host.Config.Templates` to the matching templates
   (`host.CoreTemplateFS()` for the default template).
   `host.New` validates the bundle against the feature requirements before
   startup. `host.BuildClientBundleValidation` is also available for an
   explicit preflight check. Ship server, assets, manifest and templates together.

## Add a list without assuming CRUD generation

The [DataGrid public toolkit](../toolkit/datagrid/README.md) supplies typed
list handling. Its [external-package example](../toolkit/datagrid/example_test.go)
demonstrates a repository, search/pagination, row decoration, action metadata
and page/error callbacks without a database or listening server. Its page
callback returns text: it is a list-only example, not a custom Vue CRUD app.

For your entity, implement `GetList(context.Context, datagrid.Filtered)` with
the requested rows and total matching count. Supply a `PageComponent` that
renders your Vue page through `app.Render`; choose the matching props there.
`RegisterRoutes` supplies the list page plus GET/POST data endpoints. It does
not supply create/update/delete handlers. Action visibility and UI flags do not
authorize mutations or implement business behavior.

The built-in users CRUD implementation is internal. The supported
`features/users` package mounts GoAdmin's existing users feature; it is not a
generic custom-entity factory. There is currently no complete public custom
Vue CRUD example or declarative CRUD framework. Own your forms, validation,
mutation routes, persistence and permission policy in the host rather than
copying internal handlers.

## Check the whole connection

Bundle validation checks assets and extension identities; it does not prove
that every menu URL has a route or every render name has a Vue page. Test those
links at the feature boundary, then smoke-test navigation and an authorized
mutation in a browser, including denied access. Match the build-time CSRF
cookie name to the server configuration. Use the
[embedding guide's failure matrix](embedding-client.md#add-change-or-remove-a-page)
for blank pages, fallback components and failed writes.
