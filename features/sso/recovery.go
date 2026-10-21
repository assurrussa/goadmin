package sso

import (
	"errors"
	"net/http"
	"net/url"

	browser "github.com/assurrussa/gossoclient"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
)

const (
	ssoRecoveryPath        = "/auth/session/recover"
	ssoRecoveryHeader      = "X-Goadmin-Auth-Recovery"
	ssoSameOrigin          = "same-origin"
	ssoRecoveryUnavailable = "authentication_unavailable"
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
		return recoveryFailure(c, browser.ErrDenied)
	}
	snapshot, err := f.app.BeginExternalAdminRecovery(c)
	if err != nil {
		return recoveryFailure(c, err)
	}
	old := snapshot.Binding()
	r, err := adaptor.ConvertRequest(c, true)
	if err != nil {
		return recoveryFailure(c, browser.ErrDenied)
	}
	proof, err := f.rp.Client.Verify(r)
	if err != nil {
		return recoveryFailure(c, err)
	}
	fresh, err := Build(proof, old.LocalSubject, old.LinkID)
	if err == nil {
		err = f.app.CompleteExternalAdminRecovery(c, snapshot, fresh)
	}
	if err != nil {
		return recoveryFailure(c, err)
	}
	request, err := f.app.AdminCSRFRequest(c)
	if err != nil {
		return recoveryFailure(c, err)
	}
	origin, _ := url.Parse(f.rp.Config.Origin)
	request.Domain = origin.Hostname()
	token, err := f.csrf.Create(c, request)
	if err != nil {
		return recoveryFailure(c, browser.ErrUnavailable)
	}
	if _, valid, err := f.csrf.Check(c, request, token.Token); err != nil || !valid {
		return recoveryFailure(c, browser.ErrUnavailable)
	}
	c.Cookie(&fiber.Cookie{
		Name: f.rp.csrfCookie, Value: token.Token, Path: "/", Secure: true,
		HTTPOnly: false, SameSite: "Strict", MaxAge: 24 * 60 * 60,
	})
	return c.JSON(ssoRecoveryResponse{Status: http.StatusOK, Code: "authentication_recovered", CSRFReady: true})
}

func recoveryFailure(c fiber.Ctx, err error) error {
	status, code := http.StatusForbidden, "reauthentication_required"
	if errors.Is(err, browser.ErrUnavailable) || !admissionDenied(err) {
		status, code = http.StatusServiceUnavailable, ssoRecoveryUnavailable
	}
	return c.Status(status).JSON(ssoRecoveryResponse{Status: status, Code: code})
}
