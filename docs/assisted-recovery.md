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
single-valued input, the CSRF contracts described below, no-store responses,
no-referrer and a restrictive CSP. Invitation secrets are read from fragments
and removed from browser history before submission. External SSO admin bindings
cannot be used as local password authority.

Custom senders must support `assisted_recovery` with `Data["url"]` and the secret-free
`assisted_recovery_completed` security notice. Use stable delivery IDs and the
existing encrypted queue; keep the notification worker running. Starter email
templates for both events are included. Never log bodies, tokens or raw notices.

Local macOS validation (2026-10-17, Go 1.27.2) passed `make check`
(including 570 frontend unit tests), `go build -mod=readonly ./...`, the
integration package suite against disposable PostgreSQL/Redis, and
`make security-negative-check`. The anonymous public-dependency consumer passed
with the Go 1.27.2 binary explicitly selected; GoAuth is pinned to the published
candidate ref in `go.mod`, without a dependency replacement.

Recovery-specific PostgreSQL regressions prove that ordinary account-update
permission is insufficient, the dedicated grant is required, self-initiation is
denied, and multiple active owners reject CLI recovery. The sole-owner
transaction locks subjects as well as roles and memberships, so a disabled
second owner cannot activate between verification and invitation issuance.
The regression reproduces that race before the lock and verifies it is blocked
afterward. Affected host race tests and lint passed after the fix. Independent
review and release/deployment remain separate gates; no CI trigger was added.

Authenticated initiation uses the normal GoAdmin session-bound CSRF provider and
checker. Its page obtains the native token for the current session; same-origin
JavaScript submits it in `X-CSRF-Token` with the native cookie. There is no
recovery-specific CSRF cookie or hidden token on this authenticated form. Auth
and permission middleware remain in place, and a missing, invalid, or other
session's token is rejected before the recovery command runs. Submission has no
automatic retry; an uncertain network outcome requires reconciliation.

Anonymous public completion cannot depend on an authenticated admin session. It
retains its separate exact-Origin, recovery-cookie and hidden-token contract;
the emailed one-time recovery token and existing MFA requirements are unchanged.
