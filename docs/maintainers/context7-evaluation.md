# Consumer documentation evaluation

This is a fixed, manual evaluation set for the documentation work. It is not
Context7's proprietary benchmark, does not predict its score, and does not
trigger indexing. Keep this maintainer rubric out of the consumer index: expected
answers must not leak into retrieval as if they were usage documentation.

## Comparable before/after runs

1. Record the indexed commit, effective configuration source and complete include/
   exclude settings. Preserve the original answers and retrieved sources before
   refreshing. If no baseline was captured, report that; never reconstruct one
   from memory or label current answers as the old index.
2. Use the questions below unchanged, one independent conversation per question,
   with the same model/version, reasoning setting, tool access and retrieval
   method. Record any unavailable setting. Do not give the answer criteria to the
   answering model. No account creation, sends, schema changes or payments are
   needed: answers are read-only.
3. Capture the exact answer, retrieved source titles/URLs and source revisions
   where available. A new query is not evidence that the index updated: verify
   the indexed SHA and refresh completion separately.
4. After authorized publication/index activation, repeat under the same conditions.
   If service/model behavior or settings changed, label the comparison as not
   controlled. Do not selectively rerun failed questions without preserving all
   attempts and reporting the reason.
5. Evaluate correctness against the pinned source, not against preferred wording.
   A justified statement that the index lacks needed information is better than
   an invented API. Distinguish documentation defects, missing retrieval and
   answer-generation errors. Correct the failing stage, not the measured score.

## Fixed questions and evidence

The criteria below are for a separate evaluator. Each question should produce a
usable answer with sources, not a verbatim copy of this table.

| ID | Exact question | Required substance | Consumer sources |
| --- | --- | --- | --- |
| Q01 | How do I run GoAdmin locally from a published release, without sibling repositories, and sign in for the first time? | Matching starter source/library pin, required secrets, Compose, setup token, no bundled password; published versus source evidence. | [Starter](../../examples/starter/README.md), [first CRUD](../first-crud.md) |
| Q02 | I want only the admin shell and login. Must I enable access, jobs, uploads, notifications or Redis? | Core auth/membership/RBAC/sessions/CSRF remain; optional screens/modules are explicit; PostgreSQL default; do not enable everything. | [Consumer guide](../consumer-guide.md) |
| Q03 | How do I add a normal Products list, detail, create and edit section? Do I need to build Vue? | `features/crud`, typed Definition/callbacks, embedded renderer for standard fields, module mounted through host.New, host-owned schema/repository. | [First CRUD](../first-crud.md), [CRUD reference](../crud.md) |
| Q04 | Can I use DataGrid or features/users as a generic CRUD generator for my own entities? | DataGrid is list handling, not mutations; users mounts built-in users; choose CRUD or custom page deliberately. | [Consumer guide](../consumer-guide.md), [DataGrid](../../toolkit/datagrid/README.md) |
| Q05 | If I hide a Products button or register a permission, is the API protected and is the permission granted automatically? | No: route/current permission checks, explicit grants, record scope for rows/counts/exports; navigation is not authorization. | [CRUD reference](../crud.md), [first CRUD](../first-crud.md) |
| Q06 | Two tabs of the same administrator edit the same owned record, or a save returns an unknown outcome. What should my callbacks and UI do? | Atomic revision/write/audit, conflict handling, explicit reconciliation; unknown commit result is not permission for automatic replay. | [CRUD reference](../crud.md), [first CRUD](../first-crud.md) |
| Q07 | How do I enable System/Light/Dark, change branding, or supply a custom theme? | Built-in controls, saved account choices, branding permissions; CSS/pack extension only if needed; matching assets and at most one selected pack. | [Integration recipes](../integration-recipes.md), [theme contract](../theme-customization.md) |
| Q08 | I need a saved-form success message and email notifications. Are they the same feature, and what must I connect? | Flash versus durable notification transport; jobs dependency for notifications; explicit provider/lifecycle; provider acceptance is not recipient delivery. | [Integration recipes](../integration-recipes.md), [starter](../../examples/starter/README.md) |
| Q09 | Can password-reset mail use the ordinary notification queue, and does authmail require jobs? | Separate GoAuth encrypted queue and sender contract; authmail does not require jobs; confidential payloads must not enter ordinary delivery/storage/logs. | [Integration recipes](../integration-recipes.md), [starter](../../examples/starter/README.md) |
| Q10 | My custom page opens blank, and writes fail after login. How do route, Inertia page, assets and CSRF fit together? | Authorized route/app.Render, case-sensitive page name, matching manifest/fingerprint/assets/templates and CSRF cookie/header; do not disable auth/CSRF. | [Embedding](../embedding-client.md), [integration recipes](../integration-recipes.md) |
| Q11 | Which GoAdmin packages may a host import, and may I copy internal users handlers or session wiring? | Supported manifest/facade only; exported internals are not supported SDK; business code remains host-owned. | [Public API](../../README.md#stable-public-api), [consumer guide](../consumer-guide.md) |
| Q12 | Can my host use its own theme, notification provider or workflow instead of enabling all GoAdmin modules? | Explicit selection and valid public contracts, owned/borrowed lifecycle, mandatory security and matching client integration; no invented universal replacement API. | [Integration recipes](../integration-recipes.md), [consumer guide](../consumer-guide.md) |

## Evaluation rubric

For each question score these dimensions independently, each from 0 to 2:

- **Correctness:** 0 contradicts the pinned implementation, 1 partly correct or
  materially incomplete, 2 correct for the stated version and scenario.
- **Actionability:** 0 no usable route, 1 steps need substantial reconstruction,
  2 clear next steps and required prerequisites, or a precise supported limitation.
- **Evidence:** 0 invented/unrelated sources, 1 relevant sources but missing key
  support or version uncertainty, 2 relevant consumer sources support the claims.
- **Boundaries:** 0 unsafe/unauthorized/default-on advice, 1 missing a relevant
  safety/ownership distinction, 2 preserves applicable permissions, lifecycle,
  scoping, version and confirmation boundaries.

A score is diagnostic, not a release gate by itself. Any recommendation to
bypass auth/CSRF, expose another owner's data, send secrets through the ordinary
queue or replay unknown writes is a blocking defect regardless of total score.
Report per-question deltas and remaining failures, not only the sum. Do not call
this aggregate the "Context7 score". Capture the service's own score, date and
explanation separately if the service provides a new result.

## Evidence record template

Use one private or explicitly approved report per evaluation. Do not commit
credentials, personal content or service session data.

```text
Evaluation ID:
Run: before | after
Time (UTC):
Documentation commit:
Indexed commit (observed, not assumed):
Configuration source and effective folders/exclusions:
Refresh status/evidence:
Model/version and reasoning setting:
Tool/retrieval method and relevant settings:
Question ID and unchanged question:
Exact answer or authorized artifact reference:
Retrieved source URLs/revisions:
Correctness / Actionability / Evidence / Boundaries (0..2):
Blocking finding, exact claim and supporting source:
Classification: documentation | retrieval | generation | unverified
Next action:
Service benchmark score/date/explanation (only if observed):
```

## Current acceptance boundaries

Compilation, static documentation checks and a source-candidate browser smoke
prove different things. Keep these rows separate in the handoff:

- Public dependency resolution and published starter build.
- First-admin → login → Products create/edit/reload and denied paths on a real
  isolated database/browser, with source/library revisions recorded.
- Effective remote index selection and indexed SHA after refresh.
- Fixed-question results and the independent service benchmark, if refreshed.

Mark blocked/not-run rows explicitly. Do not substitute successful local policy
matching for remote indexing, or compilation for a working user scenario.
