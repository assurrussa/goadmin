# Local administrator MFA policy

GoAdmin accepts canonical authentication settings through `host.Config.Auth`.
Choose `goauth.MFAModeOptional` or `goauth.MFAModeRequired` in
`Auth.MFAMode`. Empty means optional. There is no second Admin policy switch.

- Optional: accounts without a configured factor follow the ordinary permitted
  post-login destination. A dismissible informational notification suggests
  profile settings; no setup interstitial interrupts this redirect. Enrolled
  accounts still complete their factor.
- Required: a valid password without a factor creates only an anonymous,
  browser-bound pending setup. The admin page and APIs remain inaccessible
  until setup is confirmed. Returning to login abandons this pending flow.
- Required startup rejects missing MFA key configuration or an incompatible
  transactional store. It never silently becomes optional.
- External provider admissions retain their separate authority. Local factor
  settings do not replace provider authentication or its validity checks.

Enabling required mode rejects existing password-only sessions on their next
canonical check. Legacy sessions without recorded factor/external admission
also require fresh authentication; unknown origin is never guessed from the
account or browser metadata. New verified external admissions remain separate.

## Explicit host configuration

The embedding application owns environment loading. GoAdmin does not
automatically read this example variable:

```go
mode := goauth.MFAMode(os.Getenv("GOADMIN_MFA_MODE"))
cfg.Auth.MFAMode = mode // empty, "optional", or "required"; invalid values fail
// Keep the existing configured MFAKeys and canonical persistence settings.
runtime, err := host.New(ctx, cfg, deps, modules...)
```

Example deployment input, after configuring the required MFA keys:

```text
GOADMIN_MFA_MODE=required
```

Use `optional` to retain the default enrollment choice. Never generate a fresh
MFA encryption key at each process start; follow GoAuth key lifecycle guidance.
This document does not change any production configuration.

If `Dependencies.Auth` borrows an adapter, configure its
`AuthAdapterConfig.Runtime.MFAMode` when constructing it. Empty
`Config.Auth.MFAMode` uses that owner's policy. A nonempty host mode must match
the borrowed adapter; a conflict fails construction without changing or closing
the borrowed runtime.

## Required setup and interruption

GET `/auth/mfa` displays only pending challenge metadata. It never starts
enrollment or issues a secret. A CSRF-protected POST begins setup and returns the
secret plus inactive recovery codes once, using `Cache-Control: no-store`.
Save the codes before entering the authenticator code. Successful confirmation
activates that exact setup and immediately follows the original safe same-origin
post-login destination; there is no extra post-confirmation Continue screen.

Codes and secrets stay in component memory, never Inertia page props, flash,
session storage or local storage. Leaving, cancelling or restoring browser
history clears them; a restored page reloads authoritative state. Starting
another setup supersedes its previous ID and inactive codes. A wrong factor
may retry, but an uncertain canonical commit cannot be replayed. GoAdmin
retains its non-stealable operation owner, deadline and exact canonical-session
publication fence, and revokes a newly created canonical session if native
browser publication fails.
