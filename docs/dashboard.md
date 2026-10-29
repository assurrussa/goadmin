# Dashboard cards

The core dashboard opens with navigation and authorized card descriptors, then
loads values through `GET /dashboard/data`. It includes queue counts only when
the queues module is mounted and the actor has `queues:read`, and personal unread
notifications only when notifications is mounted. No optional module is enabled
by the dashboard. Queue counts describe stored jobs and failed jobs, not worker
health. Notification counts are scoped to the current administrator.

## Add a host card

Register cards from the existing feature extension builder. There is no new
public package, mandatory feature-interface method, client extension or separate
frontend build for ordinary cards:

```go
func (f feature) Build(host.Menu) host.ExtensionBuilder {
    return func(*host.App) (*host.Extension, error) {
        return host.NewExtension().WithDashboardCards(host.DashboardCard{
            Key: "content.pending",
            Title: host.DashboardText{
                Text: "Pending publications", RU: "Ожидают публикации", EN: "Pending publications",
            },
            PermissionKey: host.NewPermissionKey("content", "read"),
            Links: []host.DashboardLink{{
                Label: host.DashboardText{Text: "Open publications", RU: "Открыть публикации"},
                Href: "/publications?status=pending",
            }},
            Load: func(ctx context.Context, actor host.Actor) (host.DashboardContent, error) {
                count, err := f.publications.CountPending(ctx, actor.SubjectID)
                if err != nil {
                    return host.DashboardContent{}, err
                }
                return host.DashboardContent{Metrics: []host.DashboardMetric{{
                    Key: "pending",
                    Label: host.DashboardText{Text: "Awaiting publication", RU: "Ожидают публикации"},
                    Value: strconv.FormatInt(count, 10),
                }}}, nil
            },
        }), nil
    }
}
```

The snippet belongs to a host-owned feature with its own publications service;
use the normal `host.FeatureDescriptor` permission definitions and mount that
feature explicitly. `actor.AdminID` identifies the administrator projection;
`actor.SubjectID` is the canonical subject. The service retains responsibility
for tenant/resource filtering within that actor's authorized scope.

The `starter_smoke` build of the standalone starter supplies a compiled example
card using this facade. Its literal `starter` value identifies a synthetic host
source rather than a business metric. Normal published/source starter builds
exclude the fixture and retain their existing integration configuration.

## Contract and ownership

- A card needs a unique stable key, nonempty literal title, permission and loader.
  The `goadmin.` key prefix is reserved. Invalid/duplicate registrations fail
  runtime assembly; registration descriptors are copied.
- A link inherits its card's permission. Its optional additional `PermissionKey`
  may narrow access. Links must be internal root-relative URLs. They only navigate;
  target pages and mutation endpoints keep their own guards.
- `DashboardText.Text` is the literal fallback; `RU` and `EN` are optional
  client-selected translations. All text and metric values are escaped text,
  with no HTML or arbitrary Vue widgets. `DashboardContent.Status` is optional;
  its tone is neutral, info, success, warning or error.
- Loaders are borrowed, safe for concurrent requests, and must honor their
  standard context's cancellation and two-second deadline. Up to four run at
  once per snapshot. Permissions are checked before invoking a loader. No Fiber request
  context is passed to concurrent callbacks. Errors, invalid content and panics
  make that card unavailable rather than substituting zero.
- Built-in queues and notifications precede host cards in registration order.
  Initial descriptors do not invoke providers. Each data request reauthorizes
  card and link access and returns a complete snapshot with string owner ID,
  ordered cards and timestamps. Responses are not cacheable.

## Refresh behavior

Refresh is manual by default. The opt-in minute interval is stored per current
administrator in this browser and runs only while the dashboard is visible.
Returning to the visible page performs one refresh. Concurrent refresh requests
coalesce; leaving the page or losing the session stops refresh work.

A failed refresh retains previously successful metrics and status with their
original timestamps as stale. The previous status uses a neutral tone alongside
the stale warning. First-load failures show unavailable values, never fake zeros.
Omitted cards are removed with their old values. Owner changes clear the snapshot,
and late responses cannot restore a previous actor's data.

Owner changes stop the active auto-refresh interval without overwriting either
administrator's saved preference. Mounting the dashboard restores the current
owner's preference. Only an explicit toggle writes that preference; session
cleanup and leaving the page preserve it.

## Verification boundary

Use `make check` and `make externalconsumer-local` for the library and supported
consumer contract. `make starter-smoke` builds both presentation packs with
run-owned databases, an example host card and a build-only notification transport
that rejects external delivery. Its unread-notification fixtures validate
dashboard isolation; they do not prove delivery to NotifyHub. No external
notification account is required. Runtime palettes and dashboard mutations are
outside this contract.
