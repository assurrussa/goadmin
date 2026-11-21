package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
)

func (h *actionHandler) registerBrowser(app *fiber.App) {
	path := h.action.Path
	app.Get(path+"/connect", h.browserHeaders, h.connectPage)
	app.Post(path+"/connect", h.browserHeaders, h.exactOrigin, h.connect)
	app.Get(path+"/requests/:id", h.browserHeaders, h.reviewPage)
	app.Post(path+"/requests/:id", h.browserHeaders, h.exactOrigin, h.decide)
}

func (h *actionHandler) browserHeaders(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Referrer-Policy", "same-origin")
	c.Set("X-Frame-Options", "DENY")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.Next()
}

func (h *actionHandler) exactOrigin(c fiber.Ctx) error {
	origins := c.Request().Header.PeekAll("Origin")
	if len(origins) != 1 || string(origins[0]) != h.origin.String() ||
		!strings.EqualFold(c.Get("Host"), h.origin.Host) || len(c.Request().Header.PeekAll("Authorization")) != 0 {
		return agentJSON(c, 403, problemDenied)
	}
	return c.Next()
}

func (h *actionHandler) nativeTransitions(c fiber.Ctx) error {
	path := strings.TrimRight(c.Path(), "/")
	preview := c.Method() == http.MethodPost && strings.EqualFold(path, "/auth/role-preview")
	logout := c.Method() == http.MethodDelete && strings.EqualFold(path, "/auth/logout")
	cookie := c.Cookies(h.sessionName)
	if (!preview && !logout) || cookie == "" {
		return c.Next()
	}
	browser := host.CSRFSessionBinding(cookie)
	release := h.fences.acquire(browser)
	defer release()
	if err := c.Next(); err != nil {
		return err
	}
	cleared, err := http.ParseSetCookie(string(c.Response().Header.PeekCookie(h.sessionName)))
	if (preview && c.Response().StatusCode() == http.StatusOK) ||
		(logout && err == nil && cleared.Value == "" && cleared.MaxAge < 0) {
		h.grants.revoke(browser, "")
	}
	return nil
}

type actionBrowser struct {
	actor   host.Actor
	id      string
	session host.ReadSessionBinding
}

func (h *actionHandler) browser(c fiber.Ctx) (actionBrowser, error) {
	actor, ok := h.app.CurrentActor(c)
	if !ok {
		return actionBrowser{}, host.ErrReadSessionEnded
	}
	cookie := c.Cookies(h.sessionName)
	if cookie == "" {
		return actionBrowser{}, host.ErrReadSessionEnded
	}
	binding, err := h.app.BindDelegatedRead(c)
	if err == nil {
		err = h.checkSession(c.Context(), binding)
	}
	if errors.Is(err, host.ErrReadSessionDenied) || errors.Is(err, host.ErrReadSessionEnded) {
		h.grants.revoke(host.CSRFSessionBinding(cookie), "")
	}
	return actionBrowser{actor: actor, id: host.CSRFSessionBinding(cookie), session: binding}, err
}

func (h *actionHandler) connectPage(c fiber.Ctx) error {
	b, err := h.browser(c)
	if err != nil {
		return actionFailure(c, err)
	}
	return h.app.Render(c, approvalPage, map[string]any{
		propGrants: h.grants.connections(b.id), "connectPath": h.action.Path + "/connect", propTitle: h.action.Title,
		propDescription: h.action.Description, "confirmedAction": true,
	})
}

func (h *actionHandler) connect(c fiber.Ctx) error {
	var body struct {
		Code         string `json:"code"`
		Decision     string `json:"decision"`
		ConnectionID string `json:"connection_id"` //nolint:tagliatelle // Existing device protocol.
	}
	if !readAgentJSON(c, &body) {
		return agentJSON(c, 422, "invalid_request")
	}
	b, err := h.browser(c)
	if err != nil {
		return actionFailure(c, err)
	}
	release := h.fences.acquire(b.id)
	defer release()
	switch body.Decision {
	case "revoke":
		if body.ConnectionID == "" || body.Code != "" {
			return agentJSON(c, 422, "invalid_request")
		}
		h.grants.revoke(b.id, body.ConnectionID)
	case decisionApprove:
		code := strings.ToUpper(strings.TrimSpace(body.Code))
		if body.ConnectionID != "" || !codePattern.MatchString(code) {
			return agentJSON(c, 422, "invalid_request")
		}
		// Rebind while holding the login fence. A preview or logout could
		// have completed between the request middleware and this approval.
		b, err = h.browser(c)
		if err != nil {
			return actionFailure(c, err)
		}
		if !h.grants.approve(code, actionGrant{actor: b.actor, browser: b.id, session: b.session}) {
			return agentJSON(c, 404, "not_found")
		}
	default:
		return agentJSON(c, 422, "invalid_request")
	}
	return c.JSON(fiber.Map{fieldStatus: "connected", propGrants: h.grants.connections(b.id)})
}

func (h *actionHandler) reviewPage(c fiber.Ctx) error {
	b, err := h.browser(c)
	if err != nil {
		return actionFailure(c, err)
	}
	id := c.Params("id")
	if !canonicalActionID(id) {
		return agentJSON(c, 404, "not_found")
	}
	r, err := h.store.status(c.Context(), b.actor, id)
	if err != nil {
		return actionFailure(c, err)
	}
	if g, ok := h.grants.bound(r.BindingID); ok && g.browser != b.id {
		return agentJSON(c, 404, "not_found")
	}
	r, err = h.liveRequest(c.Context(), b.actor, r)
	if err != nil {
		return actionFailure(c, err)
	}
	return h.app.Render(c, "agent/ActionReview", map[string]any{
		propTitle: h.action.Title, "confirmLabel": h.action.ConfirmLabel, propDescription: h.action.Description,
		"path": h.action.Path + "/requests/" + id, "requestID": id, fieldStatus: r.State, "expiresAt": r.ExpiresAt,
		"recordID": r.Snapshot.RecordID, "revision": r.Snapshot.Revision, "fields": r.Snapshot.Fields, "result": h.resultFields(r),
	})
}

func (h *actionHandler) decide(c fiber.Ctx) error {
	var in struct {
		Decision string `json:"decision"`
	}
	id := c.Params("id")
	if !canonicalActionID(id) || !readAgentJSON(c, &in) || (in.Decision != decisionApprove && in.Decision != "decline") {
		return agentJSON(c, 422, "invalid_request")
	}
	b, err := h.browser(c)
	if err != nil {
		return actionFailure(c, err)
	}
	release := func() {}
	defer func() { release() }()
	authorize := func(ctx context.Context, _ ActionTx, r actionRequest) error {
		release = h.fences.acquire(b.id)
		g, ok := h.grants.bound(r.BindingID)
		if !ok {
			return errActionExpired
		}
		if g.browser != b.id || g.actor != b.actor {
			return errActionNotFound
		}
		err := h.checkGrantInTx(ctx, g)
		if errors.Is(err, host.ErrReadSessionEnded) || errors.Is(err, host.ErrReadSessionDenied) {
			return errActionExpired
		}
		return err
	}
	r, err := h.store.decide(c.Context(), b.actor, id, in.Decision == decisionApprove, authorize)
	if err != nil {
		return actionFailure(c, err)
	}
	return h.sendResult(c, b.actor, r)
}
