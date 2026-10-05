package adminbreadcrumbs

import (
	"strings"

	"github.com/assurrussa/goadmin/infrastructure/core/menu"
)

// Breadcrumb represents one item in the breadcrumb trail.
type Breadcrumb struct {
	Name string `json:"name"`
	Href string `json:"href,omitempty"`
}

// RootBreadcrumb returns the root breadcrumb ("/") when it exists in the menu.
func RootBreadcrumb(m menu.Menu) (Breadcrumb, bool) {
	return findRoot(m)
}

// FilterBreadcrumbs removes breadcrumb items that point to routes not present
// in the allowed href set. Breadcrumbs without href are always preserved.
func FilterBreadcrumbs(crumbs []Breadcrumb, allowed map[string]struct{}) []Breadcrumb {
	if len(crumbs) == 0 {
		return crumbs
	}

	filtered := make([]Breadcrumb, 0, len(crumbs))
	for _, crumb := range crumbs {
		if crumb.Href == "" {
			filtered = append(filtered, crumb)
			continue
		}
		if _, ok := allowed[normalizePath(crumb.Href)]; ok {
			filtered = append(filtered, crumb)
		}
	}

	return filtered
}

// BuildBreadcrumbs builds a breadcrumb trail from the provided menu and current path.
// It selects the best matching menu item by longest prefix match and returns its ancestors.
func BuildBreadcrumbs(m menu.Menu, currentPath string) []Breadcrumb {
	path := normalizePath(currentPath)
	if path == "" {
		return nil
	}

	var bestTrail []Breadcrumb
	bestScore := -1

	for _, section := range m.Sections {
		walkItems(path, section.Items, nil, &bestTrail, &bestScore)
	}

	if len(bestTrail) == 0 {
		return nil
	}

	// Ensure the dashboard root is present at the start when available.
	if root, ok := findRoot(m); ok {
		if bestTrail[0].Href != root.Href {
			bestTrail = append([]Breadcrumb{root}, bestTrail...)
		}
	}

	return bestTrail
}

// AllowedHrefs builds a set of hrefs that are present in the provided menu.
// The hrefs are normalized to match the breadcrumb path normalization rules.
func AllowedHrefs(m menu.Menu) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, section := range m.Sections {
		collectAllowedHrefs(section.Items, allowed)
	}
	return allowed
}

func walkItems(
	path string,
	items []menu.Item,
	ancestors []Breadcrumb,
	bestTrail *[]Breadcrumb,
	bestScore *int,
) {
	for _, item := range items {
		trail := ancestors
		if item.Href != "" {
			trail = append(trail, Breadcrumb{Name: item.Name, Href: item.Href})
			if score := matchScore(path, item.Href); score > *bestScore {
				*bestScore = score
				*bestTrail = trail
			}
		}

		if len(item.Children) > 0 {
			walkItems(path, item.Children, trail, bestTrail, bestScore)
		}
	}
}

func findRoot(m menu.Menu) (Breadcrumb, bool) {
	for _, section := range m.Sections {
		for _, item := range section.Items {
			if item.Href == "/" {
				return Breadcrumb{Name: item.Name, Href: item.Href}, true
			}
		}
	}
	return Breadcrumb{}, false
}

func matchScore(path, href string) int {
	h := normalizePath(href)
	if h == "" {
		return -1
	}
	if h == "/" {
		if path == "/" {
			return 1
		}
		return 0
	}

	if path == h {
		return len(h)
	}
	if strings.HasPrefix(path, h+"/") {
		return len(h)
	}
	return -1
}

func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	// Drop query string if present.
	if idx := strings.IndexByte(p, '?'); idx >= 0 {
		p = p[:idx]
	}
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	// Trim trailing slash except root.
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
		if p == "" {
			return "/"
		}
	}
	return p
}

func collectAllowedHrefs(items []menu.Item, allowed map[string]struct{}) {
	for _, item := range items {
		if item.Href != "" {
			allowed[normalizePath(item.Href)] = struct{}{}
		}
		if len(item.Children) > 0 {
			collectAllowedHrefs(item.Children, allowed)
		}
	}
}
