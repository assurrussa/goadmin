package appearance

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	middleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	"github.com/assurrussa/goadmin/internal/adminpreferences"
	core "github.com/assurrussa/goadmin/internal/appearance"
	"github.com/assurrussa/goadmin/internal/auth"
)

type Handler struct {
	app     *adminapp.App
	service *core.Service
}

func NewHandler(app *adminapp.App, service *core.Service) *Handler {
	return &Handler{app: app, service: service}
}

// RegisterRoutes registers public published assets before protected route groups.
func (h *Handler) RegisterRoutes(route fiber.Router, _ ...fiber.Handler) {
	if h.service == nil {
		return
	}
	route.Get("/public/branding/:variant", h.asset)
}

func (h *Handler) RegisterGroupRoutes(route fiber.Router, handlers ...fiber.Handler) {
	if h.service == nil {
		return
	}
	chain := []any{middleware.IsNotAuthAdminMiddleware()}
	for _, handler := range handlers {
		chain = append(chain, handler)
	}
	endpoint := func(extra ...fiber.Handler) []any {
		result := append([]any{}, chain[1:]...)
		for _, handler := range extra {
			result = append(result, handler)
		}
		return result
	}
	route.Get("/auth/profile/appearance", chain[0], endpoint(api(h.getPersonal))...)
	route.Patch("/auth/profile/appearance", chain[0], endpoint(api(h.savePersonal))...)
	readGuard := h.app.Guard(auth.PermissionDomainBranding, auth.PermissionActionRead)
	route.Get("/auth/branding", chain[0], endpoint(readGuard, api(h.getBranding))...)
	writeGuard := h.app.Guard(auth.PermissionDomainBranding, auth.PermissionActionUpdate)
	route.Put("/auth/branding", chain[0], endpoint(writeGuard, api(h.saveBranding))...)
}

func (h *Handler) asset(c fiber.Ctx) error {
	raw, etag, ok := h.service.Asset(c.Params("variant"))
	if !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}
	c.Set("Cache-Control", "public, no-cache")
	c.Set("ETag", etag)
	c.Set("X-Content-Type-Options", "nosniff")
	if c.Get("If-None-Match") == etag {
		return c.SendStatus(fiber.StatusNotModified)
	}
	c.Set("Content-Type", "image/png")
	return c.Send(raw)
}

func (h *Handler) getPersonal(c fiber.Ctx) error {
	owner := middleware.GetAdminAuth(c)
	if owner == nil {
		return fiber.ErrUnauthorized
	}
	doc, err := h.service.LoadPersonal(c, owner.ID)
	if err != nil {
		return err
	}
	return c.JSON(doc)
}

func (h *Handler) savePersonal(c fiber.Ctx) error {
	owner := middleware.GetAdminAuth(c)
	if owner == nil {
		return fiber.ErrUnauthorized
	}
	if auth.RolePreviewActive(c) {
		return fiber.ErrForbidden
	}
	var req core.PersonalRequest
	if err := decode(c, &req, 4096); err != nil {
		return err
	}
	doc, err := h.service.SavePersonal(c, owner.ID, req)
	if err != nil {
		return err
	}
	return c.JSON(doc)
}

func (h *Handler) getBranding(c fiber.Ctx) error {
	doc, err := h.service.LoadBranding(c)
	if err != nil {
		return err
	}
	return c.JSON(doc)
}

func (h *Handler) saveBranding(c fiber.Ctx) error {
	owner := middleware.GetAdminAuth(c)
	if owner == nil {
		return fiber.ErrUnauthorized
	}
	if auth.RolePreviewActive(c) {
		return fiber.ErrForbidden
	}
	var req core.SaveBrandingRequest
	if err := decode(c, &req, 2*1024*1024); err != nil {
		return err
	}
	doc, err := h.service.SaveBranding(c, owner.ID, req)
	if err != nil {
		return err
	}
	return c.JSON(doc)
}

func decode(c fiber.Ctx, dest any, limit int) error {
	if len(c.Body()) > limit {
		return fiber.ErrRequestEntityTooLarge
	}
	d := json.NewDecoder(bytes.NewReader(c.Body()))
	d.DisallowUnknownFields()
	if d.Decode(dest) != nil {
		return fiber.ErrBadRequest
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fiber.ErrBadRequest
	}
	return nil
}

func api(handler fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		err := handler(c)
		if err == nil {
			return nil
		}
		status := fiber.StatusInternalServerError
		message := "Unable to complete appearance request"
		var e *fiber.Error
		switch {
		case errors.Is(err, adminpreferences.ErrConflict):
			status = fiber.StatusConflict
			message = "Appearance changed; reload before saving"
		case errors.Is(err, core.ErrInvalid):
			status = fiber.StatusBadRequest
			message = "Invalid appearance request"
		case errors.As(err, &e):
			switch e.Code {
			case 400, 401, 403, 413:
				status = e.Code
				message = e.Message
			}
		}
		_ = c.Status(status).JSON(fiber.Map{"message": message})
		return nil
	}
}
