package importpolicy //nolint:testpackage // required

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/reference/externalconsumer"
)

func TestCheckPassesCurrentSupportedImports(t *testing.T) {
	t.Helper()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "backend/good.go", `
package backend

import (
	_ "github.com/assurrussa/goadmin/features/operations"
	_ "github.com/assurrussa/goadmin/features/users"
	_ "github.com/assurrussa/goadmin/host"
	_ "github.com/assurrussa/goadmin/hosttest"
	_ "github.com/assurrussa/goadmin/migrations"
	_ "github.com/assurrussa/goadmin/toolkit/datagrid"
	_ "github.com/assurrussa/goadmin/toolkit/formvalidator"
)
`)
	writeGoFile(t, repoRoot, "fixtures/second-go-host/good.go", `
package secondgohost

import _ "github.com/assurrussa/goadmin/host"
`)

	report, err := Check(Config{
		RepoRoot:          repoRoot,
		ConsumerRoots:     []string{"backend", "fixtures/second-go-host"}, //nolint:goconst // required
		LegacyAuthRoots:   []string{"goadmin", "backend"},
		SupportedPackages: externalconsumer.SupportedPackages,
	})
	require.NoError(t, err)
	require.True(t, report.OK(), report.Error())
}

func TestCheckRejectsUnsupportedBackendImport(t *testing.T) {
	t.Helper()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "backend/bad.go", `
package backend

import _ "github.com/assurrussa/goadmin/http/handlers/users"
`)

	report, err := Check(Config{
		RepoRoot:          repoRoot,
		ConsumerRoots:     []string{"backend", "fixtures/second-go-host"},
		SupportedPackages: externalconsumer.SupportedPackages,
	})
	require.NoError(t, err)
	require.False(t, report.OK())
	require.Equal(t, []UnsupportedImport{{
		File:       "backend/bad.go",
		ImportPath: "github.com/assurrussa/goadmin/http/handlers/users",
	}}, report.UnsupportedImports)
}

func TestCheckRejectsUnsupportedFixtureImport(t *testing.T) {
	t.Helper()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "fixtures/second-go-host/bad.go", `
package secondgohost

import _ "github.com/assurrussa/goadmin/bootstrap"
`)

	report, err := Check(Config{
		RepoRoot:          repoRoot,
		ConsumerRoots:     []string{"backend", "fixtures/second-go-host"},
		SupportedPackages: externalconsumer.SupportedPackages,
	})
	require.NoError(t, err)
	require.False(t, report.OK())
	require.Equal(t, []UnsupportedImport{{
		File:       "fixtures/second-go-host/bad.go",
		ImportPath: "github.com/assurrussa/goadmin/bootstrap",
	}}, report.UnsupportedImports)
}

func TestCheckRejectsLegacyAuthRuntimeImport(t *testing.T) {
	t.Helper()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "goadmin/runtime.go", `
package goadmin

import _ "github.com/assurrussa/goadmin/domain/auth/core"
`)

	report, err := Check(Config{
		RepoRoot:          repoRoot,
		LegacyAuthRoots:   []string{"goadmin"},
		SupportedPackages: externalconsumer.SupportedPackages,
	})
	require.NoError(t, err)
	require.False(t, report.OK())
	require.Equal(t, []string{"goadmin/runtime.go"}, report.LegacyAuthImports)
}

func writeGoFile(t *testing.T, root, name, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644)) //nolint:gosec // required
}
