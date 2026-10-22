# Default theme refresh — design QA

Review lens: UX/product engineering and frontend compatibility.

## Findings

No actionable P0/P1/P2 differences remain in the compared screens. The selected
direction is concept 2's tonal surfaces with concept 3's dot-and-text statuses.
Existing navigation, workflows, shell geometry and information density are
explicit product constraints.

## Visual truth and comparison setup

- Source visual truth: [selected light/dark target](docs/screenshots/design-refresh/selected-light-dark.png),
  a generated 2103 × 748 image, light on the left and dark on the right.
- Implementation: <http://127.0.0.1:8768/users>, real Users/DataGrid/AdminLayout
  components with eight synthetic accounts and optional capabilities disabled.
- Browser screenshots: [light](docs/screenshots/design-refresh/users-after-light.png)
  and [dark](docs/screenshots/design-refresh/users-after-dark.png), 1440 × 1024
  pixels at a 1440 × 1024 CSS viewport, 1× device scale. These final captures were
  taken in a fresh browser tab after the resource build.
- State: Russian locale, expanded sidebar, no filter, no selection or overlay;
  one inactive and seven active accounts. No browser chrome or device frame.
- Density normalization: the 1051/1052 × 748 source halves were scaled to
  1440 × 1024 for comparison. This normalizes image dimensions, not component
  geometry. The generated proposal is not a pixel-exact specification.
- Combined full-view inputs opened and inspected: [light comparison](docs/screenshots/design-refresh/comparison-light.png)
  and [dark comparison](docs/screenshots/design-refresh/comparison-dark.png).
  Each is 2880 × 1056, source left and implementation right, with a 32px label strip.
- Focused inputs opened and inspected: [light table/status detail](docs/screenshots/design-refresh/comparison-status-light.png)
  and [dark table/status detail](docs/screenshots/design-refresh/comparison-status-dark.png).
  These make labels, dots, row dividers and action icons readable at full size.

## Required fidelity surfaces

- **Fonts and typography:** the implementation uses the system sans stack,
  with a 24px/600 page heading and existing small UI text. The target's generated
  glyphs imply a similar UI sans, not an identifiable font file. Hierarchy,
  optical weights and wrapping were inspected in the full and focused views.
  Existing label copy and table content remain readable; an external font
  download is no longer required.
- **Spacing and layout rhythm:** preserve the existing 256px sidebar, 68px
  header, control heights and table row spacing. The normalized proposal has a
  wider sidebar and larger-looking text; those differences are accepted under
  the explicit density constraint. Shared controls use 8px corners and cards
  12px corners. Tonal filter grouping, flat table surfaces and the unboxed action
  header follow the selected direction. No document overflow at 1440px, 834px
  or 390px; narrow tables scroll inside their container.
- **Colors and tokens:** light surfaces are white/neutral gray; dark surfaces
  are graphite with restrained blue emphasis. Static glow, gradients and heavy
  shadows were removed from the changed shared surfaces. Dots retain semantic
  colors and always accompany text. Primary, success, warning and danger button
  text pairs measured at least 4.5:1 in both themes: light 5.77/5.46/8.33/5.57,
  dark 7.83/5.46/8.33/6.60. The automated dark text check covers 12 combinations
  of primary/secondary/tertiary text and canonical surfaces; its minimum is
  5.18:1. This is scoped contrast evidence, not a full accessibility certification.
- **Image quality and assets:** no generated raster assets ship in the app.
  The existing native brand, initials avatars and Lucide icon family are retained;
  the status marker uses Lucide Circle with decorative semantics. The target has
  no product photography or illustration requiring replacement. No invented
  decorative artwork was added. Comparison images are QA artifacts only.
- **Copy and content:** existing labels, routes, translations and table values
  are retained. Unknown status values remain visible as text. Fictional names
  and `example.invalid` addresses belong only to the fixture; design-process
  language does not enter production screens.

## Comparison history

