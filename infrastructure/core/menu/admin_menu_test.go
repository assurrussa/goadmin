package menu_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

type domainChecker struct {
	allowed map[integrationroles.PermissionDomain]bool
}

func (c domainChecker) IsSuperAdmin(context.Context, int64) bool { return false }

func (c domainChecker) AdminCan(
	_ context.Context,
	_ int64,
	domain integrationroles.PermissionDomain,
	_ integrationroles.PermissionAction,
) bool {
	return c.allowed[domain]
}

func TestBuildAdminMenuGroupsPermittedPages(t *testing.T) {
	admin := &models.SessionAdmin{ID: 7}
	checker := domainChecker{allowed: map[integrationroles.PermissionDomain]bool{
		integrationroles.PermissionDomainAdmins: true,
		integrationroles.PermissionDomainRoles:  true,
		integrationroles.PermissionDomainQueues: true,
	}}
	got := menu.BuildAdminMenu(context.Background(), admin, checker,
		menu.ModuleMenu(map[string]bool{testAccessKey: true, testQueuesKey: true}))

	require.Len(t, got.Sections, 2)
	require.Equal(t, testAccessKey, got.Sections[0].Key)
	require.Equal(t, "Доступ", got.Sections[0].Title)
	require.Equal(t, []string{"/admins", "/roles"}, []string{
		got.Sections[0].Items[0].Href,
		got.Sections[0].Items[1].Href,
	})
	require.Equal(t, testSystemKey, got.Sections[1].Key)
	require.Equal(t, "/queues", got.Sections[1].Items[0].Href)
}

const (
	testMainMenuKey  = "main"
	testMainMenuName = "Главная"
)

func TestMenu_Merge_DeduplicatesItems(t *testing.T) {
	m1 := menu.Menu{
		Sections: []menu.Section{
			{
				Key: testMainMenuKey,
				Items: []menu.Item{
					{Name: testMainMenuName, Href: "/"},
				},
			},
		},
	}
	m2 := menu.Menu{
		Sections: []menu.Section{
			{
				Key: testMainMenuKey,
				Items: []menu.Item{
					{Name: testMainMenuName, Href: "/"},
					{Name: "Demo", Href: "/demo"},
				},
			},
		},
	}
	merged := m1.Merge(m2)
	require.Len(t, merged.Sections, 1)
	require.Len(t, merged.Sections[0].Items, 2)
	require.Equal(t, "/", merged.Sections[0].Items[0].Href)
	require.Equal(t, "/demo", merged.Sections[0].Items[1].Href)
}

func TestBuildAdminMenu_DeduplicatesMainMenuForSuperAdmin(t *testing.T) {
	admin := &models.SessionAdmin{
		ID:    1,
		Roles: []string{integrationroles.SuperAdminRole},
	}
	extra := menu.Menu{
		Sections: []menu.Section{
			{
				Key: testMainMenuKey,
				Items: []menu.Item{
					{Name: testMainMenuName, Href: "/"},
					{Name: "Demo", Href: "/demo"},
				},
			},
		},
	}
	got := menu.BuildAdminMenu(context.Background(), admin, nil, extra)
	require.NotEmpty(t, got.Sections)
	require.Equal(t, testMainMenuKey, got.Sections[0].Key)
	require.Len(t, got.Sections[0].Items, 2)
	require.Equal(t, testMainMenuName, got.Sections[0].Items[0].Name)
	require.Equal(t, "Demo", got.Sections[0].Items[1].Name)
}
