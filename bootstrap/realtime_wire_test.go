package bootstrap //nolint:testpackage // verifies production wire adapters over a real Fiber WebSocket

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	inmem "github.com/assurrussa/gowebsocket/eventstream/inmem"
	ws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp/fasthttputil"

	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	server "github.com/assurrussa/goadmin/infrastructure/fiber/server"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/internal/uploadintegration"
	"github.com/assurrussa/goadmin/models"
)

func TestRealtimeDeliversPlainJSONToAuthenticatedAdmin(t *testing.T) {
	conn, stream, admin := realtimeWireConnection(t)
	event := uploadhost.NewEventAfterProcess(42, "https://admin.test/file.png", uploadhost.StatusCompleted, "avatar")
	require.NoError(t, (uploadintegration.Publisher{Stream: stream}).Publish(t.Context(), uploadhost.UserID(admin.UUID), event))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	kind, body, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, ws.TextMessage, kind)
	expected, err := json.Marshal(event)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(body))
}

func TestRealtimeRejectsInvalidFramesWithContractCloseCodes(t *testing.T) {
	for _, test := range []struct {
		name string
		kind int
		body []byte
		code int
	}{
		{"binary", ws.BinaryMessage, []byte(`{}`), ws.CloseUnsupportedData},
		{"oversized", ws.TextMessage, bytes.Repeat([]byte("x"), 96<<10+1), ws.CloseMessageTooBig},
		{"invalid JSON", ws.TextMessage, []byte(`not-json`), ws.ClosePolicyViolation},
		{"legacy base64", ws.TextMessage, []byte(`eyJldmVudFR5cGUiOiJmaWxlLmRlbGV0ZWQifQ==`), ws.ClosePolicyViolation},
		{"unknown event", ws.TextMessage, []byte(`{"eventType":"unknown"}`), ws.ClosePolicyViolation},
		{
			"missing event ID", ws.TextMessage,
			[]byte(`{"eventType":"files.upload.status","taskId":1,"status":"completed"}`), ws.ClosePolicyViolation,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, _, _ := realtimeWireConnection(t)
			require.NoError(t, conn.WriteMessage(test.kind, test.body))
			realtimeWireClose(t, conn, test.code)
		})
	}
}

func TestRealtimeTerminatedDeliveryRequestsResync(t *testing.T) {
	conn, stream, _ := realtimeWireConnection(t)
	require.NoError(t, stream.Shutdown(t.Context()))
	realtimeWireClose(t, conn, ws.CloseTryAgainLater)
}

func realtimeWireConnection(t *testing.T) (*ws.Conn, *inmem.Service, *models.SessionAdmin) {
	t.Helper()
	admin := &models.SessionAdmin{UUID: identity.NewUserID()}
	stream := inmem.New()
	handler, err := newRealtimeHandler(logger.Discard(), stream, []string{realtimeTestOrigin})
	require.NoError(t, err)
	srv, err := server.New(server.NewOptions(logger.Discard(), "localhost:8080", []string{realtimeTestOrigin},
		server.WithDisableStartupMessage(true),
		server.WithShutdownConnections(realtimeShutdown(handler, stream.Shutdown)),
		server.WithRegistered(func(app *fiber.App) {
			app.Get("/ws", func(c fiber.Ctx) error {
				c.Locals(adminsession.AuthAdminKey.String(), admin)
				return handler.Serve(c)
			})
		}),
	))
	require.NoError(t, err)
	listener := fasthttputil.NewInmemoryListener()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		select {
		case runErr := <-done:
			require.NoError(t, runErr)
		case <-time.After(5 * time.Second):
			t.Error("realtime server did not stop")
		}
	})
	dialer := &ws.Dialer{
		NetDial:          func(string, string) (net.Conn, error) { return listener.Dial() },
		HandshakeTimeout: 2 * time.Second, Subprotocols: []string{"tgmulti-service-protocol"},
	}
	conn, response, err := dialer.DialContext(t.Context(), "ws://admin.example.test/ws", http.Header{"Origin": {realtimeTestOrigin}})
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	require.NoError(t, response.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	require.Eventually(t, func() bool { return stream.Stats().Subscribers == 1 }, 2*time.Second, time.Millisecond)
	return conn, stream, admin
}

func realtimeWireClose(t *testing.T, conn *ws.Conn, code int) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err := conn.ReadMessage()
	require.True(t, ws.IsCloseError(err, code), "expected close %d, got %v", code, err)
}
