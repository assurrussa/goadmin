package host_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
)

const postgresSessionProductionEnv = "production"

func TestPostgresSessionsValidateConfig(t *testing.T) {
	_, err := host.NewPostgresSessions(host.AdminConfig{}, nil)
	require.ErrorContains(t, err, "database is required")
	db, err := sql.Open("pgx", "postgres://unused:unused@localhost/unused")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, cfg := range []host.AdminConfig{
		{SessionTTL: -time.Second},
		{SessionInactiveTTL: -time.Second},
		{SessionTTL: time.Minute, SessionInactiveTTL: 2 * time.Minute},
		{SessionTTL: time.Minute},
	} {
		_, err = host.NewPostgresSessions(cfg, db)
		require.Error(t, err)
	}
}

func TestPostgresSessionsCookiePolicyAndCancellation(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://unused:unused@localhost/unused")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	runtime, err := host.NewPostgresSessions(host.AdminConfig{
		Env: postgresSessionProductionEnv, Domain: "admin.test", SessionName: "browser",
		SessionTTL: 24 * time.Hour, SessionInactiveTTL: 30 * time.Minute,
	}, db)
	require.NoError(t, err)
	require.NotNil(t, runtime.Middleware)
	cfg := runtime.Store.Config
	require.Equal(t, "__Host-browser", cfg.Extractor.Key)
	require.Empty(t, cfg.CookieDomain)
	require.Equal(t, "/", cfg.CookiePath)
	require.True(t, cfg.CookieSecure)
	require.True(t, cfg.CookieHTTPOnly)
	require.True(t, cfg.CookieSessionOnly)
	require.Equal(t, "Lax", cfg.CookieSameSite)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, runtime.Run(ctx), context.Canceled)
	require.NoError(t, runtime.Store.Storage.Close())
	// Construction and cancelled maintenance never dial or close borrowed SQL.
	require.Zero(t, db.Stats().OpenConnections)
}
