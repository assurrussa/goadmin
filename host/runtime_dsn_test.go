package host_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
)

func TestNewPgsqlClientRejectsUnsupportedDSNBeforeConnecting(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{
		"postgres://admin:synthetic_secret@localhost/db?search_path=private_secret",
		"host=localhost password=synthetic_secret options='-c search_path=private_secret'",
		"host=localhost password=synthetic_secret role=private_secret",
	} {
		client, err := adminhost.NewPgsqlClient(t.Context(), adminhost.PgsqlConfig{DSN: dsn}, nil)
		require.Error(t, err)
		require.Nil(t, client)
		require.NotContains(t, err.Error(), "synthetic_secret")
		require.NotContains(t, err.Error(), "private_secret")
	}
}
