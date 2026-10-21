package sso //nolint:testpackage // Tests exercise internal binding and recovery invariants.

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestContinuationHasFixedSameOriginDestination(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Location", "https://attacker.invalid")
	w.Header().Add("Set-Cookie", "__Host-sso-example=synthetic; Secure; Path=/; HttpOnly")
	writeContinuation(w)
	if w.Code != http.StatusOK || w.Header().Get("Location") != "" ||
		w.Header().Get("Cache-Control") != "no-store" || len(w.Header().Values("Set-Cookie")) != 1 {
		t.Fatal("continuation changed response or lost RP cookie")
	}
	hash := sha256.Sum256([]byte(continuationScript))
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "sha256-"+base64.StdEncoding.EncodeToString(hash[:])) ||
		!strings.Contains(w.Body.String(), "window.location.replace('/sso/open')") ||
		strings.Contains(w.Body.String(), "attacker") {
		t.Fatal("continuation script contract changed")
	}
}

func TestHTTPBridgePreservesBothCookieFamilies(t *testing.T) {
	app := fiber.New()
	app.Get("/bridge", func(c fiber.Ctx) error {
		c.Cookie(&fiber.Cookie{Name: "native", Value: "synthetic", Path: "/"})
		return serveHTTP(c, func(w http.ResponseWriter, _ *http.Request) {
			http.SetCookie(w, &http.Cookie{
				Name: "rp", Value: "synthetic", Path: "/",
				Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
			})
			w.WriteHeader(http.StatusOK)
		})
	})
	res, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/bridge", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if len(res.Cookies()) != 2 {
		t.Fatal("cookie family was overwritten")
	}
}

func TestRecoveryIntentRejectsCrossOriginAndUnsafeMethods(t *testing.T) {
	f := &Feature{rp: &Runtime{Config: Config{Origin: testAdminOrigin}}}
	app := fiber.New()
	app.All("/auth/session/recover", func(c fiber.Ctx) error {
		if f.recoveryIntent(c) {
			return c.SendStatus(http.StatusNoContent)
		}
		return c.SendStatus(http.StatusForbidden)
	})
	for _, tc := range []struct {
		name, method, site, origin, suffix string
		want                               int
	}{
		{"same origin", http.MethodGet, ssoSameOrigin, "", "", http.StatusNoContent},
		{"exact origin", http.MethodGet, "", testAdminOrigin, "", http.StatusNoContent},
		{"cross origin", http.MethodGet, "cross-site", "https://attacker.invalid", "", http.StatusForbidden},
		{"missing provenance", http.MethodGet, "", "", "", http.StatusForbidden},
		{"mutation", http.MethodPost, ssoSameOrigin, "", "", http.StatusForbidden},
		{"query", http.MethodGet, ssoSameOrigin, "", "?next=https://attacker.invalid", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tc.method, "/auth/session/recover"+tc.suffix, nil)
			req.Header.Set(ssoRecoveryHeader, "1")
			if tc.site != "" {
				req.Header.Set("Sec-Fetch-Site", tc.site)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			_, _ = io.Copy(io.Discard, res.Body)
			if res.StatusCode != tc.want {
				t.Fatalf("got %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}
