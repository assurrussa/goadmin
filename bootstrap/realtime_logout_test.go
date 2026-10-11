package bootstrap //nolint:testpackage // Exercises real Fiber/WebSocket session boundaries.

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/assurrussa/gowebsocket/eventstream"
	inmem "github.com/assurrussa/gowebsocket/eventstream/inmem"
	ws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp/fasthttputil"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/infrastructure/fiber/server"
	internalauth "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/internal/realtimesession"
	"github.com/assurrussa/goadmin/internal/uploadintegration"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
)

type realtimeLogoutFixture struct {
	app         *fiber.App
	service     *adminservice.Service
	stream      *inmem.Service
	admin       models.Admin
	dialer      *ws.Dialer
	clock       atomic.Int64
	pause       atomic.Bool
	authEntered chan struct{}
	authRelease chan struct{}
}

func newRealtimeLogoutFixture(t *testing.T) *realtimeLogoutFixture {
	t.Helper()
	f := &realtimeLogoutFixture{stream: inmem.New(), authEntered: make(chan struct{}, 1), authRelease: make(chan struct{})}
	f.clock.Store(time.Now().Truncate(time.Second).UnixNano())
	canonical, err := testkit.NewRuntime(func(c *goauth.Config) {
		c.AccessTTL = time.Second
		c.Now = func() time.Time { return time.Unix(0, f.clock.Load()) }
	})
	require.NoError(t, err)
	const password = "Session-Logout-Unique-Passphrase-42" //nolint:gosec // Isolated fixture password, never a live credential.
	account, err := canonical.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "logout-socket@example.test", Password: password,
	})
	require.NoError(t, err)
	f.admin = models.Admin{ID: 42, SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
	mocks := adminappt.NewAppTest(t)
	roles := func(context.Context, int64) ([]internalauth.Role, error) {
		if f.pause.Load() {
			f.authEntered <- struct{}{}
			<-f.authRelease
		}
		return nil, nil
	}
	mocks.MockRoleService.EXPECT().GetRolesAdmin(gomock.Any(), f.admin.ID).DoAndReturn(roles).AnyTimes()
	f.service = adminservice.NewService(fibersession.NewStore(), mocks.MockRoleService,
		adminservice.WithRuntime(canonical.Runtime, realtimeAdminRepository{admin: f.admin}),
		adminservice.WithBrowserState(browserstate.NewMemory()))
	f.app = fiber.New()
	f.app.Post("/login", func(c fiber.Ctx) error {
		if _, _, err := f.service.LoginAdmin(c, account.PrimaryEmail.DisplayValue, password, false); err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	f.app.Post("/logout", func(c fiber.Ctx) error {
		if err := f.service.DelAdminAuth(c); err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	f.app.Get("/auth", func(c fiber.Ctx) error {
		admin, err := f.service.GetAdminAuth(c)
		if err != nil {
			return err
		}
		if admin == nil {
			return c.SendStatus(http.StatusUnauthorized)
		}
		return c.SendStatus(http.StatusNoContent)
	})
	f.app.Post("/rotate", func(c fiber.Ctx) error {
		id, err := f.service.RotateAdminAuth(c, f.admin)
		if err != nil {
			return err
		}
		c.Cookie(&fiber.Cookie{Name: realtimeSessionCookieName, Value: id})
		return c.SendStatus(http.StatusNoContent)
	})
	handler, err := newRealtimeHandler(logger.Discard(), f.stream, []string{realtimeTestOrigin})
	require.NoError(t, err)
	srv, err := server.New(server.NewOptions(logger.Discard(), "localhost:8080", []string{realtimeTestOrigin},
		server.WithRegistered(func(app *fiber.App) { registerRealtimeRoute(app, f.service, handler, "csrf-token") }),
		server.WithShutdownConnections(realtimeShutdown(handler, nil)), server.WithDisableStartupMessage(true)))
	require.NoError(t, err)
	listener := fasthttputil.NewInmemoryListener()
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- srv.Run(ctx, listener) }()
	t.Cleanup(func() {
		f.pause.Store(false)
		select {
		case <-f.authRelease:
		default:
			close(f.authRelease)
		}
		cancel()
		select {
		case err := <-stopped:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("server failed to stop")
		}
		require.NoError(t, f.stream.Close())
	})
	f.dialer = &ws.Dialer{
		NetDial:          func(string, string) (net.Conn, error) { return listener.Dial() },
		HandshakeTimeout: 3 * time.Second,
		Subprotocols:     []string{externalRealtimeProtocol},
	}
	return f
}

func (f *realtimeLogoutFixture) request(t *testing.T, method, path string, cookie *http.Cookie, status int) []*http.Cookie {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := f.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, status, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return resp.Cookies()
}

func realtimeLogoutCookie(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == realtimeSessionCookieName {
			return cookie
		}
	}
	t.Fatal("session cookie missing")
	return nil
}

func (f *realtimeLogoutFixture) dial(t *testing.T, cookie *http.Cookie) *ws.Conn {
	t.Helper()
	conn, resp, err := f.dialer.DialContext(t.Context(), "ws://admin.example.test/ws", http.Header{
		fiber.HeaderOrigin: {realtimeTestOrigin}, fiber.HeaderCookie: {cookie.String()},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	require.Equal(t, externalRealtimeProtocol, conn.Subprotocol())
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (f *realtimeLogoutFixture) publish(t *testing.T, id int64, conns ...*ws.Conn) {
	t.Helper()
	event := uploadhost.NewEventAfterProcess(id, "https://admin.example.test/fresh.png", uploadhost.StatusCompleted, "avatar")
	require.NoError(t, (uploadintegration.Publisher{Stream: f.stream}).Publish(t.Context(), uploadhost.UserID(f.admin.UUID), event))
	want, err := json.Marshal(event)
	require.NoError(t, err)
	for _, conn := range conns {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
		kind, body, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, ws.TextMessage, kind)
		require.JSONEq(t, string(want), string(body))
	}
}

func TestRealtimeLogoutClosesSameSessionTabsAfterRefreshAndBrowserRotation(t *testing.T) {
	f := newRealtimeLogoutFixture(t)
	a := realtimeLogoutCookie(t, f.request(t, http.MethodPost, "/login", nil, http.StatusNoContent))
	b1, b2 := f.dial(t, a), f.dial(t, a)
	c := realtimeLogoutCookie(t, f.request(t, http.MethodPost, "/login", nil, http.StatusNoContent))
	connC := f.dial(t, c)
	require.Eventually(t, func() bool { return f.stream.Stats().Subscribers == 3 }, time.Second, time.Millisecond)
	f.publish(t, 1, b1, b2, connC)
	f.clock.Add(int64(2 * time.Second))
	f.request(t, http.MethodGet, "/auth", a, http.StatusNoContent)
	rotated := realtimeLogoutCookie(t, f.request(t, http.MethodPost, "/rotate", a, http.StatusNoContent))
	require.NotEqual(t, a.Value, rotated.Value)
	f.request(t, http.MethodPost, "/logout", rotated, http.StatusNoContent)
	for _, conn := range []*ws.Conn{b1, b2} {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
		_, _, err := conn.ReadMessage()
		require.Error(t, err, "same-session tab stayed connected")
		var networkErr net.Error
		if errors.As(err, &networkErr) {
			require.False(t, networkErr.Timeout(), "read timeout is not logout closure")
		}
	}
	require.Eventually(t, func() bool { return f.stream.Stats().Subscribers == 1 }, time.Second, time.Millisecond)
	f.request(t, http.MethodGet, "/auth", rotated, http.StatusUnauthorized)
	conn, resp, err := f.dialer.DialContext(t.Context(), "ws://admin.example.test/ws", http.Header{
		fiber.HeaderOrigin: {realtimeTestOrigin}, fiber.HeaderCookie: {rotated.String()},
	})
	require.Error(t, err)
	require.Nil(t, conn)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	f.request(t, http.MethodGet, "/auth", c, http.StatusNoContent)
	f.publish(t, 2, connC)
	d := realtimeLogoutCookie(t, f.request(t, http.MethodPost, "/login", nil, http.StatusNoContent))
	connD := f.dial(t, d)
	require.Eventually(t, func() bool { return f.stream.Stats().Subscribers == 2 }, time.Second, time.Millisecond)
	f.request(t, http.MethodPost, "/logout", rotated, http.StatusNoContent)
	f.publish(t, 3, connC, connD)
	// A supplied stream remains usable; logout never owns or shuts it down.
	events, err := f.stream.Subscribe(t.Context(), eventstream.NewUserID())
	require.NoError(t, err)
	require.NotNil(t, events)
}

func TestRealtimeLogoutFencesAuthenticationAlreadyInFlight(t *testing.T) {
	f := newRealtimeLogoutFixture(t)
	cookie := realtimeLogoutCookie(t, f.request(t, http.MethodPost, "/login", nil, http.StatusNoContent))
	f.pause.Store(true)
	type result struct {
		conn     *ws.Conn
		resp     *http.Response
		err      error
		closeErr error
	}
	done := make(chan result, 1)
	go func() {
		conn, resp, err := f.dialer.Dial("ws://admin.example.test/ws", http.Header{
			fiber.HeaderOrigin: {realtimeTestOrigin}, fiber.HeaderCookie: {cookie.String()},
		})
		var closeErr error
		if resp != nil {
			closeErr = resp.Body.Close()
		}
		done <- result{conn: conn, resp: resp, err: err, closeErr: closeErr}
	}()
	select {
	case <-f.authEntered:
	case <-time.After(time.Second):
		t.Fatal("authentication did not reach barrier")
	}
	f.request(t, http.MethodPost, "/logout", cookie, http.StatusNoContent)
	f.pause.Store(false)
	close(f.authRelease)
	select {
	case got := <-done:
		require.Error(t, got.err)
		require.Nil(t, got.conn)
		require.NotNil(t, got.resp)
		require.Equal(t, http.StatusUnauthorized, got.resp.StatusCode)
		require.NoError(t, got.closeErr)
	case <-time.After(3 * time.Second):
		t.Fatal("upgrade did not finish")
	}
	require.Zero(t, f.stream.Stats().Subscribers)
}

func TestRealtimeRejectsMissingCanonicalIdentity(t *testing.T) {
	for _, name := range []string{"subject", "session", "admission"} {
		t.Run(name, func(t *testing.T) {
			stream := inmem.New()
			handler, err := newRealtimeHandler(logger.Discard(), stream, []string{realtimeTestOrigin})
			require.NoError(t, err)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, realtimeShutdown(handler, stream.Shutdown)(ctx))
			})
			var sessions realtimesession.Registry
			app := fiber.New()
			app.Get("/ws", func(c fiber.Ctx) error {
				admin := &models.SessionAdmin{UUID: identity.NewUserID(), SubjectID: goauth.NewSubjectID(), AuthSessionID: "canonical"}
				if name == "subject" {
					admin.SubjectID = goauth.SubjectID{}
				}
				if name == "session" {
					admin.AuthSessionID = ""
					admin.SessionID = "browser-id-must-not-be-used"
				}
				c.Locals(sessioncore.AuthAdminKey.String(), admin)
				if name != "admission" {
					a := sessions.Begin(time.Second)
					defer a.Cancel()
					c.Locals(realtimeAdmissionKey{}, a)
				}
				return handler.Serve(c)
			})
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ws?subject=attacker&session=attacker", nil)
			response, err := app.Test(request)
			require.NoError(t, err)
			require.Equal(t, http.StatusUnauthorized, response.StatusCode)
			require.NoError(t, response.Body.Close())
			require.Zero(t, stream.Stats().Subscribers)
		})
	}
}
