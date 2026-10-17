package auth

import "context"

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
