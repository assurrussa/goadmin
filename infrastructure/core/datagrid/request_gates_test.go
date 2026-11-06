package datagrid //nolint:testpackage // Checks optional config seams and preserved legacy defaults through HTTP.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

type requestGateRepository struct{ err error }

func (r requestGateRepository) GetList(context.Context, Filtered) ([]int, int, error) {
	return nil, 0, r.err
}

func TestCreationGateIsRequestLocalAndAdditive(t *testing.T) {
	allowed := true
	calls := 0
	cfg := FilterConfig[int]{
		Creatable: true, RoutePath: "/users", DefaultSort: "id",
		Columns: []Column{{Key: "id", Sortable: true}}, Repository: requestGateRepository{},
		CanCreate: func(context.Context) bool { calls++; return allowed },
	}
	grid := NewHandler(cfg, logger.Discard())
	app := fiber.New()
	app.Get("/data", grid.HandleData)
	check := func(want bool) {
		t.Helper()
		response, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/data", nil))
		require.NoError(t, err)
		defer response.Body.Close()
		var data APIResponse[int]
		require.NoError(t, json.NewDecoder(response.Body).Decode(&data))
		require.Equal(t, want, data.Config.Behaviour.Creatable)
	}
	check(true)
	allowed = false
	check(false)
	allowed = true
	check(true)
	require.Equal(t, 3, calls)
	require.True(t, grid.GetConfig().Creatable)
	// Nil remains the legacy static flag, and the flag still vetoes a true gate.
	cfg.CanCreate = nil
	legacy := NewHandler(cfg, logger.Discard())
	require.True(t, legacy.config.Creatable)
	cfg.Creatable = false
	cfg.CanCreate = func(context.Context) bool { t.Fatal("disabled creation must short circuit"); return true }
	disabled := NewHandler(cfg, logger.Discard())
	disabledApp := fiber.New()
	disabledApp.Get("/data", disabled.HandleData)
	response, err := disabledApp.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/data", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	var data APIResponse[int]
	require.NoError(t, json.NewDecoder(response.Body).Decode(&data))
	require.False(t, data.Config.Behaviour.Creatable)
	preview := integrationroles.WithRolePreview(t.Context(), map[integrationroles.PermissionKey]bool{
		integrationroles.NewPermissionKey("users", "read"): true,
	})
	require.False(t, legacy.previewActionAllowed(preview, "create"))
}

func TestOptionalAPIErrorCallbackRetainsLegacyDefault(t *testing.T) {
	for _, custom := range []bool{false, true} {
		cfg := FilterConfig[int]{
			RoutePath: "/rows", DefaultSort: "id", Columns: []Column{{Key: "id", Sortable: true}},
			Repository: requestGateRepository{err: errors.New("private storage details")},
		}
		calls := 0
		if custom {
			cfg.APIErrorComponent = func(c fiber.Ctx, _ error) error {
				calls++
				return c.Status(409).JSON(map[string]string{"message": "safe typed outcome"})
			}
		}
		grid := NewHandler(cfg, logger.Discard())
		app := fiber.New()
		app.Get("/data", grid.HandleData)
		response, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/data", nil))
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		if custom {
			require.Equal(t, 409, response.StatusCode)
			require.Equal(t, 1, calls)
		} else {
			require.Equal(t, 500, response.StatusCode)
			require.Zero(t, calls)
		}
	}
}
