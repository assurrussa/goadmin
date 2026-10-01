package roles //nolint:testpackage // required

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

func TestAssignAdminRolesAdapterResolvesStoredSubjectID(t *testing.T) {
	t.Parallel()

	subjectID := integrationroles.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174113")
	assigner := &assignSubjectRolesRecorder{}
	adapter := newAssignAdminRolesAdapter(
		adapterAdminRepo{byID: map[int64]models.Admin{
			7: {ID: 7, SubjectID: subjectID},
		}},
		assigner,
	)

	resp, err := adapter.Handle(context.Background(), AssignAdminRolesRequest{
		AdminID: 7,
		RoleIDs: []int64{1, 2},
	})
	require.NoError(t, err)
	require.Equal(t, AssignAdminRolesResponse{Assigned: 2}, resp)
	require.Equal(t, subjectID.String(), assigner.subjectID)
	require.Equal(t, []int64{1, 2}, assigner.roleIDs)
}

func TestListAdminRolesAdapterResolvesStoredSubjectID(t *testing.T) {
	t.Parallel()

	subjectID := integrationroles.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174132")
	lister := &listSubjectRolesRecorder{
		roles: []integrationroles.Role{{ID: 3, Slug: "editor"}},
	}
	adapter := newListAdminRolesAdapter(
		adapterAdminRepo{byID: map[int64]models.Admin{
			8: {ID: 8, SubjectID: subjectID},
			9: {ID: 9},
		}},
		lister,
	)

	resp, err := adapter.Handle(context.Background(), ListAdminRolesRequest{AdminID: 8})
	require.NoError(t, err)
	require.Equal(t, []integrationroles.Role{{ID: 3, Slug: "editor"}}, resp.Roles)
	require.Equal(t, subjectID.String(), lister.subjectID)

	_, err = adapter.Handle(context.Background(), ListAdminRolesRequest{AdminID: 9})
	require.ErrorIs(t, err, errInvalidAdminID)
}

func TestAssignAdminRolesAdapterReturnsAdminRepositoryErrors(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("db unavailable")
	adapter := newAssignAdminRolesAdapter(
		adapterAdminRepo{err: expectedErr},
		&assignSubjectRolesRecorder{},
	)

	_, err := adapter.Handle(context.Background(), AssignAdminRolesRequest{AdminID: 7, RoleIDs: []int64{1}})
	require.ErrorIs(t, err, expectedErr)
}

type adapterAdminRepo struct {
	byID map[int64]models.Admin
	err  error
}

func (r adapterAdminRepo) GetByID(_ context.Context, id int64) (models.Admin, error) {
	if r.err != nil {
		return models.Admin{}, r.err
	}

	return r.byID[id], nil
}

func (r adapterAdminRepo) GetBySubjectID(_ context.Context, subjectID integrationroles.SubjectID) (models.Admin, error) {
	for _, admin := range r.byID {
		if admin.AuthSubjectID() == subjectID {
			return admin, nil
		}
	}

	return models.Admin{}, nil
}

func (r adapterAdminRepo) GetByUUID(_ context.Context, id identity.UserID) (models.Admin, error) {
	for _, admin := range r.byID {
		if admin.UUID == id {
			return admin, nil
		}
	}

	return models.Admin{}, nil
}

func (r adapterAdminRepo) GetList(context.Context, datagrid.Filtered) ([]models.Admin, int, error) {
	result := make([]models.Admin, 0, len(r.byID))
	for _, admin := range r.byID {
		result = append(result, admin)
	}

	return result, len(result), nil
}

type assignSubjectRolesRecorder struct {
	subjectID string
	roleIDs   []int64
}

func (r *assignSubjectRolesRecorder) Handle(
	_ context.Context,
	req integrationroles.AssignSubjectRolesRequest,
) (integrationroles.AssignSubjectRolesResponse, error) {
	r.subjectID = req.SubjectID
	r.roleIDs = append([]int64(nil), req.RoleIDs...)

	return integrationroles.AssignSubjectRolesResponse{Assigned: len(req.RoleIDs)}, nil
}

type listSubjectRolesRecorder struct {
	subjectID string
	roles     []integrationroles.Role
}

func (r *listSubjectRolesRecorder) Handle(
	_ context.Context,
	req integrationroles.ListSubjectRolesRequest,
) (integrationroles.ListSubjectRolesResponse, error) {
	r.subjectID = req.SubjectID

	return integrationroles.ListSubjectRolesResponse{Roles: r.roles}, nil
}
