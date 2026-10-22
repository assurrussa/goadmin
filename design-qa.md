# Default theme refresh — design QA

Review lens: UX/product engineering and frontend compatibility.

## Findings

No actionable P0/P1/P2 differences remain in the compared screens. The selected
direction is concept 2's tonal surfaces with concept 3's dot-and-text statuses.
Existing navigation, workflows, shell geometry and information density are
explicit product constraints.

The completion pass after `13df47c` closes the remaining agreed screen/state
checklist below. This result covers that explicit local visual scope; the first
pass alone did not establish acceptance of the entire interface.

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

## Initial interaction and responsive evidence

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

## Completion pass: remaining screens and states

The same selected visual direction was checked on real Vue pages with synthetic
props. The fixture now uses the canonical inline theme initializer and does not
save a light preference on first load. Optional fixture capabilities expose the
profile/notification examples without connecting external services.

The layout matrix contains **96 captures**: 12 screens × light/dark ×
1440 × 1024 desktop / 390 × 844 mobile × Russian/English UI. All captures have
matching pixel/CSS dimensions at 1× density, without browser chrome or frames.
No document overflow was observed. DataGrid columns scroll within their own
container. English UI keeps server-supplied Russian titles/data where the
existing application does so; this pass does not add translations.

- Dashboard, permissions, roles: [Russian mobile comparison](docs/screenshots/design-refresh/acceptance/mobile-review-ru-1.png),
  [English](docs/screenshots/design-refresh/acceptance/mobile-review-en-1.png).
- Role view/edit and profile: [Russian](docs/screenshots/design-refresh/acceptance/mobile-review-ru-2.png),
  [English](docs/screenshots/design-refresh/acceptance/mobile-review-en-2.png).
- Profile edit/settings and notifications: [Russian](docs/screenshots/design-refresh/acceptance/mobile-review-ru-3.png),
  [English](docs/screenshots/design-refresh/acceptance/mobile-review-en-3.png).
- Queue dashboard/active/failed: [Russian](docs/screenshots/design-refresh/acceptance/mobile-review-ru-4.png),
  [English](docs/screenshots/design-refresh/acceptance/mobile-review-en-4.png).

Each mobile comparison puts three complete 390px captures beside each other,
light above dark, with 32px labels. The underlying individual desktop/mobile
files and DOM measurements are indexed in
[browser-checks.json](docs/screenshots/design-refresh/acceptance/browser-checks.json).
The contact sheets and representative full-size desktop captures were opened
and inspected, rather than accepted solely from overflow measurements.

Additional full-view before/after inputs use the same real pages, synthetic
props, locale, viewport and theme. The left side renders source from `654d3f3`,
before the redesign; the right side renders the current source. Each input is
2880 × 1056 (two 1440 × 1024 captures and a 32px label strip):
[dashboard light](docs/screenshots/design-refresh/acceptance/comparison-dashboard-light.png),
[dashboard dark](docs/screenshots/design-refresh/acceptance/comparison-dashboard-dark.png),
[form light](docs/screenshots/design-refresh/acceptance/comparison-form-light.png),
[form dark](docs/screenshots/design-refresh/acceptance/comparison-form-dark.png),
[login light](docs/screenshots/design-refresh/acceptance/comparison-login-light.png),
[login dark](docs/screenshots/design-refresh/acceptance/comparison-login-dark.png),
[dialog light](docs/screenshots/design-refresh/acceptance/comparison-dialog-light.png)
and [dialog dark](docs/screenshots/design-refresh/acceptance/comparison-dialog-dark.png).
All eight combined inputs were opened and compared. These compare retained
geometry and the old/new styling; the generated selected image above remains
the style target. Baseline utility scanning explicitly includes the historical
source, including its original 80% black dialog overlay.

Twenty additional table-state captures cover an unfiltered empty table,
filtered empty results, long values, loading and a failed request in both themes
and viewport sizes. The states remain distinguishable: a create action for no
data, filter feedback for no match, a loading label, and an alert/retry action
for an error. Retry after a synthetic 500 restores eight rows, and the delayed
request resolves to eight rows. Examples:
[empty](docs/screenshots/design-refresh/acceptance/users-empty-mobile-light.png),
[filtered empty](docs/screenshots/design-refresh/acceptance/users-filtered-empty-mobile-dark.png),
[long values](docs/screenshots/design-refresh/acceptance/users-long-light.png),
[loading](docs/screenshots/design-refresh/acceptance/users-loading-mobile-dark.png)
and [error](docs/screenshots/design-refresh/acceptance/users-error-mobile-light.png).

The actual Go error template is rendered through `html/template`, independently
of the Vue bundle. Eighteen captures cover 401/403/404/419/429/500/503 at 390px in
both themes, plus 404/500 at 1440px. All titles and widths were checked; both
[mobile error groups](docs/screenshots/design-refresh/acceptance/errors-review-1.png),
[remaining codes](docs/screenshots/design-refresh/acceptance/errors-review-2.png)
and a [full-size dark 500](docs/screenshots/design-refresh/acceptance/error-500-dark.png)
were inspected. The template remains self-contained.

