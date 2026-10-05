//nolint:lll // Table-driven permission fixtures keep each policy and expected result together.
package menu_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

const nestedStudentsPath = "/students/nested"

type keyChecker struct {
	allowed map[integrationroles.PermissionKey]bool
	super   bool
}

func (c keyChecker) IsSuperAdmin(context.Context, int64) bool { return c.super }
func (c keyChecker) AdminCan(_ context.Context, _ int64, domain integrationroles.PermissionDomain, action integrationroles.PermissionAction) bool {
	return c.allowed[integrationroles.NewPermissionKey(domain, action)]
}

func studentsMenu(item menu.Item) menu.Menu {
	item.Name, item.Href = "Students", testStudentsPath
	return menu.Menu{Sections: []menu.Section{{Key: testStudentsKey, Items: []menu.Item{item}}}}
}

func TestAnyPermissionKeysVisibility(t *testing.T) {
	t.Parallel()
	read := integrationroles.NewPermissionKey("users", "read")
	create := integrationroles.NewPermissionKey("users", "create")
	update := integrationroles.NewPermissionKey("users", "update")
	deleteKey := integrationroles.NewPermissionKey("users", "delete")
	keys := []integrationroles.PermissionKey{read, create, update, deleteKey}
	for _, tc := range []struct {
		name    string
		item    menu.Item
		allowed []integrationroles.PermissionKey
		want    bool
	}{
		{name: "unrestricted", want: true},
		{name: "empty OR", item: menu.Item{AnyPermissionKeys: []integrationroles.PermissionKey{}}, want: true},
		{name: "legacy denied", item: menu.Item{PermissionKey: read}},
		{name: "legacy allowed", item: menu.Item{PermissionKey: read}, allowed: []integrationroles.PermissionKey{read}, want: true},
		{name: "one denied", item: menu.Item{AnyPermissionKeys: []integrationroles.PermissionKey{update}}},
		{name: "one allowed", item: menu.Item{AnyPermissionKeys: []integrationroles.PermissionKey{update}}, allowed: []integrationroles.PermissionKey{update}, want: true},
		{name: "multiple no rights", item: menu.Item{AnyPermissionKeys: keys}},
		{name: "read only", item: menu.Item{AnyPermissionKeys: keys}, allowed: []integrationroles.PermissionKey{read}, want: true},
		{name: "create only", item: menu.Item{AnyPermissionKeys: keys}, allowed: []integrationroles.PermissionKey{create}, want: true},
		{name: "update only", item: menu.Item{AnyPermissionKeys: keys}, allowed: []integrationroles.PermissionKey{update}, want: true},
		{name: "delete only", item: menu.Item{AnyPermissionKeys: keys}, allowed: []integrationroles.PermissionKey{deleteKey}, want: true},
		{name: "unrelated right", item: menu.Item{AnyPermissionKeys: keys}, allowed: []integrationroles.PermissionKey{integrationroles.NewPermissionKey("groups", "read")}},
		{name: "legacy and OR allowed", item: menu.Item{PermissionKey: read, AnyPermissionKeys: []integrationroles.PermissionKey{update, deleteKey}}, allowed: []integrationroles.PermissionKey{read, update}, want: true},
		{name: "legacy missing", item: menu.Item{PermissionKey: read, AnyPermissionKeys: keys}, allowed: []integrationroles.PermissionKey{update}},
		{name: "OR missing", item: menu.Item{PermissionKey: read, AnyPermissionKeys: []integrationroles.PermissionKey{update}}, allowed: []integrationroles.PermissionKey{read}},
		{name: "zero cannot grant", item: menu.Item{AnyPermissionKeys: []integrationroles.PermissionKey{{}}}, allowed: []integrationroles.PermissionKey{{}}},
		{name: "partial cannot grant", item: menu.Item{AnyPermissionKeys: []integrationroles.PermissionKey{{Domain: "users"}, {Action: "read"}}}, allowed: []integrationroles.PermissionKey{{Domain: "users"}, {Action: "read"}}},
		{name: "zero does not mask match", item: menu.Item{AnyPermissionKeys: []integrationroles.PermissionKey{{}, update}}, allowed: []integrationroles.PermissionKey{update}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			allowed := make(map[integrationroles.PermissionKey]bool)
			for _, key := range tc.allowed {
				allowed[key] = true
			}
			got := menu.BuildAdminMenu(t.Context(), &models.SessionAdmin{ID: 7}, keyChecker{allowed: allowed}, studentsMenu(tc.item))
			require.Equal(t, tc.want, len(got.Sections) != 0)
			if tc.want {
				require.Equal(t, testStudentsPath, got.Sections[0].Items[0].Href)
			}
		})
	}
}

