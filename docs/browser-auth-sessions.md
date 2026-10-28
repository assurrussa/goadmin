# Opaque admin browser sessions

## Continuous maintenance

Authenticated pages advertise `adminSessionMaintenance: { version: 1, sessionEpoch }` only
when their login supports the protocol. Core password sessions are supported;
external sessions require the optional `host.ExternalSessionMaintainer`
capability. The built-in SSO runtime implements it. Custom authorities retain
their existing read-only `Validate` contract and must opt into renewal explicitly.
The epoch comes from the login used to render the page and anchors the first
maintenance response. An advertised version 1 without an epoch fails closed;
absence of the capability retains legacy behavior. A different login blocks the
mounted document, even when it belongs to the same admin.

Core owns `GET /auth/session/maintain`. Requests require exactly one
`X-Goadmin-Auth-Recovery: 1` header, unambiguous cookies, no bearer header/query,
and same-origin Origin or Fetch-Site/Referer evidence. Responses are no-store
JSON. The endpoint authenticates an existing login; it cannot create a native
admission or resurrect an expired/missing journal. The old SSO recovery route
and Begin/Complete facade remain available. New custom recovery adapters should
use BeginOwned/Cancel/Complete to acquire the native owner before SDK Verify.

HTTP 200 retains `status`, `code: authentication_recovered`, `csrfReady: true`
and adds `maintenance` version 1: server time, relative next-check interval,
CSRF/external/hard deadlines, nonbearer session epoch, revision and renewal flag.
The fixed hard deadline includes journal and canonical expiry, external absolute
expiry and known browser absolute expiry. New anonymous bootstraps retain their
browser absolute deadline across login. Legacy browser snapshots without that
metadata retain their existing Fiber expiry enforcement; their reported deadline
is the minimum of the remaining known limits, not a new lifetime promise.

Healthy maintenance reuses CSRF and keeps auth revision/cookies unchanged.
A missing, expired or unsuitable CSRF cookie is issued once with HTTP 409
`authentication_recovery_continue`; the following request must confirm the
incoming proof before a 200. A normal attempt contains at most two GETs. Busy
owners and dependency failures return retryable 503, with Retry-After. Confirmed
revocation/hard expiry/unknown native refresh consumption requires explicit
login; lost membership is access denied. Recognized authenticated CSRF failures
return tagged 419 `csrf_needs_sync` before the business handler when the client
negotiates `X-Goadmin-Session-Maintenance: 1`; older clients retain their legacy
CSRF error behavior. Invalid origins
and ambiguous credentials are not repair signals.

The native journal owner is acquired before provider renewal, with no SQL lock
held across network calls. Readers wait/re-read or fail temporarily. Error cleanup
is conditional on the captured version/owner; explicit logout still terminates
its lineage and fences late refresh. An uncertain consumed refresh secret cannot
be retried. This version does not add cross-store crash reconciliation to the SDK.

Executing visible and hidden tabs maintain the existing storage idle lease with
throttled passive saves, without renewing browser cookies, immutable limits or
user-activity timestamps. Defaults are a 60-second visible watchdog and
120-second hidden watchdog, bounded earlier by the server schedule and idle
touch interval. Sleeping/frozen tabs are checked on resume, not promised timer
execution. After hard expiry the draft stays mounted until explicit navigation.

Frontend scheduling requires the explicit versioned capability. A new client on
an old host preserves legacy recovery. A new server retains old recovery response
fields for old clients. Maintenance uses one in-flight request per tab; correctness
across tabs/instances remains the server's responsibility. Tokens, authority
references, opaque cookie identifiers and form bodies are never maintenance data.

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

## Opt-in current-page external-session recovery

### Full-document browser navigation

When the external proof deadline expires, an opted-in HTML document GET now
receives a minimal no-store recovery document, not raw JSON or protected page
data. The independent same-origin script calls the same guarded recovery
endpoint below, bounded by two GET requests and 30 seconds. Verified success
automatically reloads only the current safe GET, with a Continue fallback.
A tab-scoped sessionStorage timestamp is persisted before navigation: a recovery
document encountered again within 60 seconds requires manual retry, preventing
unattended recovery/reload loops. No URL, identity, cookie or proof is stored.
If storage is unavailable, recovery and continuation require explicit clicks.
Mutations are never replayed. HEAD, API/JSON,
Inertia, AJAX, non-document fetches and mutations retain their JSON error
contract and do not dispatch a protected handler.

