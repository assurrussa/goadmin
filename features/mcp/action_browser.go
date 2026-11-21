package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

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

type nativeTransitionOutcomeKey struct{}

func (h *actionHandler) nativeTransitions(c fiber.Ctx) error {
	path := strings.TrimRight(c.Path(), "/")
	preview := c.Method() == http.MethodPost && strings.EqualFold(path, "/auth/role-preview")
	logout := c.Method() == http.MethodDelete && strings.EqualFold(path, "/auth/logout")
	cookie := c.Cookies(h.sessionName)
	if (!preview && !logout) || cookie == "" {
		return c.Next()
	}
	browser := host.CSRFSessionBinding(cookie)
	var retired []actionGrant
	err := func() error {
		finish := h.fences.transition(browser)
		var ended bool
		defer finish(func() {
			if ended {
				retired = h.grants.revoke(browser, "")
			}
		})
		err := c.Next()
		cleared, cookieErr := http.ParseSetCookie(string(c.Response().Header.PeekCookie(h.sessionName)))
		ended = (preview && err == nil && c.Response().StatusCode() == http.StatusOK) ||
			(logout && cookieErr == nil && cleared.Value == "" && cleared.MaxAge < 0)
		// Several action modules can wrap the same native transition. Retain
		// its outcome even when an inner module reports a cleanup error.
		if outcome, ok := c.Locals(nativeTransitionOutcomeKey{}).(bool); ok {
			ended = outcome
		} else {
			c.Locals(nativeTransitionOutcomeKey{}, ended)
		}
		return err
	}()
	if expireErr := h.clearRetired(c.Context(), retired); expireErr != nil {
		// The native transition is already durable. Preserve its cookies and
		// report the cleanup failure without making the grant usable again.
		return actionFailure(c, expireErr)
	}
	return err
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
		if expireErr := h.revoke(c.Context(), host.CSRFSessionBinding(cookie), ""); expireErr != nil {
			err = expireErr
		}
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
	switch body.Decision {
	case "revoke":
		if body.ConnectionID == "" || body.Code != "" {
			return agentJSON(c, 422, "invalid_request")
		}
		if err := h.revoke(c.Context(), b.id, body.ConnectionID); err != nil {
			return actionFailure(c, err)
		}
	case decisionApprove:
		code := strings.ToUpper(strings.TrimSpace(body.Code))
		if body.ConnectionID != "" || !codePattern.MatchString(code) {
			return agentJSON(c, 422, "invalid_request")
		}
		approved, err := h.approve(c.Context(), b, code)
		if err != nil {
			return actionFailure(c, err)
		}
		if !approved {
			return agentJSON(c, 404, "not_found")
		}
	default:
		return agentJSON(c, 422, "invalid_request")
	}
	return c.JSON(fiber.Map{fieldStatus: "connected", propGrants: h.grants.connections(b.id)})
}

func (h *actionHandler) approve(ctx context.Context, browser actionBrowser, code string) (bool, error) {
	var deadline time.Time
	release := func() {}
	defer func() { release() }()
	err := h.app.TxManager().RunInTx(ctx, func(ctx context.Context) error {
		var ready bool
		ready, release = h.fences.acquire(browser.id)
		if !ready {
			return errActionTransition
		}
		var err error
		deadline, err = h.sessionDeadline(ctx, browser.session)
		return err
	})
	if err != nil {
		return false, err
	}
	grant := actionGrant{actor: browser.actor, browser: browser.id, session: browser.session, deadline: deadline}
	return h.grants.approve(code, grant), nil
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
	if h.action.LoadReview != nil && r.State == actionPending {
		r, err = h.loadBrowserReview(c.Context(), b, r.RequestID)
		if err != nil {
			return actionFailure(c, err)
		}
	}
	result, err := h.resultFields(r)
	if err != nil {
		return actionFailure(c, err)
	}
	return h.app.Render(c, "agent/ActionReview", map[string]any{
		propTitle: h.action.Title, "confirmLabel": h.action.ConfirmLabel, propDescription: h.action.Description,
		"path": h.action.Path + "/requests/" + id, "requestID": id, fieldStatus: r.State, "expiresAt": r.ExpiresAt,
		"recordID": r.Snapshot.RecordID, "revision": r.Snapshot.Revision, "fields": r.Snapshot.Fields, "result": result,
		"receiptLink": h.receiptLink(c.Context(), b.actor, r),
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
	var grant actionGrant
	release := func() {}
	defer func() { release() }()
	recheck := func(ctx context.Context) (time.Time, error) {
		deadline, err := h.checkGrantInTx(ctx, grant)
		if errors.Is(err, host.ErrReadSessionEnded) || errors.Is(err, host.ErrReadSessionDenied) {
			return time.Time{}, errActionExpired
		}
		return deadline, err
	}
	authorize := func(ctx context.Context, _ ActionTx, r actionRequest) error {
		var ready bool
		ready, release = h.fences.acquire(b.id)
		if !ready {
			return errActionTransition
		}
		var ok bool
		grant, ok = h.grants.bound(r.BindingID)
		if !ok {
			return errActionExpired
		}
		if grant.browser != b.id || grant.actor != b.actor {
			return errActionNotFound
		}
		_, err := recheck(ctx)
		return err
	}
	r, err := h.store.decide(c.Context(), b.actor, id, in.Decision == decisionApprove, authorize, recheck)
	release()
	release = func() {}
	if err != nil {
		return actionFailure(c, err)
	}
	out, err := h.wireResult(r)
	if err != nil {
		return actionFailure(c, err)
	}
	delete(out, receiptSchemaKey) // Only the stdio transport negotiates receipt schemas.
	out["receiptLink"] = h.receiptLink(c.Context(), b.actor, r)
	return c.JSON(out)
}

func (h *actionHandler) receiptLink(ctx context.Context, actor host.Actor, r actionRequest) map[string]string {
	link := h.action.ReceiptLink
	if link == nil || r.State != actionSucceeded {
		return nil
	}
	p := link.Permission
	if !h.app.Unwrap().RolesService().AdminCan(ctx, actor.AdminID, p.Domain, p.Action) {
		return nil
	}
	return map[string]string{"href": link.Href, "label": link.Label}
}
