# Local verification and manual CI

Local checks are the default development gate. `.github/workflows/ci.yml` is
retained as a reproducible, **manual-only** reference: its sole trigger is
`workflow_dispatch`. Pushes, pull requests, and schedules do not launch the
heavy suite or consume GitHub Actions quota. There is no need to run hosted CI
for every change; use focused tests while editing, then one coherent local
verification pass.

## Proven hosted baseline

The [successful run on 2026-10-03](https://github.com/assurrussa/goadmin/actions/runs/37108504733)
verified merge commit `ad815116beeafe51160f17b46827c931cc97a454` from
[PR #5](https://github.com/assurrussa/goadmin/pull/5). Both jobs passed:

- Canonical checks and public consumer: Go formatting, tidy, vet, lint,
  race/coverage tests, frontend gates (310 UI tests), production build,
  anonymous public-dependency consumer, and clean generated/dependency files.
- PostgreSQL and Redis integration: the complete integration-tagged suite
  against live disposable services, including user-update transaction/rollback,
  ProfileData JSONB, migrations, auth/session, and upload cases.

The manual workflow preserves those job definitions, pinned tools and images.
This historical success is evidence for that commit, not a substitute for local
verification of later changes. The documented skips were the optional
NotifyHub gateway and a legacy sibling-document subtest; neither PostgreSQL nor
Redis was skipped. Logs on GitHub are subject to the repository's retention.

## Local prerequisites

Run from the repository root with Bash, Git, GNU Make, a C compiler for Go's
race detector, and network access for public Go/npm dependencies. Use the Go
toolchain declared by `go.mod` (the baseline used Go 1.27.1) on `PATH`.
Node.js 24.19.0 matches the hosted baseline; the Makefile accepts 22.13+ on the
22.x line or 24+. Docker is needed only for the disposable integration example.

Install the same pinned verification tools into your selected Go environment:

```sh
export GOWORK=off GOTOOLCHAIN=local
go version
node --version
go install mvdan.cc/gofumpt@v0.12.0
go install github.com/daixiang0/gci@v0.14.0
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
# If GOBIN is set, it is the installation directory instead of GOPATH/bin.
export PATH="${GOBIN:-$(go env GOPATH)/bin}:$PATH"
```

The Makefile shares Go caches outside the checkout. Set `GO_SHARED_CACHE_ROOT`
or the documented individual cache variables if its default location is not
writable; do not create a fresh compilation cache for every ordinary check.

## Canonical checks and anonymous consumer

```sh
export GOWORK=off GOTOOLCHAIN=local
make check </dev/null
make externalconsumer-public-deps-local
git diff --exit-code
test -z "$(git status --porcelain --untracked-files=all)"
```

- `make check` runs tidy, non-mutating formatting, vet, lint, one race/coverage
  pass, manifest/frontend unit tests, ESLint, Prettier, type checking, and the
  production build. Redirecting stdin avoids gci treating an inherited pipe as
  source input. It does not need PostgreSQL or Redis.
- `make externalconsumer-public-deps-local` replaces only this GoAdmin checkout.
  The existing script creates isolated HOME/module/build caches, disables
  credentials/workspaces/direct VCS, and resolves the declared dependency graph
  through the public proxy. Expected final line:
  `PASS: anonymous public-deps-local consumer`. A shared-cache or candidate
  replacement run is not equivalent evidence.
- Run the final clean-tree checks after committing intended changes. They
  must produce no diff and no untracked files; store logs outside the checkout.
  Do not discard source edits just to make these checks pass.

Use package-scoped tests while working. Do not stack `make test-race` or
`make cover-html` on a successful `make check` unless diagnosing a specific
problem. `make prepare` is the separate mutating preparation step.

## Disposable PostgreSQL and Redis locally

Never target production or a database containing data you need. Fixtures
create/drop databases and manipulate session state. The following Bash block
starts only disposable PostgreSQL 17 and Redis 7 containers on loopback ports
15432 and 16379, waits for readiness, runs the full integration suite once, and
removes exactly those containers on exit, including failure. Data uses tmpfs;
no persistent volume or pre-existing container is removed. If either port is
busy, choose two unused ports in the assignments before running.

Official image digests match the proven workflow. Update tool/action/image
pins deliberately with verification when adopting new versions or security
fixes.

```sh
bash <<'BASH'
set -euo pipefail
pg_port=15432
redis_port=16379
pg=''
redis=''
cleanup() {
  result=$?
  trap - EXIT
  if [ -n "$redis" ]; then docker rm -f "$redis" >/dev/null || result=1; fi
  if [ -n "$pg" ]; then docker rm -f "$pg" >/dev/null || result=1; fi
  exit "$result"
}
trap cleanup EXIT

pg=$(docker run -d --rm \
  -p "127.0.0.1:${pg_port}:5432" \
  --tmpfs /var/lib/postgresql/data \
  -e POSTGRES_USER=tests-service -e POSTGRES_PASSWORD=tests-service \
  -e POSTGRES_DB=tests-db-pgsql \
  --health-cmd 'pg_isready -U tests-service -d tests-db-pgsql' \
  --health-interval 2s --health-timeout 5s --health-retries 30 \
  postgres:17-alpine@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24)
redis=$(docker run -d --rm \
  -p "127.0.0.1:${redis_port}:6379" --tmpfs /data \
  --health-cmd 'redis-cli ping' \
  --health-interval 2s --health-timeout 5s --health-retries 30 \
  redis:7-alpine@sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499)

for container in "$pg" "$redis"; do
  ready=false
  for attempt in {1..60}; do
    if [ "$(docker inspect --format '{{.State.Health.Status}}' "$container")" = healthy ]; then
      ready=true
      break
    fi
    sleep 2
  done
  if [ "$ready" != true ]; then
    docker logs "$container"
    echo 'Disposable test service did not become healthy' >&2
    exit 1
  fi
done

export GOWORK=off GOTOOLCHAIN=local GOFLAGS=-v ENV_OVERRIDE=0
export TEST_PSQL_ADDRESS=127.0.0.1 TEST_PSQL_PORT="$pg_port"
export TEST_PSQL_ADDRESS_LOCAL=127.0.0.1 TEST_PSQL_PORT_LOCAL="$pg_port"
export TEST_PSQL_USERNAME=tests-service TEST_PSQL_PASSWORD=tests-service
export TEST_PSQL_DATABASENAME=tests-db-pgsql TEST_PSQL_SSL_MODE=disable
export TEST_REDIS_ADDR=127.0.0.1 TEST_REDIS_LOCAL_ADDR=127.0.0.1
export TEST_REDIS_PORT="$redis_port"
export GOAUTH_TEST_POSTGRES_DSN="postgres://tests-service:tests-service@127.0.0.1:${pg_port}/tests-db-pgsql?sslmode=disable"
export GOAUTH_TEST_REDIS_ADDRESS="127.0.0.1:${redis_port}"
make integration-check
BASH
```

Both fixture configuration (`TEST_PSQL_*`, `TEST_REDIS_*`) and the direct
auth/session addresses (`GOAUTH_TEST_*`) must identify these same disposable
services. Explicit local-address values and `ENV_OVERRIDE=0` keep fixture
`.env`/`.env.override` settings from redirecting this example elsewhere.
The synthetic PostgreSQL user can create/drop test databases. Redis is needed
by browser-state and refresh/revocation tests; it is not a new host runtime
requirement. Missing PostgreSQL or Redis fails the suite rather than skipping it.

Successful verbose output includes PASS markers for
`TestAssemblyCommandTransactionRollsBackCanonicalAndProjectionWrites`,
`TestUserProjectionJoinsCanonicalCommandRollback`,
`TestManagedProfileDataSQLRoundTrip`,
`TestRedisStateUsesAtomicOwnerVersionAndRetainsTTL`,
`TestPostgresStateUsesIndependentConnectionsAndEncryptedRecords`, and
`TestPostgresAdminOpaqueRefreshAndRevocation` (both `pgsql` and `redis`).

## Optional manual hosted run

When a fresh hosted run is specifically useful, open
[Actions → CI](https://github.com/assurrussa/goadmin/actions/workflows/ci.yml),
choose **Run workflow**, select the intended branch, and confirm. This consumes
Actions quota. Routine changes need no hosted run. The workflow uses read-only
repository permission, does not retain checkout credentials, needs no repository
secrets, and never deploys or publishes a tag.

Do not make the manual jobs required pull-request checks: without an explicit
dispatch they will not report. Any change to branch-protection requirements is
a separate maintainer decision; this workflow change does not alter protection.

## Remaining coverage and release gates

The optional NotifyHub gateway acceptance test requires its own isolated
`GOADMIN_NOTIFYHUB_TEST_URL` and `GOADMIN_NOTIFYHUB_TEST_KEY`; without these it
retains its documented skip. This workflow does not provision that gateway.
The legacy `TestPublicSurfaceDocsMatchExternalConsumerManifest/admin_package_extraction`
subtest skips when its sibling document is absent. Frontend component tests run,
but real-browser end-to-end flows are not yet a gate.

Release security/advisory checks and published-tag consumer checks remain
explicit in `make publish-readiness VERSION=<next-tag>` and
`make release-readiness VERSION=<published-tag>`; manual CI is not the complete
release gate. See [RELEASING.md](../RELEASING.md).
