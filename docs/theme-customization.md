# Admin presentation: themes, shells and shared controls

The default presentation and build-time packs share page logic and interaction
behavior. Packs can replace the admin/auth shells and style documented component
parts; they do not replace business pages, authorization or runtime services.
The earlier shell/control CSS stages remain supported. Without a theme file, the default appearance uses neutral light and graphite dark
surfaces, a system font stack, restrained blue accents and semantic status colors.
The shell, shared controls and static cards have no shadows; floating menus retain
subtle elevation. Auth, permissions, CSRF and navigation semantics are unchanged.

## Project-owned CSS

Add a local stylesheet to your existing admin extension source root and import it
from an `extensions/theme.ts` module. A CSS-only module may export an empty default
object using the existing optional `AdminExtension` contract:

```ts
import '../theme.css'
export default {}
```

The existing extension registry eagerly imports the module and its CSS. CSS-only customization needs no theme declaration, runtime CSS editor, CDN,
relaxed CSP or additional environment variable.
Light/dark selection remains the user's existing preference; host branding is
independent of that choice. Use `:root.dark` and
`:root[data-theme-mode='dark']` for dark-only overrides.

## Shell variables

| Variable | Default | Shared use |
| --- | --- | --- |
| `--goadmin-header-height` | `68px` | Header and sidebar logo row, including border |
| `--goadmin-sidebar-width` | `16rem` | Expanded/mobile sidebar and desktop content offset |
| `--goadmin-sidebar-collapsed-width` | `5rem` | Collapsed desktop sidebar and content offset |
| `--goadmin-content-max-width` | `80rem` | Header row and main content |
| `--goadmin-menu-radius` | `1rem` | Profile dropdown trigger |
| `--goadmin-header-shadow` | `none` | Header shadow |
| `--goadmin-sidebar-shadow` | `none` | Sidebar shadow |

Override the documented shell shadow variables to add elevation. Their internal
fallbacks default to `none`; floating-menu elevation remains independent. Existing
utility classes and internal fallback variable names are not a stable customization
API.

Existing color/font variables are reused: `--hue-primary`,
`--saturation-primary`, `--hue-neutral`, `--font-sans`, and semantic `--color-*`
values. Keep status/error distinctions and visible focus. Size values must be
valid CSS lengths and allow both languages, controls and mobile navigation to
fit. Custom colors are not automatically contrast-certified.

## Stage two: shared controls and cards

| Variable | Default |
| --- | --- |
| `--goadmin-control-height-xs` | `1.75rem` (28px) |
| `--goadmin-control-height-sm` | `2rem` (32px) |
| `--goadmin-control-height` | `2.25rem` (36px), md/default and default icons |
| `--goadmin-control-height-lg` | `2.5rem` (40px) |
| `--goadmin-control-height-xl` | `2.75rem` (44px), AppButton xl |
| `--goadmin-control-radius` | Existing `--radius-md` (currently 8px); final fallback `0.375rem` |
| `--goadmin-control-radius-xs` | `0.25rem` (4px), xs buttons |
| `--goadmin-textarea-min-height` | `60px` |
| `--goadmin-card-radius` | Existing `--radius-xl` (currently 12px); final fallback `0.75rem` |
| `--goadmin-card-padding` | `1.5rem` (24px) |

The original dimension contract applies to shared `Button`/`AppButton`, `Input`/`AppInput`/
`FormInput`, `Textarea`, `SelectTrigger`, `Card`/`AppCard`, and
`CardHeader`/`CardContent`/`CardFooter`. These size tokens do not automatically resize handwritten HTML controls or
page-specific layouts. The shared interaction defaults below also cover native
controls, independently of these dimension tokens. Heights do not change font size,
horizontal padding, colors, shadows, error/readonly/disabled states or transitions.

FormInput icon/loading rows follow its selected sm/md/lg height. Icon buttons stay
square. `AppButton rounded=true` remains a pill; explicit class overrides retain
priority through the existing `cn`/Tailwind merge behavior. CardContent/Footer keep
zero top padding, unless an explicit host class overrides it. Theme radius variables remain the fallback: default controls use 8px corners and
cards use 12px corners. The documented `--goadmin-*` radius variables and explicit
classes retain priority over these defaults.

## Opt-in example

[`resources/examples/theme-shell/extension.json`](../resources/examples/theme-shell/extension.json)
is a complete source manifest with a verified source fingerprint. Its module
imports only local CSS, demonstrates `72px`/`18rem`/`5rem`/`76rem` geometry,
profile-trigger radius/shadows, branding hue/font and stage-two control/card
overrides. It is not enabled in the
default build.

Add this manifest to the same existing `VITE_ADMIN_EXTENSION_MANIFESTS` input used
for project extensions, for example from `resources/`:

```sh
VITE_ADMIN_EXTENSION_MANIFESTS=examples/theme-shell/extension.json npm run build-only
```

