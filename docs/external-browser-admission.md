# Candidate external browser admission

This is an additive unpublished patch based on v0.9.6
(`39cd199538335d1db05e359a474d96ec0a90d128`), using its existing GoAuth v0.5.1
primitives. TSOP selects compatible GoAuth
`v0.5.2-0.20261003210830-ad7d1db0bd12`. Master `d16ce6bb` instead selects
GoAuth v0.6.0; adopting that cohort/schema transition is not necessary here.
No dependency pin or migration is changed in this patch.

`host.Dependencies.ExternalAuthority` / `host.Services.ExternalAuthority` is an
optional provider-neutral interface. GoAdmin imports no private AuthHub package,
performs no OIDC token exchange, and stores no provider refresh credentials.
The reusable RP component owns verification, bounded refresh, a durable encrypted
CAS journal, unknown-response fencing and upstream revoke.

`host.ExternalBinding` records exact issuer/external subject, expected local
SubjectID, canonical link INSTANCE ID, RP authority handle, provider session ID,
external auth_time, refresh generation, immutable LoginGeneration, admission deadline/absolute end and
signed proof issue/expiry times.
The authority hook validates the full binding and current grant/mapping/local
policy on every request and confirms the original verified issue/expiry times.
Validate is a read-only snapshot check, without network or refresh. GoAdmin applies
the fixed five-minute cap from issue time, bounded by expiry and the immutable
absolute end. No receipt-time sliding or configurable cap is available. No hook read can replace the admitted snapshot or extend its saved deadline.
A refreshed provider generation requires explicit fresh browser admission. Admission needs nonempty binding;
missing hook never removes the external marker or falls back to local login.

`host.App.LinkExternalAdminIdentity(c, binding)` wraps canonical
`goauth.LinkExternalIdentity`: it requires the current local admin session and
verified RP authority for that expected subject. GoAuth enforces recent canonical
session creation itself. The host enrollment route MUST require CSRF and explicit
confirmation of both identities. The wrapper forwards only issuer and subject,
never email/profile, and never creates membership/roles. The RP callback alone
does not establish a confirmed canonical link.

`host.App.LoginExternalAdmin(c, binding)` resolves the existing canonical link,
requires its instance ID and expected local subject, and calls canonical
`goauth.LoginExternal` with realm-admin and NO email/profile. Missing links cannot
auto-link by email or provision. Membership, native token issuance, encrypted
browser auth-state, local RBAC and cookie writing use the existing service. The
old browser/native journal loses authority before the admitted session is saved;
the new opaque identifier is regenerated. Newly issued native credentials are
revoked on a failed admission/save. The opaque return value is server-only.

`GetAdminAuth` calls the authority validator inside the existing authentication
boundary before serving a projection. It rechecks canonical link identity and
current account/membership/RBAC, preserving all existing protected pages,
mutations, downloads/uploads, TUS, notifications and WebSocket handshakes. A
missing/replaced link, disabled hook, rejected proof or dependency failure fails
closed and clears native browser authority. Unlink/relink to the same local
subject has a different link ID and cannot revive an old session.

Realtime wraps the EXISTING gowebsocket upgrader: it copies only the verified
proof deadline before the pooled Fiber context is released, and closes an open
external socket at that deadline even without messages/HTTP/provider access.
It does not refresh a socket in place. Reconnect traverses GetAdminAuth again.
Legacy sockets keep their previous behavior; there is no third-repository patch.

Superseded admission cleanup and explicit user logout are separate operations.
`Detach(ctx, binding)` is called after native cleanup during fresh admission. It
may release the old native association but MUST NOT tombstone/revoke RP authority,
either the same generation nor a newer one. Ordinary replacement is not logout.

`Logout(ctx, binding)` is called by explicit native logout after journal/session
cleanup. It MUST verify the saved immutable authority reference/identity pins and LoginGeneration, then
terminally close that pinned callback-login lineage across ALL refresh generations, including fencing
an in-flight renewal commit. The saved generation can be stale: this must not turn
user logout into a no-op, nor may it target another callback login even when issuer/subject/SID/auth_time
are identical. LoginGeneration is allocated once at callback login and never
changes during refresh. Durable local
termination precedes bounded best-effort upstream revoke. Provider failure cannot
restore authority. The binding survives native journal reconstruction so logout
cannot lose its hook. Both hooks receive a detached bounded context.

Synthetic tests prove host dispatch and candidate races in both commit orders;
the RP package must independently prove its actual durable journal/CAS property.
No GoAdmin test claims to reimplement or verify that engine. A client bridge may
use its supported RevokeBinding(ctx, binding), once its final source/API is
available. ErrRevocationUnconfirmed after local terminal commit is preserved as an
error; it does not restore native or RP authority and does not trigger refresh or
an automatic revoke retry. Unknown refresh commit remains fenced by that client.

Rollback: leave the external authority nil by default. Disabling it signs out
external sessions rather than converting them to password sessions. Downgrading
to a binary that does not know the external marker is unsafe unless all external
browser/native sessions have first been evicted; legacy rollback requires an
explicit maintenance procedure, never outage-driven password fallback.

This candidate adds no public login route and enables no security mode by itself.
The host supplies routes, configuration, explicit mappings and ACL policy. Real
OAuth clients/accounts/databases and deployments are outside this patch.