1. **Blocked — P2, row selection affordance.** Initial rendered checkboxes became
   faint round outlines, whereas the target uses distinct square selection
   controls. Evidence: [initial light comparison](docs/screenshots/design-refresh/comparison-first-light.png)
   and [initial dark comparison](docs/screenshots/design-refresh/comparison-first-dark.png).
   This made unselected rows harder to identify. Fixed `Checkbox.vue` with an
   explicit 4px radius and a visible tertiary border; checked/focus/disabled
   semantics are preserved.
2. **Passed — revised comparison.** Recaptured the same viewport and row state.
   The final full-view and focused inputs linked above show distinct checkboxes,
   readable dot-and-text states, tonal surfaces and unchanged information
   density. Row selection was also exercised in the browser. No further visual
   fixes followed this comparison.

Independent source review additionally identified and verified fixes for native
primary actions using hardcoded white text in dark mode and the `text-error`
utility losing the host's canonical color override. These source-contract fixes
are not counted as extra visual-QA iterations.

## Interaction and responsive evidence

Seventeen browser checks passed: inactive filter returns its one matching row;
row selection works; dark mode survives reload; light/dark action contrast;
explicit 64px/zero-radius host classes win; the host error-color override reaches
form errors; invalid/readonly/disabled form states remain; dialog autofocus;
Escape dismissal and focus return; real user-form typing; 390px login fit;
mobile-menu Escape/focus return; local table scrolling at 390px; English mobile
fit; locale persistence; and tablet fit at 834px.

Representative captures: [light form](docs/screenshots/design-refresh/form-light.png),
[dark form](docs/screenshots/design-refresh/form-dark.png),
[light dialog](docs/screenshots/design-refresh/dialog-light.png),
[dark dialog](docs/screenshots/design-refresh/dialog-dark.png),
[light login](docs/screenshots/design-refresh/login-light.png),
[dark login](docs/screenshots/design-refresh/login-dark.png),
[mobile login](docs/screenshots/design-refresh/login-mobile.png),
[light mobile table](docs/screenshots/design-refresh/users-mobile-light.png),
[dark mobile table](docs/screenshots/design-refresh/users-mobile-dark.png),
[mobile menu](docs/screenshots/design-refresh/users-mobile-menu.png),
[English mobile](docs/screenshots/design-refresh/users-mobile-en.png) and
[tablet](docs/screenshots/design-refresh/users-tablet.png).

The final fresh users tab has zero captured console warnings/errors in both
themes. Earlier fixture startup errors caused by stale Vite dependency instances
were resolved with isolated cache/deduplication and a clean restart; they are not
present in the final run.

## Implementation checklist and validation

- [x] Apply the selected default style while preserving host token/class overrides.
- [x] Separate status indicators from counts and role/category badges.
- [x] Fix and recapture the checkbox finding in both themes.
- [x] Inspect full-view and focused source/implementation comparisons.
- [x] Exercise representative real screens, keyboard focus and responsive layouts.
- [x] Build embedded runtime assets and validate their template references.

`make check` passed Go tidy/format/vet/lint and one full race/coverage test pass.
It initially stopped in a resource test tied to the previous exact HSL values.
That assertion was replaced with contrast calculations and existing semantic
checks retained. The remaining gate, `make resources`, then passed: 102 Node
contract tests, 676 Vitest tests across 57 files, ESLint, Prettier, Vue/host type
checks, Vite production build and verification of three template assets.
Unchanged Go checks were reused rather than repeating the race pass.

## Open questions and limits

No unresolved design selection. The fixture is read-only and has no backend:
authentication, exports, persistent saves, optional service pages and deployment
are outside this visual acceptance. The standalone server-error template was
source-reviewed but not captured in the browser. There was no exhaustive screen
reader, browser-matrix, 200% zoom or physical touch-device acceptance. Default
mobile density is preserved; optional comfortable presets remain a separate
host choice. The earlier sports-club gallery is explicitly labeled as an older
visual baseline.

final result: passed
