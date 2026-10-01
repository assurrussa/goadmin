# Stale browser response and Inertia client follow-up

Baseline: `183b6d5b7a6e4410f74c6ee70612446d3d59575e`, PR #10.
Status: source fixes for the two latest review findings; the broader hardening
scope and full integration/release validation remain open.

## Missing or fenced browser authority

A journal miss means the ID presented by this request is not authoritative.
It does not mean the browser still holds that ID. The authority loader, refresh
waiter and post-claim stale-read path now discard queued session/persistence
Set-Cookie fields rather than emitting expiry cookies. They do not save the
stale snapshot or unlock the journal. Unrelated response cookies are retained.

The regression suite pauses a real Fiber request after loading its old snapshot,
rotates the browser ID through RotateAdminAuth, and applies both response orders
to net/http/cookiejar. It also covers a cookie already queued by a session writer,
remember-me, rejection of the old mutation, continued use of the new session,
explicit logout, and hidden rotate/abandoned claims remaining locked.

This change addresses missing/fenced-authority responses. It is not a claim of
global ordering between independent explicit login/logout/rotation requests.
Existing terminal canonical cleanup and explicit logout semantics are unchanged;
the broader concurrent lifecycle acceptance matrix is still required.

## Versioned HTTP errors and the client

The auth JSON response now carries `X-Goadmin-Auth-Error: 1`. The core client
registers an Inertia `invalid` listener before mounting. Only same-origin JSON
responses with that version marker and a matching status/code/body shape are
handled. Other invalid responses retain Inertia's normal diagnostics. The marker
is a response-routing discriminator, never an authorization credential.

On `401 reauthentication_required`, the client clears auth/profile state,
disconnects its socket, and performs one separate GET navigation to the fixed
same-origin `/auth/login`. It does not reuse a response-supplied URL or replay the
failed mutation. Late page events cannot restore stale auth during navigation.

On recognized 503 errors, the current page/form remains mounted and an accessible
application-level notice asks the user to resubmit manually. The notice works
outside AdminLayout as well, including on auth pages. No automatic retry is
scheduled. Server messages are not rendered; bounded Retry-After values only
inform the local message. Recognized 400 credentials errors show a local notice.

Deploy the backend and rebuilt embedded UI together. `public/dist` was not rebuilt
in this environment; the source changes must pass the normal resources gate before
shipping. Clients without this handler retain their prior behavior.

## Validation actually executed

- Node 22.16.0 / TypeScript 5.8.3: 20 Node unit tests execute the production
  dependency-free client policy and pass. The test is included by the existing
  `scripts/*.test.cjs` gate. Locally it used the installed TypeScript compiler
  through NODE_PATH, not a completed npm ci for the repository.
- Strict TypeScript checking of `browserAuthFailures.ts`: pass.
- Go formatting and `git diff --check` for the changed source subset: pass.
- The focused Go -race test command stopped before compilation while attempting
  to download Go 1.27.1: DNS/network access to proxy.golang.org failed.

Not executed: the added Fiber/cookie-jar tests and Vue/Inertia event/component
integration tests; full root/starter builds/tests; gofumpt/gci/lint; resources
build and browser E2E; PostgreSQL/Redis and clean-consumer/release gates. The
workspace is a verified source subset, not a full checkout with dependencies.
The project toolchain and dependency pins were not changed or stubbed.

Earlier unresolved dependency, email reauthentication, WebSocket, mandatory
journal, hashed-lookup and cleanup items remain listed in
`auth-session-hardening-followup.md` and the PR. No merge, release or deployment
is part of this follow-up.
