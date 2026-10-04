package menu

import (
	"context"
	"slices"
	"sort"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
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

// Section groups navigation items. Order zero uses the application default (15),
// after Home (10) and before Access (20) and System (40). Nonzero orders are
// explicit overrides; equal orders retain their current relative order.
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
		oi := sections[i].Order
		oj := sections[j].Order
		if oi == 0 {
			oi = 15
		}
		if oj == 0 {
			oj = 15
		}
		return oi < oj
	})

	return Menu{Sections: sections}
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
				Key:   "main",
				Order: 10,
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
				Key:   "access",
				Title: "Доступ",
				Order: 20,
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
				Key:   "system",
				Title: "Система",
				Order: 40,
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
		if s.Key == "main" || capabilities == nil ||
			(s.Key == "access" && capabilities["access"]) || (s.Key == "system" && capabilities["queues"]) {
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
