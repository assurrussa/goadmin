package host //nolint:testpackage // Verifies owned cleanup before the private runtime closes dependencies.

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/fiber/server"
)

func TestRuntimeCloseJoinsFinalizerBeyondPreviousWaitBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv, err := server.New(server.NewOptions(DiscardLogger(), "localhost:8080", []string{assemblyAdminOrigin},
			server.WithRegistered(func(app *fiber.App) {
				app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })
			})))
		require.NoError(t, err)
		_ = srv.App.Server()
		entered, release := make(chan struct{}), make(chan struct{})
		srv.App.Hooks().OnPostShutdown(func(error) error {
			close(entered)
			<-release
			return nil
		})
		runtime := &Runtime{Server: srv, done: make(chan struct{})}
		result := make(chan error, 1)
		go func() { result <- runtime.Close() }()
		<-entered
		// Fake time exceeds the old three-second caller wait. Owned cleanup
		// must still be joined rather than returning to a database-owning host.
		time.Sleep(4 * time.Second)
		select {
		case err := <-result:
			t.Fatalf("Close returned before finalizer completed: %v", err)
		default:
		}
		close(release)
		require.NoError(t, <-result)
	})
}
