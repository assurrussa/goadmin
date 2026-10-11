package connectionconfig_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/connectionconfig"
)

func TestPgsqlRuntimeParamsPreserveSupportedValues(t *testing.T) {
	t.Parallel()

	for name, dsn := range map[string]string{
		"url": "postgres://admin:synthetic@localhost/goadmin?sslmode=disable&application_name=admin%20worker" +
			"&TimeZone=UTC&statement_timeout=7s&lock_timeout=2s&idle_in_transaction_session_timeout=30s&search_path=public",
		"keyword": "host=localhost user=admin password=synthetic dbname=goadmin sslmode=disable " +
			"application_name='admin worker' timezone=UTC statement_timeout=7s lock_timeout=2s " +
			"idle_in_transaction_session_timeout=30s search_path=public",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			params, err := connectionconfig.PgsqlRuntimeParams(dsn)
			require.NoError(t, err)
			require.Equal(t, map[string]string{
				"application_name": "admin worker", "timezone": "UTC",
				"statement_timeout": "7s", "lock_timeout": "2s",
				"idle_in_transaction_session_timeout": "30s", "search_path": "public",
			}, params)
			params["application_name"] = "mutated"
			again, err := connectionconfig.PgsqlRuntimeParams(dsn)
			require.NoError(t, err)
			require.Equal(t, "admin worker", again["application_name"])
		})
	}
}

func TestPgsqlRuntimeParamsRejectUnsupportedParametersWithoutValues(t *testing.T) {
	t.Parallel()

	for name, suffix := range map[string]string{
		"schema":                "search_path=private_secret",
		"schema-list":           "search_path=public,private_secret",
		"schema-quoted":         "search_path=%22private_secret%22",
		"options":               "options=-c%20search_path%3Dprivate_secret",
		"role":                  "role=private_secret",
		"session-authorization": "session_authorization=private_secret",
		"unknown":               "unrecognized_setting=private_secret",
		"transport":             "connect_timeout=10",
		"tls":                   "sslrootcert=private_secret",
		"pool":                  "pool_max_conns=12",
		"pgx-mode":              "default_query_exec_mode=simple_protocol",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dsn := "postgres://admin:synthetic_secret@localhost/goadmin?sslmode=disable&" + suffix
			params, err := connectionconfig.PgsqlRuntimeParams(dsn)
			require.Error(t, err)
			require.Nil(t, params)
			require.NotContains(t, err.Error(), "synthetic_secret")
			require.NotContains(t, err.Error(), "private_secret")
			require.NotContains(t, err.Error(), dsn)
			_, err = connectionconfig.NormalizePgsql(connectionconfig.PgsqlConfig{DSN: dsn})
			require.Error(t, err)
		})
	}
}

func TestPgsqlRuntimeParamsRejectKeywordSchemaAndOptions(t *testing.T) {
	t.Parallel()

	for _, suffix := range []string{
		"search_path='private_secret, public'",
		"options='-c search_path=private_secret'",
		"role='private_secret'",
	} {
		_, err := connectionconfig.PgsqlRuntimeParams("host=localhost password='synthetic_secret' " + suffix)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "synthetic_secret")
		require.NotContains(t, err.Error(), "private_secret")
	}
}

func TestPgsqlRuntimeParamsRejectConflictingTimezoneAliases(t *testing.T) {
	t.Parallel()

	_, err := connectionconfig.PgsqlRuntimeParams("host=localhost timezone=UTC TimeZone=Europe/Paris")
	require.ErrorContains(t, err, "conflicting database dsn parameters timezone and TimeZone")
}

func TestPgsqlRuntimeParamsSanitizeParseErrors(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{
		"postgres://admin:synthetic_secret@localhost/db?port=private_secret",
		"host=localhost password='synthetic_secret",
		"postgres://admin:synthetic_secret@%zz/db",
	} {
		_, err := connectionconfig.PgsqlRuntimeParams(dsn)
		require.ErrorContains(t, err, "parse database dsn")
		require.NotContains(t, err.Error(), "synthetic_secret")
		require.NotContains(t, err.Error(), "private_secret")
	}
}

func TestPgsqlRuntimeParamsAbsentDSNLeavesScalarSettings(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{"", " \t\n"} {
		params, err := connectionconfig.PgsqlRuntimeParams(dsn)
		require.NoError(t, err)
		require.NotNil(t, params)
		require.Empty(t, params)
	}
	cfg, err := connectionconfig.PgsqlStorageConfig(connectionconfig.PgsqlConfig{
		Address: "localhost:5432", Username: "admin", Password: "synthetic", Database: "admin",
		SSLMode: "verify-full", TLSCert: "host.crt", TLSKey: "host.key",
		MinConnectionsCount: 2, MaxConnectionsCount: 7, MaxConnIdleTime: time.Minute, MaxConnLifeTime: time.Hour,
	})
	require.NoError(t, err)
	require.Equal(t, "verify-full", cfg.SSLMode)
	require.Equal(t, "host.crt", cfg.TLSCert)
	require.Equal(t, "host.key", cfg.TLSKey)
	require.Equal(t, int32(2), cfg.MinConnectionsCount)
	require.Equal(t, int32(7), cfg.MaxConnectionsCount)
}

