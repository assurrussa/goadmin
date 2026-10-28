package adminservice

import (
	"net/http"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
)

// ExternalSessionMaintainer is an optional capability. Validate remains a
// read-only check; Renew is called only under the native maintenance owner.
// Implementations must preserve login lineage and use their own durable fence
// against refresh secret replay. No business operation or new login is allowed.
type ExternalSessionMaintainer interface {
	MaintenanceEnabled() bool
	Renew(request *http.Request, current ExternalBinding) (ExternalBinding, error)
}

// SupportsSessionMaintenance does not grant authority. It advertises only the
// protocol available to the already authenticated page's login type.
func (s *Service) SupportsSessionMaintenance(c fiber.Ctx) bool {
	if s == nil || s.states == nil {
		return false
	}
	binding, err := s.ExternalAdminBinding(c)
	if err != nil {
		return false
	}
	if binding == nil {
		return true
	}
	m, ok := s.externalAuthority.(ExternalSessionMaintainer)
	return ok && m.MaintenanceEnabled()
}

// MaintainAdminSession renews an existing login only. It never resurrects an
// expired journal or creates a canonical login from a provider proof.
func (s *Service) MaintainAdminSession(c fiber.Ctx) (MaintenanceState, error) {
	c.Locals("goadmin.sessionMaintenance", true)
	binding, err := s.ExternalAdminBinding(c)
	if err != nil {
		return MaintenanceState{}, err
	}
	if binding != nil && time.Until(binding.Deadline) <= 30*time.Second {
		if err := s.renewMaintenanceAuthority(c); err != nil {
			return MaintenanceState{}, err
		}
	}

	admin, err := s.GetAdminAuth(c)
	if err != nil {
		return MaintenanceState{}, err
	}
	if admin == nil {
		return MaintenanceState{}, goauth.ErrSessionRevoked
	}
	if err := s.TouchMaintenanceSession(c); err != nil {
		return MaintenanceState{}, err
	}
	return s.MaintenanceSnapshot(c)
}

func (s *Service) renewMaintenanceAuthority(c fiber.Ctx) error {
	maintainer, ok := s.externalAuthority.(ExternalSessionMaintainer)
	if !ok || !maintainer.MaintenanceEnabled() {
		return goauth.ErrSessionRevoked
	}
	snapshot, err := s.BeginOwnedExternalAdminRecovery(c)
	if err != nil {
		return err
	}
	r, err := adaptor.ConvertRequest(c, true)
	if err != nil {
		_ = s.CancelExternalAdminRecovery(c, snapshot)
		return err
	}
	fresh, err := maintainer.Renew(r.WithContext(c.Context()), snapshot.Binding())
	if err != nil {
		_ = s.CancelExternalAdminRecovery(c, snapshot)
		return err
	}
	return s.CompleteExternalAdminRecovery(c, snapshot, fresh)
}
