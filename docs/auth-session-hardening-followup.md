# Auth/session review follow-up

Status: **partial implementation; draft PR #10; not ready to merge or release**.
Branch: `tasks/auth-session-hardening`.
Review baseline: `0e6b44c5c3206b745b84d777aa60d38002980b3a`.

The requested complete scope is not finished. The items below distinguish code
that was changed from validation that actually ran. No dependency publication,
visibility change, deployment or merge was performed.

## Rereview fixes — 2026-09-30

Follow-up baseline: `5c6f46e58e395bb42141bfead5f6bab6185187c6`.
Code and new test snapshot: `965516e312bf74f2f23f1a972da1ae32d7081bf6`.

- Access-token verification no longer uses the refresh-token failure classifier.
  After a deterministic failed refresh commits the old pair at a new version,
  a follower seeing an expired/invalid access token returns
  `ErrRefreshRetryRequired` without deleting the journal or cookies. A later
  explicit request must acquire a new CAS claim. Canonical terminal failures
  still clear authority; unknown refresh outcomes remain fenced and are never
  automatically retried.
- Browser auth failures now send safe JSON responses directly. Returning an
  error with Code=401/503 was insufficient because the installed Inertia error
  listener converted it to RedirectBack. Rejected mutations now retain their
  HTTP status, omit Location/X-Inertia-Location, and do not save flash/session
  data. Retryable refresh failures send 503 with Retry-After: 1. A joined unknown
  outcome takes precedence over malformed-token classification.
- These failure bodies contain only status/code/message and are intentionally
  non-page JSON, including for X-Inertia requests. The client must not replay a
  failed mutation automatically. No frontend retry handler or new error page
  was added; browser presentation still requires the pending E2E verification.
- CSRF issuance and parsing share an injectable internal clock. The production
  profile allows five seconds of skew for iat/nbf/exp. Expiration is strict at
  `exp + 5 seconds`; this small allowance is not a replacement for synchronized
  host clocks. The supported host options and generated options are unchanged.
- Added `refresh_retry_test.go` for concurrent leader/follower rollback and a
  later successful retry, `auth_transport_test.go` for actual Fiber/logger/
  Inertia response behavior, and `clock_test.go` for independent node clocks
  and exact expiry/skew boundaries.

Validation for this follow-up: all six edited/added Go source files were checked
with gofmt and matched against GitHub blob SHAs; git diff --check passed on that
verified source subset. This is syntax/format/whitespace verification, not type
checking. A focused go test -race invocation could not start: downloading
Go 1.27.1 failed on DNS/network access to proxy.golang.org. No new Go tests,
full-module build, gofumpt/gci/lint, backend integration or browser tests ran.
The available local Go is 1.23.2; the project requirement was not downgraded.
Historical passing helper tests below were not rerun and do not validate these
new changes. The older open implementation scope at the end remains open.

## Implemented changes

- Review 1.2 / 2.4: structural access records omit query strings and raw error
  messages. The host logger owns its sink; standalone records use encoding/json.
- Review 1.3: new avatar attach/delete jobs no longer include opaque session IDs.
  Regression assertions cover serialized requests/jobs, not only visible UI fields.
  Historical queued payloads and backups are not automatically scrubbed.
- Review 1.7: server construction rejects a partially configured TLS certificate/key.
  The existing CORS allow-method override now tests the method list, not origins.
- Review 2.1: an owner/version CAS can release a claim before Runtime.Refresh.
  Redis preserves TTL; PostgreSQL preserves version and expiry. Failed claimed
  reads and lost claim replies do not strand a definitely unconsumed token.
- Review 2.2: owned/follower terminal verification and membership failures clear
  authentication consistently. Unknown canonical outcomes retain their fence.
- Review 2.3: browser credential failures have explicit HTTP classifications.
  The final-response integration was corrected in the rereview follow-up above.
- Review 2.5: correlation IDs use Fiber's typed context accessor and are bounded;
  the previous local key is mirrored for compatibility.
- Review 2.7: starter supervision preserves all shutdown errors, including real
  errors joined with context.Canceled. A timeout explicitly reports an unjoined
  runner. The starter avoids closing shared clients under such a runner before
  main terminates the process.
