package operations

import (
	"context"
	"time"
)

const operationTimeout = 2 * time.Minute

// newDetachedOperationContext creates a bounded background context for manual
// admin operations that trigger global side effects.
func newDetachedOperationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), operationTimeout)
}
