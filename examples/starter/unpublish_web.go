package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
)

const (
	unpublishModuleKey   = "product-unpublish-mcp"
	unpublishAgentPath   = "/agent/product-unpublish"
	unpublishConnectPath = unpublishAgentPath + "/connect"
	unpublishRequestPath = unpublishAgentPath + "/requests/"
	unpublishMaxBody     = 8 << 10
)

var (
	unpublishCodePattern = regexp.MustCompile(`^[A-Z2-7]{5}-[A-Z2-7]{5}$`)
	errUnpublishSession  = errors.New("product unpublish connection ended")
	errUnpublishDenied   = errors.New("product unpublish permission denied")
)

type unpublishWeb struct {
	repo         productRepo
	rbac         *rbac.Service
	csrf         *host.CSRFService
	origin       *url.URL
	trustedProxy netip.Addr
	sessionName  string
	csrfName     string
	csrfTTL      time.Duration
}

func productUnpublishModule(db *sql.DB, origin *url.URL, trustedProxy netip.Addr, csrf *host.CSRFService, cfg host.AdminConfig) (host.Module, error) {
	loopback := origin.Hostname() == "localhost"
	if ip, err := netip.ParseAddr(origin.Hostname()); err == nil {
		loopback = ip.IsLoopback()
	}
	if origin.User != nil || origin.Hostname() == "" || origin.RawQuery != "" || origin.ForceQuery ||
		origin.Fragment != "" || origin.RawPath != "" || (origin.Path != "" && origin.Path != "/") {
		return host.Module{}, errors.New("product-unpublish-mcp: ADMIN_URL must be an absolute origin")
	}
	if origin.Scheme != "https" && !(origin.Scheme == "http" && loopback) {
		return host.Module{}, errors.New("product-unpublish-mcp: ADMIN_URL requires HTTPS or loopback HTTP")
	}
	canonical := *origin
	canonical.Path = ""
	rbacService, err := postgres.NewRBAC(db, nil)
	if err != nil {
		return host.Module{}, err
	}
	web := unpublishWeb{
		repo: productRepo{db}, rbac: rbacService, csrf: csrf, origin: &canonical, trustedProxy: trustedProxy,
		sessionName: cfg.SessionName, csrfName: cfg.CSRFTokenName, csrfTTL: cfg.CSRFTokenTTL,
	}
	return host.DefinedFeatureModule(host.ModuleDescriptor{
		Key: unpublishModuleKey, Requires: []string{"crud.products"}, RouteNamespaces: []string{unpublishAgentPath},
	}, web), nil
}

func (w unpublishWeb) Descriptor() host.FeatureDescriptor {
	return host.FeatureDescriptor{Key: unpublishModuleKey}
}

func (w unpublishWeb) Build(host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		h := &unpublishHandler{unpublishWeb: w, app: app, pairings: newUnpublishPairings()}
		return host.NewExtension().WithMiddlewares(h.nativeTransitions).
			WithPublicRegister(h.registerDevice).WithRegister(h.registerBrowser), nil
	}
}

type unpublishHandler struct {
	unpublishWeb
	app      *host.App
	pairings *unpublishPairings

	fences unpublishLoginFences
}

func (h *unpublishHandler) registerDevice(app *fiber.App) {
	group := app.Group(unpublishAgentPath)
	group.Post("/pair", h.deviceOnly, h.startPairing)
	group.Post("/token", h.deviceOnly, h.exchangePairing)
	group.Post("/propose", h.deviceOnly, h.propose)
	group.Post("/status", h.deviceOnly, h.status)
}

func unpublishJSONError(c fiber.Ctx, status int, code string) error {
	return c.Status(status).JSON(fiber.Map{"error": code})
}

func (h *unpublishHandler) deviceOnly(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	headers := &c.Request().Header
	if !strings.EqualFold(c.Get("Host"), h.origin.Host) ||
		len(headers.PeekAll("Origin")) > 0 || len(headers.PeekAll("Cookie")) > 0 ||
		len(headers.PeekAll("Sec-Fetch-Site")) > 0 {
		return unpublishJSONError(c, http.StatusForbidden, "denied")
	}
	if len(c.Request().Body()) > unpublishMaxBody {
		return unpublishJSONError(c, http.StatusRequestEntityTooLarge, "invalid_request")
	}
	return c.Next()
}

func decodeUnpublishJSON(c fiber.Ctx, dest any) bool {
	media, _, _ := mime.ParseMediaType(c.Get("Content-Type"))
	body := c.Request().Body()
	if media != "application/json" || c.Get("Content-Encoding") != "" || len(body) > unpublishMaxBody ||
		!bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	return d.Decode(dest) == nil && errors.Is(d.Decode(new(any)), io.EOF)
}

