# Presentation pack verification

Implemented and locally verified on 2026-10-22. The review lens is frontend
architecture, lifecycle ownership and accessibility. The standard sidebar and
the topbar/split-auth reference share working page components. The reference
uses its actual source manifest and extension loader.

## Contract and lifecycle

The resource checks cover pack selection/conflicts/missing modules, source-path
validation, runtime exports, optional standard shells and incompatible typed
host contracts. The typed-host gate accepts metadata-only, admin-only and
auth-only packs and rejects extra required shell props.

Interaction tests cover native/as-child disabled and loading activation,
pointerdown/up and keyboard input. Lifecycle tests verify one flash subscription,
notification/upload owner through page changes and cleanup at unmount. A real
Inertia startup regression verifies initial props and one initial flash message.
Auth shells preserve host root classes and form/title/footer slots.

Independent source review verified fixes for startup ordering, styling-token
bypasses, disabled pointerup activation, floating-shadow defaults, split-auth
class forwarding and keyboard scrolling of the horizontal menu. Checkbox
adapters use the installed Reka 2.10.1 controlled `modelValue` contract, also
documented in the [official Checkbox guide](https://reka-ui.com/docs/components/checkbox).
The fixture keeps its generated sources temporary and isolated from host ENV
unless manifests are explicitly supplied; its empty legacy root is independent
of the command's working directory.

## Initial fixture checks

- `make check`: Go tidy/fmt/vet/lint and the full race+coverage pass succeeded.
  That invocation later stopped at a resource VM harness missing the new frame
  imports. The harness was repaired and the resource gate was rerun successfully.
  Go source and dependencies did not change afterward; the Go pass was reused.
- Final `make resources`: 107 manifest/contract tests and 753 unit tests across
  69 files passed, followed by ESLint, Prettier, Vue/host type checks, Vite build
  and verification of all three template asset entries.
- `node --test resources/demo/sports-club/data.test.mjs`: all 6 tests passed;
  they also passed from the `resources/` working directory.
- `npm run build-only` with the topbar manifest and an isolated
  `VITE_ADMIN_PUBLIC_ROOT`: passed and verified the three asset entries.
- Isolated legacy builds for compact + branding together and comfortable alone:
  passed. Standard `public/dist` was preserved and the default generated registry
  was restored after alternate builds.
- `git diff --check`: passed. No public Go package, dependency or migration changed.

The checked-in embedded output contains the standard presentation. Alternate
builds verify local compatibility; they do not establish published-consumer or
deployment readiness.

## Initial fixture browser acceptance

[Machine-readable results](screenshots/presentation-packs/browser-checks.json)
contain 160 distinct runs: 10 routes × 2 packs × 2 modes × RU/EN × two viewports
(1440×1024 and 390×844). Routes cover users, edit form, login, MFA, both password
recovery screens, notifications, dialog, component catalog and long nested menu.
Each run asserted rendering, selected pack/mode/locale, exact viewport and absence
of page-level horizontal overflow. All raw capture dimensions matched the records.

There are 76 additional interaction/style checks: mobile drawer focus wrapping,
inert/scroll lock and Escape restoration; dialog focus loops and bounds; nested
menu keyboard access and parent destinations; checkbox mouse/Space behavior;
select and disabled tab behavior; disabled/loading link activation; table selection
and action cursors; readonly/invalid attributes; host size overrides; theme
persistence; empty tables, limited capabilities and missing menus. The final fresh
browser console had no warnings or errors.

Computed cursors were pointer for available actions, text for text entry,
not-allowed for disabled actions and progress for loading actions. Keyboard focus
was visible and hover brightness differed from rest. The final horizontal-menu
trigger fits inside its scroll container without moving the document vertically.

Normal primary button text contrast was 5.77:1 / 7.83:1 for standard light/dark
and 5.49:1 / 10.82:1 for reference light/dark. All 16 measured primary/success/
warning/danger combinations exceeded 4.5:1 (minimum 5.46:1). Editable text measured
14.97:1 / 13.55:1. Readonly text now measures 5.65:1 / 6.94:1 after replacing the
old tertiary text color that produced 4.49:1 in light mode. These measurements
cover those specific text/background pairs; custom host colors are not certified.

## Visual evidence

Desktop comparisons place standard on the left and reference on the right:

- Users: [light](screenshots/presentation-packs/comparison-users-light.png),
  [dark](screenshots/presentation-packs/comparison-users-dark.png).
- Login: [light](screenshots/presentation-packs/comparison-login-light.png),
  [dark](screenshots/presentation-packs/comparison-login-dark.png).

Each mobile contact sheet includes light and dark rows. Set 1 covers users,
form, login, MFA and forgot-password; set 2 covers reset-password, notifications,
dialog, catalog and long menu. All eight sheets were visually inspected:

- Standard RU: [set 1](screenshots/presentation-packs/default-ru-mobile-review-1.png),
  [set 2](screenshots/presentation-packs/default-ru-mobile-review-2.png).
- Standard EN: [set 1](screenshots/presentation-packs/default-en-mobile-review-1.png),
  [set 2](screenshots/presentation-packs/default-en-mobile-review-2.png).
- Reference RU: [set 1](screenshots/presentation-packs/topbar-ru-mobile-review-1.png),
  [set 2](screenshots/presentation-packs/topbar-ru-mobile-review-2.png).
- Reference EN: [set 1](screenshots/presentation-packs/topbar-en-mobile-review-1.png),
  [set 2](screenshots/presentation-packs/topbar-en-mobile-review-2.png).

Targeted evidence includes [keyboard navigation](screenshots/presentation-packs/topbar-long-menu-keyboard.png),
[nested menus](screenshots/presentation-packs/topbar-nested-menu.png),
[mobile drawer](screenshots/presentation-packs/topbar-dark-mobile-menu.png),
[mobile dialog](screenshots/presentation-packs/topbar-light-mobile-dialog.png),
[keyboard focus](screenshots/presentation-packs/default-light-keyboard-focus.png)
and [readonly contrast](screenshots/presentation-packs/topbar-light-readonly-contrast.png).

## Initial fixture verification limits

This is one in-app browser environment with synthetic data. Host-supplied sample
content remains Russian in some EN runs; shared UI language changes were checked.
Submissions are rejected by the fixture. Login, MFA, recovery, exports and
persistent CRUD therefore have visual/interaction coverage only, without an
authentication backend or production deployment. Screen-reader certification,
physical devices and cross-browser testing were not performed.

The second pack demonstrates the extension boundary. It is not a second polished
product design. Standalone HTML error documents retain their independent neutral
light/dark fallback and do not load pack CSS.

## Real starter acceptance

The follow-up used `make starter-smoke KEEP=1` and fresh isolated source builds,
PostgreSQL and upload volumes for core plus `access,jobs,uploads,queues,realtime`.
Run `20261022t140046z-9e218af9` passed all seven scenario groups per pack in
Chromium 155.0.8059.12 (Playwright 1.64.0-alpha-1790635538000), with zero browser
console errors or page errors. Its ignored artifacts are in
`output/playwright/starter-smoke/20261022t140046z-9e218af9/`; `report.json` includes
the source revision/dirty flag, resolved dependency versions/replacements, image
IDs, scenario results and links to screenshots, traces and container diagnostics.

Real first-admin setup, login/logout and protected-route denial passed. Profile
checks verified server rejection of whitespace names, focus on the invalid
control, correction, one flash and persistence after reload. A separately
created administrator with only `queues:read` could view queues, had no admin
menu and received HTTP 403 for a direct `/admins` request.

The avatar scenario retained genuine terminal server responses until after an
Inertia navigation unmounted the form. It released the unchanged WebSocket
payload while terminal polling remained held, then verified the global queue,
avatar URL, a non-empty stored image response and persistence after reload. Returning to the
matching field explicitly reconciled its retained completion fence without
another TUS create or PATCH. File readiness did not release the field fence or
claim that a host form was saved.

Mobile drawer focus wrapping, inert content, Escape focus restoration, absence
of horizontal overflow, RU/EN persistence and light/dark persistence passed on
both real hosts. Computed paint and desktop screenshots confirmed standard blue
and topbar teal, with dark surfaces and light text after reload. This exposed
and fixed production-only CSS layer ordering that the initial fixture had
missed: core CSS now establishes `theme, base, components, utilities` before
extension CSS. Both actual Vite production builds and the asset gate check it.

`make check` passed before the final frontend corrections. The unchanged Go
tidy/fmt/vet/lint/race+coverage pass was reused; the final `make resources` passed
108 manifest/build checks, 781 unit tests across 70 files, types, lint, formatting
and the default asset build. `make starter-smoke-test` passed eight Python and
five JavaScript checks. The nested starter readonly Go tests passed after
aligning its GoAuth requirement to the root's existing v0.12.0. The root module,
public Go APIs and migrations did not change.

The resolved runtime uses GoAuth v0.12.0, GoNotify v0.6.0, GoUploads v0.13.0 and
GoWebSocket v0.2.1; goadmin is the local source replacement, not a published
candidate. Both instances were retained at `http://localhost:18768` and
`http://127.0.0.1:18769`. Mail/NotifyHub, MFA/recovery backend scenarios,
cross-browser and physical-device acceptance remain outside this follow-up.
These local checks do not establish release publication or production readiness.
