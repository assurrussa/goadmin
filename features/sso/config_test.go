package sso //nolint:testpackage // Tests exercise internal binding and recovery invariants.

import (
	"crypto/tls"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{
		Enabled: true, Origin: testAdminOrigin, Issuer: "https://auth.example.test",
		ClientID: "11111111-1111-4111-8111-111111111111", ProjectID: "22222222-2222-4222-8222-222222222222",
		ClientSecret: "test-client-secret", StorageKey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))),
		TrustedProxyCIDRs: "172.18.0.0/16",
	}
}

func TestDisabledNeedsNoProviderConfiguration(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigurationFailsClosed(t *testing.T) {
	c := testConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Config){
		"http issuer":        func(c *Config) { c.Issuer = "http://auth.example.test" },
		"invalid origin":     func(c *Config) { c.Origin = "https://admin.example.test/path" },
		"missing secret":     func(c *Config) { c.ClientSecret = "" },
		"short key":          func(c *Config) { c.StorageKey = base64.StdEncoding.EncodeToString([]byte("short")) },
		"world proxy":        func(c *Config) { c.TrustedProxyCIDRs = "0.0.0.0/0" },
		"noncanonical proxy": func(c *Config) { c.TrustedProxyCIDRs = "172.18.0.1/16" },
	} {
		t.Run(name, func(t *testing.T) {
			c := testConfig()
			change(&c)
			if c.Validate() == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestSocketPeerAndHostCannotBeReplacedByHeaders(t *testing.T) {
	c := testConfig()
	r := httptest.NewRequestWithContext(t.Context(), "GET", c.Origin+"/sso", nil)
	r.TLS = nil
	r.RemoteAddr = "203.0.113.1:1234"
	r.Header.Set("X-Forwarded-For", "172.18.0.2")
	r.Header.Set("X-Forwarded-Proto", "https")
	if c.AllowsRequest(r) {
		t.Fatal("forwarding headers granted authority")
	}
	r.RemoteAddr = "172.18.0.2:1234"
	if !c.AllowsRequest(r) {
		t.Fatal("configured peer rejected")
	}
	r.Host = "www.example.test"
	if c.AllowsRequest(r) {
		t.Fatal("public host accepted")
	}
	r.TLS = &tls.ConnectionState{}
	if c.AllowsRequest(r) {
		t.Fatal("TLS bypassed canonical host")
	}
	r.Host = "admin.example.test"
	if !c.AllowsRequest(r) {
		t.Fatal("direct canonical HTTPS rejected")
	}
	c.Enabled = false
	if c.AllowsRequest(r) {
		t.Fatal("disabled boundary accepted request")
	}
}

func TestDeploymentTransportProfiles(t *testing.T) {
	c := testConfig()
	c.TrustedProxyCIDRs = ""
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequestWithContext(t.Context(), "GET", c.Origin+"/sso", nil)
	r.TLS = nil
	r.Header.Set("X-Forwarded-Proto", "https")
	if c.AllowsRequest(r) {
		t.Fatal("headers enabled proxy mode")
	}
	c.HTTPSReverseProxy = true
	if !c.AllowsRequest(r) {
		t.Fatal("explicit HTTPS edge profile rejected")
	}
	r.Host = "attacker.example.test"
	r.Header.Set("X-Forwarded-Host", "admin.example.test")
	if c.AllowsRequest(r) {
		t.Fatal("proxy mode bypassed canonical Host")
	}
	r.Host = "admin.example.test"
	c.TrustedProxyCIDRs = "172.18.0.0/16"
	r.RemoteAddr = "203.0.113.1:1234"
	if c.AllowsRequest(r) {
		t.Fatal("profile bypassed optional peer restriction")
	}
}

func TestAuthHubClientDeterminesProjectWhenOmitted(t *testing.T) {
	c := testConfig()
	c.ProjectID = ""
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDisabledRuntimeDoesNotAccessDependencies(t *testing.T) {
	runtime, err := NewRuntime(t.Context(), Config{}, nil)
	if err != nil || runtime != nil {
		t.Fatalf("disabled runtime: %v, %v", runtime, err)
	}
}
