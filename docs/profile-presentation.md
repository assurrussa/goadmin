# Optional SSO profile presentation (candidate)

Mount `features/profile.Component.Module()` explicitly with the same canonical
Runtime used by host.New. This is display-only. Default selected fields are all
false. AuthHub's existing bounded, allowlisted namespaced `authhub_profile` map
provides only `display_name` and `avatar_url`; Login must come from the signed
canonical identifiers contract. Unknown metadata is ignored. No provider value
modifies local BasicProfile, credentials, email verification, linking or grants.

The owned additive migration creates one per-subject presentation row. A pending
refresh lives in the same row, bounded to 8 KiB and two minutes. No additional
table, token, password or refresh credential is stored. The selector is random,
hashed at rest and carried only by a Secure HttpOnly SameSite=Lax host-only
cookie; candidate snapshots contain display data and opaque identity/session
pins only. Do not log pending JSON or selectors.

Native authenticated self-only routes at /profile-source use the existing
admin CSRF/Origin middleware. Role preview cannot mutate selections or start/
cancel refreshes. Selection updates use decimal-string CAS versions and cancel
pending refreshes. Local canonical values/uploads are always preserved.
Missing/unlinked/stale source snapshots fall back to local presentation.
No other administrator can target a subject through form input.

Host refresh protocol:
1. Start from the native CSRF-protected explicit POST. CreateIntent captures the
   exact authenticated native session, canonical link and selection version.
2. Render a new SDK Begin form carrying the selector in the state-bound return
   marker. A marker URL alone grants nothing. Use committed CallbackResult,
   not Verify, an old cookie, redirect status or browser identity JSON.
3. Resolve the exact current local subject/link, verify issuer/client/project/
   subject and browser-cookie continuity, and Capture the fresh signed namespaced
   data. This only saves a candidate. Callback failure/replay cannot select it.
4. Before ordinary LoginExternalAdmin, Claim using that same current proof and
   original native browser binding. It rechecks the original canonical session.
5. After native login succeeds, Complete reauthenticates the new native binding
   and consumes the exact claim with CAS, current subject/membership/link and
   unchanged fields. SDK and native failures leave the previous snapshot intact.
   No recovery shortcut or broad post-login hook is added.
6. The host must Cancel pending intent before an explicit authenticated logout.
   Cancellation failure is reported rather than claiming logout completion.
   Concurrent logout, selection changes and unlink/relink must fence completion.

Avatars are disabled with an empty immutable AvatarHosts allowlist. Only exact
lowercase DNS hosts and HTTPS URLs without credentials, ports or fragments are
accepted. IP/local endpoints and wildcard hosts are rejected. The server never
fetches/resizes/proxies the URL. Hosts must restrict CSP img-src to those exact
origins (including redirect destinations); do not use a wildcard. Selected
external images load in the user's browser with no-referrer, exposing the
browser's IP to that configured image service. UI explains this before selection.
Operators are responsible for trusted image-host DNS/redirect policies.

Metadata snapshots may be stale until explicit refresh. They are never fresh
authorization evidence. No polling, automatic refresh, arbitrary claims,
Telegram or remote image downloads are part of this component.

Checks required before release: feature policy/unit tests, real PostgreSQL
replay/race/CAS/link/session tests, native assembly CSRF/Origin/role-preview
negatives, Vue tests/typecheck/build, core regression/lint/format, public-surface
and clean-consumer probes, and the host's end-to-end refresh integration.
No candidate execution result is claimed in this document.
