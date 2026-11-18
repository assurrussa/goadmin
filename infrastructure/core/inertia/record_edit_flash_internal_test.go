package inertia

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/assurrussa/goinertia"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/config"
	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionredis"
	"github.com/assurrussa/goadmin/views"
)

const (
	recordEditNameField = "name"
	crudConflictMessage = "record changed"
)

type recordEditPage struct {
	Props struct {
		Flash  map[string]string `json:"flash"`
		Errors map[string]string `json:"errors"`
		Old    map[string]any    `json:"old"`
	} `json:"props"`
}

type recordEditFlashFixture struct {
	app    *fiber.App
	cookie *http.Cookie
	store  *session.Store
}

func newRecordEditFlashFixture(t *testing.T) recordEditFlashFixture {
	t.Helper()
	_, store := sessionredis.CreateAdminSessionStoreWithStorage(session.NewStore().Storage, config.Config{})
	t.Cleanup(func() { require.NoError(t, store.Storage.Close()) })
	bridge := SessionBridge{Store: store}
	manager, err := goinertia.NewWithValidation("http://admin.test",
		goinertia.WithFS(views.Templates),
		goinertia.WithSessionStore(bridge))
	require.NoError(t, err)

	app := fiber.New()
	app.Get("/start", func(c fiber.Ctx) error {
		if err := bridge.Set(c, sessioncore.AuthAdminKey.String(), true); err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	app.Use(manager.Middleware())
	app.Put("/:resource/:id", func(c fiber.Ctx) error {
		record := c.Params("resource") + "/" + c.Params("id")
		if c.Query("reject") != "" {
			manager.WithFlashWarning(c, "check the form")
			manager.WithFlashOld(c, map[string]any{recordEditNameField: record})
			return manager.RedirectBackWithErrors(c, map[string]string{recordEditNameField: "required"})
		}
		manager.WithFlashSuccess(c, "saved "+record)
		return manager.RedirectBack(c)
	})
	app.Get("/:resource/:id/edit", func(c fiber.Ctx) error {
		return manager.Render(c, "Edit", nil)
	})
	app.Put("/catalog/items/records/:id", func(c fiber.Ctx) error {
		id, err := url.PathUnescape(c.Params("id"))
		if err != nil {
			return err
		}
		conflict := c.Query("stale") == "1"
		ScopeCRUDRecordFlash(c, "/catalog/items", id, CRUDRecordEdit, conflict)
		if conflict {
			manager.WithFlashError(c, crudConflictMessage)
			return manager.RedirectBackWithErrors(c, map[string]string{"form": crudConflictMessage})
		}
		manager.WithFlashSuccess(c, "saved "+id)
		return manager.RedirectBack(c)
	})
	app.Get("/catalog/items/records/:id/edit", func(c fiber.Ctx) error {
		id, err := url.PathUnescape(c.Params("id"))
		if err != nil {
			return err
		}
		ScopeCRUDRecordFlash(c, "/catalog/items", id, CRUDRecordEdit, false)
		return manager.Render(c, "Edit", nil)
	})

	fixture := recordEditFlashFixture{app: app, store: store}
	response := fixture.request(t, http.MethodGet, "/start", "")
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
	cookies := response.Cookies()
	require.Len(t, cookies, 1)
	fixture.cookie = cookies[0]
	return fixture
}

func (f recordEditFlashFixture) request(t *testing.T, method, path, attempt string) *http.Response {
	t.Helper()
	response, err := f.app.Test(f.newRequest(t.Context(), method, path, attempt))
	require.NoError(t, err)
	return response
}

func (f recordEditFlashFixture) newRequest(ctx context.Context, method, path, attempt string) *http.Request {
	request := httptest.NewRequestWithContext(ctx, method, "http://admin.test"+path, nil)
	request.Header.Set(goinertia.HeaderInertia, "true")
	if f.cookie != nil {
		request.AddCookie(f.cookie)
		request.AddCookie(&http.Cookie{
			Name: f.cookie.Name + "_persistent", Value: "1", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
	}
	if attempt != "" {
		request.Header.Set(recordEditAttemptHeader, attempt)
	}
	if method == http.MethodPut {
		request.Header.Set(fiber.HeaderReferer, "http://admin.test"+request.URL.Path+"/edit")
	}
	return request
}

func (f recordEditFlashFixture) update(t *testing.T, path, attempt string) {
	t.Helper()
	response := f.request(t, http.MethodPut, path, attempt)
	require.Equal(t, http.StatusSeeOther, response.StatusCode)
	require.Empty(t, response.Cookies())
	require.NoError(t, response.Body.Close())
}

func (f recordEditFlashFixture) edit(t *testing.T, path, attempt string) recordEditPage {
	t.Helper()
	response := f.request(t, http.MethodGet, path, attempt)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Empty(t, response.Cookies())
	var page recordEditPage
	require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
	require.NoError(t, response.Body.Close())
	return page
}

func testRecordEditAttempt(id int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", id)
}

func TestRecordEditFlashIsScopedToTheRedirect(t *testing.T) {
	f := newRecordEditFlashFixture(t)
	first, second, rejected := testRecordEditAttempt(1), testRecordEditAttempt(2), testRecordEditAttempt(3)
	f.update(t, "/users/42", first)
	f.update(t, "/admins/42", second)
	f.update(t, "/users/42?reject=1", rejected)
	f.update(t, "/other/42", "")

	require.Empty(t, f.edit(t, "/admins/42/edit", first).Props.Flash)
	require.Empty(t, f.edit(t, "/users/43/edit", first).Props.Flash)
	require.Empty(t, f.edit(t, "/users/42/edit", "invalid").Props.Flash)
	anonymous := recordEditFlashFixture{app: f.app}
	require.Empty(t, anonymous.edit(t, "/users/42/edit", first).Props.Flash)

	failure := f.edit(t, "/users/42/edit", rejected).Props
	require.Equal(t, map[string]string{"warning": "check the form"}, failure.Flash)
	require.Equal(t, map[string]string{recordEditNameField: "required"}, failure.Errors)
	require.Equal(t, map[string]any{recordEditNameField: "users/42"}, failure.Old)

	success := goinertia.FlashLevelSuccess.String()
	require.Equal(t, map[string]string{success: "saved admins/42", recordEditAttemptProp: second},
		f.edit(t, "/admins/42/edit", second).Props.Flash)
	require.Equal(t, map[string]string{success: "saved users/42", recordEditAttemptProp: first},
		f.edit(t, "/users/42/edit", first).Props.Flash)
	require.Empty(t, f.edit(t, "/users/42/edit", first).Props.Flash)

	require.Equal(t, map[string]string{success: "saved other/42"},
		f.edit(t, "/other/42/edit", first).Props.Flash)
	f.update(t, "/users/42", "")
	require.Equal(t, map[string]string{success: "saved users/42"},
		f.edit(t, "/users/42/edit", "").Props.Flash)
}

func TestRecordEditFlashSurvivesConcurrentUpdatesAndStaleSessionSave(t *testing.T) {
	f := newRecordEditFlashFixture(t)
	stale, err := f.store.GetByID(t.Context(), f.cookie.Value)
	require.NoError(t, err)
	defer stale.Release()

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, attempt := range []string{testRecordEditAttempt(1), testRecordEditAttempt(2)} {
		request := f.newRequest(t.Context(), http.MethodPut, "/users/42", attempt)
		workers.Go(func() {
			<-start
			response, err := f.app.Test(request)
			if err != nil {
				results <- err
				return
			}
			err = response.Body.Close()
			if response.StatusCode != http.StatusSeeOther || len(response.Cookies()) != 0 {
				err = fmt.Errorf("unexpected update status %d or session cookie", response.StatusCode)
			}
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	stale.Set("unrelated-older-get", true)
	require.NoError(t, stale.SaveWithContext(t.Context()))
	for _, attempt := range []string{testRecordEditAttempt(2), testRecordEditAttempt(1)} {
		require.Equal(t, attempt, f.edit(t, "/users/42/edit", attempt).Props.Flash[recordEditAttemptProp])
		require.Empty(t, f.edit(t, "/users/42/edit", attempt).Props.Flash)
	}
}

func TestCRUDRecordFlashIsolatedAcrossConcurrentEdits(t *testing.T) {
	f := newRecordEditFlashFixture(t)
	otherSession := f
	otherSession.cookie = nil
	response := otherSession.request(t, http.MethodGet, "/start", "")
	require.NoError(t, response.Body.Close())
	require.Len(t, response.Cookies(), 1)
	otherSession.cookie = response.Cookies()[0]

	const path = "/catalog/items/records/one%2Ftwo"
	write := testRecordEditAttempt(1)
	stale := testRecordEditAttempt(2)
	var workers sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, attempt := range []string{write, stale} {
		target := path
		if attempt == stale {
			target += "?stale=1"
		}
		req := f.newRequest(t.Context(), http.MethodPut, target, attempt)
		workers.Go(func() {
			<-start
			res, err := f.app.Test(req)
			if err == nil {
				err = res.Body.Close()
				if res.StatusCode != http.StatusSeeOther || len(res.Cookies()) != 0 {
					err = fmt.Errorf("unexpected update status %d or session cookie", res.StatusCode)
				}
			}
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}

	edit := path + "/edit"
	require.Empty(t, otherSession.edit(t, edit, stale).Props.Flash)
	require.Empty(t, f.edit(t, "/catalog/items/records/other/edit", stale).Props.Flash)
	rejected := f.edit(t, edit, stale).Props
	require.Equal(t, map[string]string{
		"error": crudConflictMessage, recordEditAttemptProp: stale, crudEditConflictProp: "1",
	}, rejected.Flash)
	require.Equal(t, map[string]string{"form": crudConflictMessage}, rejected.Errors)
	accepted := f.edit(t, edit, write).Props
	require.Equal(t, map[string]string{"success": "saved one/two", recordEditAttemptProp: write}, accepted.Flash)
	require.Empty(t, accepted.Errors)
	require.Empty(t, f.edit(t, edit, stale).Props.Flash)
	require.Empty(t, f.edit(t, edit, write).Props.Flash)
}
