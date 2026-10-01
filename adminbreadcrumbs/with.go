package adminbreadcrumbs

import (
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
)

// With overrides the shared admin breadcrumbs for a single request.
// It gates breadcrumb links by the current admin's allowed menu routes.
func With(c fiber.Ctx, app *adminapp.App, extra menu.Menu, crumbs []Breadcrumb) {
	if app == nil || app.HTTPManager() == nil {
		return
	}

	admin := adminmiddleware.GetAdminAuth(c)
	if admin == nil {
		app.HTTPManager().WithProp(c, "adminBreadcrumbs", crumbs)
		return
	}

	adminMenu := menu.BuildAdminMenu(c, admin, app.RolesService(), extra)
	allowed := AllowedHrefs(adminMenu)
	filtered := FilterBreadcrumbs(crumbs, allowed)

	// Keep the dashboard root at the start when it is available and allowed.
	if root, ok := RootBreadcrumb(adminMenu); ok {
		if len(filtered) == 0 || filtered[0].Href != root.Href {
			if _, allowedRoot := allowed[root.Href]; allowedRoot {
				filtered = append([]Breadcrumb{root}, filtered...)
			}
		}
	}

	app.HTTPManager().WithProp(c, "adminBreadcrumbs", filtered)
}
