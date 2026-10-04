package menu_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	hostoperations "github.com/assurrussa/goadmin/features/operations"
	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

const (
	testStudentsPath = "/students"
	testQueuesKey    = "queues"
	testAccessKey    = "access"
	testSystemKey    = "system"
	testStudentsKey  = "students"
	testGroupsKey    = "groups"
)

func sectionKeys(value menu.Menu) []string {
	keys := make([]string, 0, len(value.Sections))
	for _, section := range value.Sections {
		keys = append(keys, section.Key)
	}
	return keys
}

func TestMenuDefaultApplicationOrderIsStable(t *testing.T) {
	t.Parallel()

	builtins := menu.ModuleMenu(map[string]bool{testAccessKey: true, testQueuesKey: true})
	custom := menu.Menu{Sections: []menu.Section{
		{Key: testStudentsKey, Items: []menu.Item{{Name: "Students", Href: testStudentsPath}}},
		{Key: testGroupsKey, Items: []menu.Item{{Name: "Groups", Href: "/groups"}}},
	}}
	want := []string{"main", testStudentsKey, testGroupsKey, testAccessKey, testSystemKey}
	merged := builtins.Merge(custom)
	require.Equal(t, want, sectionKeys(merged))
	require.Zero(t, merged.Sections[1].Order, "sorting must preserve the implicit order")
	require.Zero(t, custom.Sections[0].Order, "sorting must not rewrite host input")
	for range 3 {
		again := merged.Merge(custom).Merge(builtins)
		require.Equal(t, merged, again, "repeated registration must not reorder or duplicate items")
		merged = again
	}
	require.Equal(t, want, sectionKeys(custom.Merge(builtins)))
}

func TestMenuPreservesExplicitPlacementAndStableTies(t *testing.T) {
	t.Parallel()

	builtins := menu.ModuleMenu(map[string]bool{testAccessKey: true, testQueuesKey: true})
	extra := menu.Menu{Sections: []menu.Section{
		{Key: "last", Order: 80, Items: []menu.Item{{Href: "/last"}}},
		{Key: "default", Items: []menu.Item{{Href: "/default"}}},
		{Key: "same-order", Order: 15, Items: []menu.Item{{Href: "/same-order"}}},
		{Key: "first", Order: -10, Items: []menu.Item{{Href: "/first"}}},
		{Key: testSystemKey, Items: []menu.Item{{Href: "/custom-system"}}},
		{Key: testAccessKey, Order: 5},
	}}
	merged := builtins.Merge(extra)
	require.Equal(t, []string{"first", testAccessKey, "main", "default", "same-order", testSystemKey, "last"}, sectionKeys(merged))
	require.Equal(t, 5, merged.Sections[1].Order)
	require.Equal(t, 40, merged.Sections[5].Order, "joining a built-in section must retain its placement")
	require.Equal(t, []menu.Item{{
		Name: "Очереди", Href: "/queues", Icon: testQueuesKey,
		PermissionKey: integrationroles.NewPermissionKey(
			integrationroles.PermissionDomainQueues, integrationroles.PermissionActionRead,
		),
	}, {Href: "/custom-system"}}, merged.Sections[5].Items)
	require.Equal(t, merged, merged.Merge(extra))
}

func TestApplicationOrderSurvivesPermissionFiltering(t *testing.T) {
	t.Parallel()

	read := integrationroles.PermissionActionRead
	custom := menu.Menu{Sections: []menu.Section{
		{Key: testStudentsKey, Items: []menu.Item{{
			Href: testStudentsPath, PermissionKey: integrationroles.NewPermissionKey(testStudentsKey, read),
		}}},
		{Key: testGroupsKey, Items: []menu.Item{{
			Href: "/groups", PermissionKey: integrationroles.NewPermissionKey(testGroupsKey, read),
		}}},
	}}
	extra := menu.ModuleMenu(map[string]bool{testAccessKey: true, testQueuesKey: true}).Merge(custom)
	checker := domainChecker{allowed: map[integrationroles.PermissionDomain]bool{
		testStudentsKey: true, integrationroles.PermissionDomainQueues: true,
	}}
	got := menu.BuildAdminMenu(context.Background(), &models.SessionAdmin{ID: 7}, checker, extra)
	require.Equal(t, []string{testStudentsKey, testSystemKey}, sectionKeys(got))
	require.Equal(t, testStudentsPath, got.Sections[0].Items[0].Href)
	require.Equal(t, "/queues", got.Sections[1].Items[0].Href)
	require.Equal(t, got, menu.BuildAdminMenu(context.Background(), &models.SessionAdmin{ID: 7}, checker, extra))
	require.Empty(t, menu.BuildAdminMenu(context.Background(), nil, checker, extra).Sections)
}

func TestReservedSectionDefaultsDoNotDependOnCoreModules(t *testing.T) {
	t.Parallel()

	extra := menu.Menu{Sections: []menu.Section{
		{Key: testSystemKey, Items: []menu.Item{{Href: "/system-extension"}}},
		{Key: testStudentsKey, Items: []menu.Item{{Href: testStudentsPath}}},
		{Key: testAccessKey, Items: []menu.Item{{Href: "/access-extension"}}},
		{Key: testMainMenuKey, Items: []menu.Item{{Href: "/"}}},
	}}
	merged := (menu.Menu{}).Merge(extra)
	require.Equal(t, []string{testMainMenuKey, testStudentsKey, testAccessKey, testSystemKey}, sectionKeys(merged))
	for _, section := range merged.Sections {
		require.Zero(t, section.Order, "effective defaults must not rewrite explicit metadata")
	}
	for range 3 {
		require.Equal(t, merged, merged.Merge(extra))
	}
	// A reserved key still accepts a deliberate nonzero placement override.
	overridden := merged.Merge(menu.Menu{Sections: []menu.Section{{Key: testSystemKey, Order: 5}}})
	require.Equal(t, []string{testSystemKey, testMainMenuKey, testStudentsKey, testAccessKey}, sectionKeys(overridden))
}

func TestOperationsStaysBelowAccessWithoutQueues(t *testing.T) {
	t.Parallel()

	operations := hostoperations.NewFeature(nil, nil, nil, nil).Descriptor().Menu
	mounted := menu.ModuleMenu(map[string]bool{testAccessKey: true})
	custom := menu.Menu{Sections: []menu.Section{{
		Key: testStudentsKey, Items: []menu.Item{{Href: testStudentsPath}},
	}}}
	for _, extra := range []menu.Menu{
		mounted.Merge(operations).Merge(custom),
		operations.Merge(custom).Merge(mounted),
	} {
		got := menu.BuildAdminMenu(context.Background(), &models.SessionAdmin{ID: 1}, nil, extra)
		require.Equal(t, []string{testMainMenuKey, testStudentsKey, testAccessKey, testSystemKey}, sectionKeys(got))
		require.Len(t, got.Sections[3].Items, 1)
		require.Equal(t, "/operations", got.Sections[3].Items[0].Href, "queues must remain absent")
		require.Equal(t, extra, extra.Merge(operations))
	}
}
