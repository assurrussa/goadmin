package auth

import (
	"context"
	"errors"
)

// RolePreviewSessionKey belongs to the authenticated browser session.
const RolePreviewSessionKey = "goadmin.role-preview"

var ErrInvalidRolePreview = errors.New("role preview is unavailable")

type RolePreviewReader interface {
	GetRole(ctx context.Context, id int64) (*Role, error)
	ListRolePermissions(ctx context.Context, id int64) ([]Permission, error)
}

// ResolveRolePreview builds the current read policy for a session-owned role.
// Callers still check the operator's actual authority for each resource.
func ResolveRolePreview(ctx context.Context, owner bool, reader RolePreviewReader, id int64) (context.Context, *Role, error) {
	if id <= 0 || !owner || reader == nil {
		return ctx, nil, ErrInvalidRolePreview
	}
	role, err := reader.GetRole(ctx, id)
	if err != nil || role == nil {
		return ctx, nil, ErrInvalidRolePreview
	}
	permissions, err := reader.ListRolePermissions(ctx, id)
	if err != nil {
		return ctx, nil, err
	}
	if role.Slug == SuperAdminRole {
		return WithSuperRolePreview(ctx), role, nil
	}
	keys := make(map[PermissionKey]bool, len(permissions))
	for _, permission := range permissions {
		keys[NewPermissionKey(permission.Domain, permission.Action)] = true
	}
	return WithRolePreview(ctx, keys), role, nil
}

// PreviewPermissions is request-local presentation policy, never an identity or grant.
type PreviewPermissions struct {
	Keys     map[PermissionKey]bool
	AllowAll bool
}
type previewContextKey struct{}

func WithRolePreview(ctx context.Context, keys map[PermissionKey]bool) context.Context {
	return context.WithValue(ctx, previewContextKey{}, PreviewPermissions{Keys: keys})
}

func RolePreviewAllows(ctx context.Context, key PermissionKey) bool {
	preview, ok := rolePreviewValue(ctx)
	return !ok || preview.AllowAll || preview.Keys[key]
}

func RolePreviewActive(ctx context.Context) bool {
	_, ok := rolePreviewValue(ctx)
	return ok
}

// Fiber carries inherited request values in Context rather than its own Locals.
// Read the policy without replacing the caller's context or its cancellation.
func rolePreviewValue(ctx context.Context) (PreviewPermissions, bool) {
	if request, ok := ctx.(interface{ Context() context.Context }); ok {
		preview, exists := request.Context().Value(previewContextKey{}).(PreviewPermissions)
		return preview, exists
	}
	preview, ok := ctx.Value(previewContextKey{}).(PreviewPermissions)
	return preview, ok
}

// WithSuperRolePreview preserves the permission engine's built-in Super Admin
// bypass for presentation. Actual authority is still checked independently.
func WithSuperRolePreview(ctx context.Context) context.Context {
	return context.WithValue(ctx, previewContextKey{}, PreviewPermissions{AllowAll: true})
}
