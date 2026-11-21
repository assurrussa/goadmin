package admintx

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type sessionReadKey struct{}

// WithSessionReadTx lets the native browser stores participate in the same
// locked authorization check as the host mutation, even with a one-connection pool.
func WithSessionReadTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, sessionReadKey{}, tx)
}

func SessionReadTx(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(sessionReadKey{}).(pgx.Tx)
	return tx
}
