package sso //nolint:testpackage // Internal boundary and rendering contracts.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/assurrussa/goauth"
	browser "github.com/assurrussa/gossoclient"
	"github.com/gofiber/fiber/v3"
)

func TestDisabledRuntimeHasNoSideEffects(t *testing.T) {
	runtime, err := NewRuntime(t.Context(), Config{}, nil)
	if err != nil || runtime != nil {
		t.Fatalf("disabled runtime: %v %v", runtime, err)
	}
	if len(runtime.Modules(nil, nil, "")) != 0 {
		t.Fatal("disabled runtime installed routes")
	}
}

func TestSchemaCannotInjectSQL(t *testing.T) {
	for _, s := range []string{`a"; DROP SCHEMA public; --`, "public.x", "a b", "../x", ""} {
		if err := MigrateSchema(t.Context(), nil, s); err == nil {
			t.Fatal("accepted invalid schema or nil database")
		}
	}
}

func TestLandingAutostartsWithProtectedPOST(t *testing.T) {
	w := httptest.NewRecorder()
	renderLanding(w, nil, browser.View{}, "/materials")
	if w.Header().Get("Referrer-Policy") != "strict-origin" {
		t.Fatal("native form POST must preserve Origin")
	}
	body := w.Body.String()
	for _, fragment := range []string{beginScript, `method="post"`, `name="csrf"`, `name="return"`, "<noscript>"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("missing %s", fragment)
		}
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'sha256-") {
		t.Fatal("missing exact script policy")
	}
}

func TestAdapterBoundaryDoesNotCaptureHealthEndpoints(t *testing.T) {
	f := &Feature{rp: &Runtime{Config: Config{Enabled: true, Origin: testAdminOrigin, HTTPSReverseProxy: true}}}
	app := fiber.New()
	app.Use(f.ingress)
	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendStatus(200) })
	app.Get("/sso", func(c fiber.Ctx) error { return c.SendStatus(200) })
	for _, tc := range []struct {
		path   string
		status int
	}{{"/healthz", 200}, {"/sso", 404}} {
		response, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "http://localhost"+tc.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatalf("%s: %d", tc.path, response.StatusCode)
		}
	}
}

func TestStorageFailureIsNotPermanentAdmissionDenial(t *testing.T) {
	if admissionDenied(errors.New("database unavailable")) {
		t.Fatal("storage error classified as denial")
	}
	if !admissionDenied(goauth.ErrMembershipDenied) || !admissionDenied(ErrDenied) {
		t.Fatal("explicit denial not preserved")
	}
}

func TestRecoveryStorageFailureIsRetryable(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error { return recoveryFailure(c, errors.New("database timeout")) })
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/test", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("storage failure requires retry, not reauthentication")
	}
}
