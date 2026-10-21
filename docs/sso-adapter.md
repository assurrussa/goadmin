# Optional SSO adapter

Status: candidate, not released. Opt in only on admin origins.

- The host loads its existing ENV into `sso.Config`. No JSON configuration or new credential generation.
- `Enabled=false` means no runtime, routes, discovery or required SSO settings.
- Apply `MigrateSchema` explicitly using the host's migration workflow. Reuse the
  existing dedicated RP schema (for example `seo_sso`) and unchanged storage key.
  This creates missing adapter navigation tables without replacing existing sessions.
- Create `NewRuntime` with the borrowed canonical database; the adapter never closes it.
- Set `Dependencies.ExternalAuthority` to the runtime authority. Append
  `runtime.Modules(canonicalLinks, csrf, configuredCSRFCookie)` before `host.New`.
- The shared login page supplies a full-document SSO link only when the adapter is mounted.
- `/sso` prepares the SDK's CSRF form and submits automatically. No second user click.
  A no-JavaScript fallback remains. Do not replace this with an unsafe GET login.
- Callback validates SDK state/PKCE/nonce, then a fixed same-origin document continues
  native admission. Canonical linkage, local membership and permissions are required.
- Explicit identity linking uses a native authenticated GoAdmin page, CSRF and a
  single-use intent bound to both sessions. Success starts a fresh verified login.
- The optional account/settings card reads the canonical GoAuth link and local
  session creation time through the host boundary. It shows linked/unlinked and
  a pending reminder only for a verified, not-yet-linked SSO identity. No SSO
  module means no card or status routes. The RP store is not used to infer local
  links, and no credential or secret enters page props.
- The host passes GoAuth's effective `IdentityLinkAuthMaxAge` to the read-only
  eligibility check: zero means GoAuth's 10-minute default, otherwise 1m–1h.
  The final mutation still enforces GoAuth freshness. A stale local session
  keeps ordinary profile access but offers an explicit CSRF-protected local-only
  logout, then returns through local login to a new single-use linking form.
  It never replays the old POST or revokes the provider session. Conflict and
  dependency failures remain distinct from age expiration.
- Expired external sessions use the existing native recovery protocol. No unsafe
  request is replayed, and absent/revoked identities never gain membership.
- The host owns routing behind HTTPS, public URL, cookie policy and local RBAC.
  This adapter does not enable another site's SSO or change public authentication.

Pending release gates: real SDK/PostgreSQL flow, migration compatibility, full
GoAdmin checks, UI types and browser repeated/expired/denied linking flows,
SEO Center consumer check, independent review and published-consumer check.

Transient data retention: each new login prunes at most 128 expired rows from
both navigation tables; issuing a link intent prunes at most 128 expired link
intents. Expiry indexes and SKIP LOCKED bound concurrent cleanup. Unexpired
consumed markers are retained to deny replay. Pruning is opportunistic on use,
not a promise of physical deletion exactly five minutes later on an idle site.

Review on 2026-10-21: approved independent Sol xhigh review found no remaining
P1/P2 blockers after Origin policy, outage classification, return-target retention,
expiry pruning and canonical-link regression fixes. Browser QA remains required.