- Review 3.1: followers use bounded jittered polling. A short wait timeout does
  not erase a live owner's cookie. Abandoned/rotating identities are fenced on
  journal reads; an expired owner cannot complete and resurrect the identity.
- Review 3.2: CSRF proofs require HS256, expiry, issued-at, issuer, audience,
  purpose and exact user/session binding. Legacy proofs are deliberately rejected.

## Historical checks executed locally before this rereview

Environment: Go 1.23.2, Linux/amd64. Only a source subset was available locally;
this was not a successful clean checkout/build of the entire module. The tested
files were compared with their GitHub blob SHAs. No dependency stubs were used.
The project go.mod/toolchain requirement was not downgraded.

Passed with `GO111MODULE=off GOTOOLCHAIN=local`:

1. `go test -race -count=1 -v .` in `internal/httpsecurity`.
2. `go test -race -count=1 -v .` in `internal/refreshpolicy`.
3. `go test -race -count=1 -v lifecycle.go lifecycle_test.go lifecycle_errors_test.go`
   in `examples/starter`: all four existing and three new supervision tests passed.
4. `go test -run '^$' -fuzz '^FuzzAccessLog$' -fuzztime=5s -parallel=2`
   in `internal/httpsecurity`: passed; 50,955 executions reported.

These are isolated standard-library tests. They do NOT establish that the
Go 1.27.1 module, Fiber wiring, canonical goauth runtime or storage integrations
compile or pass. Full race/coverage, formatting/lint gates, UI build, browser
E2E, external consumer, PostgreSQL/Redis integration and release gates did not run.

`GOTOOLCHAIN=go1.27.1 go version` failed while downloading the toolchain because
DNS/network access to proxy.golang.org was unavailable. Direct GitHub checkout
also failed DNS resolution. The root still has four sibling replace directives;
the candidate dependencies and their clean checksums remain unresolved here.

A temporary read-only source-snapshot Actions helper was attempted. Run
36625610245 failed and returned no artifacts; its logs were unavailable. It did
not execute application tests. The temporary workflow was removed from the branch.

## Added regression tests not executed in this environment

- `services/adminservice/refresh_faults_test.go`: lost claim reply, post-claim
  read failure, safe recovery and no unlock after an unknown canonical outcome.
- `services/adminservice/refresh_terminal_test.go`: terminal failures in followers.
- `internal/auth/browserstate/release_test.go`: owner/version fencing and abandoned
  records remaining unclaimable rather than being deleted or unlocked.
- `internal/csrf/service_test.go`: required proof profile and rejection cases.
- `infrastructure/fiber/middlewares/security_test.go`: actual middleware JSON and
  shared audit/request correlation, including malformed incoming IDs.
- Avatar upload/deletion tests now assert that synthetic cookie credentials
  never appear in the newly serialized jobs.
- The three rereview regression test files listed above remain unexecuted too.

## Still open — implementation, not merely missing test results

- **1.1:** resolve and pin compatible goauth/gonotify/gouploads/gowebsocket versions;
  remove sibling replaces; update root/starter sums and run clean consumers.
- **1.4:** email-change reauthentication across server and UI, with fresh proof
  enforcement and the associated lifecycle tests.
- **1.5 / 1.6:** WebSocket canonical revocation/expiry and message-size limits,
  including the identity contract of the pinned dependency and real socket tests.
- **2.6:** make journal-backed browser authentication mandatory and migrate the
  existing no-journal legacy fixtures without weakening their assertions.
- **3.3:** a complete hashed-lookup/snapshot migration for read-compromise
  resistance. Encryption alone does not protect exposed opaque session IDs.
  The present threat-model limitation is documented, not claimed fixed.
- **3.4:** supervised regular journal cleanup and its retention/expiry checks.
- Finish the full fault-injection matrix: ambiguous completion, concurrent logout,
  password/status/membership changes, old/new cookie response ordering and expiry.

Before acceptance, finish the open implementations, prepare the sources using
repository tooling, run the focused package suites, root and starter tests,
`make check`, and the documented integration/release gates with actual services.
Do not interpret isolated helper tests as permission to merge this draft.
