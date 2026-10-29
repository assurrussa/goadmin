package host

import (
	"context"

	"github.com/assurrussa/goadmin/internal/dashboard"
)

// DashboardText contains a literal fallback and optional localized translations.
type DashboardText = dashboard.Text

// DashboardMetric is one provider-owned display value. Values are rendered as text.
type DashboardMetric = dashboard.Metric

// DashboardStatus is optional presentation, never a worker-health guarantee.
// Tone is one of neutral, info, success, warning, or error.
type DashboardStatus = dashboard.Status

// DashboardContent is the data returned by a card provider.
type DashboardContent = dashboard.Content

// DashboardLink is a safe root-relative navigation target. An additional
// PermissionKey narrows the card permission; an empty key inherits it.
type DashboardLink = dashboard.Link

// DashboardCard adds an authenticated, permission-scoped dashboard contribution.
// Key must be stable and unique; the goadmin. prefix is reserved. PermissionKey,
// a nonempty literal Title.Text and Load are required. Load is borrowed and must
// be safe for concurrent requests and honor cancellation/deadlines. It receives
// a standard context with a two-second deadline and a copied canonical identity.
type DashboardCard struct {
	Key           string
	Title         DashboardText
	PermissionKey PermissionKey
	Links         []DashboardLink
	Load          func(context.Context, Actor) (DashboardContent, error)
}

// WithDashboardCards appends cards in registration order. Descriptors are copied
// immediately; validation and duplicate detection run during runtime assembly.
func (e *Extension) WithDashboardCards(cards ...DashboardCard) *Extension {
	for _, card := range cards {
		copied := dashboard.Card{
			Key: card.Key, Title: card.Title, PermissionKey: card.PermissionKey,
			Links: append([]dashboard.Link{}, card.Links...),
		}
		if card.Load != nil {
			load := card.Load
			copied.Load = func(ctx context.Context, actor dashboard.Actor) (dashboard.Content, error) {
				return load(ctx, Actor{AdminID: actor.AdminID, SubjectID: actor.SubjectID})
			}
		}
		e.dashboardCards = append(e.dashboardCards, copied)
	}
	return e
}

// DashboardCards supplies detached descriptors to the internal runtime assembler.
// Hosts register contributions through WithDashboardCards.
func (e *Extension) DashboardCards() []dashboard.Card {
	cards := append([]dashboard.Card{}, e.dashboardCards...)
	for i := range cards {
		cards[i].Links = append([]dashboard.Link{}, cards[i].Links...)
	}
	return cards
}
