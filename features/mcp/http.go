package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

type resourceHandler struct {
	app          *host.App
	resource     Resource
	reader       Reader
	resolveRead  func(context.Context, host.ReadSessionBinding) (context.Context, host.Actor, error)
	grants       *grantStore
	origin       string
	originHost   string
	trustedProxy netip.Addr
}

func newHandler(app *host.App, cfg Config, resource Resource, reader Reader) *resourceHandler {
	origin, _ := adminOrigin(cfg.Origin)
	return &resourceHandler{
		app: app, resource: resource, reader: reader, resolveRead: app.ResolveDelegatedRead, grants: newGrantStore(),
		origin: cfg.Origin, originHost: origin.Host, trustedProxy: cfg.TrustedProxy,
	}
}

func (h *resourceHandler) extension() *host.Extension {
	return host.NewExtension().WithPublicRegister(h.registerDevice).WithRegister(h.registerBrowser)
}

func (h *resourceHandler) connectPath() string { return h.resource.Path + "/connect" }

func agentJSON(c fiber.Ctx, status int, code string) error {
	return c.Status(status).JSON(fiber.Map{"error": code})
}

func (h *resourceHandler) registerDevice(app *fiber.App) {
	// These routes are mounted before browser authentication. Confirmation
	// runs under the full admin middleware stack.
	path := h.resource.Path
	app.Post(path+"/describe", h.deviceOnly, h.describe)
	app.Post(path+"/pair", h.deviceOnly, h.start)
	app.Post(path+"/token", h.deviceOnly, h.token)
	if h.resource.Search != nil {
		app.Post(path+"/search", h.deviceOnly, h.search)
	}
	if h.resource.Detail != nil {
		app.Post(path+"/get", h.deviceOnly, h.get)
	}
}

func (h *resourceHandler) describe(c fiber.Ctx) error {
	if len(c.Body()) != 0 && !readAgentJSON(c, &struct{}{}) {
		return agentJSON(c, http.StatusBadRequest, "invalid_request")
	}
	return c.JSON(h.resource.description())
}

func (h *resourceHandler) deviceOnly(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	if !strings.EqualFold(c.Get("Host"), h.originHost) ||
		c.Get("Origin") != "" || c.Get("Cookie") != "" || c.Get("Sec-Fetch-Site") != "" {
		return agentJSON(c, 403, "denied")
	}
	if len(c.Body()) > maxRequestSize {
		return agentJSON(c, 413, "invalid_request")
	}
	return c.Next()
}

func readAgentJSON(c fiber.Ctx, dest any) bool {
	media, _, _ := mime.ParseMediaType(c.Get("Content-Type"))
	if media != "application/json" || len(c.Body()) > maxRequestSize || !bytes.HasPrefix(bytes.TrimSpace(c.Body()), []byte("{")) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(c.Body()))
	d.DisallowUnknownFields()
	return d.Decode(dest) == nil && errors.Is(d.Decode(new(any)), io.EOF)
}

func (h *resourceHandler) start(c fiber.Ctx) error {
	if len(c.Body()) != 0 && !readAgentJSON(c, &struct{}{}) {
		return agentJSON(c, 400, "invalid_request")
	}
	source, ok := h.pairingSource(c)
	if !ok {
		return agentJSON(c, http.StatusBadRequest, "invalid_request")
	}
	code, secret, ok := h.grants.start(source)
	if !ok {
		return agentJSON(c, 429, "too_many_requests")
	}
	return c.JSON(fiber.Map{
		"code": code, "device_secret": secret,
		"verification_url": h.origin + h.connectPath(), "expires_in": int(pairLifetime.Seconds()),
	})
}

func (h *resourceHandler) token(c fiber.Ctx) error {
	var request struct {
		Secret string `json:"device_secret"` //nolint:tagliatelle // Existing device protocol.
	}
	if !readAgentJSON(c, &request) || request.Secret == "" {
		return agentJSON(c, 400, "invalid_request")
	}
	token, seconds, status := h.grants.exchange(request.Secret)
	switch status {
	case http.StatusOK:
		return c.JSON(fiber.Map{"access_token": token, "expires_in": seconds})
	case http.StatusAccepted:
		return agentJSON(c, status, "authorization_pending")
	case http.StatusForbidden:
		return agentJSON(c, status, "denied")
	default:
		return agentJSON(c, status, "expired")
	}
}

func (h *resourceHandler) actor(c fiber.Ctx) (context.Context, host.Actor, error) {
	headers := c.Request().Header.PeekAll("Authorization")
	if len(headers) != 1 {
		return nil, host.Actor{}, host.ErrReadSessionEnded
	}
	token, ok := strings.CutPrefix(string(headers[0]), "Bearer ")
	if !ok || token == "" {
		return nil, host.Actor{}, host.ErrReadSessionEnded
	}
	grant, ok := h.grants.lookup(token)
	if !ok {
		return nil, host.Actor{}, host.ErrReadSessionEnded
	}
	ctx, actor, err := h.resolveRead(c.Context(), grant.binding)
	if err != nil {
		return nil, host.Actor{}, err
	}
	if actor != grant.owner {
		return nil, host.Actor{}, host.ErrReadSessionEnded
	}
	permission := h.resource.Permission
	if !integrationroles.RolePreviewAllows(ctx, permission) ||
		!h.app.Unwrap().RolesService().AdminCan(ctx, actor.AdminID, permission.Domain, permission.Action) {
		return nil, host.Actor{}, host.ErrReadSessionDenied
	}
	return ctx, actor, nil
}

