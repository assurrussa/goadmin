//go:build integration

package roles_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/http/handlers/roles"
	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	inertiat "github.com/assurrussa/goadmin/infrastructure/inertia/testsupport"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	"github.com/assurrussa/goadmin/infrastructure/roles/adminroles"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
	admintests "github.com/assurrussa/goadmin/tests"
)

const (
	roleMutationAttach = "attach"
	roleMutationDetach = "detach"
)

// These fixtures use the production HTTP handlers, use-case adapters, canonical
// RBAC store and PostgreSQL transactions. Wrappers only control scheduling.
type roleMutationFixture struct {
	auth     *integrationroles.Adapter
	repo     *adminrepo.Repo
	app      *adminapp.App
	useCases roles.UseCases
	actor    models.Admin
	target   models.Admin
	base     integrationroles.Role
	first    integrationroles.Role
	second   integrationroles.Role
	operator integrationroles.Role
}

func newRoleMutationFixture(t *testing.T) *roleMutationFixture {
	t.Helper()
	ctx := t.Context()
	db, _, cleanup := admintests.PrepareDB(ctx, t, "AdminRoleMutation")
	tx := outbox.PgsqlTrxNew(db.DB())
	repo := adminrepo.Must(adminrepo.NewOptions(db, tx))
	auth, err := integrationroles.New(integrationroles.Config{
		Database: db, Memberships: repo, Runtime: admintests.AuthRuntimeConfig(t),
		NotificationSender: admintests.AuthNotificationSender(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, auth.Close())
		cleanup(context.Background())
	})
	f := &roleMutationFixture{auth: auth, repo: repo}
	f.actor = f.provision(t, "actor")
	f.target = f.provision(t, "target")
	f.base = f.createRole(t, "base")
	f.first = f.createRole(t, "first")
	f.second = f.createRole(t, "second")
	f.operator = f.createRole(t, "operator")
	keys := []integrationroles.PermissionKey{
		integrationroles.NewPermissionKey(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionRead),
		integrationroles.NewPermissionKey(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionAssign),
	}
	for _, key := range keys {
		require.NoError(t, auth.Roles().EnsurePermissions(ctx, []integrationroles.CreatePermissionInput{{Key: key}}))
	}
	require.NoError(t, auth.Roles().SetRolePermissions(ctx, f.operator.ID, keys))
	require.NoError(t, auth.Roles().AssignRolesToSubject(ctx, f.actor.SubjectID.String(), []int64{f.operator.ID}))
	f.setTargetRoles(t, f.base.ID)

	guard, err := integrationroles.NewGuardService(
		integrationroles.MustListSubjectRolesUseCase(auth.Roles()), auth.Roles(), logger.Discard(),
	)
	require.NoError(t, err)
	adminRoles := adminroles.Must(guard, repo)
	store := utilst.NewTestStore()
	f.app = adminapp.Must(adminapp.NewOptions(
		inertiat.NewForTest(""), adminRoles, adminRoles, adminservice.NewService(store, adminRoles),
		store, nil, nil, tx, logger.Discard(), "stage", "https://files.example.test",
	))
	f.useCases, err = roles.BuildUseCases(auth.Roles(), auth.Roles(), repo)
	require.NoError(t, err)
	return f
}

func (f *roleMutationFixture) provision(t *testing.T, name string) models.Admin {
	t.Helper()
	account, err := f.auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: name + "@example.test", Password: "Q7!role-mutation-integration-2026",
		Profile: goauth.BasicProfile{Username: name, DisplayName: name},
	})
	require.NoError(t, err)
	admin, err := f.repo.ProvisionAccount(t.Context(), account, identity.NewUserID())
	require.NoError(t, err)
	return admin
}

func (f *roleMutationFixture) createRole(t *testing.T, slug string) integrationroles.Role {
	t.Helper()
	role, err := f.auth.Roles().CreateRole(t.Context(), integrationroles.CreateRoleInput{Slug: slug, Name: slug})
	require.NoError(t, err)
	return *role
}

func (f *roleMutationFixture) setTargetRoles(t *testing.T, ids ...int64) {
	t.Helper()
	require.NoError(t, f.auth.Roles().AssignRolesToSubject(t.Context(), f.target.SubjectID.String(), ids))
}

