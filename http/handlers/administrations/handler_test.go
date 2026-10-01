package administrations //nolint:testpackage // required

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	"github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

type adminRepoStub struct {
	list        []models.Admin
	total       int
	current     models.Admin
	provisioned []goauth.Account
	updated     models.Admin
	updateCalls int
	deletedIDs  []int64
}

type adminActionAuditStub struct {
	records []adminactionauditrepo.Record
	err     error
}

func (s *adminActionAuditStub) RecordAdminAction(_ context.Context, record adminactionauditrepo.Record) error {
	if s.err != nil {
		return s.err
	}
	s.records = append(s.records, record)
	return nil
}

type batchAdminRepoStub struct {
	*adminRepoStub
	roleNames map[int64][]string
	roleErr   error
	roleCalls int
	roleIDs   []int64
}

func (r *batchAdminRepoStub) ListRoleNamesByAdminIDs(
	_ context.Context,
	adminIDs []int64,
) (map[int64][]string, error) {
	r.roleCalls++
	r.roleIDs = append([]int64(nil), adminIDs...)
	return r.roleNames, r.roleErr
}

func (r *adminRepoStub) GetList(context.Context, datagrid.Filtered) ([]models.Admin, int, error) {
	return r.list, r.total, nil
}

func (r *adminRepoStub) GetByID(context.Context, int64) (models.Admin, error) {
	return r.current, nil
}

func (r *adminRepoStub) GetByUUID(_ context.Context, _ identity.UserID) (models.Admin, error) {
	if r.current.UUID.IsZero() {
		return models.Admin{}, nil
	}
	return r.current, nil
}

func (r *adminRepoStub) UpdateAdmin(_ context.Context, _ int64, admin models.Admin) error {
	r.updated = admin
	r.updateCalls++
	r.current = admin
	return nil
}

func (*adminRepoStub) DeleteByID(context.Context, int64) error { return nil }
func (r *adminRepoStub) SoftDeleteByID(_ context.Context, id int64) error {
	if r.current.ID == 0 {
		return pgx.ErrNoRows
	}
	r.deletedIDs = append(r.deletedIDs, id)
	return nil
}

func TestMissingAdminRoutesReturnNotFound(t *testing.T) {
	t.Parallel()
	appTest := adminappt.NewAppTest(t)
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
		return c.Next()
	})
	handler := NewHandler(
		appTest.App, &adminRepoStub{}, &adminAuthRuntimeStub{}, noopFileRepo{}, noopPreviewService{}, &adminActionAuditStub{},
	)
	handler.RegisterGroupRoutes(app)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/admins/23"},
		{http.MethodGet, "/admins/23/edit"},
		{http.MethodDelete, "/admins/23"},
	} {
		resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s %s", tc.method, tc.path)
		require.NoError(t, resp.Body.Close())
	}
}

func TestDeleteProtectsSuperAdmin(t *testing.T) {
	t.Parallel()
	appTest := adminappt.NewAppTest(t)
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()
	appTest.MockRoleService.EXPECT().GetRolesAdmin(gomock.Any(), int64(23)).
		Return([]integrationroles.Role{{Slug: integrationroles.SuperAdminRole}}, nil)
	repo := &adminRepoStub{current: models.Admin{ID: 23}}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
		return c.Next()
	})
	NewHandler(appTest.App, repo, &adminAuthRuntimeStub{}, noopFileRepo{}, noopPreviewService{}, &adminActionAuditStub{}).
		RegisterGroupRoutes(app)

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/admins/23", nil))
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.Empty(t, repo.deletedIDs)
}

func TestDeleteFailsClosedWhenRoleLookupFails(t *testing.T) {
	t.Parallel()
	appTest := adminappt.NewAppTest(t)
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()
	appTest.MockRoleService.EXPECT().GetRolesAdmin(gomock.Any(), int64(23)).Return(nil, errors.New("role store unavailable"))
	repo := &adminRepoStub{current: models.Admin{ID: 23}}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
		return c.Next()
	})
	NewHandler(appTest.App, repo, &adminAuthRuntimeStub{}, noopFileRepo{}, noopPreviewService{}, &adminActionAuditStub{}).
		RegisterGroupRoutes(app)
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/admins/23", nil))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Empty(t, repo.deletedIDs)
}

func (r *adminRepoStub) ProvisionAccount(
	_ context.Context,
	account goauth.Account,
	publicID identity.UserID,
) (models.Admin, error) {
	r.provisioned = append(r.provisioned, account)
	return models.Admin{
		ID: 77, SubjectID: account.Subject.ID, UUID: publicID,
		Email: account.PrimaryEmail.DisplayValue, Name: account.Profile.GivenName,
		Username: account.Profile.Username,
	}, nil
}

