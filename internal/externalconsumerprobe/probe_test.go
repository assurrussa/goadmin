package externalconsumerprobe //nolint:testpackage // required

import (
	"testing"

	"github.com/stretchr/testify/require"

	externalconsumer "github.com/assurrussa/goadmin/reference/externalconsumer"
)

func TestBuildGoModLocal(t *testing.T) {
	got, err := Config{
		LocalPath:          "/tmp/goadmin", //nolint:goconst // required
		GoauthLocalPath:    "/tmp/goauth",  //nolint:goconst // explicit candidate fixture path
		GouploadsLocalPath: "/tmp/gouploads",
		GonotifyLocalPath:  "/tmp/gonotify",
	}.BuildGoMod()

	require.NoError(t, err)
	require.Contains(t, got, "module example.com/goadminprobe")
	require.Contains(t, got, "require github.com/assurrussa/goadmin v0.0.0-local")
	require.Contains(t, got, "require github.com/assurrussa/goauth v0.0.0-local")
	require.Contains(t, got, `replace github.com/assurrussa/goadmin => "/tmp/goadmin"`)
	require.Contains(t, got, `replace github.com/assurrussa/goauth => "/tmp/goauth"`)
	require.Contains(t, got, `replace github.com/assurrussa/gouploads => "/tmp/gouploads"`)
	require.Contains(t, got, `replace github.com/assurrussa/gonotify => "/tmp/gonotify"`)
}

func TestBuildGoModPublished(t *testing.T) {
	got, err := Config{
		Version:       "v0.1.0", //nolint:goconst // required
		GoauthVersion: "v0.1.0",
	}.BuildGoMod()

	require.NoError(t, err)
	require.Contains(t, got, "require github.com/assurrussa/goadmin v0.1.0")
	require.Contains(t, got, "require github.com/assurrussa/goauth v0.1.0")
	require.NotContains(t, got, "replace github.com/assurrussa/goadmin")
	require.NotContains(t, got, "replace github.com/assurrussa/goauth")
}

func TestBuildProbeTestImportsSupportedPackages(t *testing.T) {
	got, err := Config{LocalPath: "/tmp/goadmin"}.BuildProbeTest()

	require.NoError(t, err)
	for _, pkg := range externalconsumer.SupportedPackages {
		require.Contains(t, got, `"`+pkg+`"`)
	}
	require.Contains(t, got, "func TestHostAuthAdapterAndMigrationContract")
	require.Contains(t, got, "adminhost.NewAuthAdapter")
	require.Contains(t, got, "adminmigrations.Migrate")
}

func TestValidateRejectsIncompleteAndAmbiguousInputs(t *testing.T) {
	tests := []Config{
		{},
		{Version: "v0.1.0", GouploadsLocalPath: "/tmp/gouploads"},
		{Version: "v0.1.0", GonotifyLocalPath: "/tmp/gonotify"},
		{Version: "v0.1.0", GoauthLocalPath: "/tmp/goauth"},
		{Version: "v0.1.0", LocalPath: "/tmp/goadmin"},
		{Version: "v0.1.0", GoauthVersion: "v0.1.0", GoauthLocalPath: "/tmp/goauth"},
	}

	for _, tt := range tests {
		require.Error(t, tt.Validate())
	}
}

func TestCleanPublishedGraphDoesNotOverrideGoauth(t *testing.T) {
	mod, err := (Config{Version: "v0.7.0"}).BuildGoMod()
	require.NoError(t, err)
	require.NotContains(t, mod, "require github.com/assurrussa/goauth")
	require.NotContains(t, mod, "replace")
}

func TestCleanHTTPAndEventsCandidatePaths(t *testing.T) {
	cfg := Config{LocalPath: "/tmp/admin", GowebsocketLocalPath: "/tmp/ws"}
	mod, err := cfg.BuildGoMod()
	require.NoError(t, err)
	require.Contains(t, mod, `replace github.com/assurrussa/gowebsocket => "/tmp/ws"`)
	cfg.LocalPath = ""
	cfg.Version = "v0.7.0"
	require.Error(t, cfg.Validate())
}
