package adminservice

import (
 "github.com/assurrussa/goauth"
 "github.com/gofiber/fiber/v3"
)

// CurrentLocalAuth revalidates the server-side local browser session. External
// SSO bindings never become local administrator password authority.
func (s *Service) CurrentLocalAuth(c fiber.Ctx) (goauth.AuthContext, error) {
 auth, _, err := s.currentMFAAuth(c)
 return auth, err
}
