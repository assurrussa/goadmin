package host

import (
	"context"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	"github.com/assurrussa/goadmin/models"
)

// Actor is the narrow authenticated-admin identity exposed to host features.
// It intentionally carries no permissions; features must authorize actions
// through the runtime permission guard instead of trusting session claims.
type Actor struct {
	AdminID   int64  `json:"adminId"`
	SubjectID string `json:"subjectId"`
}

// IsZero reports whether the actor lacks its canonical admin identity.
func (a Actor) IsZero() bool {
	return a.AdminID <= 0 || a.SubjectID == ""
}

// CurrentActor returns the canonical actor for the current admin request.
// It is safe on unauthenticated contexts and never panics.
func (a *App) CurrentActor(ctx context.Context) (Actor, bool) {
	if a == nil || ctx == nil {
		return Actor{}, false
	}

	return actorFromSession(adminmiddleware.GetAdminAuth(ctx))
}

func actorFromSession(admin *models.SessionAdmin) (Actor, bool) {
	if admin == nil || admin.ID <= 0 {
		return Actor{}, false
	}

	subjectID := admin.AuthSubjectID()
	if subjectID.IsZero() {
		return Actor{}, false
	}

	actor := Actor{
		AdminID:   admin.ID,
		SubjectID: subjectID.String(),
	}

	return actor, true
}
