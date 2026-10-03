package users //nolint:testpackage // verifies the registered routes before handler side effects

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	authcore "github.com/assurrussa/goadmin/internal/auth"
)

type permissionUserRepo struct {
	userRepoStub
	readCalls   int
	deletedIDs  []int64
	restoredIDs []int64
}

func (r *permissionUserRepo) GetByID(ctx context.Context, id int64) (authcore.Profile, error) {
	r.readCalls++
	return r.userRepoStub.GetByID(ctx, id)
}

func (r *permissionUserRepo) SoftDeleteByID(_ context.Context, id int64) error {
	r.deletedIDs = append(r.deletedIDs, id)
	return nil
}

func (r *permissionUserRepo) RestoreByID(_ context.Context, id int64) error {
	r.restoredIDs = append(r.restoredIDs, id)
	return nil
}

type permissionSubjectStore struct {
	userSubjectStoreStub
	statuses       []goauth.SubjectStatus
	statusSubjects []goauth.SubjectID
}

func (s *permissionSubjectStore) SetSubjectStatus(
	_ context.Context, id goauth.SubjectID, status goauth.SubjectStatus,
) (goauth.Subject, error) {
	s.statuses = append(s.statuses, status)
	s.statusSubjects = append(s.statusSubjects, id)
	return goauth.Subject{}, nil
}

type routePermissionActor struct {
	name                                string
	authenticated, read, update, delete bool
}

type permissionRoute struct {
	name, method, path, body string
	action                   authcore.PermissionAction
}

// The policy stub authorizes each requested key independently. This exercises
// real Fiber routing and verifies that denied routes cannot reach repositories
// or canonical auth commands; authentication/RBAC implementation has its own tests.
func TestUserRoutesRequireOperationPermission(t *testing.T) {
	t.Parallel()
	for _, actor := range []routePermissionActor{
		{name: "anonymous"},
		{name: "unrelated-role", authenticated: true},
		{name: "read-only", authenticated: true, read: true},
		{name: "reader-editor", authenticated: true, read: true, update: true},
		{name: "reader-deleter", authenticated: true, read: true, delete: true},
		{name: "all-user-permissions", authenticated: true, read: true, update: true, delete: true},
		{name: "update-without-read", authenticated: true, update: true},
		{name: "delete-without-read", authenticated: true, delete: true},
	} {
		t.Run(actor.name, func(t *testing.T) {
			t.Parallel()
			const userPath = "/users/42"
			for _, route := range []permissionRoute{
				{name: "view", method: http.MethodGet, path: userPath, action: authcore.PermissionActionRead},
				{name: "edit-form", method: http.MethodGet, path: userPath + "/edit", action: authcore.PermissionActionUpdate},
				{name: "edit-form-head", method: http.MethodHead, path: userPath + "/edit", action: authcore.PermissionActionUpdate},
				{
					name: "update", method: http.MethodPut, path: userPath,
					body: `{"name":"Changed","email":"changed@example.com"}`, action: authcore.PermissionActionUpdate,
				},
				{name: "delete", method: http.MethodDelete, path: userPath, action: authcore.PermissionActionDelete},
				{name: "restore", method: http.MethodPost, path: userPath + "/restore", action: authcore.PermissionActionDelete},
			} {
				t.Run(route.name, func(t *testing.T) {
					t.Parallel()
					checkUserRoutePermission(t, actor, route)
				})
			}
		})
	}
}

