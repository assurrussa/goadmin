package bootstrap

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	coresession "github.com/assurrussa/goadmin/infrastructure/core/session"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

const (
	rolePreviewSessionKey = "goadmin.role-preview"
	rolePreviewPath       = "/auth/role-preview"
)

// This middleware runs after native session authentication and CSRF validation.
// Session-owned state cannot be bypassed by dropping a query parameter/header.
func rolePreviewMiddleware(deps Dependencies) fiber.Handler {
	return func(c fiber.Ctx) error {
		admin := adminmiddleware.GetAdminAuth(c)
		if admin == nil || deps.System.SessionStore == nil {
			return c.Next()
		}
		sess, err := deps.System.SessionStore.Get(c)
		if err != nil {
			return err
		}
		id, _ := sess.Get(rolePreviewSessionKey).(int64)
		sess.Release()
		owner := deps.Repos.RolesService != nil && deps.Repos.RolesService.IsSuperAdmin(c, admin.ID)
		if c.Path() == rolePreviewPath {
			return controlRolePreview(c, deps, owner)
		}
		state := fiber.Map{"active": id > 0, "available": owner, "roles": []fiber.Map{}}
		if err := applyRolePreview(c, deps, owner, id, state); err != nil {
			if errors.Is(err, fiber.ErrForbidden) {
				return c.SendStatus(fiber.StatusForbidden)
			}
			return err
		}
		// Every mutation is denied before route/command execution, even without a guard.
		if id > 0 && c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
			return c.SendStatus(fiber.StatusForbidden)
		}
		if owner && deps.Repos.RolesManager != nil {
			options, err := rolePreviewOptions(c, deps)
			if err != nil {
				return err
			}
			state["roles"] = options
		}
		c.Locals("goadmin.rolePreview", state)
		return c.Next()
	}
}

func controlRolePreview(c fiber.Ctx, deps Dependencies, owner bool) error {
	if c.Method() != fiber.MethodPost {
		return fiber.ErrMethodNotAllowed
	}
	if !owner || deps.Repos.RolesManager == nil {
		return c.SendStatus(fiber.StatusForbidden)
	}
	var input struct {
		RoleID int64 `json:"roleId"`
	}
	if err := c.Bind().JSON(&input); err != nil || input.RoleID < 0 {
		return fiber.ErrBadRequest
	}
	if input.RoleID > 0 {
		role, err := deps.Repos.RolesManager.GetRole(c, input.RoleID)
		if err != nil || role == nil {
			return fiber.ErrBadRequest
		}
	}
	store := deps.System.SessionStore
	sess, err := store.Get(c)
	if err != nil {
		return err
	}
	defer sess.Release()
	if input.RoleID == 0 {
		sess.Delete(rolePreviewSessionKey)
	} else {
		sess.Set(rolePreviewSessionKey, input.RoleID)
	}
	if err := coresession.SavePassive(c, store, sess); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"active": input.RoleID > 0})
}

func applyRolePreview(c fiber.Ctx, deps Dependencies, owner bool, id int64, state fiber.Map) error {
	if id == 0 {
		return nil
	}
	if !owner || deps.Repos.RolesManager == nil {
		return fiber.ErrForbidden
	}
	role, err := deps.Repos.RolesManager.GetRole(c, id)
	if err != nil || role == nil {
		return fiber.ErrForbidden
	}
	permissions, err := deps.Repos.RolesManager.ListRolePermissions(c, id)
	if err != nil {
		return fiber.ErrServiceUnavailable
	}
	keys := make(map[integrationroles.PermissionKey]bool, len(permissions))
	for _, permission := range permissions {
		keys[integrationroles.NewPermissionKey(permission.Domain, permission.Action)] = true
	}
	if role.Slug == integrationroles.SuperAdminRole {
		c.SetContext(integrationroles.WithSuperRolePreview(c.Context()))
	} else {
		c.SetContext(integrationroles.WithRolePreview(c.Context(), keys))
	}
	state["roleName"] = role.Name
	state["roleId"] = strconv.FormatInt(id, 10)
	state["actions"] = rolePreviewActionPresentation(c, deps)
	return nil
}

func rolePreviewOptions(c fiber.Ctx, deps Dependencies) ([]fiber.Map, error) {
	roles, err := deps.Repos.RolesManager.ListRoles(c, integrationroles.RoleFilter{
		IncludeSys: true, Pagination: integrationroles.Pagination{Limit: 1000},
	})
	if err != nil {
		return nil, fiber.ErrServiceUnavailable
	}
	options := make([]fiber.Map, 0, len(roles))
	for _, role := range roles {
		options = append(options, fiber.Map{"id": role.ID, "name": role.Name})
	}
	return options, nil
}

// Detail pages consume only fresh guard decisions, never browser role claims.
func rolePreviewActionPresentation(c fiber.Ctx, deps Dependencies) map[string]bool {
	actions := make(map[string]bool)
	actor := adminmiddleware.GetAdminAuth(c)
	if actor == nil || deps.Repos.RolesService == nil {
		return actions
	}
	keys := []integrationroles.PermissionKey{
		integrationroles.NewPermissionKey(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate),
		integrationroles.NewPermissionKey(integrationroles.PermissionDomainUsers, integrationroles.PermissionActionUpdate),
		integrationroles.NewPermissionKey(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionUpdate),
		integrationroles.NewPermissionKey(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionAssign),
	}
	for _, key := range keys {
		actions[key.String()] = deps.Repos.RolesService.AdminGuardCheck(c, actor.ID, key)
	}
	return actions
}
