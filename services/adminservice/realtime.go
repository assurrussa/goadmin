package adminservice

import (
	"time"

	"github.com/assurrussa/goadmin/internal/realtimesession"
)

// BeginRealtimeAdmission fences authentication already in flight when an
// explicit logout succeeds. Each Service belongs to one admin runtime.
func (s *Service) BeginRealtimeAdmission() *realtimesession.Admission {
	if s == nil {
		return nil
	}
	// Bound stale authentication attempts as well as the later socket handoff.
	return s.realtimeSessions.Begin(30 * time.Second)
}
