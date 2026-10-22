# Optional SSO administrator admission (candidate)

This component is off unless explicitly constructed and mounted. Existing local
login, confirmed identity linking and LoginExternalAdmin keep their contracts.
This branch is a candidate, not a published feature or a deployment instruction.

## Ownership and assembly

- AuthHub grants access to a project; successful central login alone is not that grant.
- The RP verifies protocol state, exact issuer/project/client and fresh authority.
- GoAuth owns canonical identity. The strict ProvisionExternalIdentity API never
  auto-links email, creates a local password, grants membership or issues tokens.
- GoAdmin owns scoped invitations, pending decisions, admin projection and local RBAC.
- Host code supplies a trusted verifier and browser-bound opaque reference reader.
  Never bind browser JSON directly to VerifiedIdentity.
- Mount access.New() and component.Module() explicitly. No route/menu or provisioning
  is enabled by importing the package or constructing an unmounted component.
  The component must use the exact native PostgreSQL Runtime, not another pool.

Construct features/admission.New with Config{Runtime, Mode, Scope, Verifier,
Reference, SignInPath}. Use InvitationOnly or ProjectPending; no implicit mode.
Include Module() in host.New. The module owns /admissions and GET /auth/admission.
After a successful fresh host SSO callback, call Complete with the opaque RP
reference and optional invitation ID retained in the host's state-bound flow.
An Active result is server-only input to ordinary canonical binding/admission.
Still call LoginExternalAdmin; never set a cookie or authorize a request directly
from Result. For pending/approved/rejected results, use /auth/admission without a
native admin session. This waiting page is read-only; its link starts a fresh SSO flow.

Verifier.Verify may do network work before the transaction. Revalidate must be
read-only and local, without refresh/network under locks. A verifier error, missing
project access, stale proof or wrong pinned scope denies before local enrollment.
No failure consumes an invitation. Grant added later requires another successful
SSO callback. The component does not add an AuthHub grant or bypass its denial.

## Local authorization

The operator needs explicit admission:invite/approve/reject/read as appropriate.
Role assignment additionally requires roles:assign and admission:grant-<role UUID>
for every selected role. This is hierarchy-free: owning a role or a super-admin
label is not a substitute for those checks. The super_admin bootstrap role is not
enrollable through this component.

The component contributes permission definitions for roles existing at assembly.
For roles created later, register GrantRolePermission(role.PublicID) through the
normal permission catalogue and grant it explicitly; restart/reassemble also
discovers it. Defining a permission grants nothing. Do not auto-grant the catalogue
to every existing administrator.

Invitation and approval retain immutable role IDs and an exact permission-set
fingerprint. Acceptance rechecks the issuing actor's active membership, current
grant rights and role snapshot. Changed permissions or revoked authority deny;
an operator must issue a new decision. Accepted users retain normal local RBAC
semantics. Revoking an issuer's right does not revoke already granted memberships.

## Lifecycle and data

A pending canonical SSO-only account has NO administrations membership. Pending
is scoped to this application; it does not suspend a subject globally or change
other realms. Approval records a bounded decision but does not admit a browser:
activation happens on the next verified SSO callback. Rejected records do not
reactivate automatically. Deleted membership or suspended/disabled subjects deny.

Invitations target exact issuer + external subject and pinned project/client.
Their random UUID is an identifier, not a bearer credential. Never accept an
invitation based only on an email match or URL possession. TTL is at most seven
days by the public component. A failed attempt does not extend it. Invitations
are revoked explicitly; successful consumption and membership/roles commit once.
An existing local account with the same email needs explicit authenticated linking.

The migration is additive and uses the normal GoAdmin ledger; migration alone
does not enable the component. Enrollment events include pending subjects in an
append-only table; admitted operator actions also use admin_action_audit. No
tokens, credentials or provider response bodies enter audit. Existing data is
not deleted on module disablement. Ordinary authenticated routes still enforce
canonical membership/RBAC and the existing external proof/revocation rules.

## Candidate verification and release

The component requires GoAuth v0.12.0 or newer for strict provisioning.
This branch pins the published v0.15.0 module without local replacements.
Construction fails closed on an older Runtime. Candidate source comparisons may
use an explicitly selected temporary modfile; they are not publication evidence.
Before release, verify the embedded admin assets and run the clean consumer gate
against the declared public dependency graph.

Required checks: package/unit tests, public surface and assembly/off-mode tests,
PostgreSQL atomicity/concurrency/audit rollback, exact role grants, stale/revoked
invitations, wrong identity, blocked users, no protected access while pending,
current native CSRF and role-preview denial, then one focused browser flow with
the regenerated assets. Do not infer a PASS from disabled buttons or source review.
Production configuration, invitation issuance and role grants are separate actions.
