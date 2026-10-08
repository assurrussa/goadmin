package roles

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v3"

	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

type roleMutationError struct {
	cause     error
	message   string
	forbidden bool
	detach    bool
}

func (e *roleMutationError) Error() string { return e.message }
func (e *roleMutationError) Unwrap() error { return e.cause }

func roleMutationFailure(err error, message string) error {
	return &roleMutationError{cause: err, message: message}
}

func (h *Handler) checkRoleMutation(ctx context.Context, actorID int64, role roleDetail, roles []integrationroles.Role) error {
	roleIsSuper := role.Slug == integrationroles.SuperAdminRole
	if (adminRoleListHasSlug(roles, integrationroles.SuperAdminRole) || roleIsSuper) &&
		!h.adminApp.RolesService().IsSuperAdmin(ctx, actorID) {
		message := superAdminChangeForbiddenMessage
		if roleIsSuper {
			message = superRoleChangeForbiddenMessage
		}
		return &roleMutationError{message: message, forbidden: true}
	}
	return nil
}

func (h *Handler) inAdminRoleTransaction(ctx context.Context, actorID, adminID int64, fn func(context.Context) error) error {
	if h.adminRoleTransaction == nil {
		return errors.New("admin roles transaction is required")
	}
	return h.adminRoleTransaction.InAdminRoleTransaction(ctx, actorID, adminID, fn)
}

func (h *Handler) attachAdminRole(ctx context.Context, actorID, adminID, roleID int64) (models.Admin, bool, error) {
	var admin models.Admin
	var changed bool
	err := h.inAdminRoleTransaction(ctx, actorID, adminID, func(txCtx context.Context) error {
		role, err := h.fetchRoleDetail(txCtx, roleID, false)
		if err != nil {
			return roleMutationFailure(err, "Не удалось загрузить роль")
		}
		response, err := h.listAdminRoles.Handle(txCtx, ListAdminRolesRequest{AdminID: adminID})
		if err != nil {
			return roleMutationFailure(err, "Не удалось загрузить роли администратора")
		}
		if err := h.checkRoleMutation(txCtx, actorID, role, response.Roles); err != nil {
			return err
		}
		roleIDs := uniqueRoleIDs(response.Roles)
		changed = !containsRoleID(roleIDs, roleID)
		if changed {
			roleIDs = append(roleIDs, roleID)
			if _, err := h.assignAdminRoles.Handle(txCtx, AssignAdminRolesRequest{AdminID: adminID, RoleIDs: roleIDs}); err != nil {
				return roleMutationFailure(err, "Не удалось назначить роль администратору")
			}
		}
		admin, err = h.adminRepo.GetByID(txCtx, adminID)
		if err != nil {
			return roleMutationFailure(err, "Не удалось загрузить администратора")
		}
		return nil
	})
	return admin, changed, err
}

func (h *Handler) detachAdminRole(ctx context.Context, actorID, adminID, roleID int64) (bool, error) {
	var changed bool
	err := h.inAdminRoleTransaction(ctx, actorID, adminID, func(txCtx context.Context) error {
		role, err := h.fetchRoleDetail(txCtx, roleID, false)
		if err != nil {
			return roleMutationFailure(err, "Не удалось загрузить роль")
		}
		response, err := h.listAdminRoles.Handle(txCtx, ListAdminRolesRequest{AdminID: adminID})
		if err != nil {
			return roleMutationFailure(err, "Не удалось загрузить роли администратора")
		}
		if err := h.checkRoleMutation(txCtx, actorID, role, response.Roles); err != nil {
			return err
		}
		roleIDs := make([]int64, 0, len(response.Roles))
		for _, current := range response.Roles {
			if current.ID != roleID {
				roleIDs = append(roleIDs, current.ID)
				continue
			}
			adopted, err := h.adminApp.RolesService().IsAdoptedDetachRole(txCtx, actorID, adminID, roleID)
			if !adopted {
				return &roleMutationError{cause: err, message: "Нельзя удалить текущую роль", detach: true}
			}
		}
		changed = len(roleIDs) != len(response.Roles)
		if !changed {
			return nil
		}
		if _, err := h.assignAdminRoles.Handle(txCtx, AssignAdminRolesRequest{AdminID: adminID, RoleIDs: roleIDs}); err != nil {
			return roleMutationFailure(err, "Не удалось отменить роль у администратора")
		}
		return nil
	})
	return changed, err
}

func (h *Handler) respondRoleMutationError(c fiber.Ctx, err error, fallback string) error {
	var failure *roleMutationError
	if !errors.As(err, &failure) {
		return h.respondAdminDomainError(c, err, fallback, fiber.StatusInternalServerError)
	}
	if failure.forbidden {
		return h.respondAdminFormError(c, failure.message, fiber.StatusForbidden, nil)
	}
	if failure.detach {
		message := failure.message
		if failure.cause != nil {
			message += ":" + failure.cause.Error()
		}
		if isInertiaRequest(c) {
			h.withAdminIDError(c, message)
			h.adminApp.HTTPManager().WithFlashError(c, message)
			return h.adminApp.HTTPManager().RedirectBack(c)
		}
		if failure.cause != nil {
			return goinertia.NewError(fiber.StatusInternalServerError, message, failure.cause)
		}
		return goinertia.NewError(fiber.StatusForbidden, message)
	}
	return h.respondAdminDomainError(c, failure.cause, failure.message, fiber.StatusInternalServerError)
}
