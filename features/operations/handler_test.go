package operations_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/mock/gomock"

	hostoperations "github.com/assurrussa/goadmin/features/operations"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/hosttest"
)

type stubRoutes struct {
	calls                int
	err                  error
	hadDeadlineAtCall    bool
	deadlineRemaining    time.Duration
	contextErrSeenAtCall error
}

func (s *stubRoutes) Regenerate(ctx context.Context) error {
	s.calls++
	deadline, ok := ctx.Deadline()
	s.hadDeadlineAtCall = ok
	if ok {
		s.deadlineRemaining = time.Until(deadline)
	}
	s.contextErrSeenAtCall = ctx.Err()
	return s.err
}

type stubSitemap struct {
	calls                int
	err                  error
	hadDeadlineAtCall    bool
	deadlineRemaining    time.Duration
	contextErrSeenAtCall error
}

func (s *stubSitemap) Regenerate(ctx context.Context) error {
	s.calls++
	deadline, ok := ctx.Deadline()
	s.hadDeadlineAtCall = ok
	if ok {
		s.deadlineRemaining = time.Until(deadline)
	}
	s.contextErrSeenAtCall = ctx.Err()
	return s.err
}

type stubFrontendState struct {
	state hostoperations.FrontendBuildState
	err   error
}

func (s *stubFrontendState) Get(_ context.Context, scope string) (hostoperations.FrontendBuildState, error) {
	if s.err != nil {
		return hostoperations.FrontendBuildState{}, s.err
	}
	if s.state.Scope == "" {
		s.state.Scope = scope
	}
	return s.state, nil
}

type stubFrontendRebuild struct {
	enabled           bool
	hasTrigger        bool
	calls             int
	lastReason        string
	err               error
	hadDeadlineAtCall bool
	deadlineRemaining time.Duration
	contextErrAtCall  error
}

func (s *stubFrontendRebuild) Enabled() bool {
	return s.enabled
}

func (s *stubFrontendRebuild) HasTriggerConfig() bool {
	return s.hasTrigger
}

func (s *stubFrontendRebuild) TriggerNow(ctx context.Context, reason string) error {
	s.calls++
	s.lastReason = reason
	deadline, ok := ctx.Deadline()
	s.hadDeadlineAtCall = ok
	if ok {
		s.deadlineRemaining = time.Until(deadline)
	}
	s.contextErrAtCall = ctx.Err()
	return s.err
}

func newOperationsApp(
	t *testing.T,
	routes *stubRoutes,
	sitemap *stubSitemap,
	frontendState *stubFrontendState,
	frontendRebuild *stubFrontendRebuild,
) *fiber.App {
	t.Helper()

	testApp := hosttest.NewApp(t)
	testApp.ExpertGuard(host.PermissionDomainOperations, host.PermissionActionRead).AnyTimes()
	testApp.ExpertGuard(host.PermissionDomainOperations, host.PermissionActionUpdate).AnyTimes()
	testApp.MockRoleService.EXPECT().IsSuperAdmin(gomock.Any(), gomock.Any()).Return(false).AnyTimes()
	testApp.MockRoleService.EXPECT().
		AdminCan(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true).
		AnyTimes()

	handler := hostoperations.NewHandler(
		host.WrapApp(testApp.App),
		routes,
		sitemap,
		frontendState,
		frontendRebuild,
		host.Menu{},
	)

	app := fiber.New(fiber.Config{ErrorHandler: testApp.InertiaManager.MiddlewareErrorListener()})
	app.Use(hosttest.WithSessionAdmin(1))

	handler.RegisterGroupRoutes(app)

	return app
}

func TestHandler_Index(t *testing.T) {
	routes := &stubRoutes{}
	sitemap := &stubSitemap{}
	frontendState := &stubFrontendState{
		state: hostoperations.FrontendBuildState{
			Scope:          hostoperations.DefaultFrontendScope,
			Dirty:          true,
			Revision:       3,
			LastMutationAt: ptrTime(time.Date(2026, time.April, 12, 12, 0, 0, 0, time.UTC)),
		},
	}
	frontendRebuild := &stubFrontendRebuild{enabled: true, hasTrigger: true}

	app := newOperationsApp(t, routes, sitemap, frontendState, frontendRebuild)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/operations", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status %d, got %d", fiber.StatusOK, resp.StatusCode)
	}
}

