package host_test

import (
	"testing"

	adminhost "github.com/assurrussa/goadmin/host"
)

const (
	mutatedLabel = "Mutated"
	changedLabel = "Changed"
)

type storedDescriptorFeature struct {
	descriptor adminhost.FeatureDescriptor
}

func (s storedDescriptorFeature) Descriptor() adminhost.FeatureDescriptor {
	return s.descriptor
}

func (storedDescriptorFeature) Build(adminhost.Menu) adminhost.ExtensionBuilder {
	return nil
}

func TestRegistry_DescriptorsCloneMenuDeeply(t *testing.T) {
	t.Parallel()

	registry := adminhost.NewRegistry(storedDescriptorFeature{
		descriptor: adminhost.FeatureDescriptor{
			Key: "operations", //nolint:goconst // required
			Menu: adminhost.Menu{
				Sections: []adminhost.Section{{
					Key: "system", //nolint:goconst // required
					Items: []adminhost.Item{{
						Name: "Operations", //nolint:goconst // required
						Href: "/operations",
						Badge: &adminhost.Badge{
							Text:    "1",
							Variant: "info",
						},
						Children: []adminhost.Item{{
							Name: "Nested", //nolint:goconst // required
							Href: "/operations/nested",
						}},
					}},
				}},
			},
		},
	})

	descriptors := registry.Descriptors()
	descriptors[0].Menu.Sections[0].Items[0].Name = mutatedLabel
	descriptors[0].Menu.Sections[0].Items[0].Badge.Text = "999"
	descriptors[0].Menu.Sections[0].Items[0].Children[0].Name = changedLabel

	again := registry.Descriptors()
	if again[0].Menu.Sections[0].Items[0].Name != "Operations" {
		t.Fatalf("descriptor menu item was mutated: %+v", again[0].Menu.Sections[0].Items[0])
	}
	if again[0].Menu.Sections[0].Items[0].Badge == nil || again[0].Menu.Sections[0].Items[0].Badge.Text != "1" {
		t.Fatalf("descriptor badge was mutated: %+v", again[0].Menu.Sections[0].Items[0].Badge)
	}
	if again[0].Menu.Sections[0].Items[0].Children[0].Name != "Nested" {
		t.Fatalf("descriptor child was mutated: %+v", again[0].Menu.Sections[0].Items[0].Children[0])
	}
}

func TestRegistry_MenuDoesNotAliasStoredDescriptor(t *testing.T) {
	t.Parallel()

	registry := adminhost.NewRegistry(storedDescriptorFeature{
		descriptor: adminhost.FeatureDescriptor{
			Key: "operations",
			Menu: adminhost.Menu{
				Sections: []adminhost.Section{{
					Key: "system",
					Items: []adminhost.Item{{
						Name: "Operations",
						Href: "/operations",
						Children: []adminhost.Item{{
							Name: "Nested",
							Href: "/operations/nested",
						}},
					}},
				}},
			},
		},
	})

	merged := registry.Menu()
	merged.Sections[0].Items[0].Name = mutatedLabel
	merged.Sections[0].Items[0].Children[0].Name = changedLabel

	again := registry.Menu()
	if again.Sections[0].Items[0].Name != "Operations" {
		t.Fatalf("merged menu item was mutated: %+v", again.Sections[0].Items[0])
	}
	if again.Sections[0].Items[0].Children[0].Name != "Nested" {
		t.Fatalf("merged child item was mutated: %+v", again.Sections[0].Items[0].Children[0])
	}
}

func TestRegistry_PermissionDefinitionsMergeAndClone(t *testing.T) {
	t.Parallel()

	registry := adminhost.NewRegistry(
		storedDescriptorFeature{
			descriptor: adminhost.FeatureDescriptor{
				Key: "operations",
				PermissionDefinitions: []adminhost.PermissionDefinition{
					adminhost.NewPermissionDefinition(
						adminhost.NewPermissionKey(adminhost.PermissionDomainOperations, adminhost.PermissionActionRead),
						"Read operations",
					),
				},
			},
		},
		storedDescriptorFeature{
			descriptor: adminhost.FeatureDescriptor{
				Key: "exercises",
				Permissions: []adminhost.PermissionKey{
					adminhost.NewPermissionKey(adminhost.PermissionDomain("exercises"), adminhost.PermissionActionRead),
				},
				PermissionDefinitions: []adminhost.PermissionDefinition{
					adminhost.NewPermissionDefinition(
						adminhost.NewPermissionKey(adminhost.PermissionDomain("exercises"), adminhost.PermissionActionRead),
						"Read exercises",
					),
				},
			},
		},
	)

	definitions := registry.PermissionDefinitions()
	definitions[0].Description = "Mutated"

	again := registry.PermissionDefinitions()
	if len(again) != 2 {
		t.Fatalf("unexpected definitions len: %d", len(again))
	}
	if again[0].Description != "Read operations" {
		t.Fatalf("permission definitions were aliased: %+v", again[0])
	}
	if again[1].Description != "Read exercises" {
		t.Fatalf("unexpected merged definition: %+v", again[1])
	}
}
