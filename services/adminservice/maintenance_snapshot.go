package adminservice

import (
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
)

type maintenanceProjectionKey struct{}

// MaintenanceState is scheduling metadata, never proof of authentication.
// Epoch is a one-way login identifier; it cannot be used as a credential.
type MaintenanceState struct {
	Epoch              string
	Revision           int64
	HardExpiresAt      time.Time
	AccessExpiresAt    time.Time
	ExternalValidUntil time.Time
	IdleCheckAfter     time.Duration
}

// MaintenanceSnapshot reads authority metadata without renewing any lease.
// Callers must authenticate before publishing it or using it to permit work.
func (s *Service) MaintenanceSnapshot(c fiber.Ctx) (MaintenanceState, error) {
	if s == nil || s.states == nil || s.store == nil {
		return MaintenanceState{}, goauth.ErrSessionRevoked
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return MaintenanceState{}, err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return MaintenanceState{}, err
	}
	defer sess.Release()
	record, found, err := s.states.Load(c, sess.ID())
	if err != nil {
		return MaintenanceState{}, err
	}
	if !found || record.Tokens.Session.ID == "" {
		return MaintenanceState{}, goauth.ErrSessionRevoked
	}
	if record.Owner != "" {
		return MaintenanceState{}, ErrRefreshInProgress
	}
	result := MaintenanceState{
		Epoch: browserstate.SessionEpoch(record.Tokens.Session.ID), Revision: record.Version,
		HardExpiresAt:   earliestMaintenanceExpiry(record.ExpiresAt, record.Tokens.Session.ExpiresAt),
		AccessExpiresAt: record.Tokens.AccessExpiresAt, IdleCheckAfter: s.maintenanceTouchInterval(),
	}
	if state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession); ok && state != nil {
		result.HardExpiresAt = earliestMaintenanceExpiry(result.HardExpiresAt, state.HardExpiresAt)
	}
	if browserHard, ok := sess.Get("goadmin.browserHardExpiresAt").(time.Time); ok {
		result.HardExpiresAt = earliestMaintenanceExpiry(result.HardExpiresAt, browserHard)
	}
	if record.External != nil {
		result.ExternalValidUntil = record.External.Deadline
		result.HardExpiresAt = earliestMaintenanceExpiry(result.HardExpiresAt, record.External.AbsoluteUntil)
	}
	if result.HardExpiresAt.IsZero() || !time.Now().Before(result.HardExpiresAt) {
		return MaintenanceState{}, goauth.ErrSessionRevoked
	}
	return result, nil
}

func earliestMaintenanceExpiry(times ...time.Time) time.Time {
	var result time.Time
	for _, deadline := range times {
		if !deadline.IsZero() && (result.IsZero() || deadline.Before(result)) {
			result = deadline
		}
	}
	return result
}

func (s *Service) maintenanceTouchInterval() time.Duration {
	interval := time.Minute
	if s.store.IdleTimeout > 0 {
		interval = min(interval, s.store.IdleTimeout/3)
	}
	return max(time.Millisecond, interval)
}

// TouchMaintenanceSession refreshes only Fiber's idle lease, at most once per
// interval. The caller must have authenticated this request first. It does not
// change journal TTL, absolute expiry or browser credential cookies.
func (s *Service) TouchMaintenanceSession(c fiber.Ctx) error {
	snapshot, err := s.MaintenanceSnapshot(c)
	if err != nil {
		return err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return err
	}
	defer sess.Release()
	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	if !ok || state == nil {
		return goauth.ErrSessionRevoked
	}
	if projection, ok := c.Locals(maintenanceProjectionKey{}).(*browserSession); ok && projection != nil {
		if projection.Version != snapshot.Revision || projection.SubjectID != state.SubjectID {
			return ErrRefreshRetryRequired
		}
		state = projection
	}
	if !state.MaintenanceTouchedAt.IsZero() && time.Since(state.MaintenanceTouchedAt) < s.maintenanceTouchInterval() {
		return nil
	}
	state.MaintenanceTouchedAt = time.Now()
	sess.Set(sessioncore.AuthAdminKey.String(), state)
	return sessioncore.SavePassive(c, s.store, sess)
}
