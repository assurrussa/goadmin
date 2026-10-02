package users //nolint:testpackage // required

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
)

type userRepoStub struct {
	list        []authcore.Profile
	total       int
	current     authcore.Profile
	updated     authcore.Profile
	updateCalls int
}

func (r *userRepoStub) GetList(context.Context, datagrid.Filtered) ([]authcore.Profile, int, error) {
	return r.list, r.total, nil
}

func (r *userRepoStub) GetByID(context.Context, int64) (authcore.Profile, error) {
	return r.current, nil
}

func (r *userRepoStub) Update(_ context.Context, _ int64, user authcore.Profile) error {
	r.updated = user
	r.updateCalls++
	return nil
}

func (*userRepoStub) SoftDeleteByID(context.Context, int64) error { return nil }

func (*userRepoStub) RestoreByID(context.Context, int64) error { return nil }

type userSubjectStoreStub struct {
	subjectID        authcore.SubjectID
	email            string
	profileSubjectID authcore.SubjectID
	profile          goauth.BasicProfile
	profileCalls     int
}

func (s *userSubjectStoreStub) RequestEmailChange(_ context.Context, subjectID authcore.SubjectID, email string) error {
	s.subjectID = subjectID
	s.email = email
	return nil
}

func (s *userSubjectStoreStub) UpdateBasicProfile(
	_ context.Context,
	subjectID authcore.SubjectID,
	profile goauth.BasicProfile,
) (goauth.Account, error) {
	s.profileSubjectID = subjectID
	s.profile = profile
	s.profileCalls++
	return goauth.Account{}, nil
}

func (*userSubjectStoreStub) SetSubjectStatus(
	context.Context,
	goauth.SubjectID,
	goauth.SubjectStatus,
) (goauth.Subject, error) {
	return goauth.Subject{}, nil
}

func TestUpdateUserEmailUsesCanonicalSubjectStoreOnly(t *testing.T) {
	t.Parallel()

	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.ExpertGuard(authcore.PermissionDomainUsers, authcore.PermissionActionRead).AnyTimes()
	adminAppTest.ExpertGuard(authcore.PermissionDomainUsers, authcore.PermissionActionUpdate).AnyTimes()
	adminAppTest.ExpertGuard(authcore.PermissionDomainUsers, authcore.PermissionActionDelete).AnyTimes()

	username := "user"
	lastName := "Tester"
	expectedSubjectID := authcore.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174131")
	repo := &userRepoStub{
		current: authcore.Profile{
			ID:        42,
			SubjectID: expectedSubjectID,
			PublicID:  identity.NewUserID(),
			Email:     "old@example.com",
			Name:      "Old",
			Username:  &username,
			LastName:  &lastName,
		},
	}
	subjects := &userSubjectStoreStub{}

	app := fiber.New()
	handler := NewHandler(adminAppTest.App, repo, subjects)
	handler.RegisterGroupRoutes(app)

	req := httptest.NewRequestWithContext(context.Background(),
		http.MethodPut,
		"/users/42",
		strings.NewReader(`{"name":"New","lastName":"Smith","username":"new-user","email":"new@example.com"}`))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "/users/42/edit")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, fiber.StatusFound, resp.StatusCode)

	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, "New", repo.updated.Name)
	require.Equal(t, 1, subjects.profileCalls)
	require.Equal(t, expectedSubjectID, subjects.profileSubjectID)
	require.Equal(t, "New", subjects.profile.GivenName)
	require.Equal(t, "new-user", subjects.profile.Username)
	require.Equal(t, "Smith", subjects.profile.FamilyName)
	require.Equal(t, expectedSubjectID, subjects.subjectID)
	require.Equal(t, "new@example.com", subjects.email)
}

func TestHandleData_ProvidesReadableStatusAndProfileValues(t *testing.T) {
	t.Parallel()

	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.ExpertGuard(authcore.PermissionDomainUsers, authcore.PermissionActionRead).AnyTimes()
	adminAppTest.ExpertGuard(authcore.PermissionDomainUsers, authcore.PermissionActionUpdate).AnyTimes()
	adminAppTest.ExpertGuard(authcore.PermissionDomainUsers, authcore.PermissionActionDelete).AnyTimes()

	repo := &userRepoStub{
		list: []authcore.Profile{
			{
				ID:               42,
				PublicID:         identity.NewUserID(),
				Email:            "user@example.com",
				ConfirmedEmailAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
			},
		},
		total: 1,
	}

	app := fiber.New()
	handler := NewHandler(adminAppTest.App, repo, &userSubjectStoreStub{})
	handler.RegisterGroupRoutes(app)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/users/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, fiber.StatusOK, resp.StatusCode)

	var payload struct {
		Data []struct {
			Values map[string]any `json:"values"`
		} `json:"data"`
		Config struct {
			Columns []struct {
				Key   string `json:"key"`
				Label string `json:"label"`
			} `json:"columns"`
		} `json:"config"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Len(t, payload.Data, 1)

	values := payload.Data[0].Values
	require.Equal(t, "active", values["status"])
	require.Equal(t, "confirmed", values["emailStatus"])
	require.Equal(t, "unconfirmed", values["phoneStatus"])
	require.Equal(t, "—", values["username"])
	require.Equal(t, "—", values["name"])
	require.Equal(t, "—", values["lastName"])
	require.Equal(t, "—", values["phone"])

	labels := make(map[string]string, len(payload.Config.Columns))
	for _, column := range payload.Config.Columns {
		labels[column.Key] = column.Label
	}
	require.Equal(t, "Email статус", labels["emailStatus"])
	require.Equal(t, "Телефон статус", labels["phoneStatus"])
}
