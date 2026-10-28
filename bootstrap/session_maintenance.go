package bootstrap

import (
	"errors"
	"net/http"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	adminconfig "github.com/assurrussa/goadmin/config"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	"github.com/assurrussa/goadmin/services/adminservice"
)

const maintenanceHeader = "X-Goadmin-Auth-Recovery"

func sessionMaintenancePageProp(c fiber.Ctx, service *adminservice.Service) map[string]any {
	admin := adminmiddleware.GetAdminAuth(c)
	if admin == nil || !service.SupportsSessionMaintenance(c) {
		return map[string]any{}
	}
	// Anchor the rendered identity, rather than learning a possibly different
	// login from the first maintenance response after this document loads.
	return map[string]any{"version": 1, "sessionEpoch": browserstate.SessionEpoch(admin.AuthSessionID)}
}

type sessionMaintenanceMetadata struct {
	Version            int        `json:"version"`
	ServerTime         time.Time  `json:"serverTime"`
	NextCheckAfterMS   int64      `json:"nextCheckAfterMs"`
	ExternalValidUntil *time.Time `json:"externalValidUntil,omitempty"`
	CSRFExpiresAt      time.Time  `json:"csrfExpiresAt"`
	HardExpiresAt      time.Time  `json:"hardExpiresAt"`
	SessionEpoch       string     `json:"sessionEpoch"`
	Revision           int64      `json:"revision"`
	Renewed            bool       `json:"renewed"`
}

type sessionMaintenanceResponse struct {
	Status      int                         `json:"status"`
	Code        string                      `json:"code"`
	CSRFReady   bool                        `json:"csrfReady,omitempty"`
	Maintenance *sessionMaintenanceMetadata `json:"maintenance,omitempty"`
}

func maintenanceIntent(c fiber.Ctx, cfg adminconfig.Config) bool {
	if c.Method() != http.MethodGet || len(c.Request().Header.PeekAll(maintenanceHeader)) != 1 ||
		c.Get(maintenanceHeader) != "1" || len(c.Request().Header.PeekAll("Authorization")) != 0 ||
		len(c.Request().URI().QueryString()) != 0 || len(c.Request().Header.PeekAll("Sec-Fetch-Site")) > 1 ||
		len(c.Request().Header.PeekAll("Referer")) > 1 || len(c.Request().Header.PeekAll("Origin")) > 1 {
		return false
	}
	if site := c.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return false
	}
	if len(c.Request().Header.PeekAll("Origin")) > 0 || c.Get("Referer") != "" {
		return validAdminOrigin(c, cfg)
	}
	return c.Get("Sec-Fetch-Site") == "same-origin"
}

func registerSessionMaintenance(app *fiber.App, deps Dependencies, cfg adminconfig.Config) {
	if deps.System.AdminAuthService == nil || deps.System.CSRF == nil || deps.System.SessionStore == nil {
		return
	}
	app.Get("/auth/session/maintain", sessionMaintenanceHandler(deps, cfg))
}

