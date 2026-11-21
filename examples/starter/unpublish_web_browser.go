package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
)

//go:embed templates/unpublish.html
var unpublishTemplates embed.FS

var unpublishPage = template.Must(template.ParseFS(unpublishTemplates, "templates/unpublish.html"))

func (h *unpublishHandler) registerBrowser(app *fiber.App) {
	app.Get(unpublishConnectPath, h.browserHeaders, h.connectPage)
	app.Post(unpublishConnectPath, h.browserHeaders, h.exactOrigin, h.connect)
	app.Get(unpublishRequestPath+":id", h.browserHeaders, h.reviewPage)
	app.Post(unpublishRequestPath+":id", h.browserHeaders, h.exactOrigin, h.decide)
}

func (h *unpublishHandler) browserHeaders(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Referrer-Policy", "same-origin")
	c.Set("X-Frame-Options", "DENY")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.Next()
}

func (h *unpublishHandler) exactOrigin(c fiber.Ctx) error {
	origins := c.Request().Header.PeekAll("Origin")
	if len(origins) != 1 || string(origins[0]) != h.origin.String() ||
		!strings.EqualFold(c.Get("Host"), h.origin.Host) || len(c.Request().Header.PeekAll("Authorization")) > 0 {
		return unpublishJSONError(c, http.StatusForbidden, "denied")
	}
	return c.Next()
}

func (h *unpublishHandler) nativeTransitions(c fiber.Ctx) error {
	path := strings.TrimRight(c.Path(), "/")
	preview := c.Method() == http.MethodPost && strings.EqualFold(path, "/auth/role-preview")
	logout := c.Method() == http.MethodDelete && strings.EqualFold(path, "/auth/logout")
	if !preview && !logout {
		return c.Next()
	}
	cookie := c.Cookies(h.sessionName)
	if cookie == "" {
		return c.Next()
	}
	browser := host.CSRFSessionBinding(cookie)
	release := h.fences.acquire(browser)
	defer release()
	if err := c.Next(); err != nil {
		return err
	}
	if preview && c.Response().StatusCode() == http.StatusOK || logout && h.clearedSessionCookie(c) {
		h.pairings.revoke(browser, "")
	}
	return nil
}

func (h *unpublishHandler) clearedSessionCookie(c fiber.Ctx) bool {
	// Native logout can redirect after a storage error while the login is
	// still active. It expires the session cookie when it clears the login.
	cookie, err := http.ParseSetCookie(string(c.Response().Header.PeekCookie(h.sessionName)))
	return err == nil && cookie.Value == "" && cookie.MaxAge < 0
}

type unpublishBrowser struct {
	actor host.Actor
	csrf  host.CSRFRequest
}

func (h *unpublishHandler) browser(c fiber.Ctx) (unpublishBrowser, error) {
	actor, ok := h.app.CurrentActor(c)
	if !ok {
		return unpublishBrowser{}, errUnpublishSession
	}
	req, err := h.app.AdminCSRFRequest(c)
	if err != nil {
		return unpublishBrowser{}, err
	}
	req.Domain = h.origin.Hostname()
	return unpublishBrowser{actor: actor, csrf: req}, nil
}

func (h *unpublishHandler) browserPermission(ctx context.Context, browser unpublishBrowser) error {
	err := h.currentPermission(ctx, browser.actor)
	if errors.Is(err, errUnpublishDenied) {
		release := h.fences.acquire(browser.csrf.SessionID)
		defer release()
		h.pairings.revoke(browser.csrf.SessionID, "")
	}
	return err
}

type unpublishPageData struct {
	Title       string
	Action      string
	Nonce       string
	CSRF        string
	Connect     bool
	Connections []string
	Request     unpublishRequest
}

