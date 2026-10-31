package sso

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	browser "github.com/assurrussa/gossoclient"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"

	"github.com/assurrussa/goadmin/host"
)

const (
	ssoRecoveryPath             = "/auth/session/recover"
	ssoRecoveryHeader           = "X-Goadmin-Auth-Recovery"
	ssoSameOrigin               = "same-origin"
	ssoRecoveryUnavailable      = "authentication_unavailable"
	ssoRecoveryReauthentication = "reauthentication_required"
)

// Recovery stages are fixed source-owned labels, never request/error strings.
type recoveryStage string

const (
	recoveryIntentStage      recoveryStage = "intent"
	recoveryNativeStage      recoveryStage = "native_snapshot"
	recoveryRequestStage     recoveryStage = "request_adapter"
	recoveryProviderStage    recoveryStage = "provider_verify"
	recoveryProofStage       recoveryStage = "proof_build"
	recoveryCommitStage      recoveryStage = "native_complete"
	recoveryCSRFRequestStage recoveryStage = "csrf_request"
	recoveryCSRFCreateStage  recoveryStage = "csrf_create"
	recoveryCSRFCheckStage   recoveryStage = "csrf_check"
	recoveryCSRFCookieStage  recoveryStage = "csrf_cookie"
)

type ssoRecoveryResponse struct {
	Status    int    `json:"status"`
	Code      string `json:"code"`
	CSRFReady bool   `json:"csrfReady,omitempty"`
}

func (f *Feature) recoveryIntent(c fiber.Ctx) bool {
	if c.Method() != http.MethodGet || len(c.Request().Header.PeekAll(ssoRecoveryHeader)) != 1 ||
		c.Get(ssoRecoveryHeader) != "1" || len(c.Request().Header.PeekAll("Authorization")) != 0 ||
		len(c.Request().Header.PeekAll("Sec-Fetch-Site")) > 1 || len(c.Request().Header.PeekAll("Referer")) > 1 ||
		len(c.Request().URI().QueryString()) != 0 ||
		(c.Get("Sec-Fetch-Site") != "" && c.Get("Sec-Fetch-Site") != ssoSameOrigin) {
		return false
	}
	origins := c.Request().Header.PeekAll("Origin")
	if len(origins) != 0 {
		return len(origins) == 1 && c.Get("Origin") == f.rp.Config.Origin
	}
	if c.Get("Sec-Fetch-Site") == ssoSameOrigin {
		return true
	}
	referer, err := url.Parse(c.Get("Referer"))
	return err == nil && referer.Scheme+"://"+referer.Host == f.rp.Config.Origin
}

// Recover only the captured native session. The shared GoAdmin fence rechecks
// session ownership, link instance and current authorization after RP renewal.
// No new login, identity link, cookie rotation or business replay occurs here.
func (f *Feature) recover(c fiber.Ctx) error {
	c.Set(ssoRecoveryHeader, "1")
	c.Set(fiber.HeaderCacheControl, "no-store")
	if !f.recoveryIntent(c) || f.csrf == nil {
		return f.failRecovery(c, recoveryIntentStage, browser.ErrDenied)
	}
	snapshot, err := f.app.BeginOwnedExternalAdminRecovery(c)
	if err != nil {
		return f.failRecovery(c, recoveryNativeStage, err)
	}
	old := snapshot.Binding()
	r, err := adaptor.ConvertRequest(c, true)
	if err != nil {
		_ = f.app.CancelExternalAdminRecovery(c, snapshot)
		return f.failRecovery(c, recoveryRequestStage, browser.ErrDenied)
	}
	proof, err := f.rp.Client.Verify(r)
	if err != nil {
		_ = f.app.CancelExternalAdminRecovery(c, snapshot)
		return f.failRecovery(c, recoveryProviderStage, err)
	}
	fresh, err := Build(proof, old.LocalSubject, old.LinkID)
	if err != nil {
		_ = f.app.CancelExternalAdminRecovery(c, snapshot)
		return f.failRecovery(c, recoveryProofStage, err)
	}
	if err = f.app.CompleteExternalAdminRecovery(c, snapshot, fresh); err != nil {
		return f.failRecovery(c, recoveryCommitStage, err)
	}
	request, err := f.app.AdminCSRFRequest(c)
	if err != nil {
		return f.failRecovery(c, recoveryCSRFRequestStage, err)
	}
	origin, _ := url.Parse(f.rp.Config.Origin)
	request.Domain = origin.Hostname()
	if claims, valid, checkErr := f.csrf.Check(c, request, c.Cookies(f.rp.csrfCookie)); checkErr == nil && valid &&
		time.Until(claims.ExpiresAt.Time) > min(30*time.Second, f.csrf.TokenTTL()/10) {
		return c.JSON(ssoRecoveryResponse{Status: http.StatusOK, Code: "authentication_recovered", CSRFReady: true})
	}
	token, err := f.csrf.Create(c, request)
	if err != nil {
		return f.failRecovery(c, recoveryCSRFCreateStage, browser.ErrUnavailable)
	}
	if _, valid, err := f.csrf.Check(c, request, token.Token); err != nil || !valid {
		return f.failRecovery(c, recoveryCSRFCheckStage, browser.ErrUnavailable)
	}
	if err := f.app.WriteAdminCSRFCookie(c, f.rp.csrfCookie, token.Token, f.csrf.TokenTTL()); err != nil {
		return f.failRecovery(c, recoveryCSRFCookieStage, browser.ErrUnavailable)
	}
	return c.Status(http.StatusConflict).JSON(ssoRecoveryResponse{
		Status: http.StatusConflict, Code: "authentication_recovery_continue",
	})
}

func (f *Feature) failRecovery(c fiber.Ctx, stage recoveryStage, err error) error {
	var logger host.Logger
	if f.app != nil {
		logger = f.app.Logger()
	}
	return writeRecoveryFailure(c, logger, stage, err)
}

func writeRecoveryFailure(c fiber.Ctx, logger host.Logger, stage recoveryStage, err error) error {
	status, code := http.StatusForbidden, ssoRecoveryReauthentication
	if errors.Is(err, browser.ErrUnavailable) || !admissionDenied(err) {
		status, code = http.StatusServiceUnavailable, ssoRecoveryUnavailable
	}
	// Deliberately use an empty context: request context may carry identity,
	// credentials or other caller-supplied attributes. Never log the raw error,
	// request URL/headers, proof, session or browser binding.
	if logger != nil {
		logger.WarnContext(context.Background(), "admin authentication recovery failed",
			"recovery_stage", string(stage), "status", status, "code", code)
	}
	return c.Status(status).JSON(ssoRecoveryResponse{Status: status, Code: code})
}
