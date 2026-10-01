package adminroles

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v3"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

var (
	ErrAdminRoleIsCurrentRole   = errors.New("admin role is current role")
	ErrAdminRoleIsSuperRole     = errors.New("admin role is super role")
	ErrAdminRoleNotFound        = errors.New("admin role not found")
	ErrAdminPermissionsNotFound = errors.New("admin permissions not found")
)

type AdminRepository interface {
	GetByID(ctx context.Context, id int64) (models.Admin, error)
}

type Service struct {
	subjectGuard *integrationroles.GuardService
	admins       AdminRepository
}

func New(subjectGuard *integrationroles.GuardService, admins AdminRepository) (*Service, error) {
	if subjectGuard == nil {
		return nil, errors.New("admin roles: subject guard is required")
	}
	if admins == nil {
		return nil, errors.New("admin roles: admin repository is required")
	}

	return &Service{subjectGuard: subjectGuard, admins: admins}, nil
}

func Must(subjectGuard *integrationroles.GuardService, admins AdminRepository) *Service {
	svc, err := New(subjectGuard, admins)
	if err != nil {
		panic(err)
	}
	return svc
}

func (s *Service) AdminGuard(key integrationroles.PermissionKey, opts ...integrationroles.PermissionGuardOption) fiber.Handler {
	if key.IsZero() {
		panic(errors.New("permission guard: zero permission key"))
	}

	return func(c fiber.Ctx) error {
		admin := adminmiddleware.GetAdminAuth(c)
		if admin == nil {
			return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
		}

		if !s.AdminGuardCheck(c, admin.ID, key, opts...) {
			return fiber.NewError(fiber.StatusForbidden, "permission denied")
		}

		return c.Next()
	}
}

func (s *Service) AdminGuardCheck(
	ctx context.Context,
	adminID int64,
	key integrationroles.PermissionKey,
	opts ...integrationroles.PermissionGuardOption,
) bool {
	subjectID := s.subjectIDForAdmin(ctx, adminID)
	if subjectID == "" {
		return false
	}

	return s.subjectGuard.SubjectGuardCheck(ctx, subjectID, key, opts...)
}

func (s *Service) IsSuperAdmin(ctx context.Context, adminID int64) bool {
	subjectID := s.subjectIDForAdmin(ctx, adminID)
	if subjectID == "" {
		return false
	}

	return s.subjectGuard.IsSuperSubject(ctx, subjectID)
}

func (s *Service) AdminCan(
	ctx context.Context,
	adminID int64,
	domain integrationroles.PermissionDomain,
	action integrationroles.PermissionAction,
) bool {
	return s.AdminGuardCheck(ctx, adminID, integrationroles.NewPermissionKey(domain, action))
}

func (s *Service) GetRolesAdmin(ctx context.Context, adminID int64) ([]integrationroles.Role, error) {
	subjectID := s.subjectIDForAdmin(ctx, adminID)
	if subjectID == "" {
		return nil, ErrAdminRoleNotFound
	}

	roles, err := s.subjectGuard.GetRolesSubject(ctx, subjectID)
	if err != nil {
		return nil, ErrAdminRoleNotFound
	}

	return roles, nil
}

func (s *Service) GetPermissions(
	ctx context.Context,
	roles []integrationroles.Role,
) (map[int64]map[int64]integrationroles.Permission, error) {
	return s.subjectGuard.GetPermissions(ctx, roles)
}

func (s *Service) IsAdoptedDetachRole(ctx context.Context, adminAuthID int64, adminID int64, roleID int64) (bool, error) {
	roles, err := s.GetRolesAdmin(ctx, adminID)
	if err != nil || len(roles) == 0 {
		return false, ErrAdminRoleNotFound
	}

	for _, role := range roles {
		if role.ID == roleID && adminAuthID == adminID {
			if role.Slug == integrationroles.SuperAdminRole {
				return false, ErrAdminRoleIsSuperRole
			}

			return true, nil
		}
	}

	return true, nil
}

func (s *Service) subjectIDForAdmin(ctx context.Context, adminID int64) string {
	if adminID <= 0 {
		return ""
	}

	admin, err := s.admins.GetByID(ctx, adminID)
	if err != nil {
		return ""
	}
	if admin.ID <= 0 {
		return ""
	}

	return adminSubjectID(admin)
}

func adminSubjectID(admin models.Admin) string {
	subjectID := admin.AuthSubjectID()
	if subjectID.IsZero() {
		return ""
	}

	return subjectID.String()
}
