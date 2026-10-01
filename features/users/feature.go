package users

import (
	"errors"

	"github.com/assurrussa/goadmin/host"
	usershandler "github.com/assurrussa/goadmin/http/handlers/users"
)

type feature struct{}

const usersFeatureKey = "users"

// NewFeature creates the reusable admin users feature.
func NewFeature() host.Feature {
	return feature{}
}

func (feature) Descriptor() host.FeatureDescriptor {
	return host.FeatureDescriptor{
		Key: usersFeatureKey,
		Menu: host.Menu{
			Sections: []host.Section{{
				Key:   usersFeatureKey,
				Title: "Пользователи",
				Order: 30,
				Items: []host.Item{{
					Name:          "Пользователи",
					Href:          "/users",
					Icon:          usersFeatureKey,
					PermissionKey: host.NewPermissionKey(host.PermissionDomainUsers, host.PermissionActionRead),
				}},
			}},
		},
		Permissions: []host.PermissionKey{
			host.NewPermissionKey(host.PermissionDomainUsers, host.PermissionActionRead),
			host.NewPermissionKey(host.PermissionDomainUsers, host.PermissionActionUpdate),
			host.NewPermissionKey(host.PermissionDomainUsers, host.PermissionActionDelete),
		},
		PermissionDefinitions: []host.PermissionDefinition{
			host.NewPermissionDefinition(
				host.NewPermissionKey(host.PermissionDomainUsers, host.PermissionActionRead),
				"Просмотр пользователей",
			),
			host.NewPermissionDefinition(
				host.NewPermissionKey(host.PermissionDomainUsers, host.PermissionActionUpdate),
				"Редактирование пользователей",
			),
			host.NewPermissionDefinition(
				host.NewPermissionKey(host.PermissionDomainUsers, host.PermissionActionDelete),
				"Удаление и восстановление пользователей",
			),
		},
	}
}

func (feature) Build(host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		if app == nil || app.Unwrap() == nil {
			return nil, errors.New("users feature: admin app is required")
		}

		adminApp := app.Unwrap()
		userRepo := adminApp.UsersFeatureRepo()
		if userRepo == nil {
			return nil, errors.New("users feature: user repository is required")
		}
		subjects := adminApp.UsersFeatureSubjects()
		if subjects == nil {
			return nil, errors.New("users feature: subject store is required")
		}

		return host.NewExtension().WithHandlers(
			usershandler.NewHandler(adminApp, userRepo, subjects),
		), nil
	}
}
