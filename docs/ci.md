# Continuous integration

`.github/workflows/ci.yml` runs on pull requests, pushes to `master`, and manual
requests. It uses read-only repository permission, does not retain checkout
credentials, needs no repository secrets, and never deploys or publishes a tag.

## Canonical checks

The workflow uses the Go toolchain declared in `go.mod`, Node.js 24.19.0, and
pinned versions of gofumpt, gci, and golangci-lint. Official setup/checkout
actions are pinned to commit IDs. Update these pins deliberately alongside
local verification.

- `make check`: tidy, formatting, vet, lint, race/coverage tests, manifest and
  frontend unit tests, ESLint, Prettier, type checking, and production build.
- `make externalconsumer-public-deps-local`: an anonymous isolated consumer
  replacing only this GoAdmin checkout; dependencies resolve from the public
  proxy without workspace overrides or developer credentials.
- `git diff --exit-code`: generated assets and dependency files must remain
  committed and current after verification.

## Live integration services

A separate job runs `make integration-check` against disposable PostgreSQL 17
and Redis 7 services, matching the starter's supported service major versions.
Official service images are pinned by registry digest; update those deliberately
when adopting service security fixes.
Only synthetic test credentials/data are used. PostgreSQL's fixture user can
create/drop each test database. Redis is needed by the existing browser-state
and refresh/revocation tests; it is not a new runtime requirement for hosts.

The job sets both the `TEST_PSQL_*` fixture configuration and
`GOAUTH_TEST_POSTGRES_DSN`, plus `TEST_REDIS_ADDR`, `TEST_REDIS_PORT`, and
`GOAUTH_TEST_REDIS_ADDRESS`. Service health checks precede the tests. Missing
PostgreSQL or Redis fails the suite instead of silently compiling or skipping it.
This executes the existing user-update transaction/rollback and ProfileData
JSONB regression cases as well as migrations, auth/session, and upload tests.

To reproduce locally, point the same environment variables at disposable test
services and run `make integration-check`. Never target production: fixtures
create/drop databases and manipulate test session state.

## Remaining coverage

The optional NotifyHub gateway acceptance test requires its own isolated
`GOADMIN_NOTIFYHUB_TEST_URL` and `GOADMIN_NOTIFYHUB_TEST_KEY`; without these it
retains its documented skip. This CI does not provision that gateway. Frontend
component tests run, but real-browser end-to-end flows are not yet a gate.
Release security/advisory checks and published-tag consumer checks remain
explicit in `make publish-readiness` and `make release-readiness`.