func checkUserRoutePermission(t *testing.T, actor routePermissionActor, route permissionRoute) {
	t.Helper()
	harness := adminappt.NewAppTest(t)
	harness.App.SetCommandTransaction(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
	granted := map[authcore.PermissionAction]bool{
		authcore.PermissionActionRead:   actor.read,
		authcore.PermissionActionUpdate: actor.update,
		authcore.PermissionActionDelete: actor.delete,
	}
	for _, action := range []authcore.PermissionAction{
		authcore.PermissionActionRead, authcore.PermissionActionUpdate, authcore.PermissionActionDelete,
	} {
		harness.ExpertGuard(authcore.PermissionDomainUsers, action, func(c fiber.Ctx) error {
			if !actor.authenticated {
				return fiber.ErrUnauthorized
			}
			if !granted[action] {
				return fiber.ErrForbidden
			}
			return c.Next()
		}).AnyTimes()
	}
	subjectID := authcore.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174131")
	repo := &permissionUserRepo{userRepoStub: userRepoStub{current: authcore.Profile{
		ID: 42, SubjectID: subjectID, Email: "old@example.com", Name: "Old",
	}}}
	subjects := &permissionSubjectStore{}
	app := fiber.New()
	NewHandler(harness.App, repo, subjects).RegisterGroupRoutes(app)
	req := httptest.NewRequestWithContext(t.Context(), route.method, route.path, strings.NewReader(route.body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("Referer", "/users")
	response, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	allowed := actor.authenticated && actor.read && granted[route.action]
	if !allowed {
		status := http.StatusForbidden
		if !actor.authenticated {
			status = http.StatusUnauthorized
		}
		require.Equal(t, status, response.StatusCode)
		require.Zero(t, repo.readCalls)
		require.Zero(t, repo.updateCalls)
		require.Zero(t, subjects.profileCalls)
		require.Empty(t, subjects.email)
		require.Empty(t, repo.deletedIDs)
		require.Empty(t, repo.restoredIDs)
		require.Zero(t, subjects.profileCalls)
		require.Empty(t, subjects.email)
		require.Empty(t, subjects.statuses)
		return
	}
	require.Equal(t, 1, repo.readCalls)
	if route.method == http.MethodGet || route.method == http.MethodHead {
		require.Equal(t, http.StatusOK, response.StatusCode)
	} else {
		require.Equal(t, http.StatusFound, response.StatusCode)
	}
	checkAllowedUserRouteEffects(t, route.name, repo, subjects, subjectID)
}

func checkAllowedUserRouteEffects(
	t *testing.T, routeName string, repo *permissionUserRepo, subjects *permissionSubjectStore, subjectID goauth.SubjectID,
) {
	t.Helper()
	switch routeName {
	case "update":
		require.Equal(t, 1, repo.updateCalls)
		require.Equal(t, 1, subjects.profileCalls)
		require.Equal(t, subjectID, subjects.profileSubjectID)
		require.Equal(t, "changed@example.com", subjects.email)
		require.Equal(t, subjectID, subjects.subjectID)
		require.Empty(t, repo.deletedIDs)
		require.Empty(t, repo.restoredIDs)
		require.Empty(t, subjects.statuses)
	case "delete":
		require.Equal(t, []int64{42}, repo.deletedIDs)
		require.Equal(t, []goauth.SubjectStatus{goauth.SubjectStatusDisabled}, subjects.statuses)
		require.Equal(t, []goauth.SubjectID{subjectID}, subjects.statusSubjects)
		require.Zero(t, repo.updateCalls)
		require.Zero(t, subjects.profileCalls)
		require.Empty(t, subjects.email)
		require.Empty(t, repo.restoredIDs)
	case "restore":
		require.Equal(t, []int64{42}, repo.restoredIDs)
		require.Equal(t, []goauth.SubjectStatus{goauth.SubjectStatusActive}, subjects.statuses)
		require.Equal(t, []goauth.SubjectID{subjectID}, subjects.statusSubjects)
		require.Zero(t, repo.updateCalls)
		require.Zero(t, subjects.profileCalls)
		require.Empty(t, subjects.email)
		require.Empty(t, repo.deletedIDs)
	default:
		require.Empty(t, repo.deletedIDs)
		require.Empty(t, repo.restoredIDs)
		require.Empty(t, subjects.email)
		require.Zero(t, repo.updateCalls)
		require.Zero(t, subjects.profileCalls)
		require.Empty(t, subjects.statuses)
	}
}
