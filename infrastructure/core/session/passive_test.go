package session_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
)

const (
	passiveBootstrap      = "bootstrap"
	passiveMissingStorage = "missing-storage"
)

type passiveSaveFixture struct {
	store    *session.Store
	mode     string
	oldID    string
	savedID  string
	original []string
}

func (f *passiveSaveFixture) seed(c fiber.Ctx) error {
	sess, err := f.store.Get(c)
	if err != nil {
		return err
	}
	defer sess.Release()
	f.oldID = sess.ID()
	sess.Set("seed", true)
	if err := sess.Save(); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func (f *passiveSaveFixture) queueCookies(c fiber.Ctx, sess *session.Session) error {
	if f.mode == "rotated" {
		if err := sess.Regenerate(); err != nil {
			return err
		}
		// Preserve multiple same-name scopes and explicit persistence exactly.
		c.Response().Header.Add("Set-Cookie", "session_id="+sess.ID()+"; Path=/; Max-Age=123; HttpOnly; SameSite=Lax")
		c.Response().Header.Add("Set-Cookie", "session_id=other-scope; Path=/nested; Domain=example.test; Max-Age=456; Secure")
	}
	if f.mode == "deleted" {
		c.Response().Header.Add("Set-Cookie", "session_id=; Path=/; Max-Age=0; Expires=Thu, 01 Jan 1970 00:00:00 GMT; HttpOnly")
	}
	for _, value := range c.Response().Header.Cookies() {
		f.original = append(f.original, string(value))
	}
	c.Cookie(&fiber.Cookie{Name: "unrelated", Value: "keep", Path: "/"})
	return nil
}

func (f *passiveSaveFixture) save(c fiber.Ctx) error {
	sess, err := f.store.Get(c)
	if err != nil {
		return err
	}
	defer sess.Release()
	if err := f.queueCookies(c, sess); err != nil {
		return err
	}
	f.savedID = sess.ID()
	sess.Set("persisted", true)
	if err := sessioncore.SavePassive(c, f.store, sess); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func (f *passiveSaveFixture) check(t *testing.T, response *http.Response) {
	t.Helper()
	var actual []string
	for _, raw := range response.Header.Values("Set-Cookie") {
		parsed := (&http.Response{Header: http.Header{"Set-Cookie": []string{raw}}}).Cookies()
		if len(parsed) == 1 && parsed[0].Name == "session_id" {
			actual = append(actual, raw)
		}
	}
	if f.mode == passiveBootstrap || f.mode == passiveMissingStorage {
		require.Len(t, actual, 1)
	} else {
		require.Equal(t, f.original, actual)
	}
	loaded, err := f.store.GetByID(t.Context(), f.savedID)
	require.NoError(t, err)
	defer loaded.Release()
	require.Equal(t, true, loaded.Get("persisted"))
	unrelated := ""
	for _, cookie := range response.Cookies() {
		if cookie.Name == "unrelated" {
			unrelated = cookie.Value
		}
	}
	require.Equal(t, "keep", unrelated)
}

func testPassiveSaveMode(t *testing.T, mode string) {
	t.Helper()
	f := passiveSaveFixture{store: session.NewStore(), mode: mode}
	t.Cleanup(func() { require.NoError(t, f.store.Storage.Close()) })
	app := fiber.New()
	app.Get("/seed", f.seed)
	app.Get("/", f.save)
	seedResponse, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/seed", nil))
	require.NoError(t, err)
	require.NoError(t, seedResponse.Body.Close())
	require.Equal(t, http.StatusNoContent, seedResponse.StatusCode)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	if mode != passiveBootstrap {
		id := f.oldID
		if mode == passiveMissingStorage {
			id = "already-removed"
		}
		req.AddCookie(&http.Cookie{
			Name: "session_id", Value: id, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
	}
	response, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	f.check(t, response)
}

func TestPassiveSavePreservesExplicitCookiesAndStorage(t *testing.T) {
	for _, mode := range []string{passiveBootstrap, "existing", "rotated", "deleted", passiveMissingStorage} {
		t.Run(mode, func(t *testing.T) { testPassiveSaveMode(t, mode) })
	}
}
