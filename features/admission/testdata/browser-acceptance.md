# Optional admission browser acceptance

This test-only harness is excluded unless both `integration` and `browser`
build tags are supplied. It serves the real assembled host on an ephemeral
127.0.0.1 TLS listener and uses the committed embedded Vue assets/templates.
It does not start a Vite server, create production credentials, change system
trust, install packages, change CI, or expose storage/control endpoints.

Prerequisites:
- The repository's normal integration-test PostgreSQL configuration. The
  existing `tests.PrepareDB` helper creates and removes a unique disposable DB.
- A currently valid localhost/127.0.0.1 certificate and matching private key,
  already trusted by the selected Chromium/Chrome browser.
- An installed Playwright module and browser executable.
- An explicitly selected disposable browser profile directory. Do not select
  your everyday browsing profile. The harness creates additional isolated
  browser contexts and closes them at the end.

Set these environment variables to the existing local paths:
- `GOADMIN_BROWSER_TLS_CERT`: certificate PEM path
- `GOADMIN_BROWSER_TLS_KEY`: private-key PEM path
- `GOADMIN_BROWSER_EXECUTABLE`: trusted installed Chrome/Chromium executable
- `GOADMIN_BROWSER_PROFILE`: disposable profile path
- `GOADMIN_PLAYWRIGHT_MODULE`: absolute installed Playwright package path
- Optional `GOADMIN_BROWSER_HEADLESS=0` for visible browser windows

Run from the repository root, with the approved source-mode GoAuth dependency
configuration, normal shared Go caches and existing test service settings:

```sh
GOWORK=off go test -tags=integration,browser ./features/admission \
  -run '^TestAdmissionRealBrowser$' -count=1 -timeout=5m -v
```

There is deliberately no certificate-error bypass or automatic certificate
generation. An expired/untrusted certificate is a failed prerequisite.

The runner performs real UI login, renders waiting and management pages, submits
the embedded approval/invitation forms, sends adversarial browser requests, and
observes committed SQL effects over private parent/child pipes. It checks:
- pending identities cannot access protected native routes or create sessions
- forged callback state cannot enroll an identity
- observer permission denial, missing CSRF, foreign Origin, and role-preview
  mutation denial leave persisted admissions, memberships, roles, credentials,
  canonical sessions, links, invitations and audit-event counts unchanged
- approval alone does not activate membership
- a fresh, browser-bound verified callback activates membership and issues the
  native external session without creating a local password
- invitation acceptance by a different identity or without the exact project
  grant cannot consume the invitation or create authority

Synthetic scenario selectors choose fixed server-side identities. They never
accept identity JSON. One-time random callback state is bound to an HttpOnly,
Secure browser cookie; the waiting-page opaque reference is bound to that same
browser. All fixture routes exist only inside the test binary.

A pass requires the Go test and runner to reach their terminal assertions.
Compilation, source review, UI unit tests or a listening port alone are not a
browser acceptance pass.
