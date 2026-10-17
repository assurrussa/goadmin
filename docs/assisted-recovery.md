# Assisted recovery module

`host.AssistedRecoveryModule` explicitly mounts guarded administrator initiation
and a public invitation-completion page. Configure a managed mail-capable
`AuthAdapter` and `host.NewLocalAssistedRecovery` for local accounts. A central
authority may instead supply a `goauth.AssistedRecovery` scoped backend. This
module grants no roles or permissions automatically.

The dedicated `account_recovery:create` grant is rechecked on the canonical
transaction handle. A normal account-update grant is insufficient. The operator
must attest to independent identity verification, record an auditable nonsecret
reference/reason, and freshly prove password and enrolled MFA. A new email or
matching name alone is insufficient. The host remains responsible for the
human verification procedure.

Initiation is `/account-recovery`; completion is `/auth/assisted-recovery`.
Optional owner completion is isolated at `/auth/assisted-recovery/owner` and never
exposes an owner-initiation HTTP method. Forms use exact-origin checks, strict
single-valued input, a Secure HttpOnly SameSite CSRF cookie, no-store responses,
no-referrer and a restrictive CSP. Invitation secrets are read from fragments
and removed from browser history before submission. External SSO admin bindings
cannot be used as local password authority.

Custom senders must support `assisted_recovery` with `Data["url"]` and the secret-free
`assisted_recovery_completed` security notice. Use stable delivery IDs and the
existing encrypted queue; keep the notification worker running. Starter email
templates for both events are included. Never log bodies, tokens or raw notices.

The source adds focused CSRF, fragment, permission-catalog and mail tests. They
are unexecuted in the source-editing environment because a Go toolchain and a
writable build/test environment are unavailable. Format, resolve candidate
dependency sums, run aggregate checks and isolated integration tests before
review/release/deployment. No automatic CI trigger is added.
