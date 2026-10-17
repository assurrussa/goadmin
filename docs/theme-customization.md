# Theme customization: stage one

The supported first-stage contract covers the administration shell. It does not
promise arbitrary replacement of all forms or page layouts. Without a theme file,
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

## Opt-in example

[`resources/examples/theme-shell/extension.json`](../resources/examples/theme-shell/extension.json)
is a complete source manifest with a verified source fingerprint. Its module
imports only local CSS, demonstrates `72px`/`18rem`/`5rem`/`76rem` geometry,
profile-trigger radius/shadows, branding hue and font. It is not enabled in the
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

This stage exposes shell radius/shadow overrides only, not every control or dialog.
No preset switcher, component replacement, database setting or automatic consumer
update is included.