func TestAnyPermissionKeysPreservesAdminBypasses(t *testing.T) {
	t.Parallel()
	extra := studentsMenu(menu.Item{AnyPermissionKeys: []integrationroles.PermissionKey{{}}})
	for _, tc := range []struct {
		name    string
		admin   *models.SessionAdmin
		checker menu.PermissionChecker
		want    bool
	}{
		{name: "anonymous", checker: keyChecker{}, want: false},
		{name: "nil checker legacy", admin: &models.SessionAdmin{ID: 1}, want: true},
		{name: "session superadmin", admin: &models.SessionAdmin{ID: 1, RoleSlugs: []string{integrationroles.SuperAdminRole}}, checker: keyChecker{}, want: true},
		{name: "current superadmin", admin: &models.SessionAdmin{ID: 1}, checker: keyChecker{super: true}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := menu.BuildAdminMenu(t.Context(), tc.admin, tc.checker, extra)
			require.Equal(t, tc.want, len(got.Sections) > 0)
		})
	}
}

func TestAnyPermissionKeysNestedFiltering(t *testing.T) {
	t.Parallel()
	read := integrationroles.NewPermissionKey("users", "read")
	update := integrationroles.NewPermissionKey("users", "update")
	extra := menu.Menu{Sections: []menu.Section{{Key: testStudentsKey, Items: []menu.Item{
		{Name: "Visible group", Children: []menu.Item{{Href: testStudentsPath, AnyPermissionKeys: []integrationroles.PermissionKey{read, update}}}},
		{Name: "Empty group", Children: []menu.Item{{Href: "/hidden-child", AnyPermissionKeys: []integrationroles.PermissionKey{read}}}},
		{Name: "Denied parent", AnyPermissionKeys: []integrationroles.PermissionKey{read}, Children: []menu.Item{{Href: "/denied-parent-child", AnyPermissionKeys: []integrationroles.PermissionKey{update}}}},
	}}}}
	got := menu.BuildAdminMenu(t.Context(), &models.SessionAdmin{ID: 7}, keyChecker{allowed: map[integrationroles.PermissionKey]bool{update: true}}, extra)
	require.Len(t, got.Sections, 1)
	require.Len(t, got.Sections[0].Items, 1)
	require.Equal(t, testStudentsPath, got.Sections[0].Items[0].Children[0].Href)
}

func TestAnyPermissionKeysNotSerialized(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(menu.Item{Href: testStudentsPath, AnyPermissionKeys: []integrationroles.PermissionKey{integrationroles.NewPermissionKey("users", "update")}})
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"","href":"/students"}`, string(body))
}

func TestAnyPermissionKeysMergePreservesFirstPolicyAndCopiesSlices(t *testing.T) {
	t.Parallel()
	read := integrationroles.NewPermissionKey("users", "read")
	update := integrationroles.NewPermissionKey("users", "update")
	first := studentsMenu(menu.Item{PermissionKey: read})
	incoming := studentsMenu(menu.Item{AnyPermissionKeys: []integrationroles.PermissionKey{update}, Children: []menu.Item{{Href: nestedStudentsPath, AnyPermissionKeys: []integrationroles.PermissionKey{update}}}})
	got := first.Merge(incoming)
	require.Equal(t, read, got.Sections[0].Items[0].PermissionKey)
	require.Equal(t, []integrationroles.PermissionKey{update}, got.Sections[0].Items[0].AnyPermissionKeys)
	replacement := studentsMenu(menu.Item{AnyPermissionKeys: []integrationroles.PermissionKey{read}})
	require.Equal(t, got, got.Merge(replacement), "duplicate contributions must not broaden the first OR rule")
	incoming.Sections[0].Items[0].AnyPermissionKeys[0] = read
	incoming.Sections[0].Items[0].Children[0].AnyPermissionKeys[0] = read
	require.Equal(t, update, got.Sections[0].Items[0].AnyPermissionKeys[0])
	require.Equal(t, update, got.Sections[0].Items[0].Children[0].AnyPermissionKeys[0])
	cloned := got.Merge(menu.Menu{Sections: []menu.Section{{Key: "other", Items: []menu.Item{{Href: "/other"}}}}})
	cloned.Sections[0].Items[0].AnyPermissionKeys[0] = read
	cloned.Sections[0].Items[0].Children[0].AnyPermissionKeys[0] = read
	require.Equal(t, update, got.Sections[0].Items[0].AnyPermissionKeys[0])
	require.Equal(t, update, got.Sections[0].Items[0].Children[0].AnyPermissionKeys[0])
}

func TestAnyPermissionKeysKeepsLegacyNestedDuplicatePolicy(t *testing.T) {
	t.Parallel()
	read := integrationroles.NewPermissionKey("users", "read")
	first := studentsMenu(menu.Item{Children: []menu.Item{{Href: nestedStudentsPath}}})
	incoming := studentsMenu(menu.Item{Children: []menu.Item{{
		Href: nestedStudentsPath, PermissionKey: read, AnyPermissionKeys: []integrationroles.PermissionKey{read},
	}}})
	got := first.Merge(incoming)
	require.Equal(t, first, got, "matching nested children retain the first entire item, including its complete policy")
}
