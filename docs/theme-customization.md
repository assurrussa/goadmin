# Theme customization: shell and shared controls

Stage one covers the administration shell. Stage two adds dimensions and radius
for the shared components listed below. Neither stage promises arbitrary
replacement or styling of every form or page layout. Without a theme file, the default appearance uses neutral light and graphite dark
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

The existing extension registry eagerly imports the module and its CSS. No new
loader, theme manager, runtime CSS editor, CDN, relaxed CSP or ENV is required.
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

This contract applies only to shared `Button`/`AppButton`, `Input`/`AppInput`/
`FormInput`, `Textarea`, `SelectTrigger`, `Card`/`AppCard`, and
`CardHeader`/`CardContent`/`CardFooter`. Handwritten HTML inputs, buttons, cards and
page-specific layouts are outside this stage. Heights do not change font size,
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

The contract remains limited to the shell and the listed shared controls/cards.
No dialog-wide styling, preset switcher, component replacement, database setting or
automatic consumer update is included.

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
