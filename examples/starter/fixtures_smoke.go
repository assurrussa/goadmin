//go:build starter_smoke

package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/assurrussa/gonotify/transport/notifyhub"

	"github.com/assurrussa/goadmin/host"
)

// This build-only fixture never acknowledges or sends an external notification.
// Dashboard acceptance seeds in-app records directly in the run-owned database.
type rejectedSmokeDelivery struct{}

func (rejectedSmokeDelivery) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("external notification delivery is disabled in starter smoke")
}

func starterMailIntegrations(selected map[string]bool) (mailIntegrations, error) {
	if selected["authmail"] {
		return mailIntegrations{}, errors.New("authmail is outside the starter smoke profile")
	}
	if !selected["notifications"] {
		return mailIntegrations{}, nil
	}
	client, err := notifyhub.New(notifyhub.Config{
		BaseURL: "http://127.0.0.1", ProjectKey: "isolated-smoke-no-delivery",
		HTTPClient: &http.Client{Transport: rejectedSmokeDelivery{}},
	})
	return mailIntegrations{client: client}, err
}

func starterDashboardModules() []host.Module {
	return []host.Module{host.FeatureModule(dashboardExample{})}
}

type dashboardExample struct{}

func (dashboardExample) Descriptor() host.FeatureDescriptor {
	return host.FeatureDescriptor{Key: "starter.dashboard"}
}

func (dashboardExample) Build(host.Menu) host.ExtensionBuilder {
	return func(*host.App) (*host.Extension, error) {
		return host.NewExtension().WithDashboardCards(host.DashboardCard{
			Key:           "starter.summary",
			Title:         host.DashboardText{Text: "Карточка хоста", RU: "Карточка хоста", EN: "Host card"},
			PermissionKey: host.NewPermissionKey(host.PermissionDomainDashboard, host.PermissionActionRead),
			Links: []host.DashboardLink{{
				Label: host.DashboardText{Text: "Открыть профиль", RU: "Открыть профиль", EN: "Open profile"},
				Href:  "/auth/profile",
			}, {
				Label: host.DashboardText{Text: "Администраторы", RU: "Администраторы", EN: "Administrators"},
				Href:  "/admins", PermissionKey: host.NewPermissionKey(host.PermissionDomainAdmins, host.PermissionActionRead),
			}},
			Load: func(ctx context.Context, actor host.Actor) (host.DashboardContent, error) {
				if err := ctx.Err(); err != nil {
					return host.DashboardContent{}, err
				}
				if actor.IsZero() {
					return host.DashboardContent{}, errors.New("dashboard requires a current actor")
				}
				return host.DashboardContent{Metrics: []host.DashboardMetric{{
					Key: "source", Label: host.DashboardText{Text: "Источник данных", RU: "Источник данных", EN: "Data source"}, Value: "starter",
				}}}, nil
			},
		}), nil
	}
}
