# Embedded goadmin starter

This is an independent host module using only supported goadmin imports. Local
candidate mode builds the current admin UI and server together and explicitly
mounts four sibling candidates: goauth, gonotify, gouploads and gowebsocket. It never copies a developer module cache and does not require site
or a media resizer. The helper snapshots tracked and non-ignored candidate
sources (including working changes), excluding ignored caches and Git metadata.
Source mode requires Git, Python 3 and Docker Compose. Its GoUploads pin is
`v0.13.0`; the explicit source mode still replaces sibling modules. The root
explicit candidate consumer resolves GoUploads v0.13.0 without that override
when only GoAuth and GoNotify paths are selected.

## Start with the published library

Prerequisites: Git, Docker Engine with Docker Compose, and OpenSSL for generating
local secrets. No sibling repositories or Node.js installation are needed for
this mode. Use the starter source from the **GoAdmin v0.32.0** tag with the
published library and embedded UI from that release.

```sh
git clone --branch v0.32.0 --depth 1 https://github.com/assurrussa/goadmin.git
cd goadmin/examples/starter
cp .env.example .env
# Fill the required values below before starting.
GOADMIN_VERSION=v0.32.0 docker compose up --build
```

This command pins both the starter source and published library. Keep the same
version in your private `.env` for subsequent runs. When upgrading, check out the matching starter revision, read
the release notes and run the published-consumer check from the repository root:
`make externalconsumer-published VERSION=v0.32.0`. That check needs the repository's
Go toolchain; it does not replace running this starter in a browser.

For either mode, copy `.env.example` to `.env` in the checkout you will run.
Set PostgreSQL credentials, a unique CSRF secret, a first-admin setup token of
at least 32 bytes, and three distinct 32-byte keys encoded with standard base64.
Generate each key with `openssl rand -base64 32`. Keep `.env` private and untracked.

### Run the current source

To run local changes, use the current checkout and the four sibling checkouts listed above. In
`examples/starter`, configure `.env` as above, set
`ADMIN_MODULES=products,product-events`, and run
`bash ../../scripts/starter-local.sh up --build`. This builds the checkout's
server and embedded UI together.

The default core uses PostgreSQL for browser and canonical auth sessions; it
needs no Redis or SMTP. The login page offers first-admin registration until an
administrator exists. Enter the setup token from `.env` in that form.