func agentFailure(c fiber.Ctx, err error) error {
	var fields crud.FieldErrors
	switch {
	case errors.Is(err, host.ErrExternalRecoveryRequired):
		return agentJSON(c, http.StatusConflict, "browser_refresh_required")
	case errors.Is(err, host.ErrReadSessionEnded):
		return agentJSON(c, 401, "authorization_required")
	case errors.Is(err, host.ErrReadSessionDenied), errors.Is(err, crud.ErrDenied):
		return agentJSON(c, 403, "denied")
	case errors.Is(err, crud.ErrNotFound):
		return agentJSON(c, 404, "not_found")
	case errors.Is(err, crud.ErrInvalidQuery), errors.As(err, &fields):
		return agentJSON(c, 422, "invalid_request")
	default:
		return agentJSON(c, 503, "unavailable")
	}
}

func (h *resourceHandler) search(c fiber.Ctx) error {
	ctx, actor, err := h.actor(c)
	if err != nil {
		return agentFailure(c, err)
	}
	var request map[string]json.RawMessage
	if !readAgentJSON(c, &request) {
		return agentJSON(c, http.StatusUnprocessableEntity, "invalid_request")
	}
	filters, ok := h.resource.Search.filters(request)
	if !ok {
		return agentJSON(c, http.StatusUnprocessableEntity, "invalid_request")
	}
	rows, total, err := h.reader.List(ctx, actor, filters)
	if err != nil {
		return agentFailure(c, err)
	}
	if len(rows) > filters.Limit || total < len(rows) {
		return agentJSON(c, http.StatusServiceUnavailable, "unavailable")
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item, ok := project(row, h.resource.Search.Fields)
		if !ok {
			return agentJSON(c, http.StatusServiceUnavailable, "unavailable")
		}
		items = append(items, item)
	}
	return c.JSON(searchOutput{Rows: items, Total: total})
}

func (h *resourceHandler) get(c fiber.Ctx) error {
	ctx, actor, err := h.actor(c)
	if err != nil {
		return agentFailure(c, err)
	}
	var request map[string]json.RawMessage
	if !readAgentJSON(c, &request) {
		return agentJSON(c, http.StatusUnprocessableEntity, "invalid_request")
	}
	id, ok := h.resource.Detail.recordID(request)
	if !ok {
		return agentJSON(c, http.StatusUnprocessableEntity, "invalid_request")
	}
	row, err := h.reader.Get(ctx, actor, id)
	if err != nil {
		return agentFailure(c, err)
	}
	result, ok := project(row, h.resource.Detail.Fields)
	if !ok || row.ID != id {
		return agentJSON(c, http.StatusServiceUnavailable, "unavailable")
	}
	return c.JSON(result)
}

// Projection validates each declared JSON scalar. Other Data values are never
// marshaled, so nested values and fields omitted from the declaration stay private.
func project(row Record, fields []Field) (map[string]any, bool) {
	if !validRecordID(row.ID) {
		return nil, false
	}
	out := make(map[string]any, len(fields)+1)
	out["id"] = row.ID
	for _, field := range fields {
		raw, err := json.Marshal(row.Data[field.Key])
		if err != nil {
			return nil, false
		}
		if field.Nullable && bytes.Equal(raw, []byte("null")) {
			out[field.Key] = nil
			continue
		}
		if _, ok := field.Kind.parse(raw); !ok {
			return nil, false
		}
		out[field.Key] = json.RawMessage(raw)
	}
	return out, true
}

func (h *resourceHandler) registerBrowser(app *fiber.App) {
	permission := h.resource.Permission
	group := app.Group(h.connectPath(), h.app.Guard(permission.Domain, permission.Action))
	group.Get("", h.connect)
	group.Post("", h.decide)
}

func (h *resourceHandler) connect(c fiber.Ctx) error {
	actor, ok := h.app.CurrentActor(c)
	if !ok {
		return fiber.ErrUnauthorized
	}
	consent := h.resource.Consent
	if consent.Title == "" {
		consent.Title = h.resource.Name
	}
	if consent.Description == "" {
		consent.Description = h.resource.Description
	}
	c.Set("Cache-Control", "no-store")
	c.Set("Referrer-Policy", "no-referrer")
	return h.app.Render(c, approvalPage, map[string]any{
		"grants": h.grants.active(actor), "connectPath": h.connectPath(), "title": consent.Title,
		"description": consent.Description, "backHref": consent.BackHref, "backText": consent.BackText,
	})
}

func (h *resourceHandler) decide(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	actor, ok := h.app.CurrentActor(c)
	if !ok || c.Get("Origin") != h.origin {
		return agentJSON(c, 403, "denied")
	}
	var request struct {
		Code     string `json:"code"`
		Decision string `json:"decision"`
	}
	if !readAgentJSON(c, &request) || len(request.Code) != 11 {
		return agentJSON(c, 422, "invalid_request")
	}
	code := strings.ToUpper(request.Code)
	var updated bool
	switch request.Decision {
	case "approve":
		bind := h.app.BindDelegatedRead
		if h.resource.AllowExternalSessions {
			bind = h.app.BindDelegatedReadWithExternal
		}
		binding, err := bind(c)
		if err != nil {
			return agentFailure(c, err)
		}
		updated = h.grants.decide(code, true, actor, binding)
	case "deny":
		updated = h.grants.decide(code, false, actor, host.ReadSessionBinding{})
	case "revoke":
		updated = h.grants.revoke(code, actor)
	default:
		return agentJSON(c, 422, "invalid_request")
	}
	if !updated {
		return agentJSON(c, 404, "not_found")
	}
	return c.JSON(fiber.Map{"grants": h.grants.active(actor)})
}
