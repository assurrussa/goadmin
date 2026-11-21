package host

import (
	"context"

	"github.com/gofiber/fiber/v3"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	"github.com/assurrussa/goadmin/services/adminservice"
)

// ReadSessionBinding pins one verified native browser login to its host
// instance. Keep it server-side in memory. Logout, expiry or deliberate
// replacement of the browser's session identifier requires fresh approval.
type ReadSessionBinding = adminservice.ReadSessionBinding

var (
	ErrReadSessionEnded  = adminservice.ErrReadSessionEnded
	ErrReadSessionDenied = adminservice.ErrReadSessionDenied
)

// BindDelegatedRead pins the current operator on a CSRF-protected approval
// route. The caller must first require the permission being delegated.
func (a *App) BindDelegatedRead(c fiber.Ctx) (ReadSessionBinding, error) {
	if a == nil || a.inner == nil || c == nil {
		return ReadSessionBinding{}, ErrReadSessionEnded
	}
	return a.inner.AdminAuth().BindDelegatedRead(c.Context(), adminmiddleware.GetAdminAuth(c))
}

// ResolveDelegatedRead revalidates the native session and membership and
// propagates its current role preview. Pass both results to the resource
// reader, which independently checks its permission, scope and record policy.
func (a *App) ResolveDelegatedRead(ctx context.Context, pin ReadSessionBinding) (context.Context, Actor, error) {
	if a == nil || a.inner == nil || ctx == nil {
		return ctx, Actor{}, ErrReadSessionEnded
	}
	policy, admin, err := a.inner.AdminAuth().ResolveDelegatedRead(ctx, pin)
	if err != nil {
		return ctx, Actor{}, err
	}
	return policy, Actor{AdminID: admin.ID, SubjectID: admin.AuthSubjectID().String()}, nil
}
