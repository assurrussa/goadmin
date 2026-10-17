package bootstrap

import (
	"strconv"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	coresession "github.com/assurrussa/goadmin/infrastructure/core/session"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/gofiber/fiber/v3"
)

const rolePreviewSessionKey = "goadmin.role-preview"
const rolePreviewPath = "/auth/role-preview"

// This middleware runs after native session authentication and CSRF validation.
// Session-owned state cannot be bypassed by dropping a query parameter/header.
func rolePreviewMiddleware(deps Dependencies) fiber.Handler {
	return func(c fiber.Ctx) error {
		admin := adminmiddleware.GetAdminAuth(c)
		if admin == nil || deps.System.SessionStore == nil {
			return c.Next()
		}
		store := deps.System.SessionStore
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		id, _ := sess.Get(rolePreviewSessionKey).(int64)
		sess.Release()
		owner := deps.Repos.RolesService != nil && deps.Repos.RolesService.IsSuperAdmin(c, admin.ID)
		control := c.Path() == rolePreviewPath
		if control && c.Method() != fiber.MethodPost {
			return fiber.ErrMethodNotAllowed
		}
		if control {
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
		state := fiber.Map{"active": id > 0, "available": owner, "roles": []fiber.Map{}}
		if id > 0 {
			if !owner || deps.Repos.RolesManager == nil {
				return c.SendStatus(fiber.StatusForbidden)
			}
			role, err := deps.Repos.RolesManager.GetRole(c, id)
			if err != nil || role == nil {
				return c.SendStatus(fiber.StatusForbidden)
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
			// Every mutation is denied before route/command execution, even without a guard.
			if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
				return c.SendStatus(fiber.StatusForbidden)
			}
		}
		if owner && deps.Repos.RolesManager != nil {
			roles, err := deps.Repos.RolesManager.ListRoles(c, integrationroles.RoleFilter{IncludeSys: true, Pagination: integrationroles.Pagination{Limit: 1000}})
			if err != nil {
				return fiber.ErrServiceUnavailable
			}
			options := make([]fiber.Map, 0, len(roles))
			for _, role := range roles {
				options = append(options, fiber.Map{"id": role.ID, "name": role.Name})
			}
			state["roles"] = options
		}
		c.Locals("goadmin.rolePreview", state)
		return c.Next()
	}
}
