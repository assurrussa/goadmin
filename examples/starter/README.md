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
For notifications, set `NOTIFYHUB_URL` and `NOTIFYHUB_PROJECT_KEY` for an existing
NotifyHub gateway. Remote URLs must use HTTPS; the starter does not enable
insecure remote HTTP. Missing or invalid configuration fails startup. Constructing
the transport sends no requests and starts no workers. Notifications use
NotifyHub's durable acceptance contract; authmail keeps its separate SMTP sender
and never sends plaintext auth codes or tokens to NotifyHub's durable queue.
For authmail, set `SMTP_FROM` and enable Compose's `authmail` profile, for example
`bash ../../scripts/starter-local.sh --profile authmail up --build`. Mailpit then
captures SMTP messages at `http://localhost:8025`. For Redis sessions explicitly
set `APP_ADMIN_CANONICAL_SESSION_BACKEND=redis` and enable the `redis` profile.
The admin opens on `http://localhost:8080`; align `ADMIN_URL` with an overridden
`STARTER_ADMIN_PORT`.

`host.New` constructs the graph without starting workers. The starter calls the
unified runtime's `Run`, `Readiness` and `Close`; owned jobs, session cleanup and
authmail workers are supervised together. The runtime cancels and joins them
before the borrowed database and Redis clients are closed. Authmail retains
GoAuth's encrypted durable queue, claims and retries. SMTP delivery is at least
once and may produce duplicate messages after uncertain acceptance. Production
hosts should provide a TLS SMTP transport.

For an explicit migration without serving, run
`bash ../../scripts/starter-local.sh run --rm admin migrate`. PostgreSQL volumes
retain data across restarts. Outside Compose, `DATABASE_DSN` overrides the
PostgreSQL fields.

With uploads selected, gouploads uses original-only async processing and the
owned jobs runtime. `UPLOAD_ROOT` defaults to `/data/uploads`; finalized files
live under `media/v1` and resolve from `ADMIN_URL/uploads`. Staging and unfinished
records are never served as admin files or previews. Existing migration history
is preserved; this starter does not reset host data.
