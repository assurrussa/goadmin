// Package presentation carries optional, display-only request projections.
package presentation

import "github.com/gofiber/fiber/v3"

type Fields struct {
	Name bool `json:"name"`
	Login bool `json:"login"`
	Avatar bool `json:"avatar"`
}

type View struct {
	Enabled bool `json:"enabled"`
	Version string `json:"version"`
	Fields Fields `json:"fields"`
	Name string `json:"name"`
	Login string `json:"login"`
	AvatarURL string `json:"avatarUrl"`
	UpdatedAt string `json:"updatedAt"`
	Available bool `json:"available"`
}

type resolverKey struct{}

// Set is installed only by the explicitly mounted feature.
func Set(c fiber.Ctx, resolve func() (View, error)) { c.Locals(resolverKey{}, resolve) }

// Current does not authenticate or modify canonical profile data.
func Current(c fiber.Ctx) (View, error) {
	resolve, ok := c.Locals(resolverKey{}).(func() (View, error))
	if !ok { return View{}, nil }
	return resolve()
}
