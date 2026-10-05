package host_test

import (
	"testing"

	adminhost "github.com/assurrussa/goadmin/host"
)

const (
	mutatedLabel = "Mutated"
	changedLabel = "Changed"
	studentsKey  = "students"
	studentsPath = "/students"
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

func TestRegistry_RepeatedMenuRegistrationKeepsApplicationOrder(t *testing.T) {
	t.Parallel()

	students := storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
		Key: studentsKey, Menu: adminhost.Menu{Sections: []adminhost.Section{{
			Key: "learning", Items: []adminhost.Item{{Name: "Students", Href: studentsPath}},
		}}},
	}}
	groups := storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
		Key: "groups", Menu: adminhost.Menu{Sections: []adminhost.Section{{
			Key: "learning", Items: []adminhost.Item{{Name: "Groups", Href: "/groups"}},
		}}},
	}}
	registry := adminhost.NewRegistry(students, groups, students, groups)
	for range 3 {
		got := registry.Menu()
		if len(got.Sections) != 1 || len(got.Sections[0].Items) != 2 ||
			got.Sections[0].Items[0].Href != studentsPath || got.Sections[0].Items[1].Href != "/groups" ||
			got.Sections[0].Order != 0 {
			t.Fatalf("repeated registration changed the application menu: %+v", got)
		}
	}
}

func TestRegistryClonesAnyPermissionKeysAtEveryDepth(t *testing.T) {
	t.Parallel()
	key := adminhost.NewPermissionKey("users", "update")
	source := adminhost.Menu{Sections: []adminhost.Section{{Key: studentsKey, Items: []adminhost.Item{{
		Href: studentsPath, AnyPermissionKeys: []adminhost.PermissionKey{key},
		Children: []adminhost.Item{{Href: "/students/nested", AnyPermissionKeys: []adminhost.PermissionKey{key}}},
	}}}}}
	registry := adminhost.NewRegistry(storedDescriptorFeature{descriptor: adminhost.FeatureDescriptor{
		Key: studentsKey, Menu: source,
	}})
	for _, get := range []func() adminhost.Menu{registry.Menu, func() adminhost.Menu { return registry.Descriptors()[0].Menu }} {
		got := get()
		got.Sections[0].Items[0].AnyPermissionKeys[0] = adminhost.PermissionKey{}
		got.Sections[0].Items[0].Children[0].AnyPermissionKeys[0] = adminhost.PermissionKey{}
		again := get()
		item := again.Sections[0].Items[0]
		if item.AnyPermissionKeys[0] != key || item.Children[0].AnyPermissionKeys[0] != key {
			t.Fatal("menu any-of policy aliases the stored feature descriptor")
		}
	}
}
