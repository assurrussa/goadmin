package hosttest

import (
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/infrastructure/core/session"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

// WithActor creates test-only middleware that supplies a canonical actor to
// host.App.CurrentActor and host.Command. It requires a positive admin ID and a
// valid, nonzero subject ID, which is normalized by the canonical subject parser.
// Each request gets a fresh identity-only session. No roles or permissions are
// granted; tests must configure their permission guard separately.
// Never mount this fixture middleware in a production application.
func WithActor(actor host.Actor) (fiber.Handler, error) {
	if actor.AdminID <= 0 {
		return nil, errors.New("hosttest: actor admin ID must be positive")
	}

	subjectID, err := authcore.ParseSubjectIDString(actor.SubjectID)
	if err != nil {
		return nil, fmt.Errorf("hosttest: parse actor subject ID: %w", err)
	}
	if subjectID.IsZero() {
		return nil, errors.New("hosttest: actor subject ID must be nonzero")
	}

	return func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{
			ID:        actor.AdminID,
			SubjectID: subjectID,
		})
		return c.Next()
	}, nil
}
