package queues //nolint:testpackage // Tests the registered stats endpoint and guards.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	queuesmocks "github.com/assurrussa/goadmin/http/handlers/queues/mocks"
	"github.com/assurrussa/goadmin/internal/auth"
)

func TestQueueStatsRequiresReadAndReturnsAuthoritativeCounts(t *testing.T) {
	for _, tc := range []struct {
		name           string
		denied, failed bool
		status         int
	}{
		{name: "read denied", denied: true, status: 403},
		{name: "current counts", status: 200},
		{name: "storage failure is not zero", failed: true, status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := adminappt.NewAppTest(t)
			jobs := queuesmocks.NewMockjobsRepository(h.Ctrl)
			failed := queuesmocks.NewMockjobsFailedRepository(h.Ctrl)
			h.ExpertGuard(auth.PermissionDomainQueues, auth.PermissionActionRead, func(c fiber.Ctx) error {
				if tc.denied {
					return fiber.ErrForbidden
				}
				return c.Next()
			}).AnyTimes()
			h.ExpertGuard(auth.PermissionDomainQueues, auth.PermissionActionDelete, func(c fiber.Ctx) error { return c.Next() }).AnyTimes()
			h.ExpertGuard(auth.PermissionDomainQueues, auth.PermissionActionUpdate, func(c fiber.Ctx) error { return c.Next() }).AnyTimes()
			if !tc.denied {
				jobs.EXPECT().CountExact(gomock.Any()).Return(int64(17), nil)
				var countErr error
				if tc.failed {
					countErr = errors.New("private database detail")
				}
				failed.EXPECT().CountExact(gomock.Any()).Return(int64(4), countErr)
			}
			app := fiber.New()
			NewHandler(h.App, jobs, failed, nil).RegisterGroupRoutes(app)
			r, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/stats", nil))
			require.NoError(t, err)
			defer r.Body.Close()
			require.Equal(t, tc.status, r.StatusCode)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NotContains(t, string(body), "private database detail")
			if tc.status == 200 {
				require.Equal(t, "no-store", r.Header.Get("Cache-Control"))
				var counts map[string]int
				require.NoError(t, json.Unmarshal(body, &counts))
				require.Equal(t, map[string]int{"jobs": 17, "failed": 4}, counts)
			}
		})
	}
}
