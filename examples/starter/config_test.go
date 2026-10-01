package main

import (
	"net/url"
	"testing"
)

func TestDatabaseDSNEscapesPassword(t *testing.T) {
	t.Setenv("DATABASE_DSN", "")
	t.Setenv("POSTGRES_USER", "starter")
	t.Setenv("POSTGRES_PASSWORD", "p@ss?:/#word")
	t.Setenv("POSTGRES_DB", "admin")
	dsn, err := databaseDSN()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	password, ok := parsed.User.Password()
	if !ok || password != "p@ss?:/#word" {
		t.Fatalf("password did not round-trip through DSN")
	}
	if parsed.Host != "postgres:5432" || parsed.Path != "/admin" || parsed.Query().Get("sslmode") != "disable" {
		t.Fatalf("unexpected database DSN components: host=%q path=%q query=%q", parsed.Host, parsed.Path, parsed.RawQuery)
	}
}
