package session

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
)

// StorageLeaseReader reads one stored session and the expiry of that same
// payload without extending the lease. A zero time means no storage expiry.
// For relative leases, base is an already sampled time from the clock that
// enforces the deadline.
type StorageLeaseReader interface {
	GetSessionWithExpiry(ctx context.Context, key string, base time.Time) ([]byte, time.Time, error)
}

func GetSessionWithExpiry(ctx context.Context, storage fiber.Storage, key string, base time.Time) ([]byte, time.Time, error) {
	reader, ok := storage.(StorageLeaseReader)
	if !ok {
		return nil, time.Time{}, errors.New("browser session storage does not expose its lease")
	}
	return reader.GetSessionWithExpiry(ctx, key, base)
}
