package mcp //nolint:testpackage // Exercise the unexported MCP transport and grant state.

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestAgentPairingQuotaIsLocalToSource(t *testing.T) {
	const origin = "https://admin.example.test"
	for _, tc := range []struct {
		name  string
		proxy netip.Addr
	}{
		{name: "direct"},
		{name: "trusted proxy", proxy: netip.MustParseAddr("192.0.2.1")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandler(nil, Config{Origin: origin, TrustedProxy: tc.proxy}, testResource(), nil)
			now := time.Now()
			h.grants.now = func() time.Time { return now }
			pair := func(client netip.Addr) int {
				t.Helper()
				peer := client
				req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, origin+testResourcePath+"/pair", nil)
				if tc.proxy.IsValid() {
					peer = tc.proxy
					req.Header.Set("X-Real-IP", client.String())
				}
				app := fiber.New()
				app.Use(func(c fiber.Ctx) error {
					c.RequestCtx().SetRemoteAddr(net.TCPAddrFromAddrPort(netip.AddrPortFrom(peer, 44300)))
					return c.Next()
				})
				h.registerDevice(app)
				resp, err := app.Test(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				return resp.StatusCode
			}
			first := netip.MustParseAddr("192.0.2.10")
			for range maxPendingPerSource {
				if _, _, ok := h.grants.start(first); !ok {
					t.Fatal("could not reserve pending pairing")
				}
			}
			if got := pair(first); got != http.StatusTooManyRequests {
				t.Fatalf("source at capacity: got %d", got)
			}
			if got := pair(netip.MustParseAddr("192.0.2.20")); got != http.StatusOK {
				t.Fatalf("independent source: got %d", got)
			}
			now = now.Add(pairLifetime)
			if got := pair(first); got != http.StatusOK {
				t.Fatalf("source after pending codes expired: got %d", got)
			}
		})
	}
}

func TestAgentDeviceRoutesAreSeparateFromOperatorApproval(t *testing.T) {
	const origin = "http://localhost:8080"
	app := fiber.New()
	h := newHandler(nil, Config{Origin: origin}, testResource(), nil)
	h.registerDevice(app)
	request := func(path, body string, headers map[string]string, want int) (http.Header, []byte) {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, origin+testResourcePath+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s returned %d, want %d", path, resp.StatusCode, want)
		}
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.Header, raw
	}
	for _, headers := range []map[string]string{
		{"Origin": origin}, {"Cookie": "goadmin_session=unused"}, {"Sec-Fetch-Site": "same-origin"},
	} {
		request("/pair", "{}", headers, http.StatusForbidden)
	}
	request("/connect", "{}", nil, http.StatusNotFound)
	request("/get", `{"reading_id":"1"}`, nil, http.StatusUnauthorized)
	request("/pair", `{"actor":"9"}`, nil, http.StatusBadRequest)
	headers, pair := request("/pair", "{}", nil, http.StatusOK)
	if headers.Get("Cache-Control") != "no-store" {
		t.Fatal("pairing response is cacheable")
	}
	var data gatewayPairing
	if err := json.Unmarshal(pair, &data); err != nil || data.Code == "" || data.DeviceSecret == "" ||
		data.VerificationURL != origin+h.connectPath() {
		t.Fatal("incomplete device response", err)
	}
	request("/token", `{"device_secret":"`+data.Code+`"}`, nil, http.StatusGone)
	request("/token", `{"device_secret":"`+data.DeviceSecret+`"}`, nil, http.StatusAccepted)
}
