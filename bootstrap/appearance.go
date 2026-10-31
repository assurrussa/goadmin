package bootstrap

import (
	"context"
	"strconv"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	"github.com/assurrussa/goadmin/internal/appearance"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

func initAppearance(ctx context.Context, deps *Dependencies) (*appearance.Service, bool) {
	if deps.System.Appearance != nil {
		return deps.System.Appearance, false
	}
	if deps.System.GridDB == nil {
		return nil, false
	}
	service := appearance.NewService(deps.System.GridDB, deps.System.CommandTransaction,
		deps.System.AdminActionAudit, appearance.WithLogger(deps.System.Logger))
	deps.System.Appearance = service
	// The HTTP shutdown hook owns the cache. Parent Run cancellation must not
	// close it before active HTTP save/refresh operations have drained.
	if err := service.Start(context.WithoutCancel(ctx)); err != nil && deps.System.Logger != nil {
		deps.System.Logger.WarnContext(ctx, "Appearance cache initial load failed; using defaults", logger.Error(err))
	}
	return service, true
}

func closeAppearance(ctx context.Context, service *appearance.Service) error {
	if service == nil {
		return nil
	}
	// This is owned cleanup, not a request waiter: join accepted saves and the
	// cache even if an earlier Shutdown caller exhausted its waiting budget.
	return service.Close(context.WithoutCancel(ctx))
}

func appearanceSharedProps(deps Dependencies) map[string]any {
	service := deps.System.Appearance
	return map[string]any{
		"branding": goinertia.LazyProp{Key: "branding", Fn: func(context.Context) (any, error) {
			if service == nil {
				return appearance.PublicBranding{}, nil
			}
			return service.Brand(), nil
		}},
		"appearance": goinertia.LazyProp{Key: "appearance", Fn: func(ctx context.Context) (any, error) {
			admin := adminmiddleware.GetAdminAuth(ctx)
			if admin == nil || service == nil {
				return nil, nil
			}
			document, err := service.LoadPersonal(ctx, admin.ID)
			if err != nil {
				// Do not turn a failed read into defaults that an autosave can
				// overwrite. The client receives a verified owner and a fence.
				if deps.System.Logger != nil {
					deps.System.Logger.ErrorContext(ctx, "Appearance preferences unavailable", logger.Error(err))
				}
				return map[string]any{"owner": strconv.FormatInt(admin.ID, 10), "unavailable": true}, nil
			}
			return document, nil
		}},
		"appearanceAccess": goinertia.LazyProp{Key: "appearanceAccess", Fn: func(ctx context.Context) (any, error) {
			_, ok := ctx.(fiber.Ctx)
			admin := adminmiddleware.GetAdminAuth(ctx)
			if !ok || admin == nil {
				// Anonymous theme/locale controls only change device preferences.
				return map[string]bool{"brandingRead": false, "brandingUpdate": false, "readOnly": false}, nil
			}
			readOnly := integrationroles.RolePreviewActive(ctx)
			can := func(action integrationroles.PermissionAction) bool {
				return deps.Repos.RolesService != nil && deps.Repos.RolesService.AdminGuardCheck(ctx, admin.ID,
					integrationroles.NewPermissionKey("branding", action))
			}
			return map[string]bool{"brandingRead": can("read"), "brandingUpdate": !readOnly && can("update"), "readOnly": readOnly}, nil
		}},
	}
}
