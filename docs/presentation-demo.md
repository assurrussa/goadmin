# Admin UI previews

## Build-time presentation packs

The default sidebar presentation and the topbar/split-auth reference use the same
pages, core controls and runtime services. Start the default preview as shown
below. To run the reference alongside it, from the repository root:

```sh
VITE_ADMIN_EXTENSION_MANIFESTS=resources/examples/theme-topbar/extension.json \
  GOADMIN_DESIGN_PORT=8769 node resources/audit/design/server.mjs
```

Open [the topbar users page](http://127.0.0.1:8769/users) or
[split login](http://127.0.0.1:8769/auth/login). The fixture materializes the real
source manifests and installs the selected presentation through the actual
extension loader. Its registry and Tailwind sources are temporary; it does not
replace the developer's generated host state. Manifest paths are relative to the
command's working directory.

Both previews also provide `/auth/mfa`, `/auth/forgot-password` and
`/auth/reset-password`. `/__design/components` is the shared catalog of button,
field, checkbox, select, tab, dropdown, dialog and editor states. Its actions only
change local demonstration state. `/__design/long-menu` has twenty long sections
and three levels of links for scrolling and keyboard checks;
`/__design/capabilities-limited` supplies an explicitly limited capability map
and host-filtered menu, while `/__design/missing-menu` omits the menu altogether.
Neither fixture claims to enforce real backend authorization.

See [the presentation contract](theme-customization.md#build-time-presentation-packs)
for creating a pack and [presentation QA](presentation-packs-qa.md) for the current
verification evidence. The earlier galleries below document the original visual
refresh; they are not the theme-pack acceptance matrix.

## Default light and dark themes

The default theme uses neutral light surfaces, graphite dark surfaces and a
restrained blue accent. State indicators use a colored dot with a readable label;
counts and role/category labels retain badges. The existing layout, information
density, light/dark preference and host CSS overrides remain in place.

These screenshots show the real users page and shared components with synthetic
accounts, captured at **1440 × 1024**, 1× device scale:

![Default light theme, users table](screenshots/design-refresh/users-after-light.png)

![Default dark theme, users table](screenshots/design-refresh/users-after-dark.png)

Run the read-only visual fixture from the repository root. It requires the
repository's Go toolchain as well as Node: Go renders the real standalone error
template at startup, using the normal shared Go caches.

```sh
npm ci --prefix resources
node resources/audit/design/server.mjs
```

Open [the users page](http://127.0.0.1:8768/users) or
[dashboard](http://127.0.0.1:8768/). The fixture also serves:

- `/permissions`, `/roles`, `/roles/1`, `/roles/1/edit`;
- `/auth/profile`, `/auth/profile/edit`, `/auth/profile/settings`;
- `/notifications`, `/queues`, `/queues/jobs`, `/queues/jobs-failed`;
- `/users/1048/edit`, `/auth/login`, `/__design/dialog`;
- `/__design/components`: real tabs, form states, buttons, rich-text/JSON editor
  and image cropping. Cropping produces a local preview and does not upload it.

Use the real header controls to change theme or language.
Stop with Ctrl-C. The server binds only to localhost and uses temporary extension
sources; it does not overwrite the developer's extension registry.

This fixture exercises real Vue screens without an authenticated backend.
Filtering and search read deterministic local data; submissions return 405.
Export, unlisted sidebar destinations, authentication and persistent saves are
not implemented. The create route displays the same edit-form sample. It is visual
acceptance evidence, not a host integration or backend acceptance test.

Table scenarios: `/__design/users-empty` is an unfiltered empty table;
`/users?scenario=empty` has no matching results; `scenario=long` supplies long
values; `scenario=loading` delays the first data request by 12 seconds;
`scenario=error` always fails; `scenario=recovery` fails once and then succeeds
through the real Retry action. Opening a scenario afresh resets its request
counter. These query switches belong only to the fixture.

`/__design/theme` provides explicit light/dark controls and **System preference
(unset)**, which removes the local choice. The application then follows its
canonical system-theme behavior without saving a manual preference.
`/__design/error/{code}` renders the actual self-contained Go error template for
401, 403, 404, 419, 429, 500 and 503, returning the corresponding HTTP status.

Additional captures: [form](screenshots/design-refresh/form-light.png),
[dark dialog](screenshots/design-refresh/dialog-dark.png),
[mobile login](screenshots/design-refresh/login-mobile.png),
[mobile users](screenshots/design-refresh/users-mobile-light.png) and
[tablet users](screenshots/design-refresh/users-tablet.png).
See the [design QA report](../design-qa.md) for comparison evidence and limits.

### Completed screen gallery

The remaining screen matrix contains 96 captures: twelve screens, both themes,
1440px/390px widths and Russian/English UI. Server-supplied titles and synthetic
data retain their original language when switching the UI locale.

- Dashboard, permissions, roles: [Russian](screenshots/design-refresh/acceptance/mobile-review-ru-1.png),
  [English](screenshots/design-refresh/acceptance/mobile-review-en-1.png).
- Role view/edit, profile: [Russian](screenshots/design-refresh/acceptance/mobile-review-ru-2.png),
  [English](screenshots/design-refresh/acceptance/mobile-review-en-2.png).
- Profile edit/settings, notifications: [Russian](screenshots/design-refresh/acceptance/mobile-review-ru-3.png),
  [English](screenshots/design-refresh/acceptance/mobile-review-en-3.png).
- Queue dashboard/active/failed: [Russian](screenshots/design-refresh/acceptance/mobile-review-ru-4.png),
  [English](screenshots/design-refresh/acceptance/mobile-review-en-4.png).

Each contact sheet shows three complete mobile screens, light above dark.
Full-size desktop examples:
[dashboard](screenshots/design-refresh/acceptance/dashboard-light.png),
[permissions](screenshots/design-refresh/acceptance/permissions-dark.png),
[role](screenshots/design-refresh/acceptance/role-view-light.png),
[profile settings](screenshots/design-refresh/acceptance/profile-settings-dark.png),
[notifications](screenshots/design-refresh/acceptance/notifications-light.png)
and [failed queue](screenshots/design-refresh/acceptance/queues-failed-dark.png).
Special states:
[cropper](screenshots/design-refresh/acceptance/cropper-mobile-dark.png),
[invalid JSON](screenshots/design-refresh/acceptance/editor-json-error-mobile-light.png),
[empty data](screenshots/design-refresh/acceptance/users-empty-light.png),
[loading](screenshots/design-refresh/acceptance/users-loading-mobile-dark.png),
[request error](screenshots/design-refresh/acceptance/users-error-mobile-light.png)
and [standalone errors](screenshots/design-refresh/acceptance/errors-review-1.png).
The [capture index](screenshots/design-refresh/acceptance/browser-checks.json)
records the routes, dimensions, states and checked interactions.

Before/after comparisons include the actual dashboard, form, login and dialog,
using source from `654d3f3` and current source with matching fixture data.
See [dashboard light](screenshots/design-refresh/acceptance/comparison-dashboard-light.png)
and [dark](screenshots/design-refresh/acceptance/comparison-dashboard-dark.png).
To reproduce a separate historical-source preview:

```sh
goadmin_baseline=$(mktemp -d)
git archive 654d3f3 resources/src views | tar -x -C "$goadmin_baseline"
ln -s "$PWD/resources/node_modules" "$goadmin_baseline/resources/node_modules"
GOADMIN_DESIGN_SOURCE_ROOT="$goadmin_baseline/resources" \
  GOADMIN_DESIGN_PORT=8769 node resources/audit/design/server.mjs
```

The server still uses current fixture data and installed dependencies; this is
a visual source comparison, not historical release/build reproducibility.

## Sports club presentation demo

Northline is a fictional sports club workspace demonstrating the real GoAdmin
Vue shell, navigation, cards, buttons, inputs and badges. Data is deterministic
and synthetic, anchored to **19 October 2026**. No account or backend is required.

## Run locally

From the repository root, with the Node/npm version supported by `resources`:

```sh
npm ci --prefix resources
npm ci --prefix resources/demo/sports-club
node resources/demo/sports-club/server.mjs
```

Open <http://127.0.0.1:8767/demo/dashboard>. The separate fixture binds only to
`127.0.0.1:8767` and never changes the library's production routes or entry point.
Stop it with Ctrl-C. Empty extension sources are created in an owned temporary
directory, resolved through demo-only Vite aliases, and removed on shutdown.
The developer’s `resources/.admin-extensions` state is neither loaded nor overwritten.
Host extensions are intentionally absent.

The six screens are `dashboard`, `participants`, `training`, `permissions`,
`queue`, and `settings`, all below `/demo/`. Add `?preset=default`,
`?preset=compact`, or `?preset=branded`; add `&mode=dark` for a dark start.
Compact is the default. Branded composes the existing compact and brand theme
extensions. The shared header also supports live theme switching.

## Reproduce the gallery

With the demo server running, in a second terminal:

```sh
# One-time browser setup for the demo's pinned Playwright version:
cd resources/demo/sports-club
npx playwright install chromium
npm test
npm run capture
```

`capture.mjs` uses Chromium, 1× device scale, en-GB locale, UTC, reduced motion,
a fresh browser context per screenshot, and stable fixture data. It waits for
fonts, images, network idle and two layout frames. It rejects console/page errors,
external requests, document overflow and missing active navigation. Tables use
intentional local horizontal scrolling on narrow screens.

Representative interaction checks cover filters, pagination, row actions, editor
preview/save, separate role selections, shared theme and profile menus, and mobile
menu focus wrapping, Escape dismissal and focus return. The synthetic notification
dropdown and queue indicator are demo-only capability cues; the existing access
screen includes fictional administrator assignments. No extra feature page is added. This is deliberately a
curated presentation check rather than every screen/theme/viewport combination.

To reuse an existing installation, set `PLAYWRIGHT_MODULE` to its absolute
`playwright/index.mjs` and optionally `PLAYWRIGHT_EXECUTABLE_PATH` to its Chromium
binary. Browser/platform font changes can change pixels. The committed
`capture-manifest.json` records the actual Playwright package version, Chromium
version and viewport used for this gallery.

## Sports club gallery — earlier visual baseline

These seven captures predate the default-theme refresh. They illustrate the
demo's capabilities and optional presets, not the current default appearance.
Running the demo uses the current shared components; regenerate its gallery to
capture a later visual baseline.

Seven curated viewport images show capabilities rather than every available page.
Desktop dashboards use **1440 × 1100**, tables/forms/access use **1440 × 1000**,
and mobile uses **390 × 844**. Captures use `fullPage: false`, including the README
hero, so the shared fixed sidebar fills the viewport naturally. Scroll locally to
explore the rest of each page; production positioning is unchanged.

### Overview · compact · light

![Compact light sports club overview with synthetic notification and queue indicators](screenshots/dashboard-compact-light.png)

### Notifications · shared dropdown · synthetic

![Synthetic notification bell expanded using shared dropdown components](screenshots/dashboard-notifications.png)

### Tables · compact · light

![Participant directory with filters, statuses, actions and pagination](screenshots/participants-compact-light.png)

### Forms · branded · light

![Grouped training fields and local sports court illustration](screenshots/training-branded-light.png)

### Access · compact · dark

![Illustrative permissions and fictional administrator role assignments](screenshots/permissions-compact-dark.png)

### Theme extension · branded · dark

![Branded dark dashboard using the existing theme extension](screenshots/dashboard-branded-dark.png)

### Responsive shell · compact · mobile

![Mobile viewport showing the shared header, capability cues and overview cards](screenshots/dashboard-compact-mobile.png)

## Boundaries and limitations

This is a presentation fixture, not an authenticated host integration. Mutations
only show local notices or edit in-memory state; reload resets them. Permission
checkboxes and administrator assignments are illustrative, never backend-verified
authorization. Notification items are seeded and only open local notices. Queue jobs,
activity and metrics are seeded snapshots: no workers, notifications, mail,
security checks, uploads or external services execute. The court SVG is a local
original placeholder. Names and `example.invalid` addresses are fictional.

Shared header profile/settings/logout destinations belong to the production
shell and are not implemented by this fixture; use the demo sidebar for its
profile/settings screen. The profile menu is exercised for opening and focus
return only. Shared component defects are not hidden with replacement CSS.
The demo is outside `src`; the production entry never imports it, and production
Tailwind scanning excludes the demo directory. No CI workflow is added.
