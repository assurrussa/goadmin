package bootstrap //nolint:testpackage // Tests private upgrader ownership with an explicit socket fixture.

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	logger "github.com/assurrussa/gologger"
	inmem "github.com/assurrussa/gowebsocket/eventstream/inmem"
	libwebsocket "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp/fasthttputil"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/infrastructure/fiber/server"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/internal/realtimesession"
	"github.com/assurrussa/goadmin/models"
)

const externalRealtimeProtocol = "tgmulti-service-protocol"

func TestRealtimeExternalProofDeadlineClosesAlreadyOpenSocket(t *testing.T) {
	stream := inmem.New()
	handler, err := newRealtimeHandler(logger.Discard(), stream, []string{realtimeTestOrigin})
	require.NoError(t, err)
	deadline := time.Now().Add(500 * time.Millisecond)
	srv, err := server.New(server.NewOptions(
		logger.Discard(), "localhost:8080", []string{realtimeTestOrigin},
		server.WithRegistered(func(app *fiber.App) {
			app.Get("/ws", func(c fiber.Ctx) error {
				c.Locals(sessioncore.AuthAdminKey.String(), &models.SessionAdmin{
					ID: 1, UUID: identity.NewUserID(), ExternalValidUntil: deadline,
					SubjectID: goauth.NewSubjectID(), AuthSessionID: "external-canonical-session",
				})
				var sessions realtimesession.Registry
				admission := sessions.Begin(time.Second)
				defer admission.Cancel()
				c.Locals(realtimeAdmissionKey{}, admission)
				return handler.Serve(c)
			})
		}), server.WithShutdownConnections(realtimeShutdown(handler, nil)), server.WithDisableStartupMessage(true)))
	require.NoError(t, err)
	listener := fasthttputil.NewInmemoryListener()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("server failed to stop")
		}
		require.NoError(t, stream.Close())
	})
	dialer := libwebsocket.Dialer{
		NetDial:          func(string, string) (net.Conn, error) { return listener.Dial() },
		HandshakeTimeout: 2 * time.Second, Subprotocols: []string{externalRealtimeProtocol},
	}
	conn, response, err := dialer.Dial("ws://admin.example.test/ws", http.Header{fiber.HeaderOrigin: {realtimeTestOrigin}})
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	_ = response.Body.Close()
	defer conn.Close()
	require.NoError(t, conn.SetReadDeadline(deadline.Add(time.Second)))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
	require.False(t, time.Now().Before(deadline), "socket closed before its verified proof deadline")
	require.Less(t, time.Since(deadline), time.Second, "open socket exceeded proof validity")
}