This opt-in build replaces the chosen output bundle; keep a host's other manifests
in its normal manifest list. Recompute the source fingerprint with
`scripts/materialize-admin-extensions.cjs` after editing the example's source,
using its existing `fingerprintSource(sourceRoot)` function. Do not copy a stale
fingerprint or edit the generated bundle directly.

This CSS-only example does not select a shell pack. It adds no runtime preset
switcher, database setting or automatic consumer update.

## Stage three: optional density presets

Two local CSS presets use only the documented shell/control variables:

- [Compact](../resources/examples/theme-compact/extension.json): 64px header,
  15rem sidebar, 32px default controls, 16px card padding, tighter corners.
- [Comfortable](../resources/examples/theme-comfortable/extension.json): 72px header,
  17rem sidebar, 44px default controls, 28px card padding, softer corners.

Both preserve the existing semantic colors, focus styles, light/dark preference,
fonts and behavior. Neither is enabled by default. Without either manifest,
existing styling remains unchanged. On viewports below 768px or coarse-pointer
devices, shared controls have at least 44px height and card padding is 16px.
This is not a claim that every custom control has a 44px touch target.

Choose one preset manifest in your existing extension manifest list:

```sh
VITE_ADMIN_EXTENSION_MANIFESTS=examples/theme-compact/extension.json npm run build-only
# Or choose examples/theme-comfortable/extension.json instead.
```

Preserve your project's other extension manifests. Do not combine both density
presets or the earlier theme-shell example: their root variables overlap and
CSS import order is not a preset-selection contract. To customize, copy the
chosen example to your project's extension source, edit the CSS and regenerate
its source fingerprint. Standard styling is restored by removing the preset
manifest and rebuilding. Existing explicit component class overrides still win.

These are build-time examples, not a runtime theme selector or profile setting.
They do not change AuthHub/NotifyHub dependencies or deploy a consumer. Handwritten
controls and page-specific layouts remain outside the shared-component contract.

## Optional branding example

[Branding](../resources/examples/theme-brand/extension.json) adds a local system
font and blue primary/focus colors, with explicit light/dark foreground pairs.
It is not enabled by default and does not override density or status colors.
The example's primary text pairs meet 4.5:1 contrast; validate other color
combinations in your own screens when adapting the palette.

Compose it with one density preset through the existing manifest list, from
`resources/` (use `;` instead of `:` as the list separator on Windows):

```sh
VITE_ADMIN_EXTENSION_MANIFESTS=examples/theme-compact/extension.json:examples/theme-brand/extension.json npm run build-only
```

Keep other project manifests in the list. Do not combine compact and comfortable
or theme-shell. Copy the branding source into your project to customize it,
regenerate its fingerprint as above, and rebuild. Remove the branding manifest
and rebuild to restore the default palette. Light/dark preference remains under
the existing theme controls.

For a UI-only preview using the existing synthetic layout fixture, run
`GOADMIN_FIXTURE_BRANDING=1 node scripts/ui-browser-fixture.mjs` from `resources/`
and open `http://127.0.0.1:8766/__ui`. It displays the compact/branding examples
with shared Input and Button controls; use the header's existing theme toggle.


## Build-time presentation packs

Use the existing source-manifest mechanism to select one pack. In an extension
entry, `themeModule` is a POSIX path relative to `sourceRoot`, for example
`extensions/theme.ts`. It must name an existing `.ts` file under `extensions/`;
absolute paths, traversal, backslashes, declarations and symlinks are rejected.
The materializer rejects multiple selected packs before replacing generated output.
No selection uses the built-in design. The runtime bundle manifest retains its
version-1 shape; `themeModule` is source-build metadata.

The selected module exports an ordinary `AdminExtension`:

```ts
import type { AdminExtension } from '@admin-core/extensions'
import { defineAdminTheme } from '@admin-core/presentation'
import AdminShell from '../AdminShell.vue'
import AuthShell from '../AuthShell.vue'
import '../theme.css'

export default {
  theme: defineAdminTheme({
    id: 'my-project.presentation',
    apiVersion: 1,
    adminShell: AdminShell,
    authShell: AuthShell,
  }),
} satisfies AdminExtension
```

`adminShell` and `authShell` are independently optional. Omit either to reuse its
standard shell. The facade exports `DefaultAdminShell`, `DefaultAuthShell`,
`AdminThemePack`, the shell component/props/slots types and `defineAdminTheme`.
Use the helper and the resource type-check gate: a shell cannot require extra
host data that the core does not supply. At bootstrap the selected export, ID
and API version are validated. A `theme` export without a matching `themeModule`
declaration is rejected rather than silently competing with another import.

An admin shell uses `defineProps<AdminShellProps>()` and
`defineSlots<AdminShellSlots>()`. The core supplies `collapsed` and these slots:

- `header`: the complete standard header, for shells retaining that arrangement;
- `brand`, `navigationToggle`, `utilities`, `account`: independently placeable
  standard controls for other header arrangements;
