package bootstrap //nolint:testpackage // required

import (
	"testing"
	"testing/fstest"

	uploadfiles "github.com/assurrussa/goadmin/http/strategies/uploadfiles"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

func TestResolveInertiaRootTemplates(t *testing.T) {
	t.Run("local uses disk paths", func(t *testing.T) {
		root, errTpl := resolveInertiaRootTemplates("internal/admin/views", true)
		if root != "internal/admin/views/app.gohtml" {
			t.Fatalf("unexpected root template: %s", root)
		}
		if errTpl != "internal/admin/views/error.gohtml" {
			t.Fatalf("unexpected error template: %s", errTpl)
		}
	})

	t.Run("production uses embedded template names", func(t *testing.T) {
		root, errTpl := resolveInertiaRootTemplates("internal/admin/views", false)
		if root != "app.gohtml" {
			t.Fatalf("unexpected root template: %s", root)
		}
		if errTpl != "error.gohtml" {
			t.Fatalf("unexpected error template: %s", errTpl)
		}
	})
}

func TestResolveInertiaHotTemplate(t *testing.T) {
	t.Run("local uses configured hot file", func(t *testing.T) {
		hot := resolveInertiaHotTemplate("internal/admin/publicstaticdata/hot", true)
		if hot != "internal/admin/publicstaticdata/hot" {
			t.Fatalf("unexpected hot template: %s", hot)
		}
	})

	t.Run("production disables hot file lookup", func(t *testing.T) {
		hot := resolveInertiaHotTemplate("internal/admin/publicstaticdata/hot", false)
		if hot != ".hot-disabled" {
			t.Fatalf("unexpected hot template: %s", hot)
		}
	})
}

func TestResolveInertiaTemplateFS(t *testing.T) {
	templateFS := fstest.MapFS{
		"app.gohtml": &fstest.MapFile{Data: []byte("ok")},
	}

	t.Run("local uses disk templates", func(t *testing.T) {
		if got := resolveInertiaTemplateFS(templateFS, true); got != nil {
			t.Fatalf("expected nil template fs for local env, got %#v", got)
		}
	})

	t.Run("production keeps embedded templates", func(t *testing.T) {
		if got := resolveInertiaTemplateFS(templateFS, false); got == nil {
			t.Fatal("expected embedded template fs for production env")
		}
	})
}

func TestValidateUploadTransports(t *testing.T) {
	valid := UploadTransport{
		Context:       "cms-rich-text",
		Prefix:        "/cms/uploads",
		Strategy:      uploadfiles.NewGenericStrategy(),
		PermissionKey: integrationroles.NewPermissionKey("cms", "media.upload"),
	}

	if err := validateUploadTransports([]UploadTransport{valid}); err != nil {
		t.Fatalf("valid upload transport rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*UploadTransport)
	}{
		{name: "missing context", mutate: func(transport *UploadTransport) { transport.Context = "" }},
		{name: "missing prefix", mutate: func(transport *UploadTransport) { transport.Prefix = "" }},
		{name: "missing strategy", mutate: func(transport *UploadTransport) { transport.Strategy = nil }},
		{name: "missing permission", mutate: func(transport *UploadTransport) {
			transport.PermissionKey = integrationroles.PermissionKey{}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := valid
			tt.mutate(&transport)

			if err := validateUploadTransports([]UploadTransport{transport}); err == nil {
				t.Fatal("expected invalid upload transport to fail")
			}
		})
	}
}