func sessionMaintenanceHandler(deps Dependencies, cfg adminconfig.Config) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		c.Set(maintenanceHeader, "1")
		if !maintenanceIntent(c, cfg) {
			return c.Status(http.StatusForbidden).JSON(sessionMaintenanceResponse{Status: http.StatusForbidden, Code: "access_denied"})
		}
		service := deps.System.AdminAuthService
		if err := service.ValidateBrowserCredentials(c, getCookieTokenName(cfg)); err != nil {
			return maintenanceFailure(c, err)
		}
		before, _ := service.MaintenanceSnapshot(c)
		state, err := service.MaintainAdminSession(c)
		if err != nil {
			return maintenanceFailure(c, err)
		}
		admin, err := service.GetAdminAuth(c)
		if err != nil {
			return maintenanceFailure(c, err)
		}
		if admin == nil {
			return maintenanceFailure(c, goauth.ErrSessionRevoked)
		}
		current, err := service.MaintenanceSnapshot(c)
		if err != nil {
			return maintenanceFailure(c, err)
		}
		if current.Epoch != state.Epoch || current.Revision != state.Revision {
			return maintenanceFailure(c, errors.New("session changed during maintenance"))
		}
		csrf := deps.System.CSRF
		request := csrfservice.Request{SessionID: CSRFSessionBinding(admin.SessionID), UserID: admin.UUID, Domain: cfg.Domain}
		claims, valid, _ := csrf.Check(c, request, c.Cookies(getCookieTokenName(cfg)))
		if !valid || claims == nil || time.Until(claims.ExpiresAt.Time) <= min(30*time.Second, csrf.TokenTTL()/10) {
			token, createErr := csrf.Create(c, request)
			if createErr != nil {
				return maintenanceFailure(c, createErr)
			}
			WriteAdminCSRFCookie(c, deps.System.SessionStore, cfg, token.Token, token.Domain)
			return c.Status(http.StatusConflict).JSON(sessionMaintenanceResponse{
				Status: http.StatusConflict, Code: "authentication_recovery_continue",
			})
		}
		now := time.Now().UTC()
		next := min(60*time.Second, time.Until(state.HardExpiresAt),
			time.Until(claims.ExpiresAt.Time)-min(30*time.Second, csrf.TokenTTL()/10))
		if !state.ExternalValidUntil.IsZero() {
			next = min(next, time.Until(state.ExternalValidUntil)-30*time.Second)
		}
		if !state.AccessExpiresAt.IsZero() {
			next = min(next, time.Until(state.AccessExpiresAt))
		}
		if state.IdleCheckAfter > 0 {
			next = min(next, state.IdleCheckAfter)
		}
		// A successful SDK Verify may keep the old deadline before its renewal
		// window. Recheck inside that window without a zero-delay polling loop.
		next = max(time.Second, next)
		metadata := &sessionMaintenanceMetadata{
			Version: 1, ServerTime: now, NextCheckAfterMS: next.Milliseconds(),
			CSRFExpiresAt: claims.ExpiresAt.Time, HardExpiresAt: state.HardExpiresAt,
			SessionEpoch: state.Epoch, Revision: state.Revision, Renewed: before.Revision != state.Revision,
		}
		if !state.ExternalValidUntil.IsZero() {
			metadata.ExternalValidUntil = &state.ExternalValidUntil
		}
		return c.JSON(sessionMaintenanceResponse{
			Status: http.StatusOK, Code: "authentication_recovered", CSRFReady: true, Maintenance: metadata,
		})
	}
}

func maintenanceFailure(c fiber.Ctx, err error) error {
	status, code := http.StatusServiceUnavailable, "authentication_unavailable"
	switch {
	case errors.Is(err, goauth.ErrMembershipDenied):
		status, code = http.StatusForbidden, "access_denied"
	case errors.Is(err, goauth.ErrSessionRevoked), errors.Is(err, goauth.ErrExpiredToken),
		errors.Is(err, goauth.ErrOperationOutcomeUnknown), errors.Is(err, goauth.ErrSecurityVersionMismatch),
		errors.Is(err, goauth.ErrMFARequired), errors.Is(err, goauth.ErrInvalidToken),
		errors.Is(err, goauth.ErrAccountNotFound), errors.Is(err, goauth.ErrAccountUnavailable),
		errors.Is(err, browsercookie.ErrAmbiguousCookies):
		status, code = http.StatusUnauthorized, "reauthentication_required"
	default:
		c.Set(fiber.HeaderRetryAfter, "1")
	}
	return c.Status(status).JSON(sessionMaintenanceResponse{Status: status, Code: code})
}

// The Inertia v2 error listener normally redirects mutations. A recognized
// GoAdmin CSRF refusal must keep its status and stop before the handler.
func sessionMaintenanceErrorHandler(next fiber.ErrorHandler) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		if repair, _ := c.Locals("goadmin.csrfRepair").(bool); repair {
			c.Set(fiber.HeaderCacheControl, "no-store")
			c.Set("X-Goadmin-Auth-Error", "1")
			c.Response().Header.Del("X-Inertia")
			c.Response().Header.Del("X-Inertia-Location")
			c.Response().Header.Del(fiber.HeaderLocation)
			return c.Status(419).JSON(sessionMaintenanceResponse{Status: 419, Code: "csrf_needs_sync"})
		}
		return next(c, err)
	}
}

func markCSRFRepair(c fiber.Ctx, authenticated bool) {
	const versionHeader = "X-Goadmin-Session-Maintenance"
	c.Locals("goadmin.csrfRepair", authenticated && c.Get(versionHeader) == "1" &&
		len(c.Request().Header.PeekAll(versionHeader)) == 1)
}
