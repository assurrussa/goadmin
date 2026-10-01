package goadmin_test

import (
	"os"
	"strings"
	"testing"
)

func TestAdminSidebarUsesPermissionFilteredServerMenu(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("resources/src/js/components/layout/AdminSidebar.vue")
	if err != nil {
		t.Fatalf("read AdminSidebar.vue: %v", err)
	}

	for _, banned := range []string{"const defaultMenu", "href: '/admins'", "href: '/users'"} {
		if strings.Contains(string(content), banned) {
			t.Fatalf("admin sidebar must not hard-code a fallback link: found %q", banned)
		}
	}
}
