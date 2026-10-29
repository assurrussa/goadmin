package externalconsumer_test

import (
	"context"
	"testing"

	"github.com/assurrussa/goadmin/host"
)

func TestDashboardUsesOnlySupportedPublicTypes(t *testing.T) {
	extension := host.NewExtension().WithDashboardCards(host.DashboardCard{
		Key: "consumer.reports", Title: host.DashboardText{Text: "Reports", RU: "Отчёты", EN: "Reports"},
		PermissionKey: host.NewPermissionKey("reports", "read"),
		Links:         []host.DashboardLink{{Label: host.DashboardText{Text: "Open"}, Href: "/reports"}},
		Load: func(_ context.Context, actor host.Actor) (host.DashboardContent, error) {
			return host.DashboardContent{
				Metrics: []host.DashboardMetric{{Key: "count", Label: host.DashboardText{Text: "Count"}, Value: actor.SubjectID}},
				Status:  &host.DashboardStatus{Text: host.DashboardText{Text: "Current"}, Tone: "info"},
			}, nil
		},
	})
	if extension == nil {
		t.Fatal("missing extension")
	}
}
