package adminroles //nolint:testpackage // required

import (
	"context"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/stretchr/testify/require"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

func TestServiceGetRolesAdminResolvesStoredSubjectID(t *testing.T) {
	t.Parallel()

	adminID := int64(7)
	expectedSubjectID := integrationroles.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174110")
	rolesUseCase := &rolesUseCaseRecorder{
		roles: []integrationroles.Role{{ID: 11, Slug: "manager"}}, //nolint:goconst // required
	}
	svc := newTestService(t, rolesUseCase, adminRepoStub{
		byID: map[int64]models.Admin{
			adminID: {ID: adminID, SubjectID: expectedSubjectID, UUID: identity.NewUserID()},
		},
	})

	roles, err := svc.GetRolesAdmin(context.Background(), adminID)
	require.NoError(t, err)
	require.Equal(t, []integrationroles.Role{{ID: 11, Slug: "manager"}}, roles)
	require.Equal(t, expectedSubjectID.String(), rolesUseCase.subjectID)
}

func TestServiceGetRolesAdminRejectsUUIDOnlyAdmin(t *testing.T) {
	t.Parallel()

	adminUUID := identity.NewUserID()
	rolesUseCase := &rolesUseCaseRecorder{}
	svc := newTestService(t, rolesUseCase, adminRepoStub{
		byID: map[int64]models.Admin{
			9: {ID: 9, UUID: adminUUID},
		},
	})

	_, err := svc.GetRolesAdmin(context.Background(), 9)
	require.ErrorIs(t, err, ErrAdminRoleNotFound)
	require.Empty(t, rolesUseCase.subjectID)
}

func TestServiceGetRolesAdminRejectsAdminWithoutSubjectID(t *testing.T) {
	t.Parallel()

	rolesUseCase := &rolesUseCaseRecorder{}
	svc := newTestService(t, rolesUseCase, adminRepoStub{
		byID: map[int64]models.Admin{
			13: {ID: 13},
		},
	})

	_, err := svc.GetRolesAdmin(context.Background(), 13)
	require.ErrorIs(t, err, ErrAdminRoleNotFound)
	require.Empty(t, rolesUseCase.subjectID)
}

func TestServiceAdminGuardCheckDeniesUserMembershipWithoutAdminProjection(t *testing.T) {
	t.Parallel()

	rolesUseCase := &rolesUseCaseRecorder{
		roles: []integrationroles.Role{{ID: 11, Slug: "manager"}},
	}
	permissions := &permissionUseCaseStub{allowed: true}
	svc := newTestServiceWithPermission(t, rolesUseCase, adminRepoStub{
		byID: map[int64]models.Admin{},
	}, permissions)

	ok := svc.AdminGuardCheck(
		context.Background(),
		55,
		integrationroles.NewPermissionKey(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead),
	)

	require.False(t, ok)
	require.Empty(t, rolesUseCase.subjectID)
	require.Zero(t, permissions.hasPermissionCalls)
}

func TestServiceAdminGuardCheckDeniesAdminProjectionWithoutPermission(t *testing.T) {
	t.Parallel()

	subjectID := integrationroles.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174111")
	subjectIDString := subjectID.String()
	rolesUseCase := &rolesUseCaseRecorder{
		roles: []integrationroles.Role{{ID: 11, Slug: "manager"}},
	}
	permissions := &permissionUseCaseStub{allowed: false}
	svc := newTestServiceWithPermission(t, rolesUseCase, adminRepoStub{
		byID: map[int64]models.Admin{
			7: {ID: 7, SubjectID: subjectID, UUID: identity.NewUserID()},
		},
	}, permissions)

	key := integrationroles.NewPermissionKey(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead)
	ok := svc.AdminGuardCheck(context.Background(), 7, key)

	require.False(t, ok)
	require.Equal(t, subjectIDString, rolesUseCase.subjectID)
	require.Equal(t, subjectIDString, permissions.subjectID)
	require.Equal(t, key, permissions.key)
	require.Equal(t, 1, permissions.hasPermissionCalls)
}

func TestServiceAdminGuardCheckAllowsAdminProjectionWithPermission(t *testing.T) {
	t.Parallel()

	subjectID := integrationroles.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174112")
	subjectIDString := subjectID.String()
	rolesUseCase := &rolesUseCaseRecorder{
		roles: []integrationroles.Role{{ID: 11, Slug: "manager"}},
	}
	permissions := &permissionUseCaseStub{allowed: true}
	svc := newTestServiceWithPermission(t, rolesUseCase, adminRepoStub{
		byID: map[int64]models.Admin{
			7: {ID: 7, SubjectID: subjectID, UUID: identity.NewUserID()},
		},
	}, permissions)

	key := integrationroles.NewPermissionKey(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead)
	ok := svc.AdminGuardCheck(context.Background(), 7, key)

	require.True(t, ok)
	require.Equal(t, subjectIDString, rolesUseCase.subjectID)
	require.Equal(t, subjectIDString, permissions.subjectID)
	require.Equal(t, key, permissions.key)
	require.Equal(t, 1, permissions.hasPermissionCalls)
}

func newTestService(t *testing.T, rolesUseCase *rolesUseCaseRecorder, admins adminRepoStub) *Service {
	t.Helper()

	return newTestServiceWithPermission(t, rolesUseCase, admins, &permissionUseCaseStub{})
}

func newTestServiceWithPermission(
	t *testing.T,
	rolesUseCase *rolesUseCaseRecorder,
	admins adminRepoStub,
	permissions *permissionUseCaseStub,
) *Service {
	t.Helper()

	if permissions == nil {
		permissions = &permissionUseCaseStub{}
	}

	subjectGuard, err := integrationroles.NewGuardServiceWithOptions(integrationroles.NewGuardServiceOptions(
		rolesUseCase,
		permissions,
		disabledCacheStub{},
		logger.Discard(),
	))
	require.NoError(t, err)

	svc, err := New(subjectGuard, admins)
	require.NoError(t, err)

	return svc
}

type adminRepoStub struct {
	byID map[int64]models.Admin
	err  error
}

func (r adminRepoStub) GetByID(_ context.Context, id int64) (models.Admin, error) {
	if r.err != nil {
		return models.Admin{}, r.err
	}

	return r.byID[id], nil
}

type rolesUseCaseRecorder struct {
	subjectID string
	roles     []integrationroles.Role
	err       error
}

func (r *rolesUseCaseRecorder) Handle(
	_ context.Context,
	req integrationroles.ListSubjectRolesRequest,
) (integrationroles.ListSubjectRolesResponse, error) {
	r.subjectID = req.SubjectID
	if r.err != nil {
		return integrationroles.ListSubjectRolesResponse{}, r.err
	}

	return integrationroles.ListSubjectRolesResponse{Roles: r.roles}, nil
}

type permissionUseCaseStub struct {
	allowed            bool
	subjectID          string
	key                integrationroles.PermissionKey
	hasPermissionCalls int
}

func (*permissionUseCaseStub) ListAllPermissionsByRoles(context.Context, []int64) ([]integrationroles.PermissionWithRole, error) {
	return nil, nil
}

func (s *permissionUseCaseStub) HasPermission(_ context.Context, subjectID string, key integrationroles.PermissionKey) (bool, error) { //nolint:lll // required
	s.subjectID = subjectID
	s.key = key
	s.hasPermissionCalls++

	return s.allowed, nil
}

type disabledCacheStub struct{}

func (disabledCacheStub) Enabled(context.Context) bool { return false }

func (disabledCacheStub) SubjectRoles(string) []integrationroles.Role { return nil }

func (disabledCacheStub) AllRolePermissions([]integrationroles.Role) map[int64]map[int64]integrationroles.Permission {
	return nil
}
