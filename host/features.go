package host

import (
	"fmt"
	"slices"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

// RebuildBehavior documents how a feature affects public-site rebuild flows.
type RebuildBehavior struct {
	PublicContent            bool
	TriggersRoutesRegenerate bool
	TriggersFrontendDirty    bool
}

// FeatureDescriptor is the stable host-facing contract for project admin features.
type FeatureDescriptor struct {
	Key                   string
	Menu                  Menu
	Permissions           []PermissionKey
	PermissionDefinitions []PermissionDefinition
	ClientExtensions      []string
	ClientRequirements    []ClientExtensionRequirement
	Rebuild               RebuildBehavior
}

// Feature represents a host-mounted admin plugin that contributes menu and routes.
type Feature interface {
	Descriptor() FeatureDescriptor
	Build(siteMenu Menu) ExtensionBuilder
}

// Registry keeps host features together and exposes a stable integration surface.
type Registry struct {
	features []Feature
}

// NewRegistry creates a host-facing feature registry.
func NewRegistry(features ...Feature) Registry {
	filtered := make([]Feature, 0, len(features))
	for _, feature := range features {
		if feature == nil {
			continue
		}
		filtered = append(filtered, feature)
	}

	return Registry{features: filtered}
}

// Features returns a shallow clone of the registered features.
func (r Registry) Features() []Feature {
	return slices.Clone(r.features)
}

// Descriptors returns cloned feature descriptors for inspection and tests.
func (r Registry) Descriptors() []FeatureDescriptor {
	descriptors := make([]FeatureDescriptor, 0, len(r.features))
	for _, feature := range r.features {
		descriptor := feature.Descriptor()
		descriptor.Menu = cloneMenu(descriptor.Menu)
		descriptor.Permissions = slices.Clone(descriptor.Permissions)
		descriptor.PermissionDefinitions = integrationroles.ClonePermissionDefinitions(descriptor.PermissionDefinitions)
		descriptor.ClientExtensions = slices.Clone(descriptor.ClientExtensions)
		descriptor.ClientRequirements = slices.Clone(descriptor.ClientRequirements)
		descriptors = append(descriptors, descriptor)
	}

	return descriptors
}

// ClientExtensionRequirements returns validated, uniquely owned client modules.
func (r Registry) ClientExtensionRequirements() ([]ClientExtensionRequirement, error) {
	requirements := make([]ClientExtensionRequirement, 0, len(r.features))
	seen := make(map[string]struct{})

	for _, feature := range r.features {
		for _, requirement := range feature.Descriptor().ClientRequirements {
			if err := validateClientExtension(
				requirement.Key,
				requirement.APIVersion,
				requirement.Fingerprint,
			); err != nil {
				return nil, err
			}
			if _, exists := seen[requirement.Key]; exists {
				return nil, fmt.Errorf(
					"%w: duplicate required extension %q",
					ErrInvalidClientBundle,
					requirement.Key,
				)
			}
			seen[requirement.Key] = struct{}{}
			requirements = append(requirements, requirement)
		}
	}

	return requirements, nil
}

// Menu merges the menu contributions from every registered feature.
func (r Registry) Menu() Menu {
	var merged Menu
	for _, feature := range r.features {
		merged = merged.Merge(cloneMenu(feature.Descriptor().Menu))
	}

	return merged
}

// PermissionDefinitions returns merged host feature permissions for runtime/bootstrap wiring.
func (r Registry) PermissionDefinitions() []PermissionDefinition {
	if len(r.features) == 0 {
		return nil
	}

	definitionSets := make([][]PermissionDefinition, 0, len(r.features))
	for _, feature := range r.features {
		descriptor := feature.Descriptor()
		if len(descriptor.PermissionDefinitions) > 0 {
			definitionSets = append(definitionSets, descriptor.PermissionDefinitions)
			continue
		}

		if len(descriptor.Permissions) == 0 {
			continue
		}

		definitions := make([]PermissionDefinition, 0, len(descriptor.Permissions))
		for _, key := range descriptor.Permissions {
			if key.IsZero() {
				continue
			}
			definitions = append(definitions, PermissionDefinition{Key: key})
		}
		definitionSets = append(definitionSets, definitions)
	}

	return integrationroles.MergePermissionDefinitions(definitionSets...)
}

// ExtensionBuilders resolves feature builders against the provided site menu.
func (r Registry) ExtensionBuilders(siteMenu Menu) []ExtensionBuilder {
	builders := make([]ExtensionBuilder, 0, len(r.features))
	for _, feature := range r.features {
		builder := feature.Build(siteMenu)
		if builder == nil {
			continue
		}
		builders = append(builders, builder)
	}

	return builders
}

func cloneMenu(src Menu) Menu {
	if len(src.Sections) == 0 {
		return Menu{}
	}

	sections := make([]Section, len(src.Sections))
	for i := range src.Sections {
		sections[i] = cloneSection(src.Sections[i])
	}

	return Menu{Sections: sections}
}

func cloneSection(src Section) Section {
	section := src
	section.Items = cloneItems(src.Items)
	return section
}

func cloneItems(src []Item) []Item {
	if len(src) == 0 {
		return nil
	}

	items := make([]Item, len(src))
	for i := range src {
		items[i] = src[i]
		items[i].Children = cloneItems(src[i].Children)
		if src[i].Badge != nil {
			badge := *src[i].Badge
			items[i].Badge = &badge
		}
	}

	return items
}
