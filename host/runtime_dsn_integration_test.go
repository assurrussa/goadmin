//go:build integration

package host_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/tests"
)

func TestNewPgsqlClientPreservesDSNRuntimeParams(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	base, _, cleanup := tests.PrepareDB(ctx, t, "HostDSNRuntimeParams", tests.WithDatabasePathFilesMigration())
	t.Cleanup(func() { cleanup(context.Background()) })
	parsed, err := url.Parse(base.DB().Pool().Config().ConnString())
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("application_name", "goadmin dsn host")
	query.Set("TimeZone", "UTC")
	query.Set("statement_timeout", "7s")
	query.Set("lock_timeout", "2s")
	query.Set("idle_in_transaction_session_timeout", "30s")
	query.Set("search_path", "public")
	parsed.RawQuery = query.Encode()
	client, err := adminhost.NewPgsqlClient(ctx, adminhost.PgsqlConfig{DSN: parsed.String()}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	var app, zone, statement, lock, idle, schema string
	require.NoError(t, client.DB().Pool().QueryRow(ctx, `
SELECT current_setting('application_name'), current_setting('TimeZone'),
       current_setting('statement_timeout'), current_setting('lock_timeout'),
       current_setting('idle_in_transaction_session_timeout'), current_setting('search_path')`).
		Scan(&app, &zone, &statement, &lock, &idle, &schema))
	require.Equal(t, "goadmin dsn host", app)
	require.Equal(t, "UTC", zone)
	require.Equal(t, "7s", statement)
	require.Equal(t, "2s", lock)
	require.Equal(t, "30s", idle)
	require.Equal(t, "public", schema)
}