- `navigation`: call with `orientation: 'vertical' | 'horizontal'` and an optional
  `collapsed` boolean; both presentations use the same server-supplied menu;
- `mobileNavigation`: the complete core-owned mobile drawer;
- `breadcrumbs` and `default`: breadcrumbs and the unchanged page.

Render `mobileNavigation` once. Choose either `header` or the individual header
slots, so controls are not duplicated. Horizontal navigation retains nested and
parent links through accessible dropdowns; its section row scrolls when long.
On narrow screens use `navigationToggle` and `mobileNavigation`. The sidebar's
collapse preference remains stored when a horizontal shell ignores it.

An auth shell uses `AuthShellProps` (optional root `class`) and `AuthShellSlots`:
`brand`, `utilities`, `title`, optional `subtitle`, `default` and optional `footer`.
Existing login/provider, MFA, recovery and registration forms keep their slots
and handlers. Explicit host page layouts remain authoritative.

Neither shell starts services. The application frame owns flash subscriptions,
notifications and the capability-gated upload queue. Mobile navigation owns its
inert state, scroll lock, focus loop, Escape/navigation/breakpoint closing and
focus restoration. Authentication and WebSocket ownership stay in bootstrap.
Dialogs keep their existing Reka lifecycle. A shell must not recreate these
services or copy their controllers.

### Create and verify a pack

Copy the [topbar reference](../resources/examples/theme-topbar/extension.json) into
your own extension source, change its ID, shells and CSS, regenerate its source
fingerprint and include its manifest with the project's other manifests. From
`resources/`, the checked-in reference builds with:

```sh
VITE_ADMIN_EXTENSION_MANIFESTS=examples/theme-topbar/extension.json npm run build-only
```

This replaces the selected output bundle; preserve the project's other extension
manifests. Rebuild and embed the matching client as described in
[embedding-client.md](embedding-client.md). There is no runtime design picker.
Light/dark remains independent of the pack ID. CSS-only density/branding manifests
continue to work without declaring a theme. When composing CSS that writes the
same variables as a pack, explicitly own those overrides in the host stylesheet;
CSS import order is not a selection contract.

The topbar/split-auth pack is a reference, not a second product design. It changes
accent and geometry, uses both shell points and includes no copies of working
pages. The [component catalog and browser fixture](presentation-demo.md) exercise
actual source-manifest selection. Type-check, test and visually inspect a custom
pack in both modes/languages and on desktop/mobile before adopting it.

## Component styling and interaction contract

Stable `data-ui` families are `button`, `input`, `textarea`, `select`, `checkbox`,
`card`, `tabs`, `badge`, `status`, `dialog`, `dropdown-menu`, `table` and `datagrid`.
Use `data-part` to address an actual part, for example
`[data-ui='button'][data-part='root']`, `[data-ui='tabs'][data-part='trigger']`,
`[data-ui='dialog'][data-part='content']` or
`[data-ui='datagrid'][data-part='sort']`. Composite parts follow their component
names: root/header/content/footer/title/description, list/trigger/item/value,
overlay/close and table header/body/row/head/cell/empty. Select and dropdown also
expose their group, separator, scrolling and nested-menu parts. Hooks identify
presentation surfaces; existing `aria-*` and Reka `data-state` attributes identify
interaction states. Internal utility classes are not a supported selector API.

Additional optional CSS variables use the existing semantic values as fallbacks:

- `--goadmin-control-bg`, `--goadmin-control-border`;
- `--goadmin-card-bg`, `--goadmin-card-text`, `--goadmin-card-border`;
- `--goadmin-menu-bg`, `--goadmin-menu-text`, `--goadmin-menu-border`,
  `--goadmin-menu-content-radius`, `--goadmin-menu-shadow`;
- `--goadmin-dialog-bg`, `--goadmin-dialog-text`, `--goadmin-dialog-border`,
  `--goadmin-dialog-radius`, `--goadmin-dialog-padding`,
  `--goadmin-dialog-shadow`, `--goadmin-dialog-overlay`;
- `--goadmin-table-header-bg`, `--goadmin-table-row-hover`;
- `--goadmin-control-hover-brightness` (0.97) and
  `--goadmin-control-active-brightness` (0.94).

Apply pack rules in a low-priority CSS layer such as `@layer components`, scoped
to `html[data-admin-theme='your-pack-id']`, so explicit host utility classes keep
priority. Preserve semantic errors/statuses and visible focus. Controls use
pointer cursors for available actions, text cursors for text entry, and disabled
or busy cursors for unavailable actions. Native disabled and shared button/link
activation guards remain core behavior; visual CSS is not an authorization gate.
Custom CSS can invalidate contrast, focus or geometry, so passing type checks does
not certify a custom theme's accessibility.

Standalone HTML errors deliberately keep their shared neutral light/dark styling
and remain readable without the Vue bundle or pack CSS. Public-site components,
arbitrary page replacement and a runtime component registry are outside this API.
