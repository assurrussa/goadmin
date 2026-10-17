# Security UX implementation

Scope: assisted recovery, local MFA settings, read-only role preview. No deployment or production access.

| State owner | Transition | Success | Error / cancellation / replay |
| --- | --- | --- | --- |
| MFA canonical runtime; page owns ephemeral setup | disabled → begin → confirm | factor activates only on confirmed code; codes displayed once; all sessions invalidated | unavailable and failed status never imply disabled; uncertain mutations lock UI pending sign-in; secrets clear on leave; no automatic replay |
| Recovery canonical runtime; operator page owns form | selected subject + independent evidence + fresh operator proof → invitation | only queued invitation; no immediate account mutation | exact permission and native CSRF retained; unknown outcome locks submission; completion uses anonymous cookie CSRF and fragment secret |
| Role preview session; canonical roles own grants | owner enters role / changes role / exits | request grants are actual grants intersect selected role; global mutations denied | no identity or role assignment changes; revalidate owner and role each request; session persists restriction when query omitted |

Checks are delegated to the parent test lane; unrun checks are not passes.

Recovery targets resolve an established email with canonical `FindAccount`; there is no dependency on the optional users-management projection. The operator selects the resolved account rather than entering an email into a UUID field. The public completion page uses the same locale storage preference, nonce-scoped inline styles/scripts, anonymous CSRF cookie, fragment removal, and a single-attempt form. No third-party QR service is used: MFA QR pixels are generated locally by the pinned qrcode package.

Role preview is a server session policy, revalidated against current canonical Super Admin membership and current selected-role permissions on every request. It intersects permission checks with actual authority, and denies every unsafe HTTP method before handlers. Enter/exit use the existing native authenticated CSRF pipeline. It never writes actual role assignments or changes the current actor.

Verification additions: middleware regression proves session persistence, query omission, direct URL restriction, read-only denial, current-owner revalidation and exit; guard regression proves preview cannot grant missing actual authority; menu regression suppresses cached/session Super Admin navigation bypass during preview; MFA UI regressions cover unavailable/unknown, local QR/cancel, unknown mutation and late response after pagehide. Existing native recovery CSRF integration now inspects Inertia props instead of the former standalone HTML.

The existing permission engine defines the owner as canonical `super_admin`; there is no separate `owner` role alias. Previewing `super_admin` preserves that role's existing permission bypass for presentation, while actual authority and the global mutation fence remain enforced. Menu filtering explicitly suppresses the session-role `allowAll` shortcut during preview, so selected-role policy is used consistently.

A final action-metadata audit found legacy grid actions whose endpoint guards enforced permissions but whose UI callbacks did not. During preview, DataGrid now filters standard row actions and the create control through selected-role permission keys; ordinary requests retain existing callbacks. This closes an apparent edit/delete capability for viewer roles without changing real role assignments or mutation guards.
