package users //nolint:testpackage // exercises route handlers with the canonical auth test transaction

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/assurrussa/goinertia"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	authcore "github.com/assurrussa/goadmin/internal/auth"
)

type updateContractRepo struct {
	userRepoStub
	failure error
}

func (r *updateContractRepo) Update(ctx context.Context, id int64, p authcore.Profile) error {
	if r.failure != nil {
		return r.failure
	}
	return r.userRepoStub.Update(ctx, id, p)
}

type updateContractSubjects struct {
	*goauth.Runtime
	afterEmail error
	emailCalls int
}

func (s *updateContractSubjects) RequestEmailChange(ctx context.Context, id goauth.SubjectID, email string) error {
	s.emailCalls++
	if err := s.Runtime.RequestEmailChange(ctx, id, email); err != nil {
		return err
	}
	return s.afterEmail
}

func TestUserUpdateCapabilityAndTransactionContract(t *testing.T) {
	t.Parallel()
	failure := errors.New("injected command failure")
	for _, tc := range []struct {
		name                    string
		mail, changeEmail       bool
		projectionErr, emailErr error
		wantStatus              int
		wantUpdated             bool
	}{
		{name: "disabled-email-change", changeEmail: true, wantStatus: http.StatusFound},
		{name: "disabled-mail-profile-only", wantStatus: http.StatusFound, wantUpdated: true},
		{name: "enabled-email-change", mail: true, changeEmail: true, wantStatus: http.StatusFound, wantUpdated: true},
		{name: "projection-failure", mail: true, changeEmail: true, projectionErr: failure, wantStatus: http.StatusInternalServerError},
		{
			name: "email-enqueue-then-failure", mail: true, changeEmail: true, emailErr: failure,
			wantStatus: http.StatusInternalServerError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			harness := adminappt.NewAppTest(t)
			harness.App.SetCapabilities(map[string]bool{"authmail": tc.mail})
			for _, action := range []authcore.PermissionAction{
				authcore.PermissionActionRead, authcore.PermissionActionUpdate, authcore.PermissionActionDelete,
			} {
				harness.ExpertGuard(authcore.PermissionDomainUsers, action).AnyTimes()
			}
			var tx goauth.AuthTransaction
			fixture, err := testkit.NewRuntime(func(cfg *goauth.Config) {
				tx = cfg.AuthTransaction
				if !tc.mail {
					cfg.NotificationDelivery = goauth.NotificationDeliveryDisabled
					cfg.EventSink = nil
				}
			})
			require.NoError(t, err)
			//nolint:gosec // isolated synthetic test credential.
			account, err := fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
				Email: "original@example.test", Password: "Unique-User-Contract-Password-2026!",
				Profile: goauth.BasicProfile{GivenName: "Original"},
			})
			require.NoError(t, err)
			repo := &updateContractRepo{userRepoStub: userRepoStub{current: authcore.Profile{
				ID: 42, SubjectID: account.Subject.ID, Email: account.PrimaryEmail.DisplayValue, Name: "Original",
			}}, failure: tc.projectionErr}
			subjects := &updateContractSubjects{Runtime: fixture.Runtime, afterEmail: tc.emailErr}
			transactions := 0
			// Canonical effects use GoAuth's real in-memory transaction. The projection
			// stub stages its snapshot; a separate SQL-executor test checks the DB bridge.
			harness.App.SetCommandTransaction(func(ctx context.Context, fn func(context.Context) error) error {
				transactions++
				before := repo.updated
				err := tx.InAuthTransaction(ctx, fn)
				if err != nil {
					repo.updated = before
				}
				return err
			})
			app := fiber.New()
			var flashedOld map[string]any
			app.Use(captureFlashedOld(&flashedOld))
			NewHandler(harness.App, repo, subjects).RegisterGroupRoutes(app)
			email := "original@example.test"
			if tc.changeEmail {
				email = "changed@example.test"
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/users/42",
				strings.NewReader(`{"name":"Changed","email":"`+email+`"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Referer", "/users/42/edit")
			response, err := app.Test(req)
			require.NoError(t, err)
			defer func() { require.NoError(t, response.Body.Close()) }()
			require.Equal(t, tc.wantStatus, response.StatusCode)
			require.Equal(t, 1, transactions)
			current, err := fixture.Runtime.GetAccount(t.Context(), account.Subject.ID)
			require.NoError(t, err)
			if tc.wantUpdated {
				require.Equal(t, "Changed", current.Profile.GivenName)
				require.Equal(t, "Changed", repo.updated.Name)
			} else {
				require.Equal(t, "Original", current.Profile.GivenName)
				require.Zero(t, repo.updated.ID)
			}
			if !tc.mail && tc.changeEmail {
				require.Zero(t, repo.updateCalls)
				require.Zero(t, subjects.emailCalls)
				require.NotContains(t, flashedOld, "email", "readonly email must fall back to canonical edit-page data")
				require.Equal(t, "Changed", flashedOld["name"])
				followup := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/users/42",
					strings.NewReader(`{"name":"Recovered","email":"original@example.test"}`))
				followup.Header.Set("Content-Type", "application/json")
				followup.Header.Set("Referer", "/users/42/edit")
				next, err := app.Test(followup)
				require.NoError(t, err)
				require.NoError(t, next.Body.Close())
				require.Equal(t, http.StatusFound, next.StatusCode)
				require.Equal(t, "Recovered", repo.updated.Name)
				require.Zero(t, subjects.emailCalls)
			}
			if tc.wantUpdated && tc.changeEmail {
				require.Empty(t, flashedOld, "successful updates must reload canonical fields")
				followup := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/users/42",
					strings.NewReader(`{"name":"Next edit","email":"original@example.test"}`))
				followup.Header.Set("Content-Type", "application/json")
				followup.Header.Set("Referer", "/users/42/edit")
				next, err := app.Test(followup)
				require.NoError(t, err)
				require.NoError(t, next.Body.Close())
				require.Equal(t, http.StatusFound, next.StatusCode)
				require.Equal(t, "Next edit", repo.updated.Name)
				require.Equal(t, 1, subjects.emailCalls)
				require.Len(t, fixture.Events.Events(), 1)
			} else {
				require.Empty(t, fixture.Events.Events())
			}
		})
	}
}

func TestUserUpdateUnknownCommitIsNotRetried(t *testing.T) {
	t.Parallel()
	harness := adminappt.NewAppTest(t)
	for _, action := range []authcore.PermissionAction{
		authcore.PermissionActionRead, authcore.PermissionActionUpdate, authcore.PermissionActionDelete,
	} {
		harness.ExpertGuard(authcore.PermissionDomainUsers, action).AnyTimes()
	}
	attempts := 0
	harness.App.SetCommandTransaction(func(context.Context, func(context.Context) error) error {
		attempts++
		return goauth.ErrOperationOutcomeUnknown
	})
	repo := &userRepoStub{}
	subjects := &userSubjectStoreStub{}
	app := fiber.New()
	NewHandler(harness.App, repo, subjects).RegisterGroupRoutes(app)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/users/42", strings.NewReader(`{"name":"Changed"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Referer", "/users/42/edit")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	require.Equal(t, http.StatusFound, response.StatusCode)
	require.Equal(t, "/users/42/edit", response.Header.Get("Location"))
	require.Equal(t, 1, attempts)
	require.Zero(t, repo.updateCalls)
	require.Zero(t, subjects.profileCalls)
}

func captureFlashedOld(old *map[string]any) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()
		if props, ok := c.Locals(goinertia.ContextKeyProps).(map[string]any); ok {
			*old, _ = props[goinertia.ContextPropsOld].(map[string]any)
		}
		return err
	}
}
