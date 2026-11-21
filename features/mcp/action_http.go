package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
)

type actionHandler struct {
	app         *host.App
	action      Action
	store       actionStore
	origin      *url.URL
	sessionName string
	device      resourceHandler
	grants      *actionPairings
	fences      actionLoginFences
}

func (h *actionHandler) registerDevice(app *fiber.App) {
	path := h.action.Path
	app.Post(path+"/describe", h.device.deviceOnly, func(c fiber.Ctx) error { return c.JSON(h.action.describe()) })
	app.Post(path+"/pair", h.device.deviceOnly, h.pair)
	app.Post(path+"/token", h.device.deviceOnly, h.token)
	app.Post(path+"/propose", h.device.deviceOnly, h.propose)
	app.Post(path+"/status", h.device.deviceOnly, h.status)
}

func (h *actionHandler) pair(c fiber.Ctx) error {
	if !readAgentJSON(c, &struct{}{}) {
		return agentJSON(c, 400, "invalid_request")
	}
	source, ok := h.device.pairingSource(c)
	if !ok {
		return agentJSON(c, 400, "invalid_request")
	}
	code, secret, ok := h.grants.start(source)
	if !ok {
		return agentJSON(c, 429, "too_many_requests")
	}
	return c.JSON(fiber.Map{
		fieldCode: code, fieldDeviceSecret: secret, fieldVerificationURL: h.origin.String() + h.action.Path + "/connect",
		fieldExpiresIn: int(actionPairTTL.Seconds()),
	})
}

func (h *actionHandler) token(c fiber.Ctx) error {
	var in struct {
		Secret string `json:"device_secret"` //nolint:tagliatelle // Existing device protocol.
	}
	if !readAgentJSON(c, &in) || in.Secret == "" {
		return agentJSON(c, 422, "invalid_request")
	}
	grant, token, seconds, status := h.grants.exchange(in.Secret)
	switch status {
	case http.StatusOK:
		if err := h.checkGrant(c.Context(), grant); err != nil {
			return actionFailure(c, err)
		}
		return c.JSON(fiber.Map{"access_token": token, fieldExpiresIn: seconds})
	case http.StatusAccepted:
		return agentJSON(c, status, "authorization_pending")
	default:
		return agentJSON(c, status, "expired")
	}
}

func (h *actionHandler) checkGrant(ctx context.Context, g actionGrant) error {
	if _, ok := h.grants.bound(g.id); !ok {
		return host.ErrReadSessionEnded
	}
	err := h.checkSession(ctx, g.session)
	if errors.Is(err, host.ErrReadSessionEnded) || errors.Is(err, host.ErrReadSessionDenied) {
		h.grants.revoke(g.browser, g.id)
	}
	return err
}

func (h *actionHandler) checkSession(ctx context.Context, binding host.ReadSessionBinding) error {
	return h.app.TxManager().RunInTx(ctx, func(txctx context.Context) error {
		return h.app.Unwrap().AdminAuth().CheckConfirmedAction(txctx, binding, h.action.Permissions)
	})
}

func (h *actionHandler) bearer(c fiber.Ctx) (actionGrant, error) {
	values := c.Request().Header.PeekAll("Authorization")
	if len(values) != 1 {
		return actionGrant{}, host.ErrReadSessionEnded
	}
	token, ok := strings.CutPrefix(string(values[0]), "Bearer ")
	if !ok {
		return actionGrant{}, host.ErrReadSessionEnded
	}
	g, ok := h.grants.bearer(token)
	if !ok {
		return actionGrant{}, host.ErrReadSessionEnded
	}
	return g, h.checkGrant(c.Context(), g)
}

func canonicalActionID(s string) bool { id, err := actionRequestID(s); return err == nil && id == s }

func (h *actionHandler) propose(c fiber.Ctx) error {
	g, err := h.bearer(c)
	if err != nil {
		return actionFailure(c, err)
	}
	var in map[string]json.RawMessage
	if !readAgentJSON(c, &in) {
		return agentJSON(c, 422, "invalid_request")
	}
	var id string
	if json.Unmarshal(in[fieldRequestID], &id) != nil || !canonicalActionID(id) {
		return agentJSON(c, 422, "invalid_request")
	}
	delete(in, fieldRequestID)
	release := func() {}
	defer func() { release() }()
	proposal := actionProposal{RequestID: id, Input: in, BindingID: g.id, ExpiresAt: g.deadline}
	r, err := h.store.propose(c.Context(), g.actor, proposal, func(ctx context.Context, _ ActionTx) error {
		release = h.fences.acquire(g.browser)
		return h.checkGrantInTx(ctx, g)
	})
	release()
	release = func() {}
	if err != nil {
		return actionFailure(c, err)
	}
	return h.sendResult(c, g.actor, r)
}