Hosts may use `host.EnableExternalAdminRecoveryWithSignIn(c, fixedLocalPath)`
instead of `EnableExternalAdminRecovery` to provide an explicit fresh-sign-in
link on this document. The fixed public route must bypass native authentication
and use the host's ordinary verified SSO login/admission flow. Never derive it
from browser input. The native `/auth/login` and recovery endpoint are rejected
as fallback links because the retained stale binding could loop. Without a
host continuation, failure offers only an explicit new bounded recovery attempt;
the library does not invent a login route, clear cookies or discard the journal.

The script and responsive light/dark stylesheet are embedded independently of
the UI bundle, before static/auth middleware at `/public/auth-recovery.js` and
`/public/auth-recovery.css`. They use system fonts and same-origin script,
style and connect policies, without external resources, inline code or weaker CSP.

Hosts may opt a rejected mutation into the shared frontend recovery coordinator
by returning the existing versioned JSON authentication error:
HTTP 401, `X-Goadmin-Auth-Error: 1`, `Content-Type: application/json`,
and `{ "status": 401, "code": "reauthentication_required", "message": "..." }`,
plus `X-Goadmin-Auth-Recovery: 1`. Do not add this marker to ordinary terminal
native authentication failures or hosts without the endpoint below.

The client keeps the current page and its in-memory form mounted. It disconnects
realtime, then sends a fixed same-origin GET to `/auth/session/recover` with
`X-Goadmin-Auth-Recovery: 1` and `Accept: application/json`. Credentials and fetch
mode are same-origin, cache is disabled and redirects are errors. No server
response selects a URL. No original request body, mutation callback or router
visit is retained for replay.

The host owns this endpoint and the OIDC/RP refresh engine. It must reject foreign
origins/Fetch-Site and missing/duplicate request markers before doing work. No
business command, linking or provisioning is allowed. The old protected native
binding must survive the rejected mutation; metadata alone does not authorize
renewal. The host must independently establish native recovery eligibility,
then Verify a current RP proof and enforce unchanged local subject, canonical
link instance, authority reference, LoginGeneration and identity/session/scope
pins. Generations cannot move backwards and absolute expiry cannot extend.
Revocation, missing/changed links, disabled accounts, lost membership and
uncertain refresh outcomes must fail closed. Validate remains a read-only,
immutable-snapshot check; do not weaken it or extend an expired deadline.

Every endpoint response is no-store JSON with `X-Goadmin-Auth-Recovery: 1`.
After native admission rotates the cookie, return HTTP 409 with
`{ "status": 409, "code": "authentication_recovery_continue" }`.
The client makes at most one additional GET. Only after that request verifies
the current native cookie, current RP/link/local policy and a matching
session-bound CSRF proof may the host return HTTP 200 with
`{ "status": 200, "code": "authentication_recovered", "csrfReady": true }`.
An already-settled, valid admission may return 200 immediately. Do not emit
success merely because an OAuth refresh or cookie write succeeded.

One recovery attempt per failed page lifecycle is bounded by two GET requests
and a 30-second total timeout. Concurrent failures in the same page share that
attempt. New Inertia requests abort its client-side continuation; hosts still
need their durable generation/owner fences across tabs and late responses.
A successful Inertia navigation/submission resets the attempt budget.

After success the user manually submits the preserved form again, with the
current CSRF cookie. After failure the draft remains mounted and a fixed login
button is shown; no automatic navigation discards the draft. Backend auth,
RBAC and CSRF still gate every later mutation. Recovery is not a save
acknowledgement and must never automatically repeat a mutation, including
uploads or requests with unknown outcomes. Without explicit host opt-in the
existing authentication error behavior is unchanged.

## Negotiated document identity fence

Protected requests from the version-1 session-maintenance client carry
`X-Goadmin-Session-Maintenance: 1` and exactly one
`X-Goadmin-Session-Epoch` header. The latter is the public, non-credential epoch
from the document's initial `adminSessionMaintenance` page prop, not a value
read from shared cookies or learned from a later login. The authenticated core
middleware validates it against the current canonical login before dispatching
protected handlers, even when client readiness is cached and the new login's
CSRF cookie is valid. Missing, malformed, duplicate or unequal epochs for
negotiated requests fail closed with HTTP 409, `X-Goadmin-Auth-Error: 1`, and
`{ "status": 409, "code": "session_changed", "message": "..." }`.
The shared client stops this document's submissions and keeps its draft mounted;
there is no automatic mutation retry or navigation. Explicit identity
transitions are exempt. Clients which omit the maintenance negotiation header
intentionally keep their existing auth/CSRF contract. Refreshing the same login
must not change its epoch.
