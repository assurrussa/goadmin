package importpolicy

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const goadminImportPrefix = "github.com/assurrussa/goadmin/"

type Config struct {
	RepoRoot          string
	ConsumerRoots     []string
	LegacyAuthRoots   []string
	SupportedPackages []string
}

type ReportError struct {
	UnsupportedImports []UnsupportedImport
	LegacyAuthImports  []string
}

type UnsupportedImport struct {
	File       string
	ImportPath string
}

func (r ReportError) OK() bool {
	return len(r.UnsupportedImports) == 0 && len(r.LegacyAuthImports) == 0
}

func (r *ReportError) Error() string {
	var blocks []string
	if len(r.UnsupportedImports) > 0 {
		var lines []string //nolint:prealloc // required
		for _, violation := range r.UnsupportedImports {
			lines = append(lines, fmt.Sprintf("%s -> %s", violation.File, violation.ImportPath))
		}
		blocks = append(blocks, "Unsupported goadmin imports:\n"+strings.Join(lines, "\n"))
	}
	if len(r.LegacyAuthImports) > 0 {
		blocks = append(blocks, "Legacy goadmin auth runtime imports:\n"+strings.Join(r.LegacyAuthImports, "\n"))
	}

	return strings.Join(blocks, "\n\n")
}

func Check(cfg Config) (ReportError, error) {
	if cfg.RepoRoot == "" {
		cfg.RepoRoot = "."
	}

	supported := make(map[string]struct{}, len(cfg.SupportedPackages))
	for _, pkg := range cfg.SupportedPackages {
		supported[pkg] = struct{}{}
	}

	unsupported, err := unsupportedConsumerImports(cfg.RepoRoot, cfg.ConsumerRoots, supported)
	if err != nil {
		return ReportError{}, err
	}

	legacy, err := legacyAuthImports(cfg.RepoRoot, cfg.LegacyAuthRoots)
	if err != nil {
		return ReportError{}, err
	}

	return ReportError{
		UnsupportedImports: unsupported,
		LegacyAuthImports:  legacy,
	}, nil
}

func unsupportedConsumerImports(repoRoot string, roots []string, supported map[string]struct{}) ([]UnsupportedImport, error) {
	var violations []UnsupportedImport

	err := walkGoFiles(repoRoot, roots, func(path string) error {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}

			if !strings.HasPrefix(importPath, goadminImportPrefix) {
				continue
			}
			if _, ok := supported[importPath]; ok {
				continue
			}

			violations = append(violations, UnsupportedImport{
				File:       rel(repoRoot, path),
				ImportPath: importPath,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(violations, func(a, b UnsupportedImport) int {
		if a.File == b.File {
			return strings.Compare(a.ImportPath, b.ImportPath)
		}
		return strings.Compare(a.File, b.File)
	})

	return violations, nil
}

func legacyAuthImports(repoRoot string, roots []string) ([]string, error) {
	var offenders []string

	err := walkGoFiles(repoRoot, roots, func(path string) error {
		normalized := filepath.ToSlash(path)
		if strings.HasSuffix(normalized, "_test.go") ||
			strings.HasSuffix(normalized, ".gen.go") ||
			strings.Contains(normalized, "/mocks/") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		legacyAuthImportPrefix := "github.com/assurrussa/goadmin/domain/" + "auth/"
		if strings.Contains(string(content), `"`+legacyAuthImportPrefix) {
			offenders = append(offenders, rel(repoRoot, path))
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.Sort(offenders)
	return offenders, nil
}

func walkGoFiles(repoRoot string, roots []string, visit func(path string) error) error {
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}

		absoluteRoot := filepath.Join(repoRoot, filepath.FromSlash(root))
		if _, err := os.Stat(absoluteRoot); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		err := filepath.WalkDir(absoluteRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				if shouldSkipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}

			if filepath.Ext(path) != ".go" {
				return nil
			}

			return visit(path)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func shouldSkipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "dist", "node_modules", "tmp", "vendor":
		return true
	default:
		return false
	}
}

func rel(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}

	return filepath.ToSlash(relative)
}
