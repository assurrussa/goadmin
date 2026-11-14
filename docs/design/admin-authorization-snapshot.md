# Optional admin authorization snapshot: implementation boundary

Status: local opt-in integration candidate with isolated core tests, NOT released or deployed.
The design/history sections below describe the initially verified baseline; see
"Local integration update" for implemented changes and current limitations.

## Scope

Opt-in GoAdmin-owned RCU snapshot for explicit admin membership, role assignment,
role metadata and permissions. Default remains database-backed. User realm stays
separate. Neither role possession nor a shared email creates admin membership.

The snapshot does not replace GoAuth session introspection, active-subject checks,
securityVersion, password/MFA handling or session revocation. Those still perform
canonical queries. Thus "zero database calls for authentication" is not a goal.

## Verified current source

- `internal/auth/adapter.go`: user realm exits the membership gate without admin
  membership SQL; admin realm queries the membership repository. RBAC uses nil cache.
- `internal/auth/role_service.go`: guard defaults to disabledRoleCache. Its old
  interface cannot return a cache-unavailable error or pin one snapshot across
  roles and permissions. It must not simply be switched to enabled.
- `host/subject_permissions.go`: creates separate RBAC/guard instances, also nil
  cache. These must receive the same adapter policy, not an independent cache.
- GoAuth v0.17.0 `postgres/rbac_transaction.go`: reservation before a management
  write, invalidation after commit, bypass during pending/uncertain state. The
  cache invalidator is supported, but public host transaction outcome hooks are not.
- GoAuth `rbacStore.Snapshot`: standalone READ ONLY REPEATABLE READ transaction;
  explicitly rejects ambient managed transactions. It cannot currently include
  the GoAdmin-owned membership projection in that same snapshot.
- `host/install.go` CommandTransaction owns most GoAdmin commands; role mutations
  additionally use `internal/auth/role_transaction.go`. Public Runtime transactions,
  seed/provision paths and adminrepo operations also exist. A callback after an
  arbitrary nested InAuthTransaction return does NOT prove outer commit.

## Data flow

DB snapshot -> immutable indexes -> gocache/rcu atomic publication -> one pinned
view for each authorization decision. Views must not be saved across requests.
A loader must read membership and RBAC within ONE consistent database snapshot.
Partial rebuilds are never published. All authorization index data remains private.

Proposed enabled defaults: refresh 30 seconds, maximum age 120 seconds, load timeout
5 seconds. Timer is a backstop for writes made by another process or directly in DB.
A failed initial load prevents cache-enabled startup. Refresh errors retain a prior
view only within its maximum age; expired views deny rather than producing a
per-request database retry storm. Settings require explicit host opt-in.

Local committed writes fence the prior generation and request a reload. A loader
that began before invalidation cannot resurrect old grants. A short local deny
window while reloading is preferable to stale known-revoked grants. Other pods can
observe the change at the periodic refresh; this bounded propagation was accepted.

`internal/authsnapshot` implements this in isolation. It does not prove the loader,
write hooks or runtime wiring. Its RCU CacheInvalidator implementation is only for
AFTER-COMMIT invocation. Never invoke it as if SQL execution meant commit.

## Commit integration alternatives

### A. Narrow GoAuth extension (recommended, requires separate review)

Expose transaction outcome callbacks on the canonical Runtime, including nested
scope joining, outer commit, rollback and unknown-outcome handling. An API sketch:

- Register an invalidation intent on the current managed transaction, deduplicated
  by cache identity; no callback on rollback.
- After commit, invalidate using bounded detached context (HTTP cancellation must
  not preserve stale rights). Unknown commit outcome fences cache, then reloads
  authoritative storage; never assume rollback.
- Existing internal transaction callback machinery is the implementation starting
  point, not a second commit manager.
- Non-managed operations own their transaction and follow the same protocol.

Add a read-only snapshot projection callback to the PostgreSQL RBAC snapshot API,
providing SQLExecutor inside its existing repeatable-read transaction, so GoAdmin
can load memberships without copying GoAuth table SQL. Callback failure/commit
failure publishes no data. Ambient write transactions stay rejected.

