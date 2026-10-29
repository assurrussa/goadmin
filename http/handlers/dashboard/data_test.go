package dashboard_test

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/http/handlers/dashboard"
	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	auth "github.com/assurrussa/goadmin/internal/auth"
	cards "github.com/assurrussa/goadmin/internal/dashboard"
	"github.com/assurrussa/goadmin/models"
)

func TestDashboardDataReauthorizesAndDoesNotLoadInitialPage(t *testing.T) {
	fixture := adminappt.NewAppTest(t)
	fixture.ExpertGuard("dashboard", "read").Times(2)
	var calls atomic.Int64
	key := auth.NewPermissionKey("reports", "read")
	write := auth.NewPermissionKey("reports", "write")
	card := cards.Card{
		Key: "reports", Title: cards.Text{Text: "Reports"}, PermissionKey: key,
		Links: []cards.Link{
			{Label: cards.Text{Text: "Read"}, Href: "/reports"},
			{Label: cards.Text{Text: "Write"}, Href: "/reports/edit", PermissionKey: write},
		},
		Load: func(ctx context.Context, actor cards.Actor) (cards.Content, error) {
			if _, isFiber := ctx.(fiber.Ctx); isFiber {
				return cards.Content{}, errors.New("Fiber context leaked to worker")
			}
			calls.Add(1)
			return cards.Content{Metrics: []cards.Metric{{
				Key: "owner", Label: cards.Text{Text: "Owner"}, Value: strconv.FormatInt(actor.AdminID, 10),
			}}}, nil
		},
	}
	registry, err := cards.New(nil, []cards.Card{card})
	require.NoError(t, err)
	handler := dashboard.NewHandler(fixture.App, registry)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{
			ID: 42, SubjectID: auth.MustParseSubjectIDString("0065c840-bb75-469b-9529-5147449ad0ed"),
		})
		return c.Next()
	})
	// Every new HTTP request receives a fresh decision rather than caching the page's permission.
	fixture.MockRoleService.EXPECT().AdminGuardCheck(gomock.Any(), int64(42), key).Return(true).Times(1)
	fixture.MockRoleService.EXPECT().AdminGuardCheck(gomock.Any(), int64(42), write).Return(false).Times(1)
	handler.RegisterGroupRoutes(app)
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Contains(t, html.UnescapeString(string(body)), `"state":"loading"`)
	require.Contains(t, html.UnescapeString(string(body)), `"owner":"42"`)
	require.NotContains(t, string(body), "/reports/edit")
	require.Zero(t, calls.Load())
	fixture.MockRoleService.EXPECT().AdminGuardCheck(gomock.Any(), int64(42), key).Return(false).Times(1)
	response, err = app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard/data", nil))
	require.NoError(t, err)
	var denied cards.Snapshot
	require.NoError(t, json.NewDecoder(response.Body).Decode(&denied))
	require.NoError(t, response.Body.Close())
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	require.Empty(t, denied.Cards)
	require.Zero(t, calls.Load(), "a newly forbidden provider must not execute")
	fixture.MockRoleService.EXPECT().AdminGuardCheck(gomock.Any(), int64(42), key).Return(true).Times(1)
	fixture.MockRoleService.EXPECT().AdminGuardCheck(gomock.Any(), int64(42), write).Return(false).Times(1)
	response, err = app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard/data", nil))
	require.NoError(t, err)
	var loaded cards.Snapshot
	require.NoError(t, json.NewDecoder(response.Body).Decode(&loaded))
	require.NoError(t, response.Body.Close())
	require.Len(t, loaded.Cards, 1)
	require.Equal(t, "ready", loaded.Cards[0].State)
	require.Equal(t, "42", loaded.Cards[0].Metrics[0].Value)
	require.Len(t, loaded.Cards[0].Links, 1)
	require.EqualValues(t, 1, calls.Load())
}

func TestDashboardDataHonorsEffectiveRolePreviewBeforeProvider(t *testing.T) {
	fixture := adminappt.NewAppTest(t)
	fixture.ExpertGuard("dashboard", "read").Times(2)
	called := false
	registry, err := cards.New(nil, []cards.Card{{
		Key: "reports", Title: cards.Text{Text: "Reports"},
		PermissionKey: auth.NewPermissionKey("reports", "read"),
		Load:          func(context.Context, cards.Actor) (cards.Content, error) { called = true; return cards.Content{}, nil },
	}})
	require.NoError(t, err)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{
			ID: 42, SubjectID: auth.MustParseSubjectIDString("0065c840-bb75-469b-9529-5147449ad0ed"),
		})
		c.SetContext(auth.WithRolePreview(c.Context(), map[auth.PermissionKey]bool{}))
		return c.Next()
	})
	dashboard.NewHandler(fixture.App, registry).RegisterGroupRoutes(app)
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard/data", nil))
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var snapshot cards.Snapshot
	require.NoError(t, json.NewDecoder(response.Body).Decode(&snapshot))
	require.Empty(t, snapshot.Cards)
	require.False(t, called)
}

func TestDashboardDataGuardDeniesBeforeProvider(t *testing.T) {
	fixture := adminappt.NewAppTest(t)
	fixture.ExpertGuard("dashboard", "read", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusForbidden) }).Times(2)
	app := fiber.New()
	dashboard.NewHandler(fixture.App).RegisterGroupRoutes(app)
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dashboard/data", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
