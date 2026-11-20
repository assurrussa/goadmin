package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

const (
	agentPath   = "/agent/product-events"
	connectPath = agentPath + "/connect"
)

type journalAgent struct {
	app    *host.App
	reader crud.ReadOnlyReader
	grants *journalGrants
	origin string
}

func journalAdapter(origin string) crud.ReadOnlyAdapter {
	return crud.ReadOnlyAdapter{BasePath: agentPath, Build: func(app *host.App, reader crud.ReadOnlyReader) (*host.Extension, error) {
		h := &journalAgent{app: app, reader: reader, grants: newJournalGrants(), origin: origin}
		return host.NewExtension().WithPublicRegister(h.registerDevice).WithRegister(h.registerBrowser), nil
	}}
}

func agentJSON(c fiber.Ctx, status int, code string) error {
	return c.Status(status).JSON(fiber.Map{"error": code})
}

func (h *journalAgent) registerDevice(app *fiber.App) {
	// Register only the four device routes before browser authentication; the
	// confirmation page below must pass the full admin middleware stack.
	for _, route := range []struct {
		path    string
		handler fiber.Handler
	}{
		{"/pair", h.start}, {"/token", h.token}, {"/search", h.search}, {"/get", h.get},
	} {
		app.Post(agentPath+route.path, h.deviceOnly, route.handler)
	}
}

func (h *journalAgent) deviceOnly(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	origin, err := url.Parse(h.origin)
	if err != nil || !strings.EqualFold(c.Get("Host"), origin.Host) || c.Get("Origin") != "" || c.Get("Cookie") != "" || c.Get("Sec-Fetch-Site") != "" {
		return agentJSON(c, 403, "denied")
	}
	if len(c.Body()) > 4096 {
		return agentJSON(c, 413, "invalid_request")
	}
	return c.Next()
}

func readAgentJSON(c fiber.Ctx, dest any) bool {
	media, _, _ := mime.ParseMediaType(c.Get("Content-Type"))
	if media != "application/json" || len(c.Body()) > 4096 || !bytes.HasPrefix(bytes.TrimSpace(c.Body()), []byte("{")) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(c.Body()))
	d.DisallowUnknownFields()
	return d.Decode(dest) == nil && errors.Is(d.Decode(new(any)), io.EOF)
}

func (h *journalAgent) start(c fiber.Ctx) error {
	if len(c.Body()) != 0 && !readAgentJSON(c, &struct{}{}) {
		return agentJSON(c, 400, "invalid_request")
	}
	code, secret, ok := h.grants.start()
	if !ok {
		return agentJSON(c, 429, "too_many_requests")
	}
	return c.JSON(fiber.Map{"code": code, "device_secret": secret, "verification_url": h.origin + connectPath, "expires_in": int(pairLifetime.Seconds())})
}

func (h *journalAgent) token(c fiber.Ctx) error {
	var request struct {
		Secret string `json:"device_secret"`
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

func (h *journalAgent) actor(c fiber.Ctx) (context.Context, host.Actor, error) {
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
	ctx, actor, err := h.app.ResolveDelegatedRead(c.Context(), grant.binding)
	if err == nil && actor != grant.owner {
		return nil, host.Actor{}, host.ErrReadSessionEnded
	}
	return ctx, actor, err
}

func agentFailure(c fiber.Ctx, err error) error {
	var fields crud.FieldErrors
	switch {
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

func validJournalID(id string) bool {
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
}

func (h *journalAgent) search(c fiber.Ctx) error {
	ctx, actor, err := h.actor(c)
	if err != nil {
		return agentFailure(c, err)
	}
	request := struct {
		Product string `json:"product_id"`
		Action  string `json:"action"`
		Page    int    `json:"page"`
		Limit   int    `json:"page_size"`
	}{Page: 1, Limit: 25}
	if !readAgentJSON(c, &request) || !validJournalID(request.Product) || request.Limit < 1 || request.Limit > 100 || request.Page < 1 || request.Page > math.MaxInt/request.Limit {
		return agentJSON(c, 422, "invalid_request")
	}
	filters := datagrid.Filters{Page: request.Page, Limit: request.Limit, Fields: map[string]any{"productId": request.Product}}
	if request.Action != "" {
		filters.Fields["action"] = request.Action
	}
	rows, total, err := h.reader.List(ctx, actor, filters)
	if err != nil {
		return agentFailure(c, err)
	}
	items := make([]fiber.Map, 0, len(rows))
	for _, row := range rows {
		items = append(items, eventSummary(row))
	}
	return c.JSON(fiber.Map{"rows": items, "total": total})
}

func eventSummary(row crud.ReadOnlyRecord[map[string]any]) fiber.Map {
	return fiber.Map{"id": row.ID, "productId": row.Data["productId"], "action": row.Data["action"], "createdAt": row.Data["createdAt"]}
}

func (h *journalAgent) get(c fiber.Ctx) error {
	ctx, actor, err := h.actor(c)
	if err != nil {
		return agentFailure(c, err)
	}
	var request struct {
		ID string `json:"event_id"`
	}
	if !readAgentJSON(c, &request) || !validJournalID(request.ID) {
		return agentJSON(c, 422, "invalid_request")
	}
	row, err := h.reader.Get(ctx, actor, request.ID)
	if err != nil {
		return agentFailure(c, err)
	}
	result := eventSummary(row)
	result["reason"] = row.Data["reason"]
	return c.JSON(result)
}

func (h *journalAgent) registerBrowser(app *fiber.App) {
	group := app.Group(connectPath, h.app.Guard("product_events", "read"))
	group.Get("", h.connect)
	group.Post("", h.decide)
}

func (h *journalAgent) connect(c fiber.Ctx) error {
	actor, ok := h.app.CurrentActor(c)
	if !ok {
		return fiber.ErrUnauthorized
	}
	c.Set("Cache-Control", "no-store")
	c.Set("Referrer-Policy", "no-referrer")
	return h.app.Render(c, "agent/ProductEventsAccess", map[string]any{"grants": h.grants.active(actor)})
}

func (h *journalAgent) decide(c fiber.Ctx) error {
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
		binding, err := h.app.BindDelegatedRead(c)
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
