package users

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	authcore "github.com/assurrussa/goadmin/internal/auth"
)

type exportUserRepo struct {
	userRepoStub
	filters datagrid.Filtered
	matches int
}

func (r *exportUserRepo) GetList(_ context.Context, f datagrid.Filtered) ([]authcore.Profile, int, error) {
	r.filters = f
	rows := make([]authcore.Profile, min(r.matches, f.GetLimit()))
	for i := range rows {
		rows[i] = authcore.Profile{ID: int64(i + 1), Email: fmt.Sprintf("user%d@example.test", i)}
	}
	if len(rows) > 0 {
		rows[0].Name = "Имя, \"цитата\"\nстрока"
	}
	return rows, r.matches, nil
}

func TestUserExportMatchingRowsIgnoresPageSize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, query       string
		matches, wantRows int
		truncated         bool
	}{
		{"default", "", 150, 150, false},
		{"filtered-page", "?page=4&limit=20&search=example&status=active&emailStatus=confirmed" +
			"&name=Alice&sortBy=name&sortOrder=asc", 150, 150, false},
		{"at-cap", "?limit=1", 10000, 10000, false},
		{"above-cap", "?limit=1", 10001, 10000, true},
		{"empty", "", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			harness := adminappt.NewAppTest(t)
			for _, action := range []authcore.PermissionAction{
				authcore.PermissionActionRead, authcore.PermissionActionUpdate, authcore.PermissionActionDelete,
			} {
				harness.ExpertGuard(authcore.PermissionDomainUsers, action).AnyTimes()
			}
			repo := &exportUserRepo{matches: tc.matches}
			app := fiber.New()
			NewHandler(harness.App, repo, &userSubjectStoreStub{}).RegisterGroupRoutes(app)
			// Race+coverage on 10,000 rows may exceed Fiber's one-second unit default.
			response, err := app.Test(
				httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/users/export"+tc.query, nil),
				fiber.TestConfig{Timeout: 5 * time.Second, FailOnTimeout: true},
			)
			require.NoError(t, err)
			defer func() { require.NoError(t, response.Body.Close()) }()
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, "text/csv; charset=utf-8", response.Header.Get("Content-Type"))
			require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
			require.Equal(t, strconv.FormatBool(tc.truncated), response.Header.Get("X-Goadmin-Export-Truncated"))
			records, err := csv.NewReader(response.Body).ReadAll()
			require.NoError(t, err)
			require.Len(t, records, tc.wantRows+1)
			require.Equal(t, 10000, repo.filters.GetLimit())
			require.Zero(t, repo.filters.GetOffset())
			if tc.wantRows > 0 {
				require.Equal(t, "Имя, \"цитата\"\nстрока", records[1][3])
			}
			if tc.name == "filtered-page" {
				require.Equal(t, "example", repo.filters.GetSearch())
				require.Equal(t, "name", repo.filters.GetSortBy())
				require.Equal(t, "asc", repo.filters.GetSortOrder())
				require.Equal(t, map[string]any{"status": "active", "emailStatus": "confirmed", "name": "Alice"}, repo.filters.GetFields())
			}
		})
	}
}

func TestUserExportRequiresReadPermission(t *testing.T) {
	t.Parallel()
	harness := adminappt.NewAppTest(t)
	harness.ExpertGuard(authcore.PermissionDomainUsers, authcore.PermissionActionRead, func(fiber.Ctx) error {
		return fiber.ErrForbidden
	}).AnyTimes()
	harness.ExpertGuard(authcore.PermissionDomainUsers, authcore.PermissionActionUpdate).AnyTimes()
	harness.ExpertGuard(authcore.PermissionDomainUsers, authcore.PermissionActionDelete).AnyTimes()
	repo := &exportUserRepo{matches: 150}
	app := fiber.New()
	NewHandler(harness.App, repo, &userSubjectStoreStub{}).RegisterGroupRoutes(app)
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/users/export", nil))
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.Nil(t, repo.filters)
	require.Empty(t, response.Header.Get("Content-Disposition"))
}
