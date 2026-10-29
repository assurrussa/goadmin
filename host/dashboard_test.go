package host_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
)

func TestDashboardDescriptorsCopiedAtRegistrationAndRead(t *testing.T) {
	card := host.DashboardCard{
		Key: "host.reports", Title: host.DashboardText{Text: "Reports", RU: "Отчёты", EN: "Reports"},
		PermissionKey: host.NewPermissionKey("reports", "read"),
		Links:         []host.DashboardLink{{Label: host.DashboardText{Text: "Open", RU: "Открыть"}, Href: "/reports"}},
		Load:          func(context.Context, host.Actor) (host.DashboardContent, error) { return host.DashboardContent{}, nil },
	}
	extension := host.NewExtension().WithDashboardCards(card)
	card.Links[0].Href = "/changed"
	card.Title.RU = "changed title"
	first := extension.DashboardCards()
	require.Equal(t, "/reports", first[0].Links[0].Href)
	require.Equal(t, "Отчёты", first[0].Title.RU)
	first[0].Links[0].Label.RU = "changed"
	first[0].Title.EN = "changed English"
	second := extension.DashboardCards()
	require.Equal(t, "Открыть", second[0].Links[0].Label.RU)
	require.Equal(t, "Reports", second[0].Title.EN)
}
