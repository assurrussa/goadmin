# Opaque admin browser sessions

The browser continues to carry one random opaque session identifier. The
supported host and DI assemblies keep credentials in a separate encrypted state
journal, using the Runtime Outbox AEAD key ring. Fiber session snapshots contain
identity and display data, never access or refresh credentials. Canonical
subjects, memberships, current permissions and session state are checked on
each authenticated request. A cached page payload does not authorize a request.

Production session cookies use `__Host-` plus the configured session name,
`Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`, and no Domain attribute. Local
HTTP uses the configured unprefixed name. The readable CSRF proof contains a
purpose-separated SHA-256 binding of the opaque identifier, never the identifier
itself. Mutations require the exact configured admin origin, or a matching
Referer only when Origin is absent. A present bad/null Origin cannot be rescued
by Referer. Duplicate credentials and Authorization on this browser profile
fail before session lookup. A safe GET/HEAD to a public authentication page can
recover duplicate session or CSRF cookies: it discards incoming session,
persistence and CSRF values, expires legacy domain/path scopes, and starts as
anonymous. It never selects or revokes one of the ambiguous sessions. Protected requests, auth
mutations and Authorization headers continue to fail closed; login POST requires
the fresh page's ordinary CSRF proof.

Refresh uses an atomic owner/version claim. Redis uses a single-key Lua CAS
and preserves absolute TTL; the PostgreSQL alternative uses conditional updates
in `goadmin_browser_auth_state`. Concurrent nodes read the committed new version.
An unresolved/crashed owner never expires into retrying the old refresh secret:
the browser must log in again. A stale Fiber save cannot overwrite the journal.
Changing the opaque identifier fences the old journal before moving credentials
and removes its authority. No canonical `sid` is accepted as a browser secret.

`APP_ADMIN_CANONICAL_SESSION_BACKEND=redis` (the default) requires the host Redis
connection. `pgsql` requires the additive migration and does not add a Redis
requirement to install validation. Fiber's supplied session store still needs
its own working storage. Existing migrations remain immutable.

Hosts may supply `AuthAdapterConfig.NotificationSender` and
`NotificationWorker` to enable the canonical managed encrypted queue. Supervise
`Runtime.RunNotifications` and cleanup; do not send recovery secrets into an
unmanaged cleartext notification queue.

## Review follow-up behavior

The follow-up implementation and its incomplete acceptance status are tracked
in [auth-session-hardening-followup.md](auth-session-hardening-followup.md).
The branch is not a release-ready security profile yet.

A claim may be released using owner/version CAS only before the canonical
`Runtime.Refresh` call. A failed post-claim read and a lost claim reply are
recoverable without consuming a refresh token. After an ambiguous canonical
refresh, the old secret is never made claimable again.

Refresh has a 10-second operation budget and a separate bounded completion
write. A follower waits up to five seconds with bounded jittered backoff and
returns a temporary failure without deleting a live owner's cookie. Claims
older than 30 seconds are hidden from authentication, not unlocked. Late
completion cannot restore them. Rotation claims fence reads immediately.
Instances need synchronized clocks; timestamps more than ten seconds in the
future fail closed. Old-format pending owners require a new login.

CSRF JWTs now require HS256, expiration, issued-at, issuer `goadmin-csrf-v1`,
the configured app-domain audience and purpose `browser-csrf`. Subject binding
is exact, including anonymous requests. Older proofs are deliberately rejected;
reload the page to obtain a current proof rather than automatically replaying a
failed mutation.

New avatar attach/delete jobs contain no opaque browser session ID. They
update the canonical preview; the next authenticated request rebuilds its
projection. Historical jobs and their backup copies are not automatically
scrubbed by this change. Coordinate their retention and legacy browser-session
revocation before treating the old payloads as non-sensitive.

Access records omit query strings and raw error text and are encoded as JSON.
The host logger owns the sink and levels. A discard logger also discards access
records. Reverse-proxy and external observability configurations must apply
their own credential-redaction policy.

## Threat-model boundary

Encryption of access/refresh credentials is not a claim that read access to
all session storage is harmless. Opaque bearer IDs still occur in storage keys
and the Fiber display snapshot. Read access to active session storage must be
treated as credential access. Hashing only journal keys would not fix all copies;
a complete key/snapshot migration remains a separate open review item.

The current lower-level Service constructor still permits a legacy no-journal
path. Supported host and DI assemblies supply the journal; removing that
fallback and migrating its test fixtures remains outstanding. WebSocket
revocation/message bounds and email-change reauthentication also remain open.

## Coordinated migration and rollback

This checkout is a local candidate; the released dependency pins are not proof
of candidate or published readiness. Run candidate checks with temporary module
files and the clean consumer's explicit local goauth option. Do not commit
machine-local replace paths. Publish dependencies in order only after release
approval.

Apply the additive state migration before starting the new host. Preserve
canonical accounts and identity links; discard old browser login state rather
than exchange legacy tokens. Users log in again. Keep AEAD read keys through
journal/queue expiry and normal backup/WAL retention. Expired PostgreSQL journal
rows are still cleaned in bounded batches at login; a supervised regular cleanup
loop is not implemented in this follow-up.

Rollback frontend and backend together and require another login. Do not roll
back canonical audit/transaction invariants or reset auth tables. In-flight
refresh claims remain unresolved until reauthentication. Keep the legacy queued
notification handlers available to drain or expire old jobs. Restore databases
with the matching key versions and revoke restored sessions explicitly when
policy requires it.