func (h *unpublishHandler) renderPage(c fiber.Ctx, data unpublishPageData, browser unpublishBrowser) error {
	if data.Connect || data.Request.State == unpublishPending {
		var err error
		data.CSRF, err = h.formProof(c, browser.csrf)
		if err != nil {
			return fiber.ErrServiceUnavailable
		}
	}
	data.Nonce = rand.Text()
	var html bytes.Buffer
	if err := unpublishPage.Execute(&html, data); err != nil {
		return fiber.ErrServiceUnavailable
	}
	c.Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+data.Nonce+"'; style-src 'nonce-"+data.Nonce+
		"'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	return c.Type("html", "utf-8").Send(html.Bytes())
}

func (h *unpublishHandler) formProof(c fiber.Ctx, req host.CSRFRequest) (string, error) {
	if existing := c.Cookies(h.csrfName); existing != "" {
		if _, valid, err := h.csrf.Check(c.Context(), req, existing); valid && err == nil {
			return existing, nil
		}
	}
	token, err := h.csrf.Create(c.Context(), req)
	if err != nil {
		return "", err
	}
	if err := h.app.WriteAdminCSRFCookie(c, h.csrfName, token.Token, h.csrfTTL); err != nil {
		return "", err
	}
	return token.Token, nil
}

func (h *unpublishHandler) connectPage(c fiber.Ctx) error {
	browser, err := h.browser(c)
	if err != nil {
		return unpublishFailure(c, err)
	}
	connections := h.pairings.connections(browser.csrf.SessionID)
	if len(connections) == 0 {
		if err := h.browserPermission(c.Context(), browser); err != nil {
			return unpublishFailure(c, err)
		}
	}
	return h.renderPage(c, unpublishPageData{
		Title: "Подключение MCP для снятия товаров с публикации", Action: unpublishConnectPath,
		Connect: true, Connections: connections,
	}, browser)
}

func (h *unpublishHandler) connect(c fiber.Ctx) error {
	var body struct {
		Code         string `json:"code"`
		Decision     string `json:"decision"`
		ConnectionID string `json:"connection_id"`
	}
	if !decodeUnpublishJSON(c, &body) {
		return unpublishJSONError(c, http.StatusUnprocessableEntity, "invalid_request")
	}
	if body.Decision == "revoke" && body.ConnectionID != "" && body.Code == "" {
		browser, err := h.browser(c)
		if err != nil {
			return unpublishFailure(c, err)
		}
		release := h.fences.acquire(browser.csrf.SessionID)
		defer release()
		h.pairings.revoke(browser.csrf.SessionID, body.ConnectionID)
		return c.JSON(fiber.Map{"status": "revoked"})
	}
	code := strings.ToUpper(strings.TrimSpace(body.Code))
	if body.Decision != "approve" || body.ConnectionID != "" || !unpublishCodePattern.MatchString(code) {
		return unpublishJSONError(c, http.StatusUnprocessableEntity, "invalid_request")
	}
	browser, err := h.browser(c)
	if err != nil {
		return unpublishFailure(c, err)
	}
	if err := h.browserPermission(c.Context(), browser); err != nil {
		return unpublishFailure(c, err)
	}
	release := h.fences.acquire(browser.csrf.SessionID)
	defer release()
	session, err := h.app.BindDelegatedRead(c)
	switch {
	case errors.Is(err, host.ErrReadSessionEnded):
		return unpublishFailure(c, errUnpublishSession)
	case errors.Is(err, host.ErrReadSessionDenied):
		return unpublishFailure(c, errUnpublishDenied)
	case err != nil:
		return unpublishFailure(c, err)
	}
	if !h.pairings.approve(code, unpublishGrant{actor: browser.actor, browser: browser.csrf.SessionID, session: session}) {
		return unpublishJSONError(c, http.StatusNotFound, "not_found")
	}
	return c.JSON(fiber.Map{"status": "connected"})
}