func TestHandler_RegenerateRoutes(t *testing.T) {
	routes := &stubRoutes{}
	app := newOperationsApp(
		t,
		routes,
		&stubSitemap{},
		&stubFrontendState{},
		&stubFrontendRebuild{enabled: true, hasTrigger: true},
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/operations/routes/regenerate", nil)
	req.Header.Set("Referer", "/operations")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("expected status %d, got %d", fiber.StatusFound, resp.StatusCode)
	}
	if routes.calls != 1 {
		t.Fatalf("expected one routes call, got %d", routes.calls)
	}
	if !routes.hadDeadlineAtCall {
		t.Fatal("expected bounded context deadline")
	}
	if routes.contextErrSeenAtCall != nil {
		t.Fatalf("expected nil ctx err, got %v", routes.contextErrSeenAtCall)
	}
	if routes.deadlineRemaining <= time.Minute || routes.deadlineRemaining > 2*time.Minute {
		t.Fatalf("unexpected deadline remaining: %s", routes.deadlineRemaining)
	}
}

func TestHandler_RedeployFrontend(t *testing.T) {
	frontendRebuild := &stubFrontendRebuild{enabled: true, hasTrigger: true}
	app := newOperationsApp(
		t,
		&stubRoutes{},
		&stubSitemap{},
		&stubFrontendState{},
		frontendRebuild,
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/operations/frontend/redeploy", nil)
	req.Header.Set("Referer", "/operations")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("expected status %d, got %d", fiber.StatusFound, resp.StatusCode)
	}
	if frontendRebuild.calls != 1 {
		t.Fatalf("expected one frontend rebuild call, got %d", frontendRebuild.calls)
	}
	if frontendRebuild.lastReason != "Manual admin-triggered rebuild" {
		t.Fatalf("unexpected rebuild reason: %s", frontendRebuild.lastReason)
	}
	if !frontendRebuild.hadDeadlineAtCall {
		t.Fatal("expected bounded context deadline")
	}
	if frontendRebuild.contextErrAtCall != nil {
		t.Fatalf("expected nil ctx err, got %v", frontendRebuild.contextErrAtCall)
	}
	if frontendRebuild.deadlineRemaining <= time.Minute || frontendRebuild.deadlineRemaining > 2*time.Minute {
		t.Fatalf("unexpected deadline remaining: %s", frontendRebuild.deadlineRemaining)
	}
}

func TestHandler_RedeployFrontendWhenDisabled(t *testing.T) {
	frontendRebuild := &stubFrontendRebuild{enabled: false, hasTrigger: false}
	app := newOperationsApp(
		t,
		&stubRoutes{},
		&stubSitemap{},
		&stubFrontendState{},
		frontendRebuild,
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/operations/frontend/redeploy", nil)
	req.Header.Set("Referer", "/operations")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("expected status %d, got %d", fiber.StatusFound, resp.StatusCode)
	}
	if frontendRebuild.calls != 0 {
		t.Fatalf("expected no frontend rebuild calls, got %d", frontendRebuild.calls)
	}
}

func TestHandler_RegenerateSitemapError(t *testing.T) {
	app := newOperationsApp(
		t,
		&stubRoutes{},
		&stubSitemap{err: errors.New("boom")},
		&stubFrontendState{},
		&stubFrontendRebuild{enabled: true, hasTrigger: true},
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/operations/sitemap/regenerate", nil)
	req.Header.Set("Referer", "/operations")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("expected status %d, got %d", fiber.StatusFound, resp.StatusCode)
	}
}

func TestHandler_RedeployFrontendMapsPublicErrors(t *testing.T) {
	frontendRebuild := &stubFrontendRebuild{
		enabled:    true,
		hasTrigger: true,
		err:        hostoperations.ErrFrontendRebuildDeferredByDebounce,
	}
	app := newOperationsApp(
		t,
		&stubRoutes{},
		&stubSitemap{},
		&stubFrontendState{},
		frontendRebuild,
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/operations/frontend/redeploy", nil)
	req.Header.Set("Referer", "/operations")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app test failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("expected status %d, got %d", fiber.StatusFound, resp.StatusCode)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