type adminAuthRuntimeStub struct {
	provisioned      []goauth.RegisterRequest
	subjID           integrationroles.SubjectID
	email            string
	profileSubjectID integrationroles.SubjectID
	profile          goauth.BasicProfile
	profileCalls     int
	statusCalls      int
}

func (s *adminAuthRuntimeStub) ProvisionTrustedAdmin(
	_ context.Context,
	request goauth.RegisterRequest,
) (goauth.Account, error) {
	s.provisioned = append(s.provisioned, request)
	verifiedAt := time.Now().UTC()

	return goauth.Account{
		Subject: goauth.Subject{ID: goauth.NewSubjectID(), Status: goauth.SubjectStatusActive},
		PrimaryEmail: goauth.Identifier{
			Scheme: goauth.IdentifierSchemeEmail, DisplayValue: request.Email,
			NormalizedValue: strings.ToLower(request.Email), VerifiedAt: &verifiedAt,
		},
		Profile: request.Profile,
	}, nil
}

func (s *adminAuthRuntimeStub) RequestEmailChange(
	_ context.Context,
	subjectID integrationroles.SubjectID,
	email string,
) error {
	s.subjID = subjectID
	s.email = email
	return nil
}

func (s *adminAuthRuntimeStub) UpdateBasicProfile(
	_ context.Context,
	subjectID integrationroles.SubjectID,
	profile goauth.BasicProfile,
) (goauth.Account, error) {
	s.profileSubjectID = subjectID
	s.profile = profile
	s.profileCalls++
	return goauth.Account{}, nil
}

func (s *adminAuthRuntimeStub) SetSubjectStatus(
	context.Context,
	goauth.SubjectID,
	goauth.SubjectStatus,
) (goauth.Subject, error) {
	s.statusCalls++
	return goauth.Subject{}, nil
}

func TestDeleteAdminOnlyRevokesAdminMembership(t *testing.T) {
	t.Parallel()
	appTest := adminappt.NewAppTest(t)
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()
	appTest.MockRoleService.EXPECT().GetRolesAdmin(gomock.Any(), int64(23)).Return(nil, nil)
	appTest.MockTransaction.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
	)
	repo := &adminRepoStub{current: models.Admin{ID: 23, SubjectID: goauth.NewSubjectID()}}
	audit := &adminActionAuditStub{}
	authRuntime := &adminAuthRuntimeStub{}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
		return c.Next()
	})
	NewHandler(appTest.App, repo, authRuntime, noopFileRepo{}, noopPreviewService{}, audit).RegisterGroupRoutes(app)

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/admins/23", nil))
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, fiber.StatusFound, resp.StatusCode)
	require.Equal(t, []int64{23}, repo.deletedIDs)
	require.Len(t, audit.records, 1)
	require.Equal(t, adminactionauditrepo.ActionAdminSoftDeleted, audit.records[0].Action)
	require.Equal(t, "23", audit.records[0].TargetID)
	require.Zero(t, authRuntime.statusCalls)
}

func TestDeleteAdminAuditFailureAbortsTransaction(t *testing.T) {
	t.Parallel()
	appTest := adminappt.NewAppTest(t)
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()
	appTest.MockRoleService.EXPECT().GetRolesAdmin(gomock.Any(), int64(23)).Return(nil, nil)
	auditErr := errors.New("audit insert failed")
	audit := &adminActionAuditStub{err: auditErr}
	var txError error
	appTest.MockTransaction.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			txError = fn(ctx)
			return txError
		},
	)
	repo := &adminRepoStub{current: models.Admin{ID: 23, SubjectID: goauth.NewSubjectID()}}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
		return c.Next()
	})
	NewHandler(appTest.App, repo, &adminAuthRuntimeStub{}, noopFileRepo{}, noopPreviewService{}, audit).RegisterGroupRoutes(app)

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/admins/23", nil))
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, fiber.StatusFound, resp.StatusCode)
	require.ErrorIs(t, txError, auditErr)
	require.Empty(t, audit.records)
}

type noopFileRepo struct{}

func (noopFileRepo) GetByID(context.Context, int64) (uploadhost.File, error) {
	return uploadhost.File{}, nil
}

func (noopFileRepo) Update(context.Context, int64, uploadhost.File) error { return nil }

type previewFileRepo struct{ file uploadhost.File }

func (r previewFileRepo) GetByID(context.Context, int64) (uploadhost.File, error) {
	return r.file, nil
}

