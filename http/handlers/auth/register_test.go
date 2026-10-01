package auth //nolint:testpackage // verifies the unexported first-admin transaction ordering

import (
	"context"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	authmocks "github.com/assurrussa/goadmin/http/handlers/auth/mocks"
	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

func TestCreateFirstAdminAssignsCanonicalRoleBeforeHostMembership(t *testing.T) {
	t.Parallel()

	app := adminappt.NewAppTest(t)
	steps := make([]string, 0, 8)
	account := goauth.Account{
		Subject: goauth.Subject{ID: goauth.NewSubjectID(), Status: goauth.SubjectStatusActive},
	}
	adminRepo := &recordingAdminRepo{steps: &steps}
	roles := &recordingRolesManager{steps: &steps}
	setup := authmocks.NewMockfirstAdminSetupTokens(app.Ctrl)
	sessions := authmocks.NewMocksessionStoreActions(app.Ctrl)
	setup.EXPECT().Consume(gomock.Any(), "setup-token").DoAndReturn(func(context.Context, string) error {
		steps = append(steps, "setup.consume")
		return nil
	})
	sessions.EXPECT().ProvisionTrustedAdmin(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, goauth.RegisterRequest) (goauth.Account, error) {
			steps = append(steps, "auth.provision")
			return account, nil
		})

	handler := NewHandler(HandlerOptions{
		AdminApp:            app.App,
		AdminRepo:           adminRepo,
		RolesManager:        roles,
		SessionStoreActions: sessions,
		FirstAdminSetup:     setup,
		ActionAudit:         recordingSetupAudit{steps: &steps},
	})
	err := handler.createFirstAdmin(context.Background(), FormRegister{
		Name: "First", LastName: "Admin", Username: "first-admin",
		Email: "first-admin@example.test", Password: "irrelevant", SetupToken: "setup-token",
	})

	require.NoError(t, err)
	require.Equal(t, []string{
		"admin.available",
		"setup.consume",
		"auth.provision",
		"permissions.ensure",
		"roles.list",
		"roles.create",
		"roles.assign",
		"admin.provision",
		"admin.audit",
	}, steps)
	require.Equal(t, account.Subject.ID.String(), roles.assignedSubject)
	require.Equal(t, []int64{7}, roles.assignedRoleIDs)
}

type recordingAdminRepo struct {
	steps *[]string
}

func (r *recordingAdminRepo) ProvisionAccount(
	_ context.Context,
	account goauth.Account,
	_ identity.UserID,
) (models.Admin, error) {
	*r.steps = append(*r.steps, "admin.provision")
	return models.Admin{ID: 1, SubjectID: account.Subject.ID}, nil
}

func (*recordingAdminRepo) GetByID(context.Context, int64) (models.Admin, error) {
	return models.Admin{}, nil
}

func (*recordingAdminRepo) GetByUUID(context.Context, identity.UserID) (models.Admin, error) {
	return models.Admin{}, nil
}

func (*recordingAdminRepo) UpdateAdmin(context.Context, int64, models.Admin) error {
	return nil
}

func (r *recordingAdminRepo) GetList(context.Context, datagrid.Filtered) ([]models.Admin, int, error) {
	*r.steps = append(*r.steps, "admin.available")
	return nil, 0, nil
}

type recordingRolesManager struct {
	steps           *[]string
	assignedSubject string
	assignedRoleIDs []int64
}

func (r *recordingRolesManager) ListRoles(
	context.Context,
	integrationroles.RoleFilter,
) ([]integrationroles.Role, error) {
	*r.steps = append(*r.steps, "roles.list")
	return nil, nil
}

func (r *recordingRolesManager) EnsurePermissions(
	context.Context,
	[]integrationroles.CreatePermissionInput,
) error {
	*r.steps = append(*r.steps, "permissions.ensure")
	return nil
}

func (*recordingRolesManager) CreatePermissions(
	context.Context,
	[]integrationroles.CreatePermissionInput,
) ([]integrationroles.Permission, error) {
	return nil, nil
}

func (r *recordingRolesManager) CreateRole(
	context.Context,
	integrationroles.CreateRoleInput,
) (*integrationroles.Role, error) {
	*r.steps = append(*r.steps, "roles.create")
	return &integrationroles.Role{ID: 7}, nil
}

func (*recordingRolesManager) UpdateRole(
	context.Context,
	integrationroles.UpdateRoleInput,
) (*integrationroles.Role, error) {
	return &integrationroles.Role{ID: 7}, nil
}

func (r *recordingRolesManager) AssignRolesToSubject(
	_ context.Context,
	subjectID string,
	roleIDs []int64,
) error {
	*r.steps = append(*r.steps, "roles.assign")
	r.assignedSubject = subjectID
	r.assignedRoleIDs = append([]int64(nil), roleIDs...)
	return nil
}

type recordingSetupAudit struct{ steps *[]string }

func (a recordingSetupAudit) RecordAdminAction(_ context.Context, _ adminactionauditrepo.Record) error {
	*a.steps = append(*a.steps, "admin.audit")
	return nil
}
