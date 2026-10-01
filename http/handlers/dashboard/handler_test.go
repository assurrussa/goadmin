package dashboard_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/http/handlers/dashboard"
)

func TestDashboard_Basic(t *testing.T) {
	adminAppTest := adminappt.NewAppTest(t)
	h := dashboard.NewHandler(adminAppTest.App)
	app := fiber.New()
	app.Get("/", h.Dashboard)

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "dashboard/IndexPage")
	require.Contains(t, string(body), "Главная")
	require.NotContains(t, string(body), "\"total\":44")
}

func TestDashboard_WithPageQuery(t *testing.T) {
	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.ExpertGuard("test", "read").Times(1)
	h := dashboard.NewHandler(adminAppTest.App)
	app := fiber.New()
	app.Get("/", adminAppTest.App.Guard("test", "read"), h.Dashboard)

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/?page=2", nil))
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Главная")
}

func TestDashboard_PageBounds(t *testing.T) {
	adminAppTest := adminappt.NewAppTest(t)
	h := dashboard.NewHandler(adminAppTest.App)
	app := fiber.New()
	app.Get("/", h.Dashboard)

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/?page=100", nil))
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Главная")
}

func TestDashboard_RegisterGroupRoutes(t *testing.T) {
	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.ExpertGuard("dashboard", "read").Times(1)
	h := dashboard.NewHandler(adminAppTest.App)
	app := fiber.New()
	h.RegisterGroupRoutes(app)

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Главная")

	testPage, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/theme", nil))
	require.NoError(t, err)
	defer func() { assert.NoError(t, testPage.Body.Close()) }()
	require.Equal(t, http.StatusNotFound, testPage.StatusCode)
}
