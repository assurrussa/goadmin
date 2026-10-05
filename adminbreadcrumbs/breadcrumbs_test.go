package adminbreadcrumbs_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminbreadcrumbs"
)

func TestFilterBreadcrumbsDeniesLinksWhenNoMenuRouteIsAllowed(t *testing.T) {
	t.Parallel()
	crumbs := []adminbreadcrumbs.Breadcrumb{{Name: "Students", Href: "/students"}, {Name: "Current student"}}
	for _, allowed := range []map[string]struct{}{nil, {}, {"/other": {}}} {
		require.Equal(t, []adminbreadcrumbs.Breadcrumb{{Name: "Current student"}}, adminbreadcrumbs.FilterBreadcrumbs(crumbs, allowed))
	}
	require.Equal(t, crumbs, adminbreadcrumbs.FilterBreadcrumbs(crumbs, map[string]struct{}{"/students": {}}))
}
