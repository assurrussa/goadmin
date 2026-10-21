// Package sso integrates the optional AuthHub browser boundary with native admin
// authority. It never grants local membership from provider claims alone.
package sso

import (
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	browser "github.com/assurrussa/gossoclient"
)

// Config contains only this admin origin's explicit provider configuration.
// Secrets must never be rendered in page props or diagnostic output.
type Config struct {
	// LocalLoginDisabled only controls UI. The host must enforce its admission policy.
	LocalLoginDisabled bool
	Enabled            bool
	// Schema is an existing dedicated RP schema. Empty selects goadmin_sso.
	Schema            string
	Origin            string
	Issuer            string
	ClientID          string
	ProjectID         string
	ClientSecret      string
	StorageKey        string
	IssuerCAFile      string
	TrustedProxyCIDRs string
	// HTTPSReverseProxy asserts an HTTPS-only edge and a non-public backend.
	// It is supplied by the deployment profile, never inferred from headers.
	HTTPSReverseProxy bool
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if !schemaPattern.MatchString(c.schema()) {
		return errors.New("invalid admin SSO schema")
	}
	sdkConfig := browser.Config{
		Issuer: c.Issuer, ClientID: c.ClientID, ProjectID: c.ProjectID,
		Origin: c.Origin, Callback: c.Origin + callbackPath, Freshness: 5 * time.Minute,
	}
	validate := browser.ValidateConfig
	if c.ProjectID == "" {
		validate = browser.ValidateAuthHubClientConfig
	}
	if err := validate(sdkConfig); err != nil {
		return errors.New("invalid admin SSO issuer, client, project or HTTPS origin")
	}
	if c.ClientSecret == "" || len(c.ClientSecret) > 4096 {
		return errors.New("admin SSO client secret is required")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(c.StorageKey)
	if err != nil || len(key) != 32 {
		return errors.New("admin SSO storage key must encode exactly 32 bytes")
	}
	if _, err = c.proxyNetworks(); err != nil {
		return err
	}
	return nil
}

func (c Config) proxyNetworks() ([]netip.Prefix, error) {
	if c.TrustedProxyCIDRs == "" {
		return nil, nil
	}
	if len(c.TrustedProxyCIDRs) > 4096 {
		return nil, errors.New("admin SSO proxy CIDRs exceed size limit")
	}
	parts := strings.Split(c.TrustedProxyCIDRs, ",")
	if len(parts) > 64 {
		return nil, errors.New("too many admin SSO proxy CIDRs")
	}
	networks := make([]netip.Prefix, 0, len(parts))
	for _, raw := range parts {
		p, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || p != p.Masked() || p.Addr().Is4In6() || !privateNetwork(p) {
			return nil, errors.New("invalid admin SSO private proxy CIDR")
		}
		networks = append(networks, p)
	}
	return networks, nil
}

func privateNetwork(p netip.Prefix) bool {
	for _, raw := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "::1/128", "fc00::/7"} {
		bound := netip.MustParsePrefix(raw)
		if bound.Addr().BitLen() == p.Addr().BitLen() && p.Bits() >= bound.Bits() && bound.Contains(p.Addr()) {
			return true
		}
	}
	return false
}

// AllowsRequest enforces the canonical host and deployment transport contract.
// Optional CIDRs check the actual socket peer, never forwarded headers.
// This is not authentication: SDK proof, CSRF and native authority remain required.
func (c Config) AllowsRequest(r *http.Request) bool {
	if !c.Enabled || r == nil {
		return false
	}
	origin, err := url.Parse(c.Origin)
	if err != nil || origin.Scheme != "https" || r.Host != origin.Host {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if c.TrustedProxyCIDRs == "" {
		return c.HTTPSReverseProxy
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip, parseErr := netip.ParseAddr(host)
	if err != nil || parseErr != nil {
		return false
	}
	networks, err := c.proxyNetworks()
	if err != nil {
		return false
	}
	for _, p := range networks {
		if p.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

var schemaPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func (c Config) schema() string {
	if c.Schema == "" {
		return "goadmin_sso"
	}
	return c.Schema
}
