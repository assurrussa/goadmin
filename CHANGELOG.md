# Changelog

This file records changes prepared in the checkout. A release is published only
after a new tag and its clean-consumer checks complete.

## v0.33.0

- `crud.NewWithActions` adds row and detail commands to application-created
  records. GoAdmin handles permissions, confirmation, typed parameters and
  revision checks. The host commits the domain transition and audit together.
  The opt-in [Requests starter](examples/starter/README.md#review-a-request)
  demonstrates approval and rejection.
- `ListColumns` lets all three CRUD constructors reuse field labels, types and
  select options in a chosen column order. Existing `Columns` declarations
  remain supported. The Products starter keeps its saved-view column keys.
  DataGrid GET filters for text, select and badge columns preserve leading
  and trailing spaces when the value exactly matches a configured option.
- Standard CRUD tables link directly to record pages. Operators can open
  records in another tab and return to the original search, filters and page
  after a decision.
- The guides clarify [SSO role assignment permissions](docs/sso-admission.md#local-authorization)
  and [account email delivery](docs/integration-recipes.md#authentication-mail-is-a-separate-integration).

## v0.32.1

- After pairing for read access or confirmed actions, the browser prompts the
  operator to return to the MCP client and retry the request.
- Regression tests cover independent read permissions and grants for two
  resources and revocation of a confirmed-action grant at native logout.
- The starter uses the CSRF service created by `host.New` for confirmed product
  withdrawal. The first CRUD guide's Products example is checked by `make check`.

## v0.13.0 candidate (unpublished)

- Add opt-in assisted recovery, local MFA settings and owner-only read-only role
  preview, with shared Russian/English UI.
- Refine sidebar selection and shared shell/control/card theme tokens; provide
  opt-in compact/comfortable density and branding examples.
- Pin published GoAuth v0.11.0 (schema 14). Hosts upgrading from schema 13 must
  migrate before starting compatible writers/workers; older binaries cannot
  assume rollback compatibility. Rebuild consuming host bundles. See
  [upgrade and rollback](RELEASING.md#v0130-candidate).
- Release checks and publication are separate gates; this entry is not a
  readiness claim.

## Earlier checkout history

The following entries retain historical scope and do not all describe changes
since v0.12.3.

- Current-page external authentication recovery is opt-in: keep mounted form
  drafts and require manual resubmission, without retaining or replaying a
  rejected business mutation. Preserve native cookie/session identity and
  canonical revocation checks through encrypted-journal owner/version fencing.
  Legacy journal rows without a binding require one explicit external login
  after proof expiry. Coordinate application-node rollout and rollback with
  the host recovery endpoint; see [recovery](docs/external-proof-recovery.md).

- The optional `auth-providers` slot from
  [PR #35](https://github.com/assurrussa/goadmin/pull/35) is included in this
  consolidated unreleased candidate, inside the existing login card before the
  local form. It remains unavailable in published v0.11.1. Default local login
  stays unchanged when omitted.

- Preserve uncertain upload completion across navigation/remounts in both
  built-in upload widgets, with same-tab metadata recovery scoped by admin,
  endpoint, entity and context. Cancel torn-down transports without allowing a
  duplicate session. Reconciliation now unlocks definite server rejections while
  preserving ambiguous transport outcomes and pre-dispatch cancellation fences.

- Add validated, immutable `UploadsConfig.Strategies` registration on the existing
  full `/files` upload handler. Custom audio contexts can use canonical MP3/WAV
  finalization with GoUploads v0.11.0+ original-only processing; built-in policies
  remain unchanged. Cover the full client's existing audio category and numeric
  file-type response contract without changing client production logic.

- Serve owned `/public` assets without application compression to avoid the
  upstream compressed-stream reader race on client disconnect. Asset bytes and
  dynamic compression are unchanged; uncached static transfers may be larger.
  Explicitly identity-refusing requests for existing assets receive an empty
  HTTP 406 response, without replacing authentication or missing-file errors.

- Require Go 1.27 and replace the all-dependencies installer with typed
  `host.New` and explicit access/jobs/uploads/queues/notifications/realtime/authmail
  modules; the default core needs PostgreSQL and security keys.
- Supervise owned workers through one Runtime lifecycle while preserving host
  ownership of borrowed infrastructure; add persistent PostgreSQL browser sessions.
- Execute firstadmin and administrator/profile/preview commands with canonical
  GoAuth writes and action audit in one transaction. Reject stale preview bindings
  without overwriting file metadata; preserve previews when uploads is disabled.
- Gate client controls, WebSockets and upload completion polling by installed
  capabilities. Preserve authenticated sessions when bootstrap redirects save flash.

- Replace the sample admin home with permission-filtered navigation and clearer
  access, users, and system sections.
- Show actionable table loading, empty, and error states, and warn before
  discarding edited form data.
- Add a runnable local host starter, an additive outbox lifecycle facade,
  public usage and security documentation, and a deterministic UI unit-test
  gate.
- Make the external consumer probe reject a dependency version silently
  upgraded by Go's module selection.
- Refresh compatible locked npm transitive packages so the high-severity
  advisory gate passes without a frontend major-version upgrade.
