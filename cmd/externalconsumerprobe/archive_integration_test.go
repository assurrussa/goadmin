//go:build integration

package main

import (
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/tests"
)

// Run the generated, separate Go module against the same disposable PostgreSQL
// service as the other integration tests. The probe creates its own database.
func TestArchiveInExternalModule(t *testing.T) {
	cfg := tests.Config
	address, port := cfg.PostgresAddress, cfg.PostgresPort
	if cfg.PostgresAddressLocal != "" {
		address = cfg.PostgresAddressLocal
	}
	if cfg.PostgresPortLocal != 0 {
		port = cfg.PostgresPortLocal
	}
	server := url.URL{
		Scheme: "postgres", User: url.UserPassword(cfg.PostgresUser, cfg.PostgresPassword),
		Host: net.JoinHostPort(address, strconv.Itoa(port)), Path: cfg.PostgresDatabase,
	}
	server.RawQuery = url.Values{"sslmode": {cfg.PostgresSSLMode}}.Encode()
	t.Setenv("GOADMIN_PROBE_POSTGRES_URL", server.String())
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	require.NoError(t, run(t.Context(), []string{"--local-path", root}))
}