func (f *roleMutationFixture) targetRoleIDs(t *testing.T) []int64 {
	t.Helper()
	values, err := f.auth.Roles().ListSubjectRoles(t.Context(), f.target.SubjectID.String())
	require.NoError(t, err)
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.ID)
	}
	slices.Sort(ids)
	return ids
}

func (f *roleMutationFixture) httpApp(t *testing.T, useCases roles.UseCases) *fiber.App {
	t.Helper()
	handler := roles.NewHandler(f.app, f.repo, useCases)
	errorHandler := f.app.HTTPManager().MiddlewareErrorListener()
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		// Preserve production POST redirects, while exposing the original
		// handler error status to the test instead of conflating 403 and 500.
		var handlerError *goinertia.Error
		if errors.As(err, &handlerError) {
			c.Set("X-Role-Mutation-Test-Error", strconv.Itoa(handlerError.Code))
		}
		return errorHandler(c, err)
	}})
	app.Use(func(c fiber.Ctx) error {
		c.SetContext(t.Context())
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: f.actor.ID, SubjectID: f.actor.SubjectID})
		return c.Next()
	})
	handler.RegisterGroupRoutes(app)
	return app
}

type roleMutationHTTPResult struct {
	status      int
	errorStatus int
	body        string
	err         error
}

func requestRoleMutation(app *fiber.App, targetID, roleID int64, action string) roleMutationHTTPResult {
	path := fmt.Sprintf("/roles/%d/admins/%s", roleID, action)
	payload := strings.NewReader(fmt.Sprintf(`{"adminId":%d}`, targetID))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, payload)
	req.Header.Set("Content-Type", "application/json")
	response, err := app.Test(req, fiber.TestConfig{Timeout: 15 * time.Second, FailOnTimeout: true})
	if err != nil {
		return roleMutationHTTPResult{err: err}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	errorStatus, _ := strconv.Atoi(response.Header.Get("X-Role-Mutation-Test-Error"))
	return roleMutationHTTPResult{status: response.StatusCode, errorStatus: errorStatus, body: string(body), err: err}
}

func requireRoleMutationStatus(t *testing.T, result roleMutationHTTPResult, status int) {
	t.Helper()
	require.NoError(t, result.err)
	require.Equal(t, status, result.status, result.body)
}

func requireRoleMutationError(t *testing.T, result roleMutationHTTPResult, status int) {
	t.Helper()
	requireRoleMutationStatus(t, result, http.StatusFound)
	require.Equal(t, status, result.errorStatus, "original handler error before the production POST redirect")
}

type roleMutationListFunc func(context.Context, roles.ListAdminRolesRequest) (roles.ListAdminRolesResponse, error)

func (fn roleMutationListFunc) Handle(
	ctx context.Context, req roles.ListAdminRolesRequest,
) (roles.ListAdminRolesResponse, error) {
	return fn(ctx, req)
}

type roleMutationAssignFunc func(context.Context, roles.AssignAdminRolesRequest) (roles.AssignAdminRolesResponse, error)

func (fn roleMutationAssignFunc) Handle(
	ctx context.Context, req roles.AssignAdminRolesRequest,
) (roles.AssignAdminRolesResponse, error) {
	return fn(ctx, req)
}

// awaitRoleMutationContention releases no request until there is positive
// evidence of either PostgreSQL lock contention (fixed code) or the second
// stale read (old code). The ticker merely samples server state; no sleep is
// used to guess that a request reached a critical section.
func awaitRoleMutationContention(t *testing.T, f *roleMutationFixture, secondRead <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-secondRead:
			t.Log("second request read the stale snapshot before the first mutation committed")
			return
		case <-ctx.Done():
			t.Fatal("second request neither waited on a PostgreSQL lock nor reached its role read")
		case <-ticker.C:
			var waiting bool
			err := f.auth.Runtime().Database().QueryRowContext(ctx, `
				SELECT EXISTS (SELECT 1 FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock'
				AND cardinality(pg_blocking_pids(pid)) > 0)`).Scan(&waiting)
			require.NoError(t, err)
			if waiting {
				t.Log("second request is waiting on a PostgreSQL transaction lock")
				return
			}
		}
	}
}

func awaitRoleMutationSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for role mutation barrier")
	}
}

func TestAdminRoleMutationConcurrentHTTP(t *testing.T) {
	for _, firstAction := range []string{roleMutationAttach, roleMutationDetach} {
		t.Run(firstAction+"_then_attach", func(t *testing.T) {
			f := newRoleMutationFixture(t)
			if firstAction == roleMutationDetach {
				f.setTargetRoles(t, f.base.ID, f.first.ID)
			}
			firstRead, releaseFirst := make(chan struct{}), make(chan struct{})
			secondRead, firstDone := make(chan struct{}), make(chan struct{})
			var releaseOnce, doneOnce sync.Once
			defer releaseOnce.Do(func() { close(releaseFirst) })
			defer doneOnce.Do(func() { close(firstDone) })
			firstCases, secondCases := f.useCases, f.useCases
			firstCases.ListAdminRoles = roleMutationListFunc(func(
				ctx context.Context, req roles.ListAdminRolesRequest,
			) (roles.ListAdminRolesResponse, error) {
				result, err := f.useCases.ListAdminRoles.Handle(ctx, req)
				close(firstRead)
				<-releaseFirst
				return result, err
			})
			secondCases.ListAdminRoles = roleMutationListFunc(func(
				ctx context.Context, req roles.ListAdminRolesRequest,
			) (roles.ListAdminRolesResponse, error) {
				result, err := f.useCases.ListAdminRoles.Handle(ctx, req)
				close(secondRead)
				return result, err
			})
			secondCases.AssignAdminRoles = roleMutationAssignFunc(func(
				ctx context.Context, req roles.AssignAdminRolesRequest,
			) (roles.AssignAdminRolesResponse, error) {
				<-firstDone
				return f.useCases.AssignAdminRoles.Handle(ctx, req)
			})
			// Use different actors so only the shared target can serialize
			// these requests; an actor-only lock must not pass this test.
			other := *f
			other.actor = f.provision(t, "other_actor")
			require.NoError(t, f.auth.Roles().AssignRolesToSubject(t.Context(), other.actor.SubjectID.String(), []int64{f.operator.ID}))
			firstApp, secondApp := f.httpApp(t, firstCases), other.httpApp(t, secondCases)
			firstResult, secondResult := make(chan roleMutationHTTPResult, 1), make(chan roleMutationHTTPResult, 1)
			go func() { firstResult <- requestRoleMutation(firstApp, f.target.ID, f.first.ID, firstAction) }()
			awaitRoleMutationSignal(t, firstRead)
			go func() { secondResult <- requestRoleMutation(secondApp, f.target.ID, f.second.ID, roleMutationAttach) }()
			awaitRoleMutationContention(t, f, secondRead)
			releaseOnce.Do(func() { close(releaseFirst) })
			first := <-firstResult
			doneOnce.Do(func() { close(firstDone) })
			second := <-secondResult
			requireRoleMutationStatus(t, first, http.StatusOK)
			requireRoleMutationStatus(t, second, http.StatusOK)
			want := []int64{f.base.ID, f.second.ID}
			if firstAction == roleMutationAttach {
				want = append(want, f.first.ID)
			}
			slices.Sort(want)
			require.Equal(t, want, f.targetRoleIDs(t), "successful requests must preserve both mutation intents")
		})
	}
}

func TestAdminRoleMutationRepeatedHTTPIsIdempotent(t *testing.T) {
	f := newRoleMutationFixture(t)
	app := f.httpApp(t, f.useCases)
	for range 3 {
		requireRoleMutationStatus(t, requestRoleMutation(app, f.target.ID, f.first.ID, roleMutationAttach), http.StatusOK)
		require.Equal(t, []int64{f.base.ID, f.first.ID}, f.targetRoleIDs(t))
	}
	for range 3 {
		requireRoleMutationStatus(t, requestRoleMutation(app, f.target.ID, f.first.ID, roleMutationDetach), http.StatusOK)
		require.Equal(t, []int64{f.base.ID}, f.targetRoleIDs(t))
	}
}
