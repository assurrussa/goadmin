package main

import (
	"encoding/json"
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
			h := journalAgent{grants: newJournalGrants(), origin: origin, trustedProxy: tc.proxy}
			now := time.Now()
			h.grants.now = func() time.Time { return now }
			pair := func(client netip.Addr) int {
				t.Helper()
				peer := client
				req := httptest.NewRequest(http.MethodPost, origin+agentPath+"/pair", nil)
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
	h := journalAgent{grants: newJournalGrants(), origin: origin}
	h.registerDevice(app)
	request := func(path, body string, headers map[string]string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, origin+agentPath+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}
	for _, headers := range []map[string]string{{"Origin": origin}, {"Cookie": "goadmin_session=unused"}, {"Sec-Fetch-Site": "same-origin"}} {
		if result := request("/pair", "{}", headers); result.StatusCode != 403 {
			t.Fatalf("browser request returned %d", result.StatusCode)
		}
	}
	if result := request("/connect", "{}", nil); result.StatusCode != 404 {
		t.Fatal("approval was publicly mounted")
	}
	if result := request("/get", `{"event_id":"1"}`, nil); result.StatusCode != 401 {
		t.Fatalf("unpaired lookup returned %d", result.StatusCode)
	}
	if result := request("/pair", `{"actor":"9"}`, nil); result.StatusCode != 400 {
		t.Fatalf("unrecognized input returned %d", result.StatusCode)
	}
	pair := request("/pair", "{}", nil)
	if pair.StatusCode != 200 || pair.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("pairing status %d", pair.StatusCode)
	}
	var data struct {
		Code   string
		Secret string `json:"device_secret"`
		URL    string `json:"verification_url"`
	}
	if err := json.NewDecoder(pair.Body).Decode(&data); err != nil || data.Code == "" || data.Secret == "" || data.URL != origin+connectPath {
		t.Fatal("incomplete device response", err)
	}
	if r := request("/token", `{"device_secret":"`+data.Code+`"}`, nil); r.StatusCode != 410 {
		t.Fatal("human code accepted as device credential")
	}
	if r := request("/token", `{"device_secret":"`+data.Secret+`"}`, nil); r.StatusCode != 202 {
		t.Fatal("device polling did not await approval")
	}
}
