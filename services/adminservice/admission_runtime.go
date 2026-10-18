package adminservice

import "github.com/assurrussa/goauth/postgres"

// UsesCanonicalRuntime prevents an optional enrollment module from mutating a
// different authority than the native browser admission service.
func (s *Service) UsesCanonicalRuntime(rt *postgres.Runtime) bool {
	return s != nil && rt != nil && (s.runtime == rt || s.runtime == rt.Runtime)
}