func (h *unpublishHandler) startPairing(c fiber.Ctx) error {
	if len(c.Request().Body()) != 0 && !decodeUnpublishJSON(c, &struct{}{}) {
		return unpublishJSONError(c, http.StatusBadRequest, "invalid_request")
	}
	source, ok := h.pairingSource(c)
	if !ok {
		return unpublishJSONError(c, http.StatusBadRequest, "invalid_request")
	}
	code, secret, ok := h.pairings.start(source)
	if !ok {
		return unpublishJSONError(c, http.StatusTooManyRequests, "too_many_requests")
	}
	return c.JSON(fiber.Map{
		"code": code, "device_secret": secret, "verification_url": h.origin.String() + unpublishConnectPath,
		"expires_in": int(unpublishPairTTL.Seconds()),
	})
}

func (h *unpublishHandler) pairingSource(c fiber.Ctx) (netip.Addr, bool) {
	peer, ok := netip.AddrFromSlice(c.RequestCtx().RemoteIP())
	if !ok {
		return netip.Addr{}, false
	}
	if peer = peer.Unmap(); peer != h.trustedProxy {
		return peer, true
	}
	values := c.Request().Header.PeekAll("X-Real-IP")
	if len(values) != 1 {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(string(values[0]))
	ip = ip.Unmap()
	return ip, err == nil && ip.Zone() == "" && !ip.IsUnspecified() && !ip.IsMulticast()
}

func (h *unpublishHandler) exchangePairing(c fiber.Ctx) error {
	var request struct {
		Secret string `json:"device_secret"`
	}
	if !decodeUnpublishJSON(c, &request) || request.Secret == "" {
		return unpublishJSONError(c, http.StatusBadRequest, "invalid_request")
	}
	grant, token, seconds, status := h.pairings.exchange(request.Secret)
	switch status {
	case http.StatusOK:
		if err := h.checkCurrentGrant(c.Context(), grant); err != nil {
			h.pairings.revoke(grant.browser, grant.id)
			if errors.Is(err, errUnpublishSession) || errors.Is(err, errUnpublishDenied) {
				return unpublishJSONError(c, http.StatusForbidden, "denied")
			}
			return unpublishFailure(c, err)
		}
		return c.JSON(fiber.Map{"access_token": token, "expires_in": seconds})
	case http.StatusAccepted:
		return unpublishJSONError(c, status, "authorization_pending")
	default:
		return unpublishJSONError(c, status, "expired")
	}
}

func (h *unpublishHandler) currentPermission(ctx context.Context, actor host.Actor) error {
	subject, err := goauth.ParseSubjectID(actor.SubjectID)
	if err != nil {
		return errUnpublishDenied
	}
	allowed, err := h.rbac.Check(ctx, subject, rbac.PermissionKey("products:unpublish"))
	if err != nil {
		return err
	}
	if !allowed {
		return errUnpublishDenied
	}
	return nil
}

type unpublishPolicyContext struct{ context.Context }

func (h *unpublishHandler) checkGrant(ctx context.Context, grant unpublishGrant) error {
	if grant.id == "" || !h.pairings.now().Before(grant.deadline) {
		return errUnpublishSession
	}
	// ResolveDelegatedRead returns a derived policy context when the native
	// session carries role preview. This starter denies all preview modes.
	// Use a comparable context as the baseline, independent of the caller.
	baseline := &unpublishPolicyContext{Context: ctx}
	policy, actor, err := h.app.ResolveDelegatedRead(baseline, grant.session)
	switch {
	case errors.Is(err, host.ErrReadSessionEnded):
		return errUnpublishSession
	case errors.Is(err, host.ErrReadSessionDenied):
		return errUnpublishDenied
	case err != nil:
		return err
	case actor != grant.actor:
		return errUnpublishSession
	case policy != baseline:
		return errUnpublishDenied
	}
	return h.currentPermission(ctx, actor)
}

func (h *unpublishHandler) bearerGrant(c fiber.Ctx) (unpublishGrant, error) {
	values := c.Request().Header.PeekAll("Authorization")
	if len(values) != 1 {
		return unpublishGrant{}, errUnpublishSession
	}
	token, ok := strings.CutPrefix(string(values[0]), "Bearer ")
	if !ok || token == "" {
		return unpublishGrant{}, errUnpublishSession
	}
	grant, ok := h.pairings.bearer(token)
	if !ok {
		return unpublishGrant{}, errUnpublishSession
	}
	return grant, h.checkCurrentGrant(c.Context(), grant)
}

func (h *unpublishHandler) checkCurrentGrant(ctx context.Context, grant unpublishGrant) error {
	active, ok := h.pairings.bound(grant.id)
	if !ok || active.actor != grant.actor {
		return errUnpublishSession
	}
	err := h.checkGrant(ctx, active)
	if errors.Is(err, errUnpublishSession) || errors.Is(err, errUnpublishDenied) {
		h.pairings.revoke(active.browser, active.id)
	}
	return err
}

type unpublishOutput struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	ReviewURL string `json:"review_url,omitempty"`
	EventID   string `json:"event_id,omitempty"`
}

