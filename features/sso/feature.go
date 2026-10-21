package sso

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/assurrussa/goauth"
	browser "github.com/assurrussa/gossoclient"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"

	"github.com/assurrussa/goadmin/host"
)

type Feature struct {
	rp    *Runtime
	links LinkResolver
	app   *host.App
	csrf  *host.CSRFService
}

func NewFeature(rp *Runtime, links LinkResolver, csrf *host.CSRFService, csrfCookie string) *Feature {
	if rp != nil {
		rp.csrfCookie = csrfCookie
		if rp.csrfCookie == "" {
			rp.csrfCookie = "csrf_token"
		}
	}
	return &Feature{rp: rp, links: links, csrf: csrf}
}

func (*Feature) Descriptor() host.FeatureDescriptor {
	return host.FeatureDescriptor{Key: "goadmin-sso"}
}

func (f *Feature) Build(host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		if app == nil || f.rp == nil || f.rp.Client == nil || f.links == nil || f.csrf == nil {
			return nil, ErrDenied
		}
		f.app = app
		return host.NewExtension().WithMiddlewares(f.ingress).WithPublicRegister(f.register), nil
	}
}

func (f *Feature) ingress(c fiber.Ctx) error {
	r, err := adaptor.ConvertRequest(c, true)
	if err != nil || !f.rp.Config.AllowsRequest(r) {
		switch c.Path() {
		case "/sso", "/login", callbackPath, "/sso/open", "/logout", "/sso-link", "/auth/session/recover":
			return c.SendStatus(http.StatusNotFound)
		}
		// The optional adapter does not own health endpoints or other host routing.
		return c.Next()
	}
	host.EnableExternalAdminRecoveryWithSignIn(c, "/sso")
	f.app.WithProp(c, "adminSSOEnabled", true)
	return c.Next()
}

func (f *Feature) register(app *fiber.App) {
	app.Get("/sso", f.page)
	app.Post("/login", f.begin)
	app.Get(callbackPath, f.callback)
	app.Get("/sso/open", f.open)
	app.Post("/logout", f.logout)
	app.All("/auth/session/recover", f.recover)
}

func (f *Feature) page(c fiber.Ctx) error {
	if c.Get("X-Inertia") != "" {
		c.Set("X-Inertia-Location", c.OriginalURL())
		return c.SendStatus(http.StatusConflict)
	}
	return serveHTTP(c, func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path, r.URL.RawPath, r.URL.RawQuery, r.RequestURI = "/", "", "", "/"
		target := safeReturnPath(c.Query("redirectback"), f.rp.Config.Origin)
		f.rp.Client.Handler(func(w http.ResponseWriter, r *http.Request, view browser.View) {
			renderLanding(w, r, view, target, f.rp.Config.Issuer)
		}).ServeHTTP(w, r)
	})
}

func (f *Feature) callback(c fiber.Ctx) error {
	if c.OriginalURL() == callbackPath {
		return c.Redirect().Status(http.StatusSeeOther).To("/sso")
	}
	return serveHTTP(c, func(w http.ResponseWriter, r *http.Request) {
		outcome, err := f.rp.Client.CallbackResult(w, r)
		if err != nil {
			return
		}
		target, err := f.rp.takeLoginReturn(r.Context(), r.URL.Query().Get("state"))
		if err == nil {
			err = f.rp.saveReturn(r.Context(), outcome.Proof().Binding(), target)
		}
		if err != nil {
			http.Error(w, "Не удалось сохранить путь возврата", http.StatusServiceUnavailable)
			return
		}
		// End the cross-site redirect chain on a document before touching native
		// Strict cookies. Only /sso/open performs native admission.
		writeContinuation(w)
	})
}

func (f *Feature) open(c fiber.Ctx) error {
	r, err := adaptor.ConvertRequest(c, true)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	proof, err := f.rp.Client.Verify(r.WithContext(c.Context()))
	if err != nil {
		if errors.Is(err, browser.ErrUnavailable) {
			return f.failure(c, http.StatusServiceUnavailable, "provider_unavailable",
				"Сервис авторизации временно недоступен. Повторите вход позже.", false)
		}
		return f.failure(c, http.StatusUnauthorized, "sso_session_expired",
			"Подтверждение SSO истекло. Вернитесь к странице входа.", false)
	}
	target, err := f.rp.peekReturn(c.Context(), proof.Binding())
	if err != nil {
		return f.failure(c, http.StatusServiceUnavailable, "return_store_unavailable",
			"Не удалось восстановить путь возврата. Повторите вход позже.", false)
	}
	binding, err := f.rp.resolve(c.Context(), proof, f.links)
	if errors.Is(err, goauth.ErrIdentityLinkNotFound) {
		if f.rp.Config.LocalLoginDisabled {
			return f.failure(c, http.StatusForbidden, "identity_not_linked",
				"Аккаунт AuthHub не связан с местным администратором. Локальный вход отключён "+
					"политикой сайта: обратитесь к администратору для настройки привязки.", false, target)
		}
		return f.failure(c, http.StatusForbidden, "identity_not_linked",
			"Вы вошли в AuthHub, но эта учётная запись ещё не связана с локальным "+
				"администратором. Войдите локально и подтвердите привязку.", true, target)
	}
	if err != nil {
		if !admissionDenied(err) {
			return f.failure(c, http.StatusServiceUnavailable, "identity_store_unavailable",
				"Не удалось проверить локальный доступ. Повторите попытку позже.", false, target)
		}
		return f.failure(c, http.StatusForbidden, "identity_not_admitted",
			"Учётная запись не допущена к этой админке. Доступ в AuthHub сам по себе не предоставляет локальных прав.", false, target)
	}

	if _, err = f.app.LoginExternalAdmin(c, binding); err != nil {
		if !admissionDenied(err) {
			return f.failure(c, http.StatusServiceUnavailable, "admin_admission_unavailable",
				"Проверка допуска временно недоступна. Повторите вход позже.", false, target)
		}
		return f.failure(c, http.StatusForbidden, "admin_admission_rejected",
			"SSO подтверждено, но доступ администратора не предоставлен. Обратитесь к администратору сайта.", false, target)
	}
	// Native admission is committed. Navigation cleanup must not change it into an error.
	cleanup, cancel := context.WithTimeout(c.Context(), 500*time.Millisecond)
	_, _ = f.rp.takeReturn(cleanup, proof.Binding())
	cancel()
	// GoAdmin owns the normalized local landing; browser target params are ignored.
	return c.Redirect().Status(http.StatusSeeOther).To("/auth/login?local=1&redirectback=" + url.QueryEscape(target))
}

