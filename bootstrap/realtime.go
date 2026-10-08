package bootstrap

import (
	"context"

	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/assurrussa/gowebsocket/eventstream"
	"github.com/assurrussa/gowebsocket/websocketstream"
	"github.com/assurrussa/gowebsocket/websocketstream/eventadapter"
	"github.com/assurrussa/gowebsocket/websocketstream/handlers"
	"github.com/gofiber/fiber/v3"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	"github.com/assurrussa/goadmin/internal/uploadintegration"
	"github.com/assurrussa/goadmin/services/adminservice"
)

func newRealtimeHandler(lg logger.Logger, stream eventstream.EventStream, origins []string) (*handlers.HTTPHandler, error) {
	upgrader, err := websocketstream.NewUpgraderChecked(origins, []string{"tgmulti-service-protocol"}, websocketstream.Config{})
	if err != nil {
		return nil, err
	}
	return handlers.NewHTTPHandler(handlers.NewOptions(lg, stream, externalDeadlineUpgrader{upgrader}, nil, "",
		handlers.WithUserIDExtractor(realtimeUserID),
		handlers.WithWireFormat(handlers.JSON),
		handlers.WithEventAdapters(map[string]eventadapter.EventAdapter{
			uploadhost.EventTypeUploadStatus: uploadintegration.Processor[*uploadhost.FileUploadStatusEvent]{},
			uploadhost.EventTypeAfterProcess: uploadintegration.Processor[*uploadhost.EventAfterProcess]{},
			uploadhost.EventTypeDeleted:      uploadintegration.Processor[*uploadhost.FileDeletedEvent]{},
		}),
	))
}

// AuthAdminMiddleware has already validated the current browser authority and
// admin membership. Only its trusted projection UUID reaches the stream.
func realtimeUserID(c fiber.Ctx) (eventstream.UserID, error) {
	admin := adminmiddleware.GetAdminAuth(c)
	if admin == nil || admin.UUID.IsZero() {
		return eventstream.UserID{}, fiber.ErrUnauthorized
	}
	return eventstream.UserID(admin.UUID), nil
}

// WebSocket authentication is checked before the upgrade. Browser page redirects
// become an explicit unauthenticated handshake response; dependency failures keep
// the authentication middleware's status.
func registerRealtimeRoute(app *fiber.App, service *adminservice.Service, handler *handlers.HTTPHandler, csrfCookieName string) {
	authenticate := adminmiddleware.AuthAdminMiddleware(service, csrfCookieName)
	app.Get("/ws", func(c fiber.Ctx) error {
		err := authenticate(c)
		if c.Response().StatusCode() >= 300 && c.Response().StatusCode() < 400 {
			c.Response().Header.Del(fiber.HeaderLocation)
			c.Set(fiber.HeaderCacheControl, "no-store")
			return c.Status(fiber.StatusUnauthorized).SendString("Authentication required")
		}
		return err
	}, handler.Serve)
}

func realtimeShutdown(handler *handlers.HTTPHandler, shutdownStream func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		if handler != nil {
			if err := handler.Shutdown(ctx); err != nil {
				return err
			}
		}
		if shutdownStream != nil {
			return shutdownStream(ctx)
		}
		return nil
	}
}
