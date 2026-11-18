package externalconsumer //nolint:testpackage // required

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSupportedPackagesMatchesBlankImports(t *testing.T) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(".", "imports.go"), nil, parser.ImportsOnly)
	require.NoError(t, err)

	var imported []string //nolint:prealloc // required
	for _, spec := range file.Imports {
		require.NotNil(t, spec)
		require.NotNil(t, spec.Name)
		require.Equal(t, "_", spec.Name.Name)

		path, err := strconv.Unquote(spec.Path.Value)
		require.NoError(t, err)
		imported = append(imported, path)
	}

	slices.Sort(imported)

	supported := slices.Collect(slices.Values(SupportedPackages))
	slices.Sort(supported)

	require.Equal(t, supported, imported)
	require.Equal(t, len(SupportedPackages), SupportedPackageCount)
}

func TestImportsFileContainsOnlyBlankImports(t *testing.T) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(".", "imports.go"), nil, 0)
	require.NoError(t, err)

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}

		for _, spec := range genDecl.Specs {
			importSpec, ok := spec.(*ast.ImportSpec)
			require.True(t, ok)
			require.NotNil(t, importSpec.Name)
			require.Equal(t, "_", importSpec.Name.Name)
		}
	}
}

func TestSupportedPackagesHaveReleaseSurfaceCategory(t *testing.T) {
	t.Helper()

	supported := slices.Collect(slices.Values(SupportedPackages))
	slices.Sort(supported)

	categorized := make([]string, 0,
		len(EmbeddingPackages)+
			len(TestSupportPackages)+
			len(FeaturePackages)+
			len(ToolkitSupportPackages),
	)
	categorized = append(categorized, EmbeddingPackages[:]...)
	categorized = append(categorized, TestSupportPackages[:]...)
	categorized = append(categorized, FeaturePackages[:]...)
	categorized = append(categorized, ToolkitSupportPackages[:]...)
	slices.Sort(categorized)

	require.NotEmpty(t, EmbeddingPackages)
	require.NotEmpty(t, TestSupportPackages)
	require.NotEmpty(t, FeaturePackages)
	require.NotEmpty(t, ToolkitSupportPackages)
	require.Equal(t, supported, categorized)
}

// Keep consumer documentation aligned with the supported manifest. Runtime
// behavior and publication readiness are checked separately.
func TestReadmeDocumentsSupportedPackages(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	require.NoError(t, err)
	section := regexp.MustCompile(`(?s)## Stable public API
(.*?)
## `).FindSubmatch(content)
	require.Len(t, section, 2)
	matches := regexp.MustCompile("`(github.com/assurrussa/goadmin/[^`]+)`").FindAllSubmatch(section[1], -1)
	documented := make([]string, 0, len(matches))
	for _, match := range matches {
		documented = append(documented, string(match[1]))
	}
	require.ElementsMatch(t, SupportedPackages, documented)
}

func TestStarterPublishedVersionDocumentation(t *testing.T) {
	root := filepath.Join("..", "..")
	env, err := os.ReadFile(filepath.Join(root, "examples", "starter", ".env.example"))
	require.NoError(t, err)
	version := regexp.MustCompile(`(?m)^GOADMIN_VERSION=(v[0-9]+\.[0-9]+\.[0-9]+)$`).FindSubmatch(env)
	require.Len(t, version, 2)
	for _, path := range []string{"README.md", "examples/starter/README.md", "docs/first-crud.md"} {
		t.Run(path, func(t *testing.T) {
			content, readErr := os.ReadFile(filepath.Join(root, path))
			require.NoError(t, readErr)
			require.Contains(t, string(content), string(version[1]))
			for _, assignment := range regexp.MustCompile(`GOADMIN_VERSION=(v[0-9]+\.[0-9]+\.[0-9]+)`).FindAllSubmatch(content, -1) {
				require.Equal(t, string(version[1]), string(assignment[1]))
			}
		})
	}
}
