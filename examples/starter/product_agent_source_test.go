package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestPairingTrustedProxy(t *testing.T) {
	for value, want := range map[string]netip.Addr{
		"":                  {},
		"192.0.2.10":        netip.MustParseAddr("192.0.2.10"),
		"2001:db8::10":      netip.MustParseAddr("2001:db8::10"),
		"::ffff:192.0.2.10": netip.MustParseAddr("192.0.2.10"),
	} {
		ip, err := pairingTrustedProxy(value)
		if err != nil || ip != want {
			t.Errorf("trusted proxy %q: got %v, %v; want %v", value, ip, err, want)
		}
	}
	for _, value := range []string{"proxy.example", "192.0.2.0/24", "192.0.2.10:443", "0.0.0.0", "::", "fe80::1%lo0"} {
		if _, err := pairingTrustedProxy(value); err == nil {
			t.Errorf("accepted invalid trusted proxy %q", value)
		}
	}
}

func TestPairingSource(t *testing.T) {
	const proxy = "192.0.2.10"
	const client = "198.51.100.8"
	for _, tc := range []struct {
		name, trusted, peer string
		headers             []string
		want                string
	}{
		{"direct", "", client, nil, client},
		{"headers ignored by default", "", client, []string{proxy}, client},
		{"untrusted peer", proxy, client, []string{proxy}, client},
		{"trusted peer", proxy, proxy, []string{client}, client},
		{"mapped IPv4", proxy, "::ffff:192.0.2.10", []string{"::ffff:198.51.100.8"}, client},
		{"IPv6 client", proxy, proxy, []string{"2001:db8:0:0::4"}, "2001:db8::4"},
		{"missing", proxy, proxy, nil, ""},
		{"malformed", proxy, proxy, []string{"bad"}, ""},
		{"scoped", proxy, proxy, []string{"fe80::10%eth0"}, ""},
		{"chain", proxy, proxy, []string{proxy + ", " + client}, ""},
		{"duplicate", proxy, proxy, []string{client, client}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trusted, err := pairingTrustedProxy(tc.trusted)
			if err != nil {
				t.Fatal(err)
			}
			agent := journalAgent{trustedProxy: trusted}
			app := fiber.New()
			app.Get("/", func(c fiber.Ctx) error {
				c.RequestCtx().SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP(tc.peer), Port: 54321})
				ip, ok := agent.pairingSource(c)
				if tc.want == "" {
					if ok {
						t.Errorf("unexpected pairing source %v", ip)
					}
				} else if !ok || ip != netip.MustParseAddr(tc.want) {
					t.Errorf("pairing source = %v, %t; want %s", ip, ok, tc.want)
				}
				return c.SendStatus(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
			req.Header.Set("X-Forwarded-For", client)
			for _, value := range tc.headers {
				req.Header.Add("X-Real-IP", value)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
		})
	}
}
