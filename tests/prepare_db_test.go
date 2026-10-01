//go:build integration

package tests_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	admintests "github.com/assurrussa/goadmin/tests"
)

func TestInitDB(t *testing.T) {
	ctx := context.Background()
	dbTestPath := filepath.Join(admintests.Config.BasePath, "testdata")
	if _, err := os.Stat(dbTestPath); os.IsNotExist(err) {
		dbTestPath = ""
	}
	pgsql, db, cleanUp := admintests.PrepareDB(
		ctx,
		t,
		"tests-dbname",
		admintests.WithDatabasePathFilesMigration(dbTestPath),
		admintests.WithDatabaseFixedName(false),
		admintests.WithDatabaseVerbose(true),
		admintests.WithDatabaseLog(t.Logf),
	)
	require.NotNil(t, pgsql)
	require.NotNil(t, db)
	require.NotNil(t, cleanUp)
	assert.NotPanics(t, func() {
		cleanUp(ctx)
	})
}