### Confirmed findings, fixes and revised comparisons

1. **P2 — ImageCropper keyboard isolation.** The initial cropper kept focus on
   its opener, exposed no dialog semantics and did not close with Escape.
   Reused the installed Reka dialog primitives with focus containment/return,
   a named title and labeled controls; dismissal stays blocked while processing.
   [Initial/final focused comparison](docs/screenshots/design-refresh/acceptance/iteration-cropper.png),
   [final light](docs/screenshots/design-refresh/acceptance/cropper-light.png)
   and [mobile dark](docs/screenshots/design-refresh/acceptance/cropper-mobile-dark.png)
   were inspected. Reverse Tab, Escape/focus return and a real PNG crop passed.
2. **P2 — queue tabs selected together.** `/queues/jobs-failed` also matched the
   `/queues/jobs` prefix. Match exact routes or slash-delimited descendants,
   strip query/hash, and expose `aria-current`. The revised
   [light navigation](docs/screenshots/design-refresh/acceptance/iteration-queues-light.png)
   and [dark navigation](docs/screenshots/design-refresh/acceptance/iteration-queues-dark.png)
   show one current queue tab. The shell's parent queue link remains current
   separately. Nine tests cover all three pages and query/hash variants.
3. **P2 — notification surface/divider drift.** Removed the remaining header
   gradient and restricted each level color to its left edge, leaving horizontal
   dividers neutral. The [initial/final detail](docs/screenshots/design-refresh/acceptance/iteration-notifications.png)
   and both final theme captures were inspected.
4. **P2 — role-card elevation drift.** Removed page-owned static shadows and
   aligned card radii with the 12px default and host radius token. The
   [initial/final detail](docs/screenshots/design-refresh/acceptance/iteration-role-view.png)
   and role view/edit matrix confirm the flat grouping in both themes.

The raw JSON editor additionally received an accessible label and invalid state;
parsing behavior is unchanged. Its
[invalid mobile state](docs/screenshots/design-refresh/acceptance/editor-json-error-mobile-light.png)
remains readable and clears when valid JSON is restored. Tabs respond to arrow
keys and skip the disabled tab; the mobile formatting disclosure works.

Twelve additional browser assertions passed, including those interactions,
retry/loading recovery, unset system preference without accidental persistence,
manual theme priority and one current queue tab. Four canonical theme unit tests
cover both initial system modes, system change events while unset, and saved
manual choice. The browser exercised the actual current system setting and
manual override; changing the OS preference itself was not emulated.

The five fidelity surfaces were rechecked: the same system font hierarchy and
wrapping remain readable; existing spacing/density and internal table scrolling
are retained; page-owned colors/elevation now follow the neutral tokens; existing
icons and the fixture-only training image render cleanly; original app copy is
retained and synthetic data stays in the fixture. No generated raster becomes
a runtime dependency. No actionable P0/P1/P2 finding remains after recapture.

After the final build and a clean server restart, a fresh tab rendered 14 routes
in both themes: 28 runs, 1× density, no document overflow and zero captured
console warnings/errors. Deliberate HTTP 500 table scenarios and standalone
error responses are recorded separately from this healthy-route console check.

## Implementation checklist and validation

- [x] Apply the selected default style while preserving host token/class overrides.
- [x] Separate status indicators from counts and role/category badges.
- [x] Fix and recapture the checkbox finding in both themes.
- [x] Inspect full-view and focused source/implementation comparisons.
- [x] Exercise representative real screens, keyboard focus and responsive layouts.
- [x] Close the remaining dashboard/access/profile/notification/queue matrix.
- [x] Capture empty/loading/error/long states and the standalone error template.
- [x] Verify tabs, raw JSON editor, real cropping and system/manual theme priority.
- [x] Fix confirmed findings, then inspect revised full and focused evidence.
- [x] Build embedded runtime assets and validate their template references.

The completion pass's **`make check` exited 0**: Go tidy/format/vet/lint, one full
race/coverage pass, 102 Node contract tests, 691 Vitest tests across 60 files,
ESLint, Prettier, Vue/host type checks, Vite production build and verification of
three template assets. This supersedes the first pass's partial `make check`
plus successful `make resources` history. Added meaningful tests cover cropper
focus/processing, queue route boundaries and canonical theme preference behavior.
Independent review found no remaining substantive issue; manifest references
and relative JS imports resolve, and fixture-only markers do not enter the
production bundle. No dependency/public SDK changes were made.

## Open questions and limits

No unresolved design selection. The fixture is read-only and has no backend:
authentication, exports, persistent saves, unlisted optional service pages and
deployment are outside this visual acceptance. There was no exhaustive screen
reader, browser-matrix, 200% zoom or physical touch-device acceptance. Default
mobile density is preserved; optional comfortable presets remain a separate
host choice. The earlier sports-club gallery is explicitly labeled as an older
visual baseline.

final result: passed
