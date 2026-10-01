package host

import (
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminbreadcrumbs"
)

// WithBreadcrumbs overrides breadcrumbs for the current request and filters links
// against the current admin's allowed menu routes.
func WithBreadcrumbs(c fiber.Ctx, app *App, extra Menu, crumbs []Breadcrumb) {
	if app == nil || app.inner == nil {
		return
	}

	internal := make([]adminbreadcrumbs.Breadcrumb, 0, len(crumbs))
	for _, crumb := range crumbs {
		internal = append(internal, adminbreadcrumbs.Breadcrumb{
			Name: crumb.Name,
			Href: crumb.Href,
		})
	}

	adminbreadcrumbs.With(c, app.inner, extra, internal)
}
