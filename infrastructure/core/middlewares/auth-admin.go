package adminmiddleware

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/infrastructure/core/session"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
)

var excludePath = map[string]struct{}{
	"/auth/login": {}, "/auth/register": {}, "/auth/forgot-password": {},
	"/reset-password": {}, "/auth/reset-password": {},
}

func AuthAdminMiddleware(serviceSession *adminservice.Service, extraCookieNames ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		var admin *models.SessionAdmin
		err := serviceSession.ValidateBrowserCredentials(c, extraCookieNames...)
		if err == nil {
			admin, err = serviceSession.GetAdminAuth(c)
		}
		if err != nil {
			_, publicAuth := excludePath[c.Path()]
			if publicAuth && (c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead) &&
				errors.Is(err, browsercookie.ErrAmbiguousCookies) {
				serviceSession.RecoverAmbiguousBrowserCookies(c, extraCookieNames...)
				return c.Next()
			}
			return browserAuthFailure(c, err)
		}
		c.Locals(session.AuthAdminKey.String(), admin)
		return c.Next()
	}
}

func browserAuthFailure(c fiber.Ctx, err error) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	switch {
	case errors.Is(err, goauth.ErrOperationOutcomeUnknown), errors.Is(err, goauth.ErrSessionRevoked),
		errors.Is(err, goauth.ErrExpiredToken), errors.Is(err, goauth.ErrMembershipDenied),
		errors.Is(err, goauth.ErrSecurityVersionMismatch), errors.Is(err, goauth.ErrAccountUnavailable),
		errors.Is(err, goauth.ErrAccountNotFound):
		if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
			return goinertia.Redirect(c, "/auth/login")
		}
		return sendBrowserAuthFailure(c, fiber.StatusUnauthorized, "reauthentication_required", "Sign in again")
	case errors.Is(err, goauth.ErrInvalidToken):
		return sendBrowserAuthFailure(c, fiber.StatusBadRequest, "invalid_browser_credentials", "Invalid browser credentials")
	case errors.Is(err, adminservice.ErrRefreshInProgress), errors.Is(err, adminservice.ErrRefreshRetryRequired):
		c.Set(fiber.HeaderRetryAfter, "1")
		return sendBrowserAuthFailure(
			c, fiber.StatusServiceUnavailable, "authentication_retry_required", "Authentication is temporarily busy",
		)
	default:
		// A failed dependency is not evidence of invalid credentials.
		return sendBrowserAuthFailure(
			c, fiber.StatusServiceUnavailable, "authentication_unavailable", "Authentication service unavailable",
		)
	}
}

type browserAuthResponse struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Send, rather than return a goinertia.Error: the generic Inertia error listener
// turns mutation and X-Inertia errors into RedirectBack. Auth failures must keep
// their actual HTTP status and must not enter the flash/session-save path.
// These are non-page JSON responses, including for X-Inertia requests. Clients
// may present the error but must not automatically replay the rejected mutation.
func sendBrowserAuthFailure(c fiber.Ctx, status int, code, message string) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Goadmin-Auth-Error", "1")
	c.Response().Header.Del(fiber.HeaderLocation)
	c.Response().Header.Del("X-Inertia-Location")
	c.Response().Header.Del(goinertia.HeaderInertia)
	return c.Status(status).JSON(browserAuthResponse{Status: status, Code: code, Message: message})
}

func IsAuthAdminMiddleware(redirectFor ...func(fiber.Ctx, *models.SessionAdmin) string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if adminAuth := GetAdminAuth(c); adminAuth != nil {
			target := "/"
			if len(redirectFor) > 0 && redirectFor[0] != nil {
				if resolved := redirectFor[0](c, adminAuth); resolved != "" {
					target = resolved
				}
			}
			return goinertia.Redirect(c, target)
		}
		return c.Next()
	}
}

func IsNotAuthAdminMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if adminAuth := GetAdminAuth(c); adminAuth == nil {
			path := c.Path()
			if strings.HasPrefix(path, "/.well-known/") {
				return nil
			}
			if _, ok := excludePath[path]; !ok {
				if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
					return sendBrowserAuthFailure(c, fiber.StatusUnauthorized, "reauthentication_required", "Sign in again")
				}
				return goinertia.Redirect(c, "/auth/login?redirectback="+url.QueryEscape(c.OriginalURL()))
			}
		}
		return c.Next()
	}
}

func MustGetAdminAuth(ctx context.Context) *models.SessionAdmin {
	val := GetAdminAuth(ctx)
	if val == nil {
		panic(errors.New("admin auth not found"))
	}
	return val
}

func GetAdminAuth(ctx context.Context) *models.SessionAdmin {
	val, ok := ctx.Value(session.AuthAdminKey.String()).(*models.SessionAdmin)
	if !ok {
		return nil
	}
	return val
}

func GetAdminAuthUUID(ctx context.Context) identity.UserID {
	val, ok := ctx.Value(session.AuthAdminKey.String()).(*models.SessionAdmin)
	if !ok || val == nil {
		return identity.UserIDNil
	}
	return val.UUID
}
