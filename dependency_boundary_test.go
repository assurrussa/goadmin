package goadmin_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Include generated files and build-tagged tests: tidy considers these imports too.
func TestNoLegacySharedOrRedisImports(t *testing.T) {
	require.NoError(t, filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if shouldSkipSourcePolicyDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" { //nolint:goconst // source extension policy
			return nil
		}
		source, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range source.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			for _, module := range []string{
				"github.com/assurrussa/goshared", "github.com/assurrussa/goredis", "github.com/assurrussa/gofiber",
			} {
				if imported == module || strings.HasPrefix(imported, module+"/") {
					t.Errorf("%s imports forbidden module %s", path, imported)
				}
			}
		}
		return nil
	}))
}
