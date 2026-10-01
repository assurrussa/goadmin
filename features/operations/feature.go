package operations

import (
	"github.com/assurrussa/goadmin/host"
)

type feature struct {
	routes          RoutesRegenerator
	sitemap         SitemapRegenerator
	frontendState   FrontendStateReader
	frontendRebuild FrontendRedeployer
}

// NewFeature creates the reusable admin operations feature.
func NewFeature(
	routes RoutesRegenerator,
	sitemap SitemapRegenerator,
	frontendState FrontendStateReader,
	frontendRebuild FrontendRedeployer,
) host.Feature {
	return feature{
		routes:          routes,
		sitemap:         sitemap,
		frontendState:   frontendState,
		frontendRebuild: frontendRebuild,
	}
}

func (feature) Descriptor() host.FeatureDescriptor {
	return host.FeatureDescriptor{
		Key: "operations",
		Menu: host.Menu{
			Sections: []host.Section{{
				Key: "system",
				Items: []host.Item{{
					Name:          "Операции", //nolint:goconst // required
					Href:          "/operations",
					Icon:          "queues",
					PermissionKey: host.NewPermissionKey(host.PermissionDomainOperations, host.PermissionActionRead),
				}},
			}},
		},
		Permissions: []host.PermissionKey{
			host.NewPermissionKey(host.PermissionDomainOperations, host.PermissionActionRead),
			host.NewPermissionKey(host.PermissionDomainOperations, host.PermissionActionUpdate),
		},
		PermissionDefinitions: []host.PermissionDefinition{
			host.NewPermissionDefinition(
				host.NewPermissionKey(host.PermissionDomainOperations, host.PermissionActionRead),
				"Просмотр служебных операций и состояния frontend rebuild",
			),
			host.NewPermissionDefinition(
				host.NewPermissionKey(host.PermissionDomainOperations, host.PermissionActionUpdate),
				"Запуск ручных операций и frontend redeploy",
			),
		},
	}
}

func (f feature) Build(siteMenu host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		return host.NewExtension().WithHandlers(
			NewHandler(app, f.routes, f.sitemap, f.frontendState, f.frontendRebuild, siteMenu),
		), nil
	}
}
