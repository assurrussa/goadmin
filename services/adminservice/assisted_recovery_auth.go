package adminservice

import (
	"context"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
)

// CurrentLocalAuth revalidates the server-side local browser session. External
// SSO bindings never become local administrator password authority.
func (s *Service) CurrentLocalAuth(c fiber.Ctx) (goauth.AuthContext, error) {
	auth, _, err := s.currentMFAAuth(c)
	return auth, err
}

// FindRecoveryAccount resolves an established canonical email without relying on
// optional users/admin projection screens. Callers must enforce recovery permission.
func (s *Service) FindRecoveryAccount(ctx context.Context, email string) (goauth.Account, error) {
	finder, ok := s.runtime.(interface {
		FindAccount(ctx context.Context, identifier goauth.IdentifierInput) (goauth.Account, error)
	})
	if !ok {
		return goauth.Account{}, goauth.ErrAssistedRecoveryUnavailable
	}
	return finder.FindAccount(ctx, goauth.IdentifierInput{Scheme: goauth.IdentifierSchemeEmail, Value: email})
}
