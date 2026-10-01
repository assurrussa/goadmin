package main

import (
	"context"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
)

type healthFeature struct {
	ready func(context.Context) error
}

func (healthFeature) Descriptor() host.FeatureDescriptor {
	return host.FeatureDescriptor{Key: "starter.health"}
}

func (h healthFeature) Build(host.Menu) host.ExtensionBuilder {
	return func(*host.App) (*host.Extension, error) {
		return host.NewExtension().WithPublicRegister(func(app *fiber.App) {
			app.Get("/healthz", func(c fiber.Ctx) error {
				if err := h.ready(c.Context()); err != nil {
					return c.SendStatus(fiber.StatusServiceUnavailable)
				}
				return c.SendStatus(fiber.StatusNoContent)
			})
		}), nil
	}
}
