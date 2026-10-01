package users_test

import (
	"testing"

	usersfeature "github.com/assurrussa/goadmin/features/users"
	"github.com/assurrussa/goadmin/host"
)

func TestFeatureDescriptor_ExposesUsersMenuAndPermissions(t *testing.T) {
	t.Parallel()

	descriptor := usersfeature.NewFeature().Descriptor()

	if descriptor.Key != "users" {
		t.Fatalf("unexpected key: %s", descriptor.Key)
	}
	if len(descriptor.Menu.Sections) != 1 {
		t.Fatalf("unexpected menu sections: %+v", descriptor.Menu.Sections)
	}
	items := descriptor.Menu.Sections[0].Items
	if len(items) != 1 || items[0].Href != "/users" {
		t.Fatalf("unexpected menu items: %+v", items)
	}
	if len(descriptor.PermissionDefinitions) != 3 {
		t.Fatalf("unexpected permission definitions: %+v", descriptor.PermissionDefinitions)
	}
	if descriptor.PermissionDefinitions[0].Key.Domain != host.PermissionDomainUsers {
		t.Fatalf("unexpected permission domain: %+v", descriptor.PermissionDefinitions[0].Key)
	}
}

func TestFeatureBuildRequiresAdminApp(t *testing.T) {
	t.Parallel()

	builder := usersfeature.NewFeature().Build(host.Menu{})
	if builder == nil {
		t.Fatal("expected extension builder")
	}

	if _, err := builder(nil); err == nil {
		t.Fatal("expected nil app error")
	}
}
