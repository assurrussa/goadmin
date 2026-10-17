# Admin interface changes, 2026-10-17

Scope: persisted RU/EN core interface, extensible dictionary and collapsible
sidebar, alongside separately committed recovery/MFA/role-preview changes.
No optional upload module is enabled. Production and workflow configuration are
outside scope. Base: 28b109703dde8cfecd274e1a4f55eb4a2068e730.

State transitions:

- Language owner: browser preference. Initial/corrupt/unsupported -> Russian;
  select RU/EN -> immediate labels + persisted preference; storage failure ->
  session choice; reload -> stored supported language. Translation only changes
  displayed UI messages, never field values, canonical subjects or permissions.
- Sidebar owner: browser preference and component state. Desktop expanded <- ->
  collapsed, persisted; mobile always opens expanded, overlay/Escape/navigation
  closes and restores focus; reload retains desktop preference. Nested groups
  remain individually accessible in both modes, active descendants initially
  expand their groups. Group toggles do not navigate or submit forms.

Verification is recorded by the separate Sol low check lane with exact source
hashes and raw command logs. No prior completed test result is claimed as a new
pass.
