package adminservice

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
)

// ValidateBrowserCredentials rejects ambiguous cookies before session lookup.
func (s *Service) ValidateBrowserCredentials(c fiber.Ctx, extraNames ...string) error {
	if s == nil || s.store == nil {
		return nil // GetAdminAuth reports missing runtime dependencies.
	}
	return browsercookie.Check(c, s.store, extraNames...)
}

// RecoverAmbiguousBrowserCookies starts a public auth GET as anonymous. Call
// only after the cookie policy rejected duplicate cookies, never Authorization.
// Do not read, destroy or revoke any of the untrusted incoming session IDs.
func (s *Service) RecoverAmbiguousBrowserCookies(c fiber.Ctx, extraNames ...string) {
	names := append([]string{s.sessionCookieName(), s.policyCookieName()}, extraNames...)
	for _, name := range names {
		c.Request().Header.DelCookie(name)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	host := strings.ToLower(c.Hostname())
	domain := strings.ToLower(strings.TrimPrefix(s.store.CookieDomain, "."))
	canonicalDomain := domain
	if canonicalDomain == "" {
		canonicalDomain = host
	}
	paths := recoveryCookiePaths(c.Path(), s.store.CookiePath)
	for _, name := range names {
		// The canonical scope goes first: Fiber's next session save replaces
		// this expiry with its fresh anonymous cookie by name. Append other
		// scopes as separate fields; Fiber Cookie() would replace by name.
		s.appendRecoveryCookie(c, name, domain, s.store.CookiePath)
		for _, cookieDomain := range recoveryCookieDomains(host, domain) {
			for _, cookiePath := range paths {
				normalizedDomain := cookieDomain
				if normalizedDomain == "" {
					normalizedDomain = host
				}
				if normalizedDomain == canonicalDomain && cookiePath == s.store.CookiePath {
					continue
				}
				// Browsers accept __Host- cookies only without Domain and at /.
				if strings.HasPrefix(name, "__Host-") && (cookieDomain != "" || cookiePath != "/") {
					continue
				}
				s.appendRecoveryCookie(c, name, cookieDomain, cookiePath)
			}
		}
	}
}

func (s *Service) appendRecoveryCookie(c fiber.Ctx, name, domain, cookiePath string) {
	cookie := http.Cookie{ //nolint:gosec // Secure follows the validated store config; local development permits HTTP.
		Name: name, Path: cookiePath, Domain: domain, Secure: s.store.CookieSecure,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0),
	}
	c.Response().Header.Add(fiber.HeaderSetCookie, cookie.String())
}

func recoveryCookieDomains(host, configured string) []string {
	domains := []string{""}
	if configured != "" && configured != host {
		domains = append(domains, configured)
	}
	if net.ParseIP(host) != nil {
		return domains
	}
	for current := host; strings.Contains(current, "."); {
		if current != configured {
			domains = append(domains, current)
		}
		_, current, _ = strings.Cut(current, ".")
	}
	return domains
}

func recoveryCookiePaths(requestPath, configured string) []string {
	paths := []string{"/"}
	for index := 1; index < len(requestPath); index++ {
		if requestPath[index] == '/' {
			paths = append(paths, requestPath[:index], requestPath[:index+1])
		}
	}
	paths = append(paths, requestPath)
	if configured != "/" && configured != requestPath {
		paths = append(paths, configured)
	}
	return paths
}