func (f *Feature) logout(c fiber.Ctx) error {
	return serveHTTP(c, func(w http.ResponseWriter, r *http.Request) {
		result := httptest.NewRecorder()
		f.rp.Client.Logout(result, r)
		terminal := result.Code == http.StatusSeeOther ||
			(result.Code == http.StatusServiceUnavailable && result.Header().Get("X-AuthHub-Logout-Scope") == "local-only")
		if terminal {
			//nolint:contextcheck // The public Fiber facade takes its request context from c.
			if err := f.app.DetachAdmin(c); err != nil {
				for _, value := range result.Header().Values("Set-Cookie") {
					w.Header().Add("Set-Cookie", value)
				}
				http.Error(w, "Не удалось подтвердить локальный выход", http.StatusServiceUnavailable)
				return
			}
		}
		for key, values := range result.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		if result.Code == http.StatusSeeOther {
			w.Header().Set("Location", "/sso")
		}
		w.WriteHeader(result.Code)
		_, _ = w.Write(result.Body.Bytes())
	})
}

func serveHTTP(c fiber.Ctx, handler http.HandlerFunc) error {
	r, err := adaptor.ConvertRequest(c, true)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	w := httptest.NewRecorder()
	handler(w, r.WithContext(c.Context()))
	for name, values := range w.Header() {
		if name != "Set-Cookie" {
			c.Response().Header.Del(name)
		}
		for _, value := range values {
			c.Response().Header.Add(name, value)
		}
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Status(w.Code).Send(w.Body.Bytes())
}

const beginScript = "const f=document.getElementById('sso-begin');" +
	"history.replaceState(null,'','/auth/login?local=1&redirectback='+" +
	"encodeURIComponent(f.elements['return'].value));f.submit();"

var landing = template.Must(template.New("sso").Parse(`<!doctype html><html lang="ru">
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Переход к SSO</title>
<form id="sso-begin" method="post" action="/login"><input type="hidden" name="csrf" value="{{.CSRF}}">
<input type="hidden" name="return" value="{{.Return}}">
<noscript><p>JavaScript отключён. Нажмите кнопку для перехода.</p><button type="submit">Войти через SSO</button></noscript></form>
<script>` + beginScript + `</script><p>Переходим к авторизации…</p>
<p><a href="/auth/login?local=1">Вернуться к локальному входу</a></p></html>`))

func renderLanding(w http.ResponseWriter, _ *http.Request, view browser.View, target, issuer string) {
	hash := sha256.Sum256([]byte(beginScript))
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'sha256-"+base64.StdEncoding.EncodeToString(hash[:])+
			"'; form-action 'self' "+issuer+"; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "strict-origin")
	_ = landing.Execute(w, struct {
		browser.View
		Return string
	}{view, target})
}

// The SDK owns OAuth state, PKCE and CSRF. This wrapper only associates a
// validated navigation target after Begin accepts the request.
func (f *Feature) begin(c fiber.Ctx) error {
	return serveHTTP(c, func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid login form", http.StatusBadRequest)
			return
		}
		target := safeReturnPath(r.PostForm.Get("return"), f.rp.Config.Origin)
		result := httptest.NewRecorder()
		f.rp.Client.Begin(result, r)
		if result.Code == http.StatusSeeOther || result.Code == http.StatusFound {
			location, err := url.Parse(result.Header().Get("Location"))
			if err != nil || location.Scheme+"://"+location.Host != f.rp.Config.Issuer ||
				f.rp.saveLoginReturn(r.Context(), location.Query().Get("state"), target) != nil {
				http.Error(w, "Не удалось начать вход", http.StatusServiceUnavailable)
				return
			}
		}
		for key, values := range result.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(result.Code)
		_, _ = w.Write(result.Body.Bytes())
	})
}

func (f *Feature) failure(c fiber.Ctx, status int, code, message string, link bool, targets ...string) error {
	target := "/"
	if len(targets) > 0 {
		target = safeReturnPath(targets[0], f.rp.Config.Origin)
	}
	c.Status(status)
	return f.app.Render(c, "auth/SSOErrorPage", map[string]any{
		titleProp:     "Не удалось завершить вход",
		errorCodeProp: code, messageProp: message, "linkAvailable": link, "startAvailable": true, "returnTarget": target,
	})
}

func admissionDenied(err error) bool {
	return errors.Is(err, ErrDenied) || errors.Is(err, goauth.ErrIdentityLinkNotFound) ||
		errors.Is(err, goauth.ErrAccountNotFound) ||
		errors.Is(err, goauth.ErrAccountUnavailable) || errors.Is(err, goauth.ErrMembershipDenied) ||
		errors.Is(err, goauth.ErrSessionRevoked) || errors.Is(err, goauth.ErrIdentityLinkConflict) ||
		errors.Is(err, goauth.ErrExplicitIdentityLink)
}
