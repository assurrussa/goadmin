package session

import (
	"context"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

const RecordEditFlashKeyPrefix = "goadmin:record-edit:"

// RecordEditFlashStorage keeps redirect feedback separate from browser sessions.
type RecordEditFlashStorage interface {
	SetRecordEditFlash(ctx context.Context, key string, payload []byte, ttl time.Duration) error
	TakeRecordEditFlash(ctx context.Context, key string) ([]byte, error)
}

// Fiber's standalone memory store has no atomic take API. Serialize the
// fallback within this process; production backends implement atomic take.
var recordEditFlashFallback sync.Mutex

func SetRecordEditFlash(ctx context.Context, storage fiber.Storage, key string, payload []byte, ttl time.Duration) error {
	if specialized, ok := storage.(RecordEditFlashStorage); ok {
		return specialized.SetRecordEditFlash(ctx, key, payload, ttl)
	}
	recordEditFlashFallback.Lock()
	defer recordEditFlashFallback.Unlock()
	return storage.SetWithContext(ctx, key, payload, ttl)
}

func TakeRecordEditFlash(ctx context.Context, storage fiber.Storage, key string) ([]byte, error) {
	if specialized, ok := storage.(RecordEditFlashStorage); ok {
		return specialized.TakeRecordEditFlash(ctx, key)
	}
	recordEditFlashFallback.Lock()
	defer recordEditFlashFallback.Unlock()
	payload, err := storage.GetWithContext(ctx, key)
	if err != nil || len(payload) == 0 {
		return payload, err
	}
	if err := storage.DeleteWithContext(ctx, key); err != nil {
		return nil, err
	}
	return payload, nil
}
