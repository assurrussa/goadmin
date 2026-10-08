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

type roleTransactionContextKey struct{}

type subjectRoleTransactionFunc func(context.Context, []string, func(context.Context) error) error

func (f subjectRoleTransactionFunc) InSubjectRoleTransaction(
	ctx context.Context, subjects []string, fn func(context.Context) error,
) error {
	return f(ctx, subjects, fn)
}

type changingAdminProjection struct {
	adapterAdminRepo
	changedID   int64
	replacement models.Admin
	err         error
}

func (r changingAdminProjection) GetByID(ctx context.Context, id int64) (models.Admin, error) {
	if ctx.Value(roleTransactionContextKey{}) != nil && id == r.changedID {
		return r.replacement, r.err
	}
	return r.adapterAdminRepo.GetByID(ctx, id)
}

func TestAdminRoleTransactionRechecksCanonicalSubjectMapping(t *testing.T) {
	actor := integrationroles.MustParseSubjectIDString("00000000-0000-4000-8000-000000000011")
	target := integrationroles.MustParseSubjectIDString("00000000-0000-4000-8000-000000000012")
	foreign := integrationroles.MustParseSubjectIDString("00000000-0000-4000-8000-000000000013")
	repository := adapterAdminRepo{byID: map[int64]models.Admin{1: {ID: 1, SubjectID: actor}, 2: {ID: 2, SubjectID: target}}}
	failure := errors.New("transaction failed")
	for _, tc := range []struct {
		name          string
		changed       int64
		replacement   models.Admin
		repositoryErr error
		expected      error
	}{
		{name: "original mapping", expected: failure},
		{name: "target remapped", changed: 2, replacement: models.Admin{ID: 2, SubjectID: foreign}, expected: errInvalidAdminID},
		{name: "actor remapped", changed: 1, replacement: models.Admin{ID: 1, SubjectID: foreign}, expected: errInvalidAdminID},
		{name: "membership disappeared", changed: 2, repositoryErr: failure, expected: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			adapter := &adminRoleTransactionAdapter{
				admins: changingAdminProjection{
					adapterAdminRepo: repository, changedID: tc.changed, replacement: tc.replacement, err: tc.repositoryErr,
				},
				service: subjectRoleTransactionFunc(func(ctx context.Context, subjects []string, fn func(context.Context) error) error {
					require.Equal(t, []string{actor.String(), target.String()}, subjects)
					return fn(context.WithValue(ctx, roleTransactionContextKey{}, true))
				}),
			}
			err := adapter.InAdminRoleTransaction(t.Context(), 1, 2, func(ctx context.Context) error {
				require.Equal(t, true, ctx.Value(roleTransactionContextKey{}))
				called = true
				return failure
			})
			require.ErrorIs(t, err, tc.expected)
			require.Equal(t, tc.changed == 0, called)
		})
	}
}
