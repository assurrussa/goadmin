package roles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

type roleEditAccess string

const (
	roleEditAllowed  roleEditAccess = ""
	roleEditListOnly roleEditAccess = "list"
	roleEditDenied   roleEditAccess = "denied"
)

type roleEditResult struct {
	Role    *roleDetail    `json:"role,omitempty"`
	Message string         `json:"message,omitempty"`
	Access  roleEditAccess `json:"access,omitempty"`
}

func (h *Handler) withRoleEdit(ctx context.Context, roleID int64, fn func(context.Context) error) error {
	if h.roleEditTx == nil {
		return fn(ctx)
	}
	return h.roleEditTx(ctx, roleID, fn)
}

func (h *Handler) roleEditorAccess(ctx context.Context, adminID int64) roleEditAccess {
	// The route has already applied the host action guard. Inside the owned
	// transaction, read permissions through the canonical Runtime's SQL handle.
	check := h.adminApp.RolesService().AdminGuardCheck
	if !check(ctx, adminID, integrationroles.NewPermissionKey(
		integrationroles.PermissionDomainRoles, integrationroles.PermissionActionRead,
	)) {
		return roleEditDenied
	}
	if !check(ctx, adminID, integrationroles.NewPermissionKey(
		integrationroles.PermissionDomainRoles, integrationroles.PermissionActionUpdate,
	)) {
		return roleEditListOnly
	}
	return roleEditAllowed
}

func (h *Handler) loadRoleEdit(ctx context.Context, roleID, adminID int64) (roleDetail, error) {
	response, err := h.getRole.Handle(ctx, integrationroles.GetRoleRequest{RoleID: roleID, IncludePermissions: true})
	if err != nil {
		return roleDetail{}, err
	}
	if err := h.checkSuperRole(ctx, response.Slug, adminID); err != nil {
		return roleDetail{}, fiber.ErrForbidden
	}
	detail := mapRoleDetail(response)
	if h.roleEditTx != nil {
		detail.Revision, err = roleRevision(detail, response.UpdatedAt)
	}
	return detail, err
}

func roleRevision(role roleDetail, updatedAt time.Time) (string, error) {
	keys := make([]string, 0, len(role.Permissions))
	for _, permission := range role.Permissions {
		keys = append(keys, integrationroles.NewPermissionKey(permission.Domain, permission.Action).String())
	}
	slices.Sort(keys)
	canonical, err := json.Marshal(struct {
		UUID        string    `json:"uuid"`
		Slug        string    `json:"slug"`
		Name        string    `json:"name"`
		Description *string   `json:"description"`
		System      bool      `json:"system"`
		UpdatedAt   time.Time `json:"updatedAt"`
		Permissions []string  `json:"permissions"`
	}{role.UUID, role.Slug, role.Name, role.Description, role.IsSystem, updatedAt.UTC(), keys})
	if err != nil {
		return "", fmt.Errorf("encode role revision: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func (h *Handler) updateRoleEdit(c fiber.Ctx, roleID int64, payload updateRolePayload) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	revision, err := hex.DecodeString(*payload.Revision)
	if err != nil || len(revision) != sha256.Size {
		return c.Status(fiber.StatusBadRequest).JSON(roleEditResult{Message: "Некорректная версия роли"})
	}
	if h.roleEditTx == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(roleEditResult{Message: "Редактор ролей недоступен"})
	}

	adminID := adminmiddleware.MustGetAdminAuth(c).ID
	result, status, err := h.compareAndUpdateRole(c, roleID, adminID, payload)
	if err != nil {
		return h.roleEditError(c, err)
	}
	return c.Status(status).JSON(result)
}

func (h *Handler) compareAndUpdateRole(
	ctx context.Context, roleID, adminID int64, payload updateRolePayload,
) (roleEditResult, int, error) {
	var result roleEditResult
	status := fiber.StatusOK
	err := h.roleEditTx(ctx, roleID, func(txCtx context.Context) error {
		if access := h.roleEditorAccess(txCtx, adminID); access != roleEditAllowed {
			result.Access = access
			status = fiber.StatusForbidden
			return nil
		}
		current, err := h.loadRoleEdit(txCtx, roleID, adminID)
		if err != nil {
			return err
		}
		if current.Revision != *payload.Revision {
			result.Role = &current
			status = fiber.StatusConflict
			return nil
		}
		if _, err := h.updateRole.Handle(txCtx, payload.request(roleID, current.Slug)); err != nil {
			return err
		}
		// Saving the actor's own role may remove either permission. Commit
		// the write, but do not return editor data the actor can no longer read.
		if access := h.roleEditorAccess(txCtx, adminID); access != roleEditAllowed {
			result.Access = access
			return nil
		}
		saved, err := h.loadRoleEdit(txCtx, roleID, adminID)
		if err != nil {
			return err
		}
		result.Role = &saved
		return nil
	})
	return result, status, err
}

func (h *Handler) roleEditError(c fiber.Ctx, err error) error {
	message, known := mapRoleDomainError(err, "Не удалось обновить роль")
	switch {
	case errors.Is(err, fiber.ErrForbidden):
		return c.Status(fiber.StatusForbidden).JSON(roleEditResult{Message: superRoleChangeForbiddenMessage})
	case errors.Is(err, integrationroles.ErrRoleNotFound):
		return c.Status(fiber.StatusNotFound).JSON(roleEditResult{Message: message})
	case known:
		return c.Status(fiber.StatusUnprocessableEntity).JSON(roleEditResult{Message: message})
	default:
		h.logger.ErrorContext(c, "update role", logger.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(roleEditResult{})
	}
}

func (payload updateRolePayload) request(roleID int64, currentSlug string) integrationroles.UpdateRoleRequest {
	slug := strings.TrimSpace(payload.Slug)
	if currentSlug == integrationroles.SuperAdminRole {
		slug = currentSlug
	}
	return integrationroles.UpdateRoleRequest{
		ID: roleID, Slug: slug, Name: strings.TrimSpace(payload.Name), Description: payload.Description,
		IsSystem: payload.IsSystem, PermissionKeys: payload.Permissions,
	}
}
