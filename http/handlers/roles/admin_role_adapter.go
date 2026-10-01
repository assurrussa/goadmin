package roles

import (
	"context"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

type assignSubjectRolesUseCase interface {
	Handle(ctx context.Context, req integrationroles.AssignSubjectRolesRequest) (integrationroles.AssignSubjectRolesResponse, error)
}

type listSubjectRolesUseCase interface {
	Handle(ctx context.Context, req integrationroles.ListSubjectRolesRequest) (integrationroles.ListSubjectRolesResponse, error)
}

type assignAdminRolesAdapter struct {
	admins  adminRepository
	subject assignSubjectRolesUseCase
}

func newAssignAdminRolesAdapter(
	admins adminRepository,
	subject assignSubjectRolesUseCase,
) *assignAdminRolesAdapter {
	return &assignAdminRolesAdapter{admins: admins, subject: subject}
}

func (a *assignAdminRolesAdapter) Handle(
	ctx context.Context,
	req AssignAdminRolesRequest,
) (AssignAdminRolesResponse, error) {
	subjectID, err := resolveAdminSubjectID(ctx, a.admins, req.AdminID)
	if err != nil {
		return AssignAdminRolesResponse{}, err
	}

	resp, err := a.subject.Handle(ctx, integrationroles.AssignSubjectRolesRequest{
		SubjectID: subjectID,
		RoleIDs:   req.RoleIDs,
	})
	if err != nil {
		return AssignAdminRolesResponse{}, err
	}

	return AssignAdminRolesResponse{Assigned: resp.Assigned}, nil
}

type listAdminRolesAdapter struct {
	admins  adminRepository
	subject listSubjectRolesUseCase
}

func newListAdminRolesAdapter(
	admins adminRepository,
	subject listSubjectRolesUseCase,
) *listAdminRolesAdapter {
	return &listAdminRolesAdapter{admins: admins, subject: subject}
}

func (a *listAdminRolesAdapter) Handle(
	ctx context.Context,
	req ListAdminRolesRequest,
) (ListAdminRolesResponse, error) {
	subjectID, err := resolveAdminSubjectID(ctx, a.admins, req.AdminID)
	if err != nil {
		return ListAdminRolesResponse{}, err
	}

	resp, err := a.subject.Handle(ctx, integrationroles.ListSubjectRolesRequest{SubjectID: subjectID})
	if err != nil {
		return ListAdminRolesResponse{}, err
	}

	return ListAdminRolesResponse{Roles: resp.Roles}, nil
}

func resolveAdminSubjectID(ctx context.Context, admins adminRepository, adminID int64) (string, error) {
	if adminID <= 0 {
		return "", errInvalidAdminID
	}

	admin, err := admins.GetByID(ctx, adminID)
	if err != nil {
		return "", err
	}
	if admin.ID <= 0 {
		admin.ID = adminID
	}

	subjectID := fallbackAdminSubjectID(admin)
	if subjectID == "" {
		return "", errInvalidAdminID
	}

	return subjectID, nil
}

func fallbackAdminSubjectID(admin models.Admin) string {
	subjectID := admin.AuthSubjectID()
	if subjectID.IsZero() {
		return ""
	}

	return subjectID.String()
}
