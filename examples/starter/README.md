# Embedded goadmin starter

This is an independent host module using only supported goadmin imports. Local
candidate mode builds the current admin UI and server together and explicitly
mounts four sibling candidates: goauth, gonotify, gouploads and gowebsocket. It never copies a developer module cache and does not require site
or a media resizer. The helper snapshots tracked and non-ignored candidate
sources (including working changes), excluding ignored caches and Git metadata.
Source mode requires Git, Python 3 and Docker Compose. Its GoUploads pin is
`v0.10.0`; the explicit source mode still replaces sibling modules. The root
explicit candidate consumer resolves GoUploads v0.10.0 without that override
when only GoAuth and GoNotify paths are selected.

For source candidates use `bash ../../scripts/starter-local.sh up --build`. For a released version use `GOADMIN_VERSION=<exact-tag> docker
compose up --build`; that build removes local overrides and resolves libraries
anonymously through the public Go proxy. Its embedded UI comes from the selected
published goadmin release. This mode cannot pass before compatible public tags
exist. Always run the root published consumer gate for that tag first.

For a local run, copy `.env.example` to `.env`. Set PostgreSQL credentials,
a unique CSRF secret and first-admin setup token of at least 32 bytes, and three
distinct 32-byte keys encoded with standard base64. Generate each key with
`openssl rand -base64 32`. Then run `bash ../../scripts/starter-local.sh up --build`.
The default core uses PostgreSQL for browser and canonical auth sessions; it
needs no Redis or SMTP. The login page offers first-admin registration until an
administrator exists. Enter the setup token from `.env` in that form.

`ADMIN_MODULES` selects comma-separated modules: `access,jobs,uploads,queues,
notifications,realtime,authmail`. Uploads, queues and notifications require jobs.
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