This preserves ownership of auth tables and avoids new DB triggers. Requires
GoAuth tests and release/consumer gates before GoAdmin pins a published version.
External writers do not automatically trigger local callbacks; the documented
periodic refresh bounds their propagation. No claim of immediate cross-pod revoke.

### B. Transactional PostgreSQL NOTIFY

Statement-level triggers can notify only on committed mutations, with LISTEN on a
dedicated connection and periodic refresh for reconnect/missed notifications.
This covers direct DB writes and other pods. It adds trigger migrations on
GoAuth-owned tables, listener lifecycle/reconnect behavior and a connection per
instance. NOTIFY is not a durable event log. Notification payloads must not contain
identities or permission data. It still does not by itself solve the single
consistent membership+RBAC load. Not selected without explicit architecture review.

### Rejected shortcuts

- Refresh from inside adminrepo SQL write: outer commit/rollback unknown.
- Refresh after nested InAuthTransaction returns: may only have joined outer tx.
- Sequential membership query + independent RBAC Snapshot: can publish a mixed
  policy from different committed states.
- Turn on the old guard cache: multiple unpinned snapshots, no stale/error contract.
- Infer admin from roles, email, JWT realm alone, or skip session introspection.

## Required wiring and write coverage before opt-in is offered

1. Adapter owns snapshot/lifecycle. Host AuthAdapterConfig exposes typed opt-in;
   bootstrap/DI must pass the same policy. Close cancels worker before DB close.
2. Admin realm gate uses snapshot membership only for eligible read paths. User
   realm must perform zero admin-membership lookups, including negative identity.
3. Route guards and host SubjectPermissionChecker use a pinned view. Super-role
   bypass and role-preview restrictions retain their current semantics.
4. In-transaction management invariants use canonical transaction reads, not cache:
   last super-admin, reciprocal role mutation, revoke, assisted recovery. Preserve
   existing lock ordering and transaction boundary tests.
5. GoAuth RBAC write invalidations, admin provisioning, deletion/soft deletion,
   membership restoration and subject retirement all covered after outer commit.
   Raw external writes are periodic-refresh paths unless B is chosen.
6. Admin-mail authorization must share this explicit policy where appropriate;
   password reset/session/MFA security operations must not silently become cached.
   Existing FOR SHARE candidate is not an answer to read-path caching.

## Verification gates

Core: initial failure, immutable role copies, fresh negative membership/permission,
max-age failure/recovery, canceled reads, Close, in-flight invalidation fence,
concurrent readers/rebuilds, load-count assertion (1000 reads, one initial load).
Current targeted test: `go test -race -count=5 ./internal/authsnapshot` PASS.

Integration still required: commit vs rollback, nested tx, unknown commit outcome,
reader crossing invalidate, bulk roles/permissions, membership grant/delete/restore,
subject retirement, no privilege from shared identity, disabled-mode parity, two
processes with stale bounded refresh, loader DB failure, dependency lifecycle.

Count SQL separately: warm membership/RBAC reads should use no per-request SQL;
login/session/security/MFA queries are measured and retained, not reported as a
regression or hidden. Benchmark memory/load at realistic 100 and 1000 admins.

No production settings, migrations or CI triggers are changed. The local candidate
adds an opt-in public configuration; default runtime behavior remains unchanged.

## Local integration update (not released)

