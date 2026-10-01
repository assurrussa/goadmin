package operations_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	hostoperations "github.com/assurrussa/goadmin/features/operations"
	adminhost "github.com/assurrussa/goadmin/host"
)

func TestFeature_FitsReusableHostContract(t *testing.T) {
	t.Parallel()

	registry := adminhost.NewRegistry(
		hostoperations.NewFeature(nil, nil, nil, nil),
	)

	descriptors := registry.Descriptors()
	if len(descriptors) != 1 {
		t.Fatalf("expected one descriptor, got %d", len(descriptors))
	}
	if descriptors[0].Key != "operations" {
		t.Fatalf("unexpected descriptor key: %s", descriptors[0].Key)
	}

	wantRead := adminhost.NewPermissionKey(adminhost.PermissionDomainOperations, adminhost.PermissionActionRead)
	wantUpdate := adminhost.NewPermissionKey(adminhost.PermissionDomainOperations, adminhost.PermissionActionUpdate)
	if descriptors[0].Permissions[0] != wantRead || descriptors[0].Permissions[1] != wantUpdate {
		t.Fatalf("unexpected permissions: %+v", descriptors[0].Permissions)
	}
	if len(descriptors[0].PermissionDefinitions) != 2 {
		t.Fatalf("unexpected permission definitions: %+v", descriptors[0].PermissionDefinitions)
	}

	plan := adminhost.BuildBootstrapOptionsPlan(adminhost.BootstrapOptionsInput{
		Env:      "production",
		Registry: registry,
	})

	if !plan.IncludePublicFS {
		t.Fatal("expected public fs to be included in production")
	}
	if len(plan.Menu.Sections) != 1 || plan.Menu.Sections[0].Key != "system" {
		t.Fatalf("unexpected menu sections: %+v", plan.Menu.Sections)
	}
	if len(plan.Menu.Sections[0].Items) != 1 || plan.Menu.Sections[0].Items[0].Href != "/operations" {
		t.Fatalf("unexpected operations menu items: %+v", plan.Menu.Sections[0].Items)
	}
	if len(plan.PermissionDefinitions) != 2 {
		t.Fatalf("unexpected plan permission definitions: %+v", plan.PermissionDefinitions)
	}
	if len(plan.ExtensionBuilders) != 1 || plan.ExtensionBuilders[0] == nil {
		t.Fatalf("unexpected extension builders: %+v", plan.ExtensionBuilders)
	}
}

func TestFeature_ShipsOperationsUIAssets(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	for _, relPath := range []string{
		"resources/src/js/Pages/operations/IndexPage.vue",
		"resources/src/js/Pages/operations/components/HelpTooltip.vue",
	} {
		if _, err := os.Stat(filepath.Join(moduleRoot, relPath)); err != nil {
			t.Fatalf("expected reusable asset %s to exist: %v", relPath, err)
		}
	}
}
