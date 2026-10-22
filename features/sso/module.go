package sso

import "github.com/assurrussa/goadmin/host"

// Modules explicitly installs the browser adapter and authenticated linking UI.
// A disabled runtime has no routes, middleware, discovery or migration side effects.
// Supply Authority separately as Dependencies.ExternalAuthority before host.New.
func (s *Runtime) Modules(links LinkResolver, csrf *host.CSRFService, csrfCookie string) []host.Module {
	if s == nil || !s.Config.Enabled {
		return nil
	}
	feature := NewFeature(s, links, csrf, csrfCookie)
	link := s.LinkFeature(csrf)
	return []host.Module{
		host.DefinedFeatureModule(host.ModuleDescriptor{
			Key: feature.Descriptor().Key,
			Routes: []string{
				"GET /sso", "POST /login", "GET /callback",
				"GET /sso/open", "POST /logout", "GET /auth/session/recover",
			},
		}, feature),
		host.DefinedFeatureModule(host.ModuleDescriptor{
			Key:    link.Descriptor().Key,
			Routes: []string{"GET /sso-link", "POST /sso-link", "GET /sso-account", "GET /sso-provider-account", "POST /sso-link/reauth"},
		}, link),
	}
}