func (previewFileRepo) Update(context.Context, int64, uploadhost.File) error { return nil }

func TestPreviewAssignmentRejectsForeignOrUnsafeFile(t *testing.T) {
	t.Parallel()
	actorID, otherID := int64(1), int64(2)
	fileID := int64(17)
	base := uploadhost.File{
		ID: fileID, ManagerID: &actorID, ObjectType: uploadhost.ObjectTypeAdmin, MimeType: "image/png",
		FolderPath: "media/v1/images", FileName: "preview.png",
		Data: &uploadhost.FileData{Presets: map[uploadhost.PresetName]uploadhost.FilePreset{
			"main": {RelativePath: "media/v1/images/preview.png"},
		}},
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*uploadhost.File)
		adminID int64
		valid   bool
	}{
		{name: "own unattached upload", valid: true},
		{name: "foreign uploader", mutate: func(f *uploadhost.File) { f.ManagerID = &otherID }},
		{name: "foreign attached admin", adminID: 3, mutate: func(f *uploadhost.File) {
			objectID := uploadhost.ObjectID(2)
			f.ObjectID = &objectID
		}},
		{name: "deleted file", mutate: func(f *uploadhost.File) { f.DeletedAt = sql.NullTime{Valid: true} }},
		{name: "active content", mutate: func(f *uploadhost.File) { f.MimeType = "image/svg+xml" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file := base
			if tc.mutate != nil {
				tc.mutate(&file)
			}
			sessionAdmin := &models.SessionAdmin{ID: actorID}
			//nolint:staticcheck // GetAdminAuth requires the legacy string key.
			ctx := context.WithValue(context.Background(), session.AuthAdminKey.String(), sessionAdmin)
			handler := &Handler{fileRepo: previewFileRepo{file: file}}
			got, validation, err := handler.fetchPreviewFile(ctx, &fileID, tc.adminID)
			require.NoError(t, err)
			if tc.valid {
				require.NotNil(t, got)
				require.Empty(t, validation)
			} else {
				require.Nil(t, got)
				require.NotEmpty(t, validation["previewId"])
			}
		})
	}
}

type noopPreviewService struct{}

func (noopPreviewService) LoadPreview(context.Context, int64) (*uploadhost.File, error) {
	return nil, nil //nolint:nilnil // required
}

func TestStoreUsesCanonicalAdminWriter(t *testing.T) {
	t.Parallel()

	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()
	adminAppTest.MockTransaction.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)

	repo := &adminRepoStub{}
	authRuntime := &adminAuthRuntimeStub{}

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
		return c.Next()
	})
	handler := NewHandler(adminAppTest.App, repo, authRuntime, noopFileRepo{}, noopPreviewService{}, &adminActionAuditStub{})
	handler.RegisterGroupRoutes(app)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admins/create", strings.NewReader(`{"username":"admin","name":"Admin","lastName":"Root","email":"admin@example.com","password":"password","passwordConfirm":"password"}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, fiber.StatusFound, resp.StatusCode)
	require.Len(t, authRuntime.provisioned, 1)
	require.Len(t, repo.provisioned, 1)
	require.Equal(t, "admin@example.com", authRuntime.provisioned[0].Email)
}

func TestHandleDataFailsWhenRoleLookupFails(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"batch", "fallback"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			appTest := adminappt.NewAppTest(t)
			appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
			appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
			appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
			appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()
			baseRepo := &adminRepoStub{
				list:  []models.Admin{{ID: 7, Data: &models.AdminData{Roles: []string{"cached"}}}},
				total: 1,
			}
			var repo adminRepo = baseRepo
			if name == "batch" {
				repo = &batchAdminRepoStub{adminRepoStub: baseRepo, roleErr: errors.New("role store unavailable")}
			} else {
				appTest.MockRoleService.EXPECT().GetRolesAdmin(gomock.Any(), int64(7)).
					Return(nil, errors.New("role store unavailable"))
			}
			app := fiber.New()
			NewHandler(appTest.App, repo, &adminAuthRuntimeStub{}, noopFileRepo{}, noopPreviewService{}, &adminActionAuditStub{}).
				RegisterGroupRoutes(app)

			resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admins/data", nil))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
			require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		})
	}
}