func (h *unpublishHandler) recheckExpired(ctx context.Context, actor host.Actor, requestID string) (unpublishRequest, error) {
	// A decision may have begun before the original authority ended. Check
	// again through the store's row-lock-aware lookup before reporting expiry.
	current, err := h.repo.lookupUnpublish(ctx, actor, requestID)
	if err == nil && current.State == unpublishPending {
		current = unpublishRequest{RequestID: requestID, State: unpublishExpired}
	}
	return current, err
}

func (h *unpublishHandler) sendResult(c fiber.Ctx, actor host.Actor, request unpublishRequest) error {
	if request.State == unpublishPending {
		grant, _ := h.pairings.bound(request.BindingID)
		err := h.checkCurrentGrant(c.Context(), grant)
		if errors.Is(err, errUnpublishSession) || errors.Is(err, errUnpublishDenied) {
			request, err = h.recheckExpired(c.Context(), actor, request.RequestID)
		}
		if err != nil {
			return unpublishFailure(c, err)
		}
	}
	out := unpublishOutput{RequestID: request.RequestID, Status: request.State, EventID: request.EventID}
	if out.Status == unpublishPending {
		out.ReviewURL = h.origin.String() + unpublishRequestPath + request.RequestID
	}
	return c.JSON(out)
}

func (h *unpublishHandler) propose(c fiber.Ctx) error {
	grant, err := h.bearerGrant(c)
	if err != nil {
		return unpublishFailure(c, err)
	}
	var body struct {
		RequestID string `json:"request_id"`
		ProductID string `json:"product_id"`
		Reason    string `json:"reason"`
	}
	if !decodeUnpublishJSON(c, &body) || !canonicalUnpublishID(body.RequestID) {
		return unpublishJSONError(c, http.StatusUnprocessableEntity, "invalid_request")
	}
	release := func() {}
	result, err := h.repo.proposeUnpublish(c.Context(), grant.actor, unpublishProposal{
		RequestID: body.RequestID, ProductID: body.ProductID, Reason: body.Reason, BindingID: grant.id, ExpiresAt: grant.deadline,
	}, func(ctx context.Context) error {
		release = h.fences.acquire(grant.browser)
		return h.checkCurrentGrant(ctx, grant)
	})
	release()
	if err != nil {
		return unpublishFailure(c, err)
	}
	return h.sendResult(c, grant.actor, result)
}

func (h *unpublishHandler) status(c fiber.Ctx) error {
	grant, err := h.bearerGrant(c)
	if err != nil {
		return unpublishFailure(c, err)
	}
	var body struct {
		RequestID string `json:"request_id"`
	}
	if !decodeUnpublishJSON(c, &body) || !canonicalUnpublishID(body.RequestID) {
		return unpublishJSONError(c, http.StatusUnprocessableEntity, "invalid_request")
	}
	result, err := h.repo.lookupUnpublish(c.Context(), grant.actor, body.RequestID)
	if err != nil {
		return unpublishFailure(c, err)
	}
	return h.sendResult(c, grant.actor, result)
}

func canonicalUnpublishID(raw string) bool {
	id, err := unpublishRequestID(raw)
	return err == nil && id == raw
}

func unpublishFailure(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, errUnpublishSession), errors.Is(err, errUnpublishExpired), errors.Is(err, goauth.ErrSessionRevoked):
		return unpublishJSONError(c, http.StatusUnauthorized, "authorization_required")
	case errors.Is(err, errUnpublishDenied), errors.Is(err, errUnpublishUnauthorized):
		return unpublishJSONError(c, http.StatusForbidden, "denied")
	case errors.Is(err, errUnpublishInvalid):
		return unpublishJSONError(c, http.StatusUnprocessableEntity, "invalid_request")
	case errors.Is(err, errUnpublishNotFound):
		return unpublishJSONError(c, http.StatusNotFound, "not_found")
	case errors.Is(err, errUnpublishConflict):
		return unpublishJSONError(c, http.StatusConflict, "conflict")
	case errors.Is(err, errUnpublishUnknown):
		return unpublishJSONError(c, http.StatusServiceUnavailable, "outcome_unknown")
	default:
		return unpublishJSONError(c, http.StatusServiceUnavailable, "unavailable")
	}
}
