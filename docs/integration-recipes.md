# Integration recipes: appearance, notifications and Inertia

Use this guide after the [component-selection checklist](consumer-guide.md#choose-the-integration-components-first).
It describes the public contracts at **GoAdmin v0.32.1**. Select only the modules
your host needs; a theme does not enable mail, and a notification module does not
configure password recovery. Keep the server, client sources and embedded assets
on the same GoAdmin version.

## What is already there, and what must be connected?

| Requirement | Provided by GoAdmin (core or optional) | Explicit host work |
| --- | --- | --- |
| System/light/dark appearance | Theme controls, personal appearance, branding routes and normal migration storage | Serve matching built assets/templates; authorize branding editors |
| Different colors, geometry or shells | Documented CSS variables and typed presentation extension contract | Build and embed chosen CSS extension or one presentation pack |
| Success/error message after a form action | Inertia flash helpers and built-in presentation | Set flash in the handler; no notification transport needed |
| Durable ordinary notifications | Optional module and transport facade | Select `jobs` and `notifications`, configure transport and worker lifecycle |
| Password recovery / account email confirmation | Separate GoAuth delivery contract | Select `authmail`, supply its sender or compatible borrowed Auth configuration |
| Custom internal admin page | Core Inertia renderer, session/CSRF integration | Authorized route, exact page name, Vue extension, matching manifest and assets |
| Standard CRUD page | Built-in CRUD renderer | Configure the [CRUD module](crud.md); no custom Vue page needed |

Core authentication, membership and RBAC are not optional. Hiding a menu item is
not endpoint authorization. Selecting a module neither provisions an external
provider nor grants permissions to its users.

## 1. Appearance: start without a custom build extension

For the built-in appearance, do **not** add an optional theme module. Run the
normal migrations, build/serve the matching core client, then use the header
control to cycle System → Light → Dark. The Interface settings persist the
administrator's preference; System follows that device's OS setting. Anonymous
device preferences and signed-in account preferences are distinct.

Installation branding is also core, not `features/profile` opt-in wiring.
`GET/PATCH /auth/profile/appearance` manages the current account's appearance;
`GET/PUT /auth/branding` uses `branding:read` / `branding:update`. Paths are
relative to the admin mount. Retain normal CSRF and role-preview restrictions.
Grant these permissions deliberately through your authorization policy.

For project-owned CSS, create an extension module such as `extensions/theme.ts`:

```ts
import '../theme.css'
export default {}
```

Its local `theme.css` can use documented variables, for example:

```css
:root {
  --goadmin-header-height: 72px;
  --goadmin-content-max-width: 76rem;
}
```

This is a **source fragment**, not a standalone build. Package it in the source
manifest described in [embedding the client](embedding-client.md#build-and-embed),
generate its fingerprint and include it in `VITE_ADMIN_EXTENSION_MANIFESTS`.
Preserve other host manifests. Rebuild and embed the resulting output rather
than editing generated CSS. The complete existing
[theme-shell example](../resources/examples/theme-shell/extension.json) is a
starting point; it is not selected by default.

For a different shell, use the typed `AdminExtension.theme` and `themeModule`
contract in [theme customization](theme-customization.md#build-time-presentation-packs).
Choose at most one pack. Either shell may be omitted to retain the default;
render the core navigation, account and mobile-navigation slots as documented.
A CSS-only extension does not need `themeModule`.

**Acceptance check:** switch all three modes, reload and sign in again; verify
System follows an OS appearance change. Check branding with authorized and
unauthorized accounts. Check desktop/mobile navigation, visible keyboard focus,
validation errors and contrast in both themes. An arbitrary host stylesheet is
not automatically contrast-certified by the library.

**If it looks unchanged:** verify that the intended manifest was included, its
fingerprint was regenerated, and the host serves the new assets. Check whether
a saved account/branding choice or an explicit component class takes precedence.
Missing branding controls can be a permission issue, not a missing theme module.

## 2. Notifications: distinguish flash, ordinary delivery and authmail

### In-page feedback needs no delivery provider

Inside an existing protected handler, after a successful mutation:

```go
app.WithFlashSuccess(c, "Saved")
return app.Redirect(c, "/reports")
```

Here `app` is `*host.App`, `c` is `fiber.Ctx`, and `/reports` must be an existing
host route. These are handler fragments, not complete programs. There are also
`WithFlashInfo`, `WithFlashWarning`, `WithFlashError` and
`RedirectBackWithValidationErrors` helpers. Ordinary protected routes persist
flash through core middleware. A public extension route that returns before
that middleware must call `app.PersistFlash(c)` after setting flash and before
redirecting, and handle its error. Do not use a mail provider to display a toast.

### Durable ordinary notifications require jobs and transport

In the host's existing assembly function, with `modules []host.Module` and a
server-side `projectKey`, import `github.com/assurrussa/goadmin/host` and add:

```go
manager, err := host.NewNotificationManager(host.NotificationConfig{
    BaseURL:    "https://notify.example.com",
    ProjectKey: projectKey,
})
if err != nil {
    return err
}
modules = append(modules,
    host.JobsModule(host.JobsConfig{}),
    host.NotificationsModule(manager),
)
```

Pass these modules to the normal `host.New(ctx, cfg, deps, modules...)` assembly.
This fragment assumes the core PostgreSQL dependencies and migrations are
already configured. Do not append a second `jobs` module if the host already
selected one. Empty `JobsConfig` constructs an admin-owned outbox worker;
`Runtime.Run` supervises it. A borrowed queue is instead supervised by the host.
See the full [starter assembly](../examples/starter/main.go) for lifecycle and
error handling. `features/notifications.New(manager)` is the public feature-package
wrapper for `host.NotificationsModule(manager)`; use one, not both.

The manager constructor validates configuration **without sending a request**.
Successful construction does not prove provider connectivity or delivery.
Missing `jobs`, nil transport, missing `BaseURL` or `ProjectKey` are configuration
errors. Use HTTPS for remote transport; do not enable insecure HTTP as a
production workaround. Credentials stay server-side, not in Vue props or Vite
variables. A supplied HTTP client remains owned by the host.

A custom `host.NotificationManager` is supported: it implements GoNotify's
`transport.Transport` contract, including durable acceptance and idempotency.
A fire-and-forget sender with the same method names is not an equivalent
implementation. Preserve retry identity and errors, and supervise the worker.

**Acceptance check:** with an explicitly authorized test destination, submit one
ordinary notification, observe durable acceptance and final provider outcome,
and verify retry/deduplication behavior without duplicate delivery. Exercise
provider failure and worker restart. A visible notifications page alone is not
proof of delivery. Check that users without permission cannot invoke its actions.

### Authentication mail is a separate integration

`features/authmail.New(host.AuthMailConfig{...})` / `host.AuthMailModule` accepts
`Sender` (GoAuth's `NotificationSender`), optional `PasswordRecovery` routes and
worker configuration. It does **not** use `NotificationManager` as its sender.
For borrowed Auth, the adapter must support managed mail; its sender/worker
configuration belongs to that Auth. Do not supply a second sender configuration
or claim a borrowed worker without borrowed Auth.

Follow the [starter notification adapters](../examples/starter/notifyhub.go) for the
separate ordinary-notification and authentication-delivery wiring. Do not send
plaintext authentication codes or tokens into NotifyHub's durable message store.
Test the actual password-recovery / confirmation flow separately from ordinary
notifications, using test identities and the chosen sender's contract.

Delivery acceptance is not proof that a recipient received a message. Verify the
final outcome using the selected provider's documented contract and available
receipts; do not assume that every transport produces an inbox notification.

## 3. Inertia: one route, one page name, one matching bundle

Core assembly already provides the Inertia integration; there is no optional
`inertia` module to enable. For a host-specific page, all four pieces must agree:

1. Feature menu URL and authorized server GET route, for example `/reports`.
2. Handler rendering `app.Render(c, "reports/Index", props)` through `*host.App`.
3. Extension file `Pages/reports/Index.vue`, matching case exactly.
4. Built extension manifest/fingerprint, embedded `dist` and compatible templates.

A minimal **page component**, inside that extension's `Pages/reports/Index.vue`:

```vue
<script setup lang="ts">
defineProps<{ title: string }>()
</script>

<template>
  <h1>{{ title }}</h1>
</template>
```

Its authorized handler returns:

```go
return app.Render(c, "reports/Index", map[string]any{"title": "Reports"})
```

These fragments do not register a route, grant permissions or create a manifest.
Complete those steps using the [client embedding contract](embedding-client.md#one-page-crosses-four-boundaries).
Normal admin navigation expects an Inertia page, not a separate HTML response.
Keep shared Vue components outside `Pages/`. Use core flash/validation helpers
for redirects, and protect mutation routes independently of the page route.

For custom HTTP requests, use `adminFetch`, `createAdminAxios` or
`ensureSessionReady` from `@admin-core/http` as described in
[shared authenticated requests](embedding-client.md#shared-authenticated-requests).
There is no global replacement of `fetch`. Preserve session-maintenance version
and epoch props, CSRF cookie-name alignment, submit guards and cancellation.
Session repair does not replay a failed mutation; an unknown result requires a
domain result check before retrying.

**Acceptance check:** open the URL directly, navigate from the menu, reload and
use Back/Forward. Check validation/flash after redirects, denied permission,
expired session, repeated clicks and mobile layout. Confirm deployment contains
the exact server requirement and matching client manifest. Startup validation
is useful, but does not replace the browser scenario.

**Common failures:** page not found means check the case-sensitive component
name and bundle; a menu that opens an HTML error means check its route/response;
login working but writes failing can mean mismatched CSRF cookie names; an old
open tab may indicate stale fixed-name entry JS/CSS. Publish matching server,
assets, templates and manifest together with the documented cache policy.

## Freedom to customize, without hidden obligations

These are supported defaults and recipes, not a requirement to select every
module or use every visual component. Hosts own domain logic and may choose
their own styles, typed shells and transport implementations. Reusing the
existing contracts keeps core lifecycle and security coherent. A different
frontend/protocol stack is not a drop-in theme: it must explicitly replace the
integration obligations rather than bypassing authentication, CSRF, permission,
owner-scope or retry guarantees. Record selected and deferred components before
implementation so an unconfigured capability is never mistaken for working.
