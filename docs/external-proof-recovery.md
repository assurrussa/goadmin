# Explicit external proof recovery

The additive host API is `BeginExternalAdminRecovery(c)`, followed by the
host SDK's fresh provider verification and
`CompleteExternalAdminRecovery(c, snapshot, freshBinding)`.
`snapshot.Binding()` returns a metadata copy; the snapshot is server-only and
is never an authentication result or browser credential.

Trusted host middleware must call `host.EnableExternalAdminRecovery(c)` only
when its guarded recovery endpoint is registered. This request-scoped opt-in
allows a proof deadline crossed inside native middleware to emit the recovery
marker; browser headers cannot enable it. Hosts without this endpoint retain
ordinary login behavior.

The dedicated GET endpoint must check same-origin intent and the explicit
recovery header before calling either method. Business mutations never invoke
recovery implicitly and never retry themselves. After completion the host must
obtain `AdminCSRFRequest`, create a CSRF proof, and only then return recovery
success. Recovery preserves the native cookie and canonical session ID, so this
path needs no cookie-settlement redirect or 409 round trip.

External evidence now resides in the existing encrypted browser-state journal.
Owner/version Claim and Complete fence logout, concurrent refresh and stale
snapshots; Complete never inserts a missing row. Normal auth reloads this
binding rather than trusting a late Fiber projection save. The native session
ID is unchanged, so concurrent canonical revocation remains effective. An
expired canonical access token may refresh only after fresh provider evidence
passes validation, and only through the existing one-consumption claim rules.

Issuer, subject, canonical subject/link instance, client, project, authority,
provider session, auth time and login generation must remain identical.
Generation and proof issue time cannot go backwards. Signed absolute expiry
cannot increase. Provider checks remain read-only; remote SDK verification
occurs outside native journal claims and database transactions.

Pure proof freshness expiry returns `ErrExternalRecoveryRequired` without
authorizing the request or deleting its native fence. Terminal identity/link,
absolute-expiry and other authority errors retain fail-closed behavior.
Expiry during projection building also denies without extending any deadline.

## Rollout

Existing encrypted rows without the new external binding remain compatible
with ordinary authentication but cannot use recovery. They require one normal
explicit external login after their current proof expires. No migration trusts
or promotes a stale Fiber projection into journal authority. Newly admitted
external sessions store the binding from the start. Roll back the host endpoint
with the library; do not let older application nodes overwrite journal records
after deploying recovery. A coordinated rollout is required.

## Verification

Security tests cover native session preservation, snapshot replay, lineage and
absolute-expiry rejection, native logout/revocation during provider verification,
membership/link changes, legacy rows and deletion immediately before the CAS
commit. Run focused adminservice/browserstate/host tests, race tests, the public
consumer checks and the full repository gate before merge or release. Source
preparation alone is not evidence that these gates passed.