func TestHandleDataLoadsPageRoleNamesInOneCall(t *testing.T) {
	t.Parallel()
	appTest := adminappt.NewAppTest(t)
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
	appTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()
	repo := &batchAdminRepoStub{
		adminRepoStub: &adminRepoStub{
			list: []models.Admin{
				{ID: 7, Data: &models.AdminData{Roles: []string{"stale"}}},
				{ID: 9, Data: &models.AdminData{Roles: []string{"stale"}}},
			},
			total: 2,
		},
		roleNames: map[int64][]string{7: {"Super", "Editor"}},
	}
	app := fiber.New()
	NewHandler(appTest.App, repo, &adminAuthRuntimeStub{}, noopFileRepo{}, noopPreviewService{}, &adminActionAuditStub{}).
		RegisterGroupRoutes(app)

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admins/data", nil))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var payload struct {
		Data []struct {
			Values map[string]any `json:"values"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Len(t, payload.Data, 2)
	require.Equal(t, "Super, Editor", payload.Data[0].Values["rolesSummary"])
	require.Equal(t, "—", payload.Data[1].Values["rolesSummary"])
	require.Equal(t, []int64{7, 9}, repo.roleIDs)
	require.Equal(t, 1, repo.roleCalls)
}

func TestHandleData_ProvidesRolesAndAuthMetadataValues(t *testing.T) {
	t.Parallel()

	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()

	lastLoginAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	emailConfirmedAt := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	repo := &adminRepoStub{
		list: []models.Admin{
			{
				ID:       9,
				UUID:     identity.NewUserID(),
				Name:     "Admin",
				LastName: "User",
				Username: "root",
				Email:    "admin@example.com",
				Data: &models.AdminData{
					LastLoginAt: &lastLoginAt,
				},
				ConfirmedEmailAt: &emailConfirmedAt,
			},
		},
		total: 1,
	}

	adminAppTest.MockRoleService.EXPECT().GetRolesAdmin(gomock.Any(), int64(9)).Return([]integrationroles.Role{
		{Name: "Супер-администратор"},
		{Name: "Контент"},
	}, nil)

	app := fiber.New()
	handler := NewHandler(
		adminAppTest.App, repo, &adminAuthRuntimeStub{}, noopFileRepo{}, noopPreviewService{}, &adminActionAuditStub{},
	)
	handler.RegisterGroupRoutes(app)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admins/data", nil)
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
	require.Equal(t, "Супер-администратор, Контент", values["rolesSummary"])
	require.NotEmpty(t, values["lastLoginAt"])
	require.NotEmpty(t, values["emailConfirmedAt"])

	labels := make(map[string]string, len(payload.Config.Columns))
	for _, column := range payload.Config.Columns {
		labels[column.Key] = column.Label
	}
	require.Equal(t, "Роли", labels["rolesSummary"])
	require.Equal(t, "Email статус", labels["emailStatus"])
}

func TestUpdateAdminUsesCanonicalProfileWriterOnly(t *testing.T) {
	t.Parallel()

	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead).AnyTimes()
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate).AnyTimes()
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate).AnyTimes()
	adminAppTest.ExpertGuard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete).AnyTimes()
	adminAppTest.MockTransaction.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)

	adminUUID := identity.NewUserID()
	expectedSubjectID := integrationroles.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174130")
	repo := &adminRepoStub{
		current: models.Admin{
			ID:        42,
			SubjectID: expectedSubjectID,
			UUID:      adminUUID,
			Email:     "old@example.com",
			Name:      "Old",
			LastName:  "Admin",
			Username:  "old-admin",
		},
	}
	authRuntime := &adminAuthRuntimeStub{}

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
		return c.Next()
	})
	handler := NewHandler(adminAppTest.App, repo, authRuntime, noopFileRepo{}, noopPreviewService{}, &adminActionAuditStub{})
	handler.RegisterGroupRoutes(app)

	req := httptest.NewRequestWithContext(context.Background(),
		http.MethodPut,
		"/admins/42",
		strings.NewReader(`{"username":"new-admin","name":"New","lastName":"Admin","email":"new@example.com"}`))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "/admins/42/edit")

	resp, err := app.Test(req) //nolint:bodyclose // required
	require.NoError(t, err)
	require.Equal(t, fiber.StatusFound, resp.StatusCode)

	require.Equal(t, 1, authRuntime.profileCalls)
	require.Equal(t, expectedSubjectID, authRuntime.profileSubjectID)
	require.Equal(t, "New", authRuntime.profile.GivenName)
	require.Equal(t, "Admin", authRuntime.profile.FamilyName)
	require.Equal(t, "new-admin", authRuntime.profile.Username)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, "New", repo.updated.Name)
	require.Equal(t, "new-admin", repo.updated.Username)
	require.Equal(t, expectedSubjectID, authRuntime.subjID)
	require.Equal(t, "new@example.com", authRuntime.email)
}
