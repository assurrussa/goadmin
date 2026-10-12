//nolint:testpackage // Exercises private browser policy with fully completed responses.
package adminservice

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

// The generic in-memory testkit deliberately has no MFAStore/codec. Stub only
// enrollment material; login/logout, token introspection, admin authorization,
// browser state and the completed HTTP responses remain real. PostgreSQL and
// browser enrollment tests cover factor storage/crypto separately.
type cookieDeliveryRuntime struct{ *goauth.Runtime }

func (r cookieDeliveryRuntime) BeginMFAEnrollment(
	ctx context.Context, request goauth.MFAEnrollmentRequest,
) (goauth.MFAEnrollment, error) {
	if err := ctx.Err(); err != nil {
		return goauth.MFAEnrollment{}, err
	}
	if request.Auth.SubjectID.IsZero() || request.Auth.SessionID == "" ||
		request.Auth.Realm != goauth.RealmAdmin || request.Binding == ([32]byte{}) || request.Password != testAdminPassword {
		return goauth.MFAEnrollment{}, goauth.ErrMFALoginDenied
	}
	return goauth.MFAEnrollment{
		ID: "cookie-delivery-enrollment", Secret: "synthetic-test-material", ExpiresAt: time.Now().Add(time.Minute),
	}, nil
}

type cookieDeliveryAdmins map[goauth.SubjectID]models.Admin

func (a cookieDeliveryAdmins) GetBySubjectID(_ context.Context, id goauth.SubjectID) (models.Admin, error) {
	admin, ok := a[id]
	if !ok {
		return models.Admin{}, errors.New("unknown test admin")
	}
	return admin, nil
}

func TestCompletedMFAResponseCannotReplaceNewLoginCookie(t *testing.T) {
	for _, newFirst := range []bool{false, true} {
		name := "old-response-first"
		if newFirst {
			name = "new-response-first"
		}
		t.Run(name, func(t *testing.T) { testCompletedMFACookieDelivery(t, newFirst) })
	}
}

func testCompletedMFACookieDelivery(t *testing.T, newFirst bool) {
	t.Helper()
	fixture, err := testkit.NewRuntime()
	require.NoError(t, err)
	admins := cookieDeliveryAdmins{}
	accounts := make(map[string]goauth.Account)
	for i, name := range []string{"a", "b"} {
		account, err := fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
			Email: name + "@cookie-delivery.example.test", Password: testAdminPassword,
		})
		require.NoError(t, err)
		accounts[name] = account
		admins[account.Subject.ID] = models.Admin{ID: int64(i + 1), SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
	}
	store := session.NewStore()
	t.Cleanup(func() { require.NoError(t, store.Storage.Close()) })
	svc := NewService(store, roleServiceStub{},
		WithRuntime(cookieDeliveryRuntime{Runtime: fixture.Runtime}, admins),
		WithBrowserState(browserstate.NewMemory()))
	app := fiber.New()
	app.Post("/login/:who", func(c fiber.Ctx) error {
		_, _, err := svc.LoginAdmin(c, accounts[c.Params("who")].PrimaryEmail.DisplayValue, testAdminPassword, true)
		if err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/auth/profile/settings/mfa/begin", func(c fiber.Ctx) error {
		// The production authentication middleware reads first; management reads
		// again. Both projection saves must be cookie-neutral.
		if _, err := svc.GetAdminAuth(c); err != nil {
			return err
		}
		result, err := svc.ManageLocalMFA(c, "begin", MFAManagementInput{Password: testAdminPassword})
		if err != nil {
			return err
		}
		if result.Enrollment == nil {
			return fiber.ErrInternalServerError
		}
		c.Cookie(&fiber.Cookie{Name: "unrelated", Value: "preserved", Path: "/"})
		return c.SendStatus(http.StatusOK)
	})
	app.Post("/logout", func(c fiber.Ctx) error {
		if err := svc.DelAdminAuth(c); err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	app.Get("/profile", func(c fiber.Ctx) error {
		admin, err := svc.GetAdminAuth(c)
		if err != nil || admin == nil || admin.SubjectID != accounts["b"].Subject.ID {
			return fiber.ErrUnauthorized
		}
		return c.SendStatus(http.StatusNoContent)
	})
	origin, err := url.Parse("https://admin.example.test/")
	require.NoError(t, err)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	request := func(method, path string, status int) *http.Response {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
		for _, cookie := range jar.Cookies(origin) {
			req.AddCookie(cookie)
		}
		response, err := app.Test(req)
		require.NoError(t, err)
		body, readErr := io.ReadAll(response.Body)
		require.NoError(t, readErr)
		require.NoError(t, response.Body.Close())
		require.Equal(t, status, response.StatusCode, "test endpoint %s: %s", path, body)
		return response
	}
	//nolint:bodyclose // request reads and closes the body before returning headers.
	loginA := request(http.MethodPost, "/login/a", http.StatusNoContent)
	jar.SetCookies(origin, loginA.Cookies())
	oldCookie := requireSessionCookie(t, loginA)
	// Finish the entire server response BEFORE logout/login. Only its browser
	// delivery is withheld, matching the Playwright transport-level reproducer.
	//nolint:bodyclose // request reads and closes the body before returning headers.
	held := request(http.MethodPost, "/auth/profile/settings/mfa/begin", http.StatusOK)
	//nolint:bodyclose // request reads and closes the body before returning headers.
	logout := request(http.MethodPost, "/logout", http.StatusNoContent)
	require.Negative(t, requireSessionCookie(t, logout).MaxAge)
	jar.SetCookies(origin, logout.Cookies())
	//nolint:bodyclose // request reads and closes the body before returning headers.
	loginB := request(http.MethodPost, "/login/b", http.StatusNoContent)
	newCookie := requireSessionCookie(t, loginB)
	require.NotEqual(t, oldCookie.Value, newCookie.Value)
	ordered := []*http.Response{held, loginB}
	if newFirst {
		ordered = []*http.Response{loginB, held}
	}
	for _, response := range ordered {
		jar.SetCookies(origin, response.Cookies())
	}
	for _, cookie := range held.Cookies() {
		require.NotEqual(t, svc.sessionCookieName(), cookie.Name)
		require.NotEqual(t, svc.policyCookieName(), cookie.Name)
	}
	values := make(map[string]string)
	for _, cookie := range jar.Cookies(origin) {
		values[cookie.Name] = cookie.Value
	}
	require.Equal(t, newCookie.Value, values[svc.sessionCookieName()])
	require.Equal(t, "1", values[svc.policyCookieName()])
	require.Equal(t, "preserved", values["unrelated"])
	//nolint:bodyclose // request reads and closes the body before returning headers.
	request(http.MethodGet, "/profile", http.StatusNoContent)
}
