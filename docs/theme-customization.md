# Theme customization: shell and shared controls

Stage one covers the administration shell. Stage two adds dimensions and radius
for the shared components listed below. Neither stage promises arbitrary
replacement or styling of every form or page layout. Without a theme file,
geometry, shadows and radius retain their previous defaults. Auth, permissions,
CSRF and navigation semantics are unchanged.

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
| `--goadmin-header-shadow` | Previous Tailwind `shadow-sm` | Header shadow |
| `--goadmin-sidebar-shadow` | Previous Tailwind `shadow-lg` | Sidebar shadow |

The two original Tailwind shadows are preserved in internal fallback variables;
they differ from the existing card-level `--shadow-sm`/`--shadow-lg` values.
Override only the documented shell variables. Existing utility classes and
internal fallback variable names are not a stable customization API.

Existing color/font variables are reused: `--hue-primary`,
`--saturation-primary`, `--hue-neutral`, `--font-sans`, and semantic `--color-*`
values. Keep status/error distinctions and visible focus. Size values must be
valid CSS lengths and allow both languages, controls and mobile navigation to
fit. Custom colors are not automatically contrast-certified.

## Stage two: shared controls and cards

| Variable | Previous default |
| --- | --- |
| `--goadmin-control-height-xs` | `1.75rem` (28px) |
| `--goadmin-control-height-sm` | `2rem` (32px) |
| `--goadmin-control-height` | `2.25rem` (36px), md/default and default icons |
| `--goadmin-control-height-lg` | `2.5rem` (40px) |
| `--goadmin-control-height-xl` | `2.75rem` (44px), AppButton xl |
| `--goadmin-control-radius` | Existing `--radius-md` (currently 12px); final fallback `0.375rem` |
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
zero top padding, unless an explicit host class overrides it. Existing theme radius
variables remain the fallback, rather than silently changing the current 12px
controls to a literal 6px radius.

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
