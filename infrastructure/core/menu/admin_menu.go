package menu

import (
	"context"
	"slices"
	"sort"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

const (
	mainSectionKey          = "main"
	accessSectionKey        = "access"
	systemSectionKey        = "system"
	mainSectionOrder        = 10
	applicationSectionOrder = 15
	accessSectionOrder      = 20
	systemSectionOrder      = 40
)

// PermissionChecker defines the minimal interface needed to filter menu items.
type PermissionChecker interface {
	IsSuperAdmin(ctx context.Context, adminID int64) bool
	AdminCan(ctx context.Context, adminID int64, domain integrationroles.PermissionDomain, action integrationroles.PermissionAction) bool //nolint:lll // required
}

type Item struct {
	Name          string                         `json:"name"`
	Href          string                         `json:"href"`
	Icon          string                         `json:"icon,omitempty"`
	Badge         *Badge                         `json:"badge,omitempty"`
	PermissionKey integrationroles.PermissionKey `json:"-"`
	Children      []Item                         `json:"children,omitempty"`
}

type Badge struct {
	Text       string `json:"text"`
	Variant    string `json:"variant,omitempty"`
	Dot        bool   `json:"dot,omitempty"`
	HideIfZero bool   `json:"hideIfZero,omitempty"`
}

// Section groups navigation items. Zero uses the application default (15), except
// reserved keys main (10), access (20), and system (40), even without their core
// module. Nonzero orders override placement; ties retain current relative order.
type Section struct {
	Key   string `json:"key"`
	Title string `json:"title,omitempty"`
	Order int    `json:"order,omitempty"`
	Items []Item `json:"items"`
}

type Menu struct {
	Sections []Section `json:"sections"`
}

func (m Menu) Merge(other Menu) Menu {
	if len(other.Sections) == 0 {
		return m
	}

	merged := make(map[string]*Section)
	order := make([]string, 0, len(m.Sections)+len(other.Sections))

	appendSection := func(sec Section) {
		if sec.Key == "" {
			return
		}
		if existing, ok := merged[sec.Key]; ok {
			if sec.Title != "" {
				existing.Title = sec.Title
			}
			if sec.Order != 0 {
				existing.Order = sec.Order
			}
			for _, item := range sec.Items {
				if idx := findItemIndex(existing.Items, item); idx >= 0 {
					mergeItem(&existing.Items[idx], item)
				} else {
					existing.Items = append(existing.Items, item)
				}
			}
			return
		}

		s := sec
		s.Items = slices.Clone(sec.Items)
		merged[sec.Key] = &s
		order = append(order, sec.Key)
	}

	for _, sec := range m.Sections {
		appendSection(sec)
	}
	for _, sec := range other.Sections {
		appendSection(sec)
	}

	sections := make([]Section, 0, len(order))
	for _, key := range order {
		if sec, ok := merged[key]; ok {
			sections = append(sections, *sec)
		}
	}

	sort.SliceStable(sections, func(i, j int) bool {
		return sectionOrder(sections[i]) < sectionOrder(sections[j])
	})

	return Menu{Sections: sections}
}

// A feature can be the first contributor to a reserved section when the
// corresponding core module is disabled. Its placement must not become custom.
func sectionOrder(section Section) int {
	if section.Order != 0 {
		return section.Order
	}
	switch section.Key {
	case mainSectionKey:
		return mainSectionOrder
	case accessSectionKey:
		return accessSectionOrder
	case systemSectionKey:
		return systemSectionOrder
	default:
		return applicationSectionOrder
	}
}

// BuildAdminMenu builds the admin sidebar menu and filters items by permissions.
func BuildAdminMenu(ctx context.Context, admin *models.SessionAdmin, checker PermissionChecker, extra Menu) Menu {
	if admin == nil {
		return Menu{}
	}

	allowAll := admin.HasRole(integrationroles.SuperAdminRole)
	if checker == nil {
		allowAll = true
	} else if checker.IsSuperAdmin(ctx, admin.ID) {
		allowAll = true
	}

	menu := ModuleMenu(map[string]bool{}).Merge(extra)
	filteredSections := make([]Section, 0, len(menu.Sections))
	for _, section := range menu.Sections {
		items := filterItems(ctx, admin, checker, allowAll, section.Items)
		if len(items) == 0 {
			continue
		}
		section.Items = items
		filteredSections = append(filteredSections, section)
	}

	return Menu{Sections: filteredSections}
}

func coreMenu() Menu {
	return Menu{
		Sections: []Section{
			{
				Key:   mainSectionKey,
				Order: mainSectionOrder,
				Items: []Item{
					{
						Name:          "Главная",
						Href:          "/",
						Icon:          "home",
						PermissionKey: integrationroles.NewPermissionKey(integrationroles.PermissionDomainDashboard, integrationroles.PermissionActionRead), //nolint:lll // required
					},
				},
			},
			{
				Key:   accessSectionKey,
				Title: "Доступ",
				Order: accessSectionOrder,
				Items: []Item{
					{
						Name:          "Администраторы",
						Href:          "/admins",
						Icon:          "admins",
						PermissionKey: integrationroles.NewPermissionKey(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead), //nolint:lll // required
					},
					{
						Name:          "Роли",
						Href:          "/roles",
						Icon:          "roles",
						PermissionKey: integrationroles.NewPermissionKey(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionRead), //nolint:lll // required
					},
					{
						Name:          "Разрешения",
						Href:          "/permissions",
						Icon:          "permissions",
						PermissionKey: integrationroles.NewPermissionKey(integrationroles.PermissionDomainPermissions, integrationroles.PermissionActionRead), //nolint:lll // required
					},
				},
			},
			{
				Key:   systemSectionKey,
				Title: "Система",
				Order: systemSectionOrder,
				Items: []Item{
					{
						Name:          "Очереди",
						Href:          "/queues",
						Icon:          "queues",
						PermissionKey: integrationroles.NewPermissionKey(integrationroles.PermissionDomainQueues, integrationroles.PermissionActionRead), //nolint:lll // required
					},
				},
			},
		},
	}
}

func filterItems(
	ctx context.Context,
	admin *models.SessionAdmin,
	checker PermissionChecker,
	allowAll bool,
	items []Item,
) []Item {
	filtered := make([]Item, 0, len(items))
	for _, item := range items {
		if !allowAll && !canView(ctx, admin, checker, item.PermissionKey) {
			continue
		}
		if len(item.Children) > 0 {
			item.Children = filterItems(ctx, admin, checker, allowAll, item.Children)
		}
		if item.Href == "" && len(item.Children) == 0 {
			continue
		}
		filtered = append(filtered, item)
	}

	return filtered
}

func canView(
	ctx context.Context,
	admin *models.SessionAdmin,
	checker PermissionChecker,
	key integrationroles.PermissionKey,
) bool {
	if key.IsZero() {
		return true
	}
	if checker == nil {
		return true
	}

	return checker.AdminCan(ctx, admin.ID, key.Domain, key.Action)
}

// ModuleMenu returns only explicitly mounted built-in screen contributions.
func ModuleMenu(capabilities map[string]bool) Menu {
	m := coreMenu()
	sections := make([]Section, 0, len(m.Sections))
	for _, s := range m.Sections {
		if s.Key == mainSectionKey || capabilities == nil ||
			(s.Key == accessSectionKey && capabilities[accessSectionKey]) || (s.Key == systemSectionKey && capabilities["queues"]) {
			sections = append(sections, s)
		}
	}
	return Menu{Sections: sections}
}

func findItemIndex(items []Item, target Item) int {
	for i, item := range items {
		if target.Href != "" && item.Href == target.Href {
			return i
		}
		if target.Href == "" && target.Name != "" && item.Name == target.Name {
			return i
		}
	}
	return -1
}

func mergeItem(existing *Item, incoming Item) {
	if existing.Icon == "" && incoming.Icon != "" {
		existing.Icon = incoming.Icon
	}
	if existing.Badge == nil && incoming.Badge != nil {
		existing.Badge = incoming.Badge
	}
	if existing.PermissionKey.IsZero() && !incoming.PermissionKey.IsZero() {
		existing.PermissionKey = incoming.PermissionKey
	}
	for _, child := range incoming.Children {
		if findItemIndex(existing.Children, child) < 0 {
			existing.Children = append(existing.Children, child)
		}
	}
}
