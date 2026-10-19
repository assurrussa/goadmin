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
5. After native login succeeds, Arm pins its uniquely issued response cookie hash; a subsequent CSRF-protected Complete reauthenticates the new native binding
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


## Native-cookie delivery boundary

After ordinary native login succeeds, the host reads exactly one newly issued
native Set-Cookie for its configured native cookie name. Missing, duplicate,
unchanged or insecure cookie delivery fails closed. The component arms the
claimed candidate with only that cookie's CSRFSessionBinding hash. It also sets
a separate Secure HttpOnly SameSite=Lax host-only completion cookie at Path=/.
Its random claim is stored only as a SHA-256 hash in pending state. Neither the
claim nor native session ID appears in URL, JavaScript, page props or logs.

The bridge then navigates to /profile-source?complete=1. This authenticated GET
uses the newly accepted native cookie and produces fresh native CSRF. A boolean
completionReady is only a UI hint. The page removes the navigation marker from
browser history and records a nonsecret same-tab attempt flag before exactly one automatic CSRF-protected completion POST. Only an explicit new Refresh clears that flag; storage failure skips automatic completion.
There is no automatic refresh/retry and no additional click. A plain return,
reload or back navigation does not start another completion attempt.

The POST rechecks current canonical authentication, exact native cookie hash,
issuer/subject/client/project/link/proof, active canonical session, role-preview
fence, selection CAS, claim hash and expiry. A same-subject intervening native
login cannot complete the earlier intent. Only the successful transaction
updates the snapshot/version and clears pending. The handler expires both
temporary cookies after its completion attempt. A core CSRF/Origin rejection occurs
before the handler and may leave cookies until their bounded expiry; it changes no
snapshot, and the removed marker/same-tab flag prevent automatic retries. Cancel/logout and a new
selection/refresh clear temporary browser state; cancellation also clears pending.

Server expiry is fixed at two minutes from CreateIntent and is never extended
by capture, claim, arm or page navigation. The refresh cookie has a two-minute
browser lifetime from creation; the completion cookie has at most two minutes
from arm, but cannot authorize after the earlier fixed server expiry. Expired
pending data cannot be applied even while physically present. It is lazily
cleared on the next authenticated presentation read, or replaced/cleared by a
new intent, selection, cancellation, successful completion or subject deletion.
There is no background janitor or promise of wall-clock physical deletion.
Previous selected snapshot and local values survive every denial/expiry.
