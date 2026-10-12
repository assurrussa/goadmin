package host_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/host"
	coreinertia "github.com/assurrussa/goadmin/infrastructure/core/inertia"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	"github.com/assurrussa/goadmin/views"
)

func flashTestApp(store *session.Store, manager *goinertia.Inertia) *host.App {
	return host.WrapApp(&adminapp.App{Options: adminapp.NewOptions(
		manager, nil, nil, nil, store, nil, nil, nil, gologger.Discard(), "test", "",
	)})
}

func TestPersistFlashFollowsRotatedCookieWithoutCoreMiddleware(t *testing.T) {
	store := session.NewStore()
	t.Cleanup(func() { require.NoError(t, store.Storage.Close()) })
	manager, err := goinertia.NewWithValidation("https://admin.test",
		goinertia.WithFS(views.Templates),
		goinertia.WithSessionStore(coreinertia.SessionBridge{Store: store}))
	require.NoError(t, err)
	hostApp := flashTestApp(store, manager)
	app := fiber.New()
	var oldID, newID string
	app.Get("/admit", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		defer sess.Release()
		oldID = sess.ID()
		if err := sess.RegenerateWithContext(c); err != nil {
			return err
		}
		sess.Set("authenticated", true)
		newID = sess.ID()
		if err := sess.SaveWithContext(c); err != nil {
			return err
		}
		hostApp.WithProp(c, "requestOnly", "must not persist")
		hostApp.WithFlashInfo(c, "optional reminder")
		if err := hostApp.PersistFlash(c); err != nil {
			return err
		}
		return c.Redirect().Status(http.StatusSeeOther).To("/page")
	})
	app.Get("/page", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		authenticated := sess.Get("authenticated")
		sess.Release()
		return hostApp.Render(c, "Page", map[string]any{"authenticated": authenticated})
	})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admit", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusSeeOther, response.StatusCode)
	require.NotEqual(t, oldID, newID)
	cookies := response.Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, newID, cookies[0].Value)
	for _, wantReminder := range []bool{true, false} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/page", nil)
		request.Header.Set("X-Inertia", "true")
		request.AddCookie(cookies[0])
		response, err := app.Test(request)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		var page struct {
			Props struct {
				Authenticated bool              `json:"authenticated"`
				RequestOnly   string            `json:"requestOnly"`
				Flash         map[string]string `json:"flash"`
			} `json:"props"`
		}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
		require.NoError(t, response.Body.Close())
		require.True(t, page.Props.Authenticated)
		require.Empty(t, page.Props.RequestOnly)
		require.Equal(t, wantReminder, page.Props.Flash["info"] == "optional reminder")
	}
}

type failingFlashStorage struct {
	fiber.Storage
	err error
}

func (s failingFlashStorage) SetWithContext(context.Context, string, []byte, time.Duration) error {
	return s.err
}

func TestPersistFlashReturnsStorageFailure(t *testing.T) {
	store := session.NewStore()
	t.Cleanup(func() { require.NoError(t, store.Storage.Close()) })
	want := errors.New("synthetic flash storage failure")
	store.Storage = failingFlashStorage{Storage: store.Storage, err: want}
	manager, err := goinertia.NewWithValidation("https://admin.test",
		goinertia.WithFS(views.Templates),
		goinertia.WithSessionStore(coreinertia.SessionBridge{Store: store}))
	require.NoError(t, err)
	hostApp := flashTestApp(store, manager)
	app := fiber.New()
	app.Get("/flash", func(c fiber.Ctx) error {
		hostApp.WithFlashInfo(c, "optional reminder")
		require.ErrorIs(t, hostApp.PersistFlash(c), want)
		return c.SendStatus(http.StatusNoContent)
	})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/flash", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusNoContent, response.StatusCode)
}

func TestPersistFlashNoValuesDoesNotCreateSession(t *testing.T) {
	store := session.NewStore()
	t.Cleanup(func() { require.NoError(t, store.Storage.Close()) })
	manager, err := goinertia.NewWithValidation("https://admin.test", goinertia.WithFS(views.Templates))
	require.NoError(t, err)
	hostApp := flashTestApp(store, manager)
	app := fiber.New()
	app.Get("/empty", func(c fiber.Ctx) error {
		hostApp.WithProp(c, "requestOnly", "not a flash")
		require.NoError(t, hostApp.PersistFlash(c))
		return c.SendStatus(http.StatusNoContent)
	})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/empty", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Empty(t, response.Cookies())
}
