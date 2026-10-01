package loginaudit

import (
	"context"
	"time"

	authcore "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
)

// Record describes one successful admin login event captured by the host.
type Record struct {
	LoggedAt  time.Time
	AdminID   int64
	AdminUUID identity.UserID
	SubjectID authcore.SubjectID
	Email     string
	Name      string
	LastName  string
	Username  string
	IP        string
	UserAgent string
	RequestID string
}

// Writer persists successful admin login events into host-owned storage.
type Writer interface {
	RecordSuccessfulLogin(ctx context.Context, record Record) error
}
