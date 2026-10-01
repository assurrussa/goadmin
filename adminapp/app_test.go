package adminapp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/internal/envs"
)

func TestCreateApp(t *testing.T) {
	appTest := adminappt.NewAppTest(t)
	appTest.ExpertGuard("test", "read").Times(1)

	assert.Equal(t, envs.EnvProd, appTest.App.Env())
	assert.True(t, appTest.App.IsProduction())
	assert.NotNil(t, appTest.App.HTTPManager())
	assert.NotNil(t, appTest.App.Logger())
	assert.NotNil(t, appTest.App.TxManager())
	assert.NotNil(t, appTest.App.Session())
	assert.NotNil(t, appTest.App.AdminAuth())
	assert.NotNil(t, appTest.App.EventStream())
	assert.NotNil(t, appTest.App.Outbox())
	assert.NotNil(t, appTest.App.PermissionGuard())
	assert.NotNil(t, appTest.App.RolesService())
	assert.NotNil(t, appTest.App.Guard("test", "read"))
	assert.Equal(t, "https://example.com", appTest.App.ComposeFileURL("https://example.com"))
	assert.Equal(t, "https://ceph.localhost/path/file.png", appTest.App.ComposeFileURL("path/file.png"))
	assert.Equal(t, "https://ceph.localhost/path/file2.png", appTest.App.ComposeFileURLFallback("", "path/file2.png"))
	assert.Equal(
		t,
		"https://ceph.localhost/path/file.png",
		appTest.App.ComposeFileURLFallback("", "https://ceph.localhost/path/file.png"),
	)
}

func TestCreateAppError(t *testing.T) {
	assert.Panics(t, func() {
		adminapp.Must(adminapp.NewOptions(
			nil, nil, nil, nil, nil, nil, nil,
			nil, nil, "", "",
		))
	})
}