func (h *actionHandler) checkGrantInTx(ctx context.Context, g actionGrant) error {
	if _, ok := h.grants.bound(g.id); !ok {
		return host.ErrReadSessionEnded
	}
	err := h.app.Unwrap().AdminAuth().CheckConfirmedAction(ctx, g.session, h.action.Permissions)
	if errors.Is(err, host.ErrReadSessionEnded) || errors.Is(err, host.ErrReadSessionDenied) {
		h.grants.revoke(g.browser, g.id)
	}
	if err == nil && !h.grants.now().Before(g.deadline) {
		return host.ErrReadSessionEnded
	}
	return err
}

func (h *actionHandler) status(c fiber.Ctx) error {
	g, err := h.bearer(c)
	if err != nil {
		return actionFailure(c, err)
	}
	var in struct {
		ID string `json:"request_id"` //nolint:tagliatelle // Existing device protocol.
	}
	if !readAgentJSON(c, &in) || !canonicalActionID(in.ID) {
		return agentJSON(c, 422, "invalid_request")
	}
	r, err := h.store.status(c.Context(), g.actor, in.ID)
	if err != nil {
		return actionFailure(c, err)
	}
	return h.sendResult(c, g.actor, r)
}

func (h *actionHandler) liveRequest(ctx context.Context, actor host.Actor, r actionRequest) (actionRequest, error) {
	if r.State != actionPending {
		return r, nil
	}
	if g, ok := h.grants.bound(r.BindingID); ok {
		err := h.checkGrant(ctx, g)
		if err == nil {
			return r, nil
		}
		if !errors.Is(err, host.ErrReadSessionDenied) && !errors.Is(err, host.ErrReadSessionEnded) {
			return actionRequest{}, err
		}
	}
	// A confirmation may have committed after status read the row. Reconcile
	// under its lock before declaring an orphaned binding expired.
	return h.store.expire(ctx, actor, r.RequestID)
}

func (h *actionHandler) sendResult(c fiber.Ctx, actor host.Actor, r actionRequest) error {
	r, err := h.liveRequest(c.Context(), actor, r)
	if err != nil {
		return actionFailure(c, err)
	}
	return c.JSON(h.wireResult(r))
}

func (h *actionHandler) wireResult(r actionRequest) fiber.Map {
	out := fiber.Map{fieldRequestID: r.RequestID, fieldStatus: r.State}
	if r.State == actionPending {
		out[fieldReviewURL] = h.origin.String() + h.action.Path + "/requests/" + r.RequestID
		out[fieldExpiresAt] = r.ExpiresAt
	}
	for key, value := range h.resultFields(r) {
		out[key] = value
	}
	return out
}

func (h *actionHandler) resultFields(r actionRequest) map[string]string {
	result := make(map[string]string, len(r.Result))
	for _, f := range h.action.ResultFields {
		if value, ok := r.Result[f.Key]; ok {
			result[f.Key] = value
		}
	}
	return result
}

func actionFailure(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, errActionExpired), errors.Is(err, host.ErrReadSessionEnded):
		return agentJSON(c, 401, "authorization_required")
	case errors.Is(err, errActionUnauthorized), errors.Is(err, host.ErrReadSessionDenied), errors.Is(err, crud.ErrDenied):
		return agentJSON(c, 403, problemDenied)
	case errors.Is(err, errActionInvalid), errors.Is(err, crud.ErrInvalidQuery):
		return agentJSON(c, 422, "invalid_request")
	case errors.Is(err, errActionNotFound), errors.Is(err, crud.ErrNotFound):
		return agentJSON(c, 404, "not_found")
	case errors.Is(err, errActionConflict), errors.Is(err, crud.ErrConflict):
		return agentJSON(c, 409, "conflict")
	case errors.Is(err, errActionUnknown):
		return agentJSON(c, 503, "outcome_unknown")
	default:
		return agentJSON(c, 503, "unavailable")
	}
}
