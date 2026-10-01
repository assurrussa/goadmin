package host_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	gouploadsModulePath = "github.com/assurrussa/gouploads"
	gouploadsHostPath   = "github.com/assurrussa/gouploads/host"
)

func TestHostPackageDoesNotImportGouploadsInternals(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	hostDir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(hostDir)
	require.NoError(t, err)

	var unsupported []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".gen.go") {
			continue
		}

		path := filepath.Join(hostDir, name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		require.NoError(t, err)

		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			if importPath == gouploadsModulePath || strings.HasPrefix(importPath, gouploadsModulePath+"/") {
				if importPath != gouploadsHostPath {
					unsupported = append(unsupported, name+" -> "+importPath)
				}
			}
		}
	}

	require.Empty(t, unsupported)
}
