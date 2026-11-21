# Confirm an MCP action in GoAdmin

Use `mcp.ConfirmedModule` when an agent may propose a specific change and a
native admin operator must review the record and authorize each execution.
Install the returned module explicitly in `host.New` and run `migrations.Migrate`.
The [starter withdrawal](../examples/starter/unpublish_action.go) declares the
permissions, request arguments, review and result of one such action.

## Connect and confirm

1. Run `goadmin-mcp -admin-url https://admin.example.org -resource-path /agent/product-unpublish`.
   It discovers exactly the proposal and status tools for the configured path.
2. Save a canonical lowercase request UUID before proposing. The first tool call
   supplies a connection URL and a one-use code. Enter it in an authenticated
   native browser session that has all the action's permissions.
3. Repeat the proposal with that UUID. The tool returns a review URL and a deadline.
   The proposal records the specified version and an immutable review snapshot.
   Open it in the original login, inspect the displayed fields, and confirm or decline.
4. Read the receipt with the status tool. Possible outcomes include `succeeded`,
   `declined`, `conflict` for a changed record, and `expired`. Check the original
   UUID after a timeout, database interruption or an `outcome_unknown` response.
   A new UUID represents a new decision.

Grants last up to 15 minutes and authorize exactly one declared action. The
server verifies the original session and current permissions at proposal and
execution. Logout, successful role preview, explicit revocation and restart
end the grant. A separately authorized login of the same operator can retrieve
a terminal receipt. It cannot confirm a pending proposal from the original
login. Keep one active web process. Use a canonical PostgreSQL or Redis native
session and a single trusted proxy address when running behind HTTPS. Configure
the proxy to replace `X-Real-IP` with the socket client's address, and preserve
the external Host. Browser decisions require GoAdmin's session CSRF token and
the exact configured Origin.

## Implement the action

- Choose a stable host namespace, action key and contract version. UUIDs are
  unique per namespace and operator across all actions. An identical retry
  returns its receipt. Reusing a UUID with other normalized arguments or an
  unrelated action returns a conflict.
- Declare all required permission keys, typed input parameters, tool names and
  allowed result fields. Keep the tools' parameter contracts compatible with
  issued proposals. Keep `Normalize` deterministic and able to recognize
  previous versions whose receipts can still be replayed. When a version changes
  the result schema, keep its original fields in `PreviousResultFields` under
  the original version. Replays then return the same values and field names.
  An incomplete receipt or an unknown schema returns `outcome_unknown`.
- `Inspect` must scope the query to the actor, load the identity and revision,
  and lock every relevant row when requested. Include changes to related data
  in the revision when they affect the action. Return `mcp.ErrActionNotFound`
  for unavailable or ineligible records. The fields it returns are stored for
  browser review, with up to 32 plain-text fields, 256 KiB per value and 1 MiB
  total for labels and values. For larger reviews, set `LoadReview` and return
  only the identity and revision from `Inspect`. `LoadReview` must read the
  actor-scoped identity, revision and full review fields from a consistent view
  without locking the domain record. GoAdmin compares the identity and revision
  before displaying the fields to the originating browser login. These fields
  are rendered only in that browser response. The client receives the review URL.
- `Apply` receives the checked snapshot and the current transaction. Write the
  same domain transition used by the native UI, its history and any outbox
  entries through that transaction. Use the request UUID for host correlations.
  Schedule externally visible work in the outbox after commit. Return only
  declared result keys suitable for a durable receipt. For an installed host page,
  set the optional browser-only `ReceiptLink` to a local path, label and required
  permission. The destination must enforce its normal access checks.
- Terminal receipts retain the operator, request ID, contract, normalized
  input hash, record identity, reviewed revision, outcome and declared result.
  Confirmation clears the raw input, binding and review fields in the same
  transaction. Decline, conflict and expiry also clear them. A periodic cleanup
  expires abandoned proposals while the admin runtime runs, including after disabling every action. Keep the library schema
  when disabling the module or rolling back an application.

Run the [starter HTTPS acceptance](../examples/starter/README.md#confirmed-product-withdrawal)
to exercise the standard client and operator interface against a candidate or
a published GoAdmin version.
