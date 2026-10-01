package bootstrap //nolint:testpackage // checks the private route assembly and connection ownership boundary.

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	logger "github.com/assurrussa/gologger"
	"github.com/assurrussa/gowebsocket/eventstream"
	inmem "github.com/assurrussa/gowebsocket/eventstream/inmem"
	libwebsocket "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp/fasthttputil"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/infrastructure/fiber/server"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
)

const (
	realtimeTestOrigin        = "http://admin.example.test"
	realtimeSessionCookieName = "session_id"
)

type realtimeAdminRepository struct{ admin models.Admin }

func (r realtimeAdminRepository) GetBySubjectID(context.Context, goauth.SubjectID) (models.Admin, error) {
	return r.admin, nil
}

type realtimeObservedStream struct {
	*inmem.Service
	subscribed chan eventstream.UserID
}

func (s *realtimeObservedStream) Subscribe(ctx context.Context, userID eventstream.UserID) (<-chan eventstream.Event, error) {
	events, err := s.Service.Subscribe(ctx, userID)
	if err == nil {
		s.subscribed <- userID
	}
	return events, err
}

func TestRealtimeHandshakeRequiresCheckedAdminIdentity(t *testing.T) {
	service, cookie, admin := realtimeAuthenticatedService(t)
	stream := &realtimeObservedStream{Service: inmem.New(), subscribed: make(chan eventstream.UserID, 2)}
	handler, err := newRealtimeHandler(logger.Discard(), stream, []string{realtimeTestOrigin})
	require.NoError(t, err)
	srv, err := server.New(server.NewOptions(logger.Discard(), "localhost:8080", []string{realtimeTestOrigin},
		server.WithRegistered(func(app *fiber.App) { registerRealtimeRoute(app, service, handler, "csrf-token") }),
		server.WithShutdownConnections(realtimeShutdown(handler, nil)),
		server.WithDisableStartupMessage(true),
	))
	require.NoError(t, err)
	listener := fasthttputil.NewInmemoryListener()
	exited := make(chan error, 1)
	runCtx, cancelRun := context.WithCancel(t.Context())
	go func() { exited <- srv.Run(runCtx, listener) }()
	t.Cleanup(func() {
		cancelRun()
		select {
		case runErr := <-exited:
			require.NoError(t, runErr)
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
		require.NoError(t, stream.Close())
	})
	dialer := libwebsocket.Dialer{
		NetDial:          func(string, string) (net.Conn, error) { return listener.Dial() },
		HandshakeTimeout: 2 * time.Second,
		Subprotocols:     []string{"tgmulti-service-protocol"},
	}
	headers := http.Header{"Origin": {realtimeTestOrigin}, "X-User-ID": {identity.NewUserID().String()}}
	conn, response, err := dialer.Dial("ws://admin.example.test/ws?userID=attacker", headers)
	require.Error(t, err)
	require.Nil(t, conn)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.Zero(t, handler.Stats().Accepted)
	headers.Set("Cookie", cookie.String()+"; "+cookie.String())
	conn, response, err = dialer.Dial("ws://admin.example.test/ws", headers)
	require.Error(t, err)
	require.Nil(t, conn)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
	headers.Set("Cookie", cookie.String())
	conn, response, err = dialer.Dial("ws://admin.example.test/ws?userID=attacker", headers)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	require.NoError(t, response.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	select {
	case subscribed := <-stream.subscribed:
		require.Equal(t, eventstream.UserID(admin.UUID), subscribed)
	case <-time.After(time.Second):
		t.Fatal("authenticated socket did not subscribe")
	}
	peerClosed := make(chan error, 1)
	go func() { _, _, closeErr := conn.ReadMessage(); peerClosed <- closeErr }()
	require.NoError(t, srv.Shutdown(t.Context()))
	select {
	case closeErr := <-peerClosed:
		require.True(t, libwebsocket.IsCloseError(closeErr, libwebsocket.CloseNormalClosure))
	case <-time.After(time.Second):
		t.Fatal("peer did not receive the shutdown close frame")
	}
	require.Zero(t, handler.Stats().Active)
	// Closing the admin server must leave its borrowed host stream available.
	_, err = stream.Subscribe(t.Context(), eventstream.NewUserID())
	require.NoError(t, err)
}

func realtimeAuthenticatedService(t *testing.T) (*adminservice.Service, *http.Cookie, models.Admin) {
	t.Helper()
	fixture, err := testkit.NewRuntime()
	require.NoError(t, err)
	const password = "Realtime-Unique-Admin-Passphrase-42"
	account, err := fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "socket@example.test", Password: password,
	})
	require.NoError(t, err)
	admin := models.Admin{ID: 42, SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
	appTest := adminappt.NewAppTest(t)
	appTest.MockRoleService.EXPECT().GetRolesAdmin(gomock.Any(), admin.ID).Return(nil, nil).AnyTimes()
	store := fibersession.NewStore()
	service := adminservice.NewService(store, appTest.MockRoleService,
		adminservice.WithRuntime(fixture.Runtime, realtimeAdminRepository{admin: admin}))
	app := fiber.New()
	app.Post("/login", func(c fiber.Ctx) error {
		_, _, loginErr := service.LoginAdmin(c, account.PrimaryEmail.DisplayValue, password, false)
		if loginErr != nil {
			return loginErr
		}
		return c.SendStatus(http.StatusNoContent)
	})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	for _, cookie := range response.Cookies() {
		if cookie.Name == realtimeSessionCookieName {
			return service, cookie, admin
		}
	}
	t.Fatal("authenticated browser cookie missing")
	return nil, nil, models.Admin{}
}

func TestRealtimeShutdownWaitsBeforeOwnedStream(t *testing.T) {
	stream := inmem.New()
	handler, err := newRealtimeHandler(logger.Discard(), stream, []string{realtimeTestOrigin})
	require.NoError(t, err)
	called := false
	stop := realtimeShutdown(handler, func(ctx context.Context) error {
		called = true
		app := fiber.New()
		app.Get("/ws", handler.Serve)
		response, requestErr := app.Test(httptest.NewRequestWithContext(ctx, http.MethodGet, "/ws", nil))
		require.NoError(t, requestErr)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
		require.Zero(t, handler.Stats().Active)
		return stream.Shutdown(ctx)
	})
	require.NoError(t, stop(t.Context()))
	require.True(t, called)
}

func TestRealtimeRouteKeepsAuthenticationDependencyFailure(t *testing.T) {
	stream := inmem.New()
	handler, err := newRealtimeHandler(logger.Discard(), stream, []string{realtimeTestOrigin})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, realtimeShutdown(handler, stream.Shutdown)(ctx))
	})
	app := fiber.New()
	registerRealtimeRoute(app, nil, handler, "csrf-token")
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ws", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	require.Zero(t, handler.Stats().Accepted)
}