func (h *unpublishHandler) reviewPage(c fiber.Ctx) error {
	browser, err := h.browser(c)
	if err != nil {
		return unpublishFailure(c, err)
	}
	if err := h.browserPermission(c.Context(), browser); err != nil {
		return unpublishFailure(c, err)
	}
	id := c.Params("id")
	if !canonicalUnpublishID(id) {
		return fiber.ErrNotFound
	}
	request, err := h.repo.lookupUnpublish(c.Context(), browser.actor, id)
	if err != nil {
		return unpublishFailure(c, err)
	}
	if request.State == unpublishPending {
		err := h.checkReview(c.Context(), browser, request)
		if errors.Is(err, errUnpublishExpired) {
			request, err = h.recheckExpired(c.Context(), browser.actor, request.RequestID)
		}
		if err != nil {
			return unpublishFailure(c, err)
		}
	}
	return h.renderPage(c, unpublishPageData{
		Title: "Снятие товара с публикации", Action: unpublishRequestPath + id, Request: request,
	}, browser)
}

func (h *unpublishHandler) checkReview(ctx context.Context, browser unpublishBrowser, request unpublishRequest) error {
	grant, ok := h.pairings.bound(request.BindingID)
	if !ok {
		return errUnpublishExpired
	}
	if grant.browser != browser.csrf.SessionID || grant.actor != browser.actor {
		return errUnpublishNotFound
	}
	err := h.checkCurrentGrant(ctx, grant)
	if errors.Is(err, errUnpublishSession) || errors.Is(err, errUnpublishDenied) {
		return errUnpublishExpired
	}
	return err
}

func (h *unpublishHandler) decide(c fiber.Ctx) error {
	var body struct {
		Decision string `json:"decision"`
	}
	id := c.Params("id")
	if !canonicalUnpublishID(id) || !decodeUnpublishJSON(c, &body) ||
		(body.Decision != "approve" && body.Decision != "decline") {
		return unpublishJSONError(c, http.StatusUnprocessableEntity, "invalid_request")
	}
	browser, err := h.browser(c)
	if err != nil {
		return unpublishFailure(c, err)
	}
	if err := h.browserPermission(c.Context(), browser); err != nil {
		return unpublishFailure(c, err)
	}
	release := func() {}
	result, err := h.repo.decideUnpublish(c.Context(), browser.actor, id, body.Decision == "approve",
		func(ctx context.Context, tx *sql.Tx, request unpublishRequest) error {
			var err error
			release, err = h.lockBrowserAuthority(ctx, tx, browser)
			if err != nil {
				return err
			}
			return h.checkReview(ctx, browser, request)
		})
	release()
	if err != nil {
		return unpublishFailure(c, err)
	}
	return h.sendResult(c, browser.actor, result)
}

// Logout takes the login mutex before locking the GoAuth subject. Use the
// same order here, after the request/product waits. Hold both through COMMIT.
func (h *unpublishHandler) lockBrowserAuthority(ctx context.Context, tx *sql.Tx, browser unpublishBrowser) (func(), error) {
	release := h.fences.acquire(browser.csrf.SessionID)
	err := lockUnpublishAuthority(ctx, tx, browser.actor)
	if errors.Is(err, errUnpublishDenied) {
		h.pairings.revoke(browser.csrf.SessionID, "")
	}
	return release, err
}

// GoAuth v0.18 grants a permission through subject-role and role-permission
// rows. Lock one effective grant and the native admin membership in the same
// PostgreSQL transaction as the product update. CHECK alone cannot protect
// against revocation by a different process between that check and COMMIT.
func lockUnpublishAuthority(ctx context.Context, tx *sql.Tx, actor host.Actor) error {
	const query = `SELECT 1 FROM auth_subjects s
		JOIN administrations a ON a.subject_id = s.id
		JOIN auth_subject_roles sr ON sr.subject_id = s.id
		JOIN auth_role_permissions rp ON rp.role_id = sr.role_id
		JOIN auth_permissions p ON p.id = rp.permission_id
		WHERE s.id = $1 AND a.id = $2 AND a.deleted_at IS NULL
		  AND s.status = 'active' AND s.retired_at IS NULL AND p.permission_key = 'products:unpublish'
		ORDER BY sr.role_id LIMIT 1 FOR SHARE OF s,a,sr,rp,p`
	var present int
	err := tx.QueryRowContext(ctx, query, actor.SubjectID, actor.AdminID).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return errUnpublishDenied
	}
	return err
}
