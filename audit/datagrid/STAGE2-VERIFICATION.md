# Stage-two DataGrid runtime verification

Base: `5abd35dac19310311fb81c08ce736c0eb8ed9780` (2026-10-05).
This continues the stage-one ledger; earlier limitations remain historical.

## Demonstrated defect and fix

Real PostgreSQL execution exposed an `IN`/`NOT IN` binding defect: the helper
produced `id IN ($1)` with an entire Go slice as a scalar argument. PostgreSQL's
driver rejected integer, string, and empty slices. SQL-shape tests had accepted
this invalid execution contract. Use Squirrel's existing element expansion and
empty-list semantics instead. Scalar NULL retains SQL membership semantics.
No public API, query cap, selection policy, bulk action, or inline-create feature
changes are introduced.

## Focused automated coverage

- `TestDataGridPostgresOperators`: 32 executed-query cases on PostgreSQL 17.11,
  covering every exported operator, integer/string/scalar/empty membership,
  scalar NULL membership, JSONB paths/keys, and hostile bound input. The table
  survives hostile input. Baseline had six list-binding failures; repaired
  execution passes.
- `TestDataGridAuthenticatedUsersAndExport`: assembled users feature, real
  PostgreSQL migrations, first-admin registration and browser session, anonymous
  denial, paginated list versus all-matching CSV, search/filter/sort, CSV quoting
  and newline preservation, hostile search, and same-session 403 denial
  after canonical role removal. Export responses are CSV with no-store; denied
  sessions do not receive a download attachment.
- Ordinary focused tests: core/public DataGrid and users handler/feature.
- Integration-tagged lint covers the new tests as well as implementation.

This is real host HTTP/database evidence, not browser download evidence. The
HTTP fixture invokes Fiber's request transport without a browser; rendering,
keyboard, actual downloads, responsive layout, and paint need the next gate.

## Native browser gate

The cloud browser was rechecked on 2026-10-05 and returned
`net::ERR_BLOCKED_BY_CLIENT` for its local loopback preview. No alternate
browser or port was used to bypass it. Run the following on an explicitly
approved local QA environment with PostgreSQL and this exact candidate tree.

Use an isolated disposable PostgreSQL instance/user with CREATE DATABASE rights;
never point these tests at production. They create random per-test databases and
run migrations inside them. Set the existing test configuration:

```sh
export TEST_PSQL_ADDRESS_LOCAL=127.0.0.1
export TEST_PSQL_PORT_LOCAL=<isolated PostgreSQL port>
export TEST_PSQL_USERNAME=<test database owner>
export TEST_PSQL_PASSWORD=<test database password>
export TEST_PSQL_DATABASENAME=postgres
```

Run the actual embedded host (no Docker or local module replacements required):

```sh
GOADMIN_DATAGRID_BROWSER=1 go test -tags=integration -count=1 -v -timeout=45m \
  ./features/users -run '^TestDataGridAuthenticatedUsersAndExport$'
```

After automated assertions it serves `http://127.0.0.1:5185/users`. Log in with
the synthetic fixture account printed by the test. These fixed test credentials
are not real credentials and must never be used on a deployed host. The account
has three synthetic users, including a quoted/newline name. The harness uses
embedded production assets and current users routes. Stop it after QA; the
disposable instance can then be stopped. A forced process termination may leave
only its isolated test database for later disposable-instance cleanup.

Check native download filename and CSV bytes, search/filter/sort export scope,
repeated export clicks, interrupted navigation, login expiry/error handling, and
real keyboard activation. The mock demo remains useful for larger rendering and
controlled response races:

```sh
cd resources
npm ci
npx vite --config audit/datagrid/vite.config.ts
```

Open `http://127.0.0.1:5184/`. Cover tab order and Enter/Space activation,
search/sort/filter/pagination, 401/403/500/retry, out-of-order requests,
unmount/remount, narrow viewport overflow, horizontal/vertical scroll and
100/1000-row rendering. Keep the 1000-row case labeled synthetic; the production
GET endpoint cap remains 100. Report actual browser/viewport/measurements rather
than converting JSDOM or subjective speed into a production SLA.

## Native browser baseline and nullable-date repair

The approved native IAB pass used Chrome 154 against candidate `eafcaff`.
The actual authenticated users host passed 1440px and 390px contained-scroll
checks, keyboard name sorting, pagination at limit 1, forward navigation to the
second-page Beta record after login-history traversal, Alpha filtering with
page reset, and expired-session return to login. The separate synthetic demo
covered keyboard selection/reset, 401/403/500 with retry, out-of-order 600/30ms
responses, unmount/remount, safe HTML rendering, and 100/1000-row paints. These
synthetic paints are not production performance/SLA evidence. The demo alone
had 414px horizontal width at a 390px viewport; the actual host measured
390px/390px.

The baseline exposed `Invalid Date` in all six nullable confirmation-date
cells. Go `sql.NullTime` arrives as `{Time, Valid}`; the grid was converting it
to `[object Object]` before formatting. Preserve the value until formatting,
honor `Valid`, and leave absent or malformed dates blank. Component regression
coverage includes valid RFC3339 strings and nullable timestamp objects, invalid
flags, malformed timestamps, null/missing/empty values, and unexpected shapes.
All 98 focused table tests and focused ESLint passed after this repair. The
final aggregate `make check` passed: Go race/coverage, 40 manifest/tooling
tests, 347 frontend tests across 31 files, lint, formatting, type-check, production
asset rebuild, and verification of three admin template assets. The final
native browser recheck remains pending.

Native CSV download remains **unverified**, not passed or a confirmed product
failure. The export button's download event timed out after 20 seconds; direct
attachment navigation then hung and browser control was interrupted before a
saved path or filename could be established. HTTP CSV status, content, scope,
quoting, and authorization evidence above does not prove native download
filename or saved bytes. No alternate browser route was used to bypass a
restriction, and no speculative export implementation change was made.
