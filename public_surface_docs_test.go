package goadmin_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/assurrussa/goadmin/reference/externalconsumer"
)

func TestPublicSurfaceDocsMatchExternalConsumerManifest(t *testing.T) { //nolint:gocognit // required
	t.Helper()

	docs := []struct {
		name    string
		path    string
		heading string
	}{
		{
			name:    "AGENTS",
			path:    "AGENTS.md",
			heading: "## Public Surface Rules",
		},
		{
			name:    "project context",
			path:    "docs/project-context.md",
			heading: "## Stable Public Surface",
		},
		{
			name:    "README",
			path:    "README.md",
			heading: "## Stable public API",
		},
		{
			name:    "RELEASING",
			path:    "RELEASING.md",
			heading: "## Supported External Surface",
		},
		{
			name:    "admin package extraction",
			path:    "../docs/admin/package-extraction.md",
			heading: "## Stable vs Unstable",
		},
	}

	unsupported := []string{
		"github.com/assurrussa/goadmin/adminapp",
		"github.com/assurrussa/goadmin/bootstrap",
		"github.com/assurrussa/goadmin/di",
		"github.com/assurrussa/goadmin/domain/",
		"github.com/assurrussa/goadmin/http/",
		"github.com/assurrussa/goadmin/infrastructure/",
		"github.com/assurrussa/goadmin/seeders",
		"github.com/assurrussa/goadmin/shared",
	}

	for _, doc := range docs {
		t.Run(doc.name, func(t *testing.T) {
			content, err := os.ReadFile(doc.path)
			if err != nil {
				if os.IsNotExist(err) && strings.HasPrefix(doc.path, "../docs/") {
					t.Skip("repo-level docs are only available in the source checkout")
				}
				t.Fatalf("read %s: %v", doc.path, err)
			}

			section := markdownSection(string(content), doc.heading)
			if section == "" {
				t.Fatalf("%s missing section %q", doc.path, doc.heading)
			}

			var missing []string
			for _, pkg := range externalconsumer.SupportedPackages {
				if !strings.Contains(section, pkg) {
					missing = append(missing, pkg)
				}
			}
			slices.Sort(missing)
			if len(missing) > 0 {
				t.Fatalf("%s does not document supported packages:\n%s", doc.path, strings.Join(missing, "\n"))
			}

			var advertisedInternals []string
			for _, pkg := range unsupported {
				if strings.Contains(section, pkg) {
					advertisedInternals = append(advertisedInternals, pkg)
				}
			}
			slices.Sort(advertisedInternals)
			if len(advertisedInternals) > 0 {
				t.Fatalf("%s stable public section advertises unsupported packages:\n%s", doc.path, strings.Join(advertisedInternals, "\n"))
			}
		})
	}
}

func markdownSection(content, heading string) string {
	start := strings.Index(content, heading)
	if start < 0 {
		return ""
	}

	section := content[start+len(heading):]
	next := strings.Index(section, "\n## ")
	if next < 0 {
		return section
	}

	return section[:next]
}