`ADMIN_MODULES` accepts a comma-separated selection from `users`, `access`,
`jobs`, `uploads`, `queues`, `notifications`, `realtime`, `authmail`, `products`,
`product-events` and `product-events-mcp`. Empty means core only. Select `products`
for [your first CRUD resource](../../docs/first-crud.md). Select `product-events`
for the [product event journal](#product-event-journal).
Uploads, queues and notifications require jobs.
For notifications or authmail, set `NOTIFYHUB_URL` and `NOTIFYHUB_PROJECT_KEY`
for an existing NotifyHub gateway. Both modules reuse one client and project key;
no second SMTP connection is needed. Use the gateway origin or deployment prefix,
not an API endpoint. Remote URLs must use HTTPS; the starter does not enable
insecure remote HTTP. Missing or invalid configuration fails startup. Constructing
the client sends no requests and starts no workers. For authmail, also set
`AUTH_MAIL_FROM` to a bare sender email configured in the same NotifyHub project.
The gateway must support `POST /v1/confidential-email`; the starter pins GoNotify
`v0.6.0`, which supplies that client API. A sibling
GoNotify checkout used in source mode must include that release or a compatible
successor. No Mailpit/SMTP Compose profile is required.

Ordinary notifications use NotifyHub's durable acceptance contract. Authmail uses
its synchronous confidential endpoint: only provider acceptance completes a
GoAuth delivery, not inbox delivery. Plaintext auth codes and reset links never
enter the ordinary NotifyHub queue. NotifyHub retains metadata-only confidential
receipts; keep request bodies, credentials and rendered mail out of application,
proxy and gateway logs.

For Redis sessions explicitly set `APP_ADMIN_CANONICAL_SESSION_BACKEND=redis`
and enable the `redis` profile. The admin opens on `http://localhost:8080`; align
`ADMIN_URL` with an overridden `STARTER_ADMIN_PORT`.

`host.New` constructs the graph without starting workers. The starter calls the
unified runtime's `Run`, `Readiness` and `Close`; owned jobs, session cleanup and
authmail workers are supervised together. The runtime cancels and joins them
before the borrowed database and Redis clients are closed. Authmail retains
GoAuth's encrypted durable queue, claims and bounded retry/expiry policy. Each
attempt uses the same delivery ID, rendered content and original `ValidUntil`;
the SDK also bounds requests to ten seconds and respects worker cancellation.
A timeout, unknown, dispatching or simulated result is never success. Same-key
replay reconciles an uncertain provider outcome; it never creates a new key or
falls back to SMTP/ordinary notifications. GoAuth's generic sender contract
retries every error within its existing bounds, including terminal rejection;
the gateway prevents redispatch of a terminal or unknown operation.

Keep sender/template configuration stable while deliveries are pending. Drain or
expire existing deliveries before changing rendered content; a changed payload
under the same key fails closed with an idempotency conflict. To roll back this
example, revert its source/configuration and restart; encrypted queue data and
Hub receipts are retained. Do not replay uncertain deliveries through a different
transport during rollback.

For an explicit migration without serving, run
`bash ../../scripts/starter-local.sh run --rm admin migrate`. PostgreSQL volumes
retain data across restarts. Outside Compose, `DATABASE_DSN` overrides the
PostgreSQL fields.

With uploads selected, gouploads uses original-only async processing and the
owned jobs runtime. `UPLOAD_ROOT` defaults to `/data/uploads`; finalized files
live under `media/v1` and resolve from `ADMIN_URL/uploads`. Staging and unfinished
records are never served as admin files or previews. Existing migration history
is preserved; this starter does not reset host data.

## Restart and recovery

PostgreSQL uses a named volume, but persistence does not guarantee that the app
is ready after a restart. The Compose file gates initial startup on PostgreSQL's
health check; do not assume a simultaneous `docker compose restart` of the app
and database waits for the database to become ready again.

In the PR #96 Mac acceptance run, restarting both services together caused the
app to exit on the migration database ping. Running `docker compose up -d` with
the **same project name and configuration** recovered the app, and the existing
records were still present. This confirms recovery and persistence in that run,
not automatic restart recovery or database-outage resilience.

If you encounter this failure, inspect `docker compose ps -a` and
`docker compose logs admin postgres`, then run `docker compose up -d` and verify
application readiness and the saved records. Preserve any `-p` and `-f` options
used for the original stack in every command: a different project name can use
different volumes. Do not delete volumes or reset the database to fix readiness.
For changed `.env` configuration, use `docker compose up -d --build` rather than
`restart`, which [does not apply changed container configuration](https://docs.docker.com/reference/cli/docker/compose/restart/).

## Source-candidate acceptance (maintainers)

### Compare presentation packs on a real backend

From the repository root, run the opt-in local acceptance command:

```sh
make starter-smoke KEEP=1
```

It builds the current source twice: the standard presentation at
`http://localhost:18768` and the topbar reference at `http://127.0.0.1:18769`.
The different hostnames isolate browser cookies when both are open together.
Both instances enable core plus `users,access,jobs,uploads,queues,notifications,realtime`; each has
its own fresh PostgreSQL and upload volumes. The command generates private test
credentials and keys, without reading this starter's `.env`. The source smoke
build alone selects the `starter_smoke` Go tag: it adds an example host dashboard
card and a notification transport that rejects external delivery. In-app
notification records are seeded only in each run's isolated database. Auth mail
and external NotifyHub delivery remain outside this acceptance profile. Normal
starter builds exclude these fixtures. See [dashboard cards](../../docs/dashboard.md)
for the host extension contract.

Use `SMOKE_RECORDS_ONLY=1 make starter-smoke` to run the narrower record-editing
acceptance: setup, profile/uploads, presentation preferences and users/admins
editing on both packs. Its report is explicitly marked `record-editing`; the
normal profile remains `full` and additionally checks role assignment/restricted
sessions and dashboard summaries, private unread counts, host cards, refresh,
locale and mobile presentation. Both profiles require zero browser console/page errors.


The browser checks first-admin registration, login/logout, profile validation
and persistence, an actual restricted account, avatar processing across Inertia
navigation, WebSocket delivery and responsive presentation. Theme checks measure
the rendered accent, surfaces and text after toggling dark mode and reloading.
Source builds and
the report record the modules actually resolved; copied candidate sources alone
are not evidence that a dependency override was selected. These are local
candidate checks, not published-consumer or production validation.

The command prints its `RUN_ID` and the ignored artifact directory
`output/playwright/starter-smoke/<run-id>/`. Its `report.json` records the source
revision, resolved dependency versions/replacements, image IDs and scenario
results. The directory also holds screenshots, diagnostics and protected test
credential files. `KEEP=1` retains
the two instances for interactive inspection. Without `KEEP=1`, the command
removes its own instances and volumes after the run. To remove a retained run:

```sh
make starter-smoke-down RUN_ID=<printed-run-id>
```

Cleanup targets only that recorded run. The smoke infrastructure's unit checks
are available separately through `make starter-smoke-test` and do not start
Docker. The source Docker build accepts `VITE_ADMIN_EXTENSION_MANIFESTS`; an
empty value retains the standard presentation. Custom packs use the normal
manifest validation and the same pinned client/backend sources.

## Declarative Products example

Add `products` to `ADMIN_MODULES` to enable the host-owned example at `/products`.
`STARTER_MIGRATE=1` or the explicit `migrate` command installs its own Products
schema/ledger after the core migrations. Disabling the module retains its data.
The example defines six simple fields, scoped PostgreSQL queries, per-record
revision checks and transactional domain audit, plus guarded Publish/Unpublish
commands. The form displays the 200-character name limit and the minimum
stock of zero before the operator types. Create, update and direct repository
writes use the same field declarations. See
[Write constraints](../../docs/crud.md#write-constraints).
Each administrator sees their own records. Category is an optional editable
select; publication Status is read-only in normal CRUD and changes only through
the separately authorized Publish/Unpublish commands. Unpublish requires its own
`products:unpublish` permission. From the list or product detail, enter a reason
of up to 500 Unicode code points and confirm. The server trims its outer
whitespace, returns the product to draft and records the reason atomically
with the new revision. The versioned Products
ledger upgrades existing example tables without resetting records. Standard pages use the embedded
renderer without a Vue extension. See [CRUD resources](../../docs/crud.md).

## Product event journal

Add `product-events` to `ADMIN_MODULES` and grant the operator
`product_events:read`. Either product module uses the same starter-owned
migration. To try both, set `ADMIN_MODULES=products,product-events`, start the
starter and create or change a product. Open `/product-events` and search
for the exact product number. Events show the action and UTC time, newest
first, and remain available after deletion. Open an Unpublished event to read
the full reason. Events created before starter migration v3 have no recorded
reason and display an em dash. The detail page takes you back to
the same search and page. Each administrator sees events for their own products.
See [Connect a read-only section](../../docs/crud.md#connect-a-read-only-section)
for the public host contract.

## Connect a local MCP client to Product activity

Enable
`ADMIN_MODULES=products,product-events,product-events-mcp`. The MCP option
requires `product-events`. The operator signs in with a native starter account
that has `product_events:read`. Each admin instance owns its connections in
memory. Restarting the instance clears them.

Each source can have four pairing codes awaiting a decision. An approval,
denial or five-minute expiry frees the source slot. The instance also caps
all outstanding device requests and active grants at 128. Pairing returns
HTTP 429 when either limit is reached.

The starter groups clients by IPv4 address or IPv6 /64 subnet. Clients sharing
a NAT address share its quota. Behind an HTTPS reverse proxy, set
`MCP_PAIRING_TRUSTED_PROXY_IP` to the proxy's exact TCP peer address as seen by
the starter. Configure that proxy to overwrite `X-Real-IP` with the client
address on every request. With this setting, pairing requires a single valid
address from that peer. For every other peer, the starter uses the socket
address. Leaving the setting empty groups connections through a proxy under
the proxy's own quota.

Build the supplied Products launcher from `examples/starter`:

```sh
go build -o /tmp/product-events-mcp ./cmd/product-events-mcp
```

Configure your MCP client to launch that executable as its stdio server. For a
starter listening on the default local port, the server configuration is:

```json
{
  "command": "/tmp/product-events-mcp",
  "args": ["-admin-url", "http://localhost:8080"]
}
```

From the GoAdmin root, the general-purpose client is `go build -o /tmp/goadmin-mcp ./cmd/goadmin-mcp`. Its equivalent arguments are `-admin-url http://localhost:8080 -resource-path /agent/product-events`. The same executable serves another registered resource by changing the path.

Use the origin configured as `ADMIN_URL`. Plain HTTP is supported on loopback;
use HTTPS for a remote starter. The client needs no database credentials.

1. Call `search_product_events` with a product number as text and
   `action: "unpublish"`. Use the positive decimal ID shown in the journal,
   for example `"47"`. The client checks IDs and pagination before it asks for
   access. On the first valid call the program supplies a verification
   URL and a five-minute code. Open the URL in the starter, enter the code from
   your own client and choose **Allow reading**.
2. Repeat the search. It returns the event IDs and the same scoped result
   count as the journal. Optional `page` and `page_size` use 1 and 25 by
   default, with a maximum page size of 100.
3. Call `get_product_event` with the selected `event_id`. The response includes
   the stored reason, with its original internal whitespace. Older events with
   no recorded reason return `null`. Deleted products retain their events.

The grant expires after 15 minutes at the latest. The next call checks the
issuing login, current admin membership, role preview and journal permission
again. Sign out or open `/agent/product-events/connect` and choose **Revoke**
to end access. The client keeps its credential in process memory.

To run browser and real stdio acceptance together, use
`SMOKE_PRODUCT_EVENTS_MCP=1 make starter-smoke` from the repository root. The
default instance enables the pilot and the topbar instance checks the disabled
configuration.

## Confirmed product withdrawal

Enable `ADMIN_MODULES=products,product-unpublish-mcp` and run the starter migration. The example requires a single starter process. Use the standard `goadmin-mcp -admin-url <origin> -resource-path /agent/product-unpublish` command. With a published checkout, install the exact client using `GOWORK=off go install github.com/assurrussa/goadmin/cmd/goadmin-mcp@v0.32.0`. The original `./cmd/product-unpublish-mcp` command still selects this action automatically. Use HTTPS for remote connections.

1. Before the first call, generate and save a lowercase UUID for this proposal. Call `propose_unpublish` with `request_id`, a decimal `product_id` string and a required `reason` of up to 500 Unicode characters. The first call returns a verification URL and a code.
2. In your native admin login, enter the code and select **Подключить**. You need the `products:unpublish` permission. The connection lasts up to 15 minutes. Repeat the same call to get the proposal's review URL.
3. Open that URL in the same login. Check the product name, ID, transition from published to draft and exact reason. **Снять с публикации** immediately changes the product and records the reason. To change the wording, choose **Отклонить** and have the client submit a new proposal with a fresh UUID.
4. Call `get_unpublish_request` with the original `request_id`. A successful receipt includes its product journal event ID. If a response is lost, look up the same UUID. Once the server is reachable, a pending request can be reviewed again. A repeated proposal with a different product or reason is rejected.

Reapplying requires a new proposal after a change to the product, a rejection or expiry. On logout, connection revocation or a server restart, pending proposals from that connection can no longer be approved. An authorized operator can pair again to retrieve completed receipts, including after deletion of the product. The connection and the journal reader have separate pairing codes and permissions.

The upgrade imports earlier receipts into the common ledger. Existing pending
proposals expire because their grants belonged to the old process. The old
table is retained. Disable the write module before rolling back the application.

From `scripts/starter-smoke`, run `npm ci` and `npx playwright install firefox`.
From the repository root, run the following with Docker, OpenSSL and NSS
`certutil` available. Each run creates a private starter, database, proxy,
test CA and Firefox profiles and removes them on exit.

```sh
python3 scripts/starter-smoke/confirmed-https.py --candidate
python3 scripts/starter-smoke/confirmed-https.py --candidate --sessions redis
python3 scripts/starter-smoke/confirmed-https.py --published v0.32.0
```

The published mode builds the starter and stdio client with all source overrides removed.
Set `QA_NATURAL_EXPIRY=1` to wait for the actual 15-minute grant deadline and
`QA_FIREFOX` for an existing Playwright Firefox executable. The default expiry
check advances only the isolated proposal's database deadline.
