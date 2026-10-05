# Changelog

This file records changes prepared in the checkout. A release is published only
after a new tag and its clean-consumer checks complete.

## Unreleased

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
