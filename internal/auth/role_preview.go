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
	if request, ok := ctx.(interface{ Context() context.Context }); ok {
		ctx = request.Context()
	}
	preview, ok := ctx.Value(previewContextKey{}).(PreviewPermissions)
	return !ok || preview.AllowAll || preview.Keys[key]
}

func RolePreviewActive(ctx context.Context) bool {
	if request, ok := ctx.(interface{ Context() context.Context }); ok {
		ctx = request.Context()
	}
	_, ok := ctx.Value(previewContextKey{}).(PreviewPermissions)
	return ok
}

// WithSuperRolePreview preserves the permission engine's built-in Super Admin
// bypass for presentation. Actual authority is still checked independently.
func WithSuperRolePreview(ctx context.Context) context.Context {
	return context.WithValue(ctx, previewContextKey{}, PreviewPermissions{AllowAll: true})
}