The prerequisite GoAuth API is published in v0.18.0 (GoAuth PR #47). This
candidate pins that released dependency directly, without a local replacement.
Earlier verification against the exact PR source tree remains historical evidence;
release readiness additionally requires the normal gates on the published pin.

GoAdmin now wires the core through `AuthAdapterConfig.AuthorizationCache` and
`host.Config.AuthorizationCache`. Nil keeps current behavior. When supplying an
AuthAdapter explicitly, configure the cache there. Example:

```go
adapter, err := host.NewAuthAdapter(host.AuthAdapterConfig{
    Database: db, TxManager: tx, Runtime: authConfig,
    AuthorizationCache: &host.AuthorizationCacheConfig{
        RefreshInterval: 30 * time.Second,
        MaxAge: 2 * time.Minute,
        LoadTimeout: 5 * time.Second,
    },
})
// Check err and retain adapter until runtime shutdown.
checker, err := adapter.SubjectPermissionChecker(logger)
```

Do not supply an independently constructed SubjectPermissionChecker with an
opted-in adapter: assembly rejects that split policy. The adapter-owned checker
shares its cache and lifecycle. Config passed to an already-supplied adapter is
not a runtime toggle. Reconfiguration is startup-only.

Membership insert/provision/delete/soft-delete paths register one observer per
outer canonical auth transaction. Committed and unknown outcomes invalidate;
rollback leaves cache untouched. Ambient legacy Outbox/pgx transactions are
explicitly rejected in cache-enabled membership writes rather than silently
committing in a second, independent transaction. Migrate those callers to
Runtime.InAuthTransaction or leave cache disabled. Raw SQL writers and other
processes are covered by periodic refresh, not an immediate-local-hook guarantee.

Membership snapshot deliberately contains membership, not canonical account
status. Account status/session/securityVersion/MFA remain GoAuth's responsibility;
The separately prepared active-admin guard for sensitive reset policy is not
included in this isolated candidate; it must remain canonical and transactional.

Added verification: real PostgreSQL nested rollback, committed membership revoke,
two adapters, direct SQL restoration and RBAC permission revoke PASS. Warm guard
unit test performs 1000 membership-ID/role/permission decisions with no repository
objects installed. Isolated read microbenchmark on this cloud CPU: ~77 ns/op,
0 B/op and 0 allocs/op; this is not an end-to-end authentication benchmark.


Latest local wiring also uses GoAuth `AuthTransactionActive` (pure context check)
to bypass cache for direct canonical transaction contexts, not just GoAdmin's
bridge marker. Foreign, cancelled and completed transaction contexts fail closed.

The shared GoAuth Runtime receives an **invalidation-only** cache adapter: its
HasPermission reports no cached answer, preserving generic user-realm RBAC
semantics. GoAdmin admin guards consume the admin snapshot directly. Integration
regression verifies that a customer with an RBAC role still passes generic GoAuth
RBAC while never gaining admin membership/permissions through this cache.

Remaining release gates: clean external consumer without development replacements, final full repository gate,
and host-specific adoption with explicit opt-in. Existing integration-tag lint
findings elsewhere in host tests are not fixed by this change; new integration
files have no reported lint findings.

The legacy internal `di` module is intentionally not given a cache opt-in: it
has no explicit AuthAdapter shutdown owner. Supported opt-in is through `host`
assembly (owned adapter) or an explicitly supplied adapter whose caller closes it.
No previously enabled legacy cache is disabled by this change.

Scoped-loader update: GoAdmin now uses the additive GoAuth
`ScopedAuthorizationSnapshot` API. Its selector reads memberships first inside
the SAME repeatable-read transaction, then the assignment query filters on the
selected admin subject IDs using the `(subject_id, role_id)` index. Empty admin
set means empty assignments, never "all customers". No customer assignments are
loaded or discarded in application memory.

The metadata catalogue of roles, permissions and role-permission definitions is
still read in full, by explicit design: the admin UI must display/edit definitions
not currently assigned to an administrator. Refresh cost is therefore O(metadata
catalogue + selected admin assignments), not O(all customer assignments). The
catalogue is not exposed to the client realm by this feature. A schema with a huge
number of per-customer role definitions still needs separate measurement.

Verified scale run (PostgreSQL 17, synthetic local data, race-enabled tests): 100
admin memberships and 100,000 customer assignments sharing one role. Full API
returned 100,100 assignments in 486.65 ms; scoped API returned 100 in 4.325 ms.
EXPLAIN ANALYZE used a Bitmap Index Scan on auth_subject_roles_pkey (100 rows,
0.144 ms assignment-query execution). These are diagnostic single-run timings,
not production throughput promises. Permissions and client isolation assertions
passed; full metadata catalog was identical between full and scoped APIs.