func TestPgsqlRuntimeParamsKeepDSNSSLModeAndExplicitTLS(t *testing.T) {
	t.Parallel()

	cfg, err := connectionconfig.PgsqlStorageConfig(connectionconfig.PgsqlConfig{
		DSN:     "host=localhost port=5432 user=admin dbname=admin sslmode=verify-full application_name=admin",
		SSLMode: "disable", TLSCert: "host.crt", TLSKey: "host.key",
	})
	require.NoError(t, err)
	require.Equal(t, "verify-full", cfg.SSLMode)
	require.Equal(t, "host.crt", cfg.TLSCert)
	require.Equal(t, "host.key", cfg.TLSKey)
}

func TestPgsqlRuntimeParamsRejectNULStartupValues(t *testing.T) {
	t.Parallel()

	const (
		keywordPrefix = "host=localhost user=credential_user_secret password=synthetic_secret " +
			"dbname=credential_database_secret sslmode=disable "
		value = "startup_secret\x00search_path\x00private_secret"
	)
	urlPrefix := "postgres://" + url.UserPassword("credential_user_secret", "synthetic_secret").String() +
		"@localhost/credential_database_secret?sslmode=disable&"
	dsns := map[string]string{
		"url-userinfo": "postgres://" + url.QueryEscape(value) + ":synthetic_secret@localhost/db?sslmode=disable",
		"url-path":     "postgres://admin:synthetic_secret@localhost/" + url.PathEscape(value) + "?sslmode=disable",
	}
	for _, key := range []string{
		"application_name", "timezone", "TimeZone", "statement_timeout", "lock_timeout",
		"idle_in_transaction_session_timeout", "search_path", "options", "user", "database", "dbname",
	} {
		dsns["url-"+key] = urlPrefix + key + "=" + url.QueryEscape(value)
		dsns["keyword-"+key] = keywordPrefix + key + "='" + value + "'"
	}
	for name, suffix := range map[string]string{
		"matching":      "timezone=" + url.QueryEscape(value) + "&TimeZone=" + url.QueryEscape(value),
		"mixed-lower":   "timezone=" + url.QueryEscape(value) + "&TimeZone=UTC",
		"mixed-capital": "timezone=UTC&TimeZone=" + url.QueryEscape(value),
	} {
		dsns["url-alias-"+name] = urlPrefix + suffix
	}
	for name, suffix := range map[string]string{
		"matching":      "timezone='" + value + "' TimeZone='" + value + "'",
		"mixed-lower":   "timezone='" + value + "' TimeZone=UTC",
		"mixed-capital": "timezone=UTC TimeZone='" + value + "'",
	} {
		dsns["keyword-alias-"+name] = keywordPrefix + suffix
	}
	for name, dsn := range dsns {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			params, err := connectionconfig.PgsqlRuntimeParams(dsn)
			require.Nil(t, params)
			require.EqualError(t, err, "parse database dsn: NUL in PostgreSQL startup value")
			require.NotContains(t, err.Error(), "synthetic_secret")
			require.NotContains(t, err.Error(), "credential_user_secret")
			require.NotContains(t, err.Error(), "credential_database_secret")
			require.NotContains(t, err.Error(), "startup_secret")
			require.NotContains(t, err.Error(), "private_secret")
			require.NotContains(t, err.Error(), "\x00")
			require.NotContains(t, err.Error(), dsn)
			_, err = connectionconfig.NormalizePgsql(connectionconfig.PgsqlConfig{DSN: dsn})
			require.EqualError(t, err, "parse database dsn: NUL in PostgreSQL startup value")
		})
	}
}

func TestPgsqlRuntimeParamsPreserveNonNULBytesAndMatchingAliases(t *testing.T) {
	t.Parallel()

	const value = "worker%00 \t\r\n=+\u00e9"
	for name, dsn := range map[string]string{
		"url": "postgres://admin%2500:synthetic@localhost/db%2500?sslmode=disable&timezone=UTC&TimeZone=UTC" +
			"&application_name=" + url.QueryEscape(value),
		"keyword": "host=localhost user=admin%00 password=synthetic dbname=db%00 sslmode=disable " +
			"timezone=UTC TimeZone=UTC application_name='" + value + "'",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			params, err := connectionconfig.PgsqlRuntimeParams(dsn)
			require.NoError(t, err)
			require.Equal(t, value, params["application_name"])
			require.Equal(t, "UTC", params["timezone"])
			require.NotContains(t, params, "TimeZone")
			cfg, err := connectionconfig.NormalizePgsql(connectionconfig.PgsqlConfig{DSN: dsn})
			require.NoError(t, err)
			require.Equal(t, "admin%00", cfg.Username)
			require.Equal(t, "db%00", cfg.Database)
		})
	}
}
