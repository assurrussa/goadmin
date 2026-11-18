# Context7 documentation index

`context7.json` selects consumer documentation, not the library's supported Go
imports. The root README is always included by Context7 when `folders` is set.
`docs/` admits new consumer guides without another config edit. Starter and
public toolkit documentation are included explicitly. Source `.go`/`.vue` files
are not made indexable by these folders: publish important usage examples as
checked Markdown snippets as well as runnable source.

## Deliberate exclusions

- `audit`, `docs/design`, `docs/screenshots`, and this maintainer directory are
  engineering evidence, not consumer recipes.
- Exact filenames exclude historical implementation/review/follow-up notes,
  repository CI instructions and the fixture-only presentation demo. Consumer
  recovery, session, MFA, SSO, localization and theme contracts remain included.
- `project-context.md` remains available for maintainers but is not a second
  consumer entrypoint. `AGENTS.md` is development policy, not an SDK tutorial.
- Both exclusion arrays retain the documented Context7 defaults as checked on
  2026-11-18. Custom arrays **replace**, not extend, the service defaults. Review
  that snapshot when changing these arrays. Filenames cannot contain paths;
  don't exclude all `README.md` files or use broad `*notes*` filename patterns.
- The default branch is intentionally inherited. No version tags, ownership
  keys, redirects, indexing prohibition, extra source extensions or agent rules
  are changed by this configuration.

See [configuration](https://context7.com/docs/library-owners),
[schema](https://context7.com/schema/context7.json) and
[indexed formats](https://context7.com/docs/adding-libraries).

## Local verification

From the repository root:

```sh
python3 scripts/check-context7-config.py
python3 scripts/check-doc-snippets.py
python3 -m unittest discover -s scripts -p check_doc_snippets_test.py
```

This checks the selection policy against tracked Markdown paths and synthetic
regression cases. It models only the documented matching forms used here, not
Context7's crawler. Validate the JSON against the official schema separately
when changing its shape; successful JSON/schema checks do not prove indexing.
The test does not access the network, change the dashboard, or trigger refresh.

## Activation after approval

1. Record the effective dashboard configuration and indexed revision. The
   observed source was `dashboard`; a repository file alone is not evidence of
   activation. Inspect which source is effective before changing anything.
2. Publish the reviewed configuration together with consumer documentation
   through the approved Git process. Reconcile the dashboard with this policy
   only when separately authorized; do not assume an undocumented precedence.
3. Request one manual refresh and wait for successful completion. Verify the
   effective includes/excludes and source SHA, plus parser warnings/errors.
4. Query the actual index for core setup, CRUD, permissions, custom pages and
   uploads. Confirm current user-facing examples appear and audit/QA material
   does not. Use the [fixed question set, rubric and evidence template](context7-evaluation.md).
   Compare its unchanged before/after questions and benchmark explanations; fewer
   snippets is acceptable when irrelevant snippets were removed.

A refresh is asynchronous and subject to limits. It is not proof of a new
benchmark score or a retroactive regrade. Do not add automatic CI or refresh
triggers as part of this configuration-only change.
