package bootstrap //nolint:testpackage // verifies private library build metadata selection

import (
	"runtime/debug"
	"testing"
)

func TestResolveAdminVersion(t *testing.T) {
	const releaseVersion = "v0.6.1"

	tests := []struct {
		name string
		info debug.BuildInfo
		want string
	}{
		{
			name: "library dependency rather than host version",
			info: debug.BuildInfo{
				Main: debug.Module{Path: "example.com/host", Version: "v9.0.0"},
				Deps: []*debug.Module{
					{Path: "example.com/other", Version: "v2.0.0"},
					{Path: adminModulePath, Version: releaseVersion},
				},
			},
			want: releaseVersion,
		},
		{
			name: "local replacement does not claim required release",
			info: debug.BuildInfo{Deps: []*debug.Module{{
				Path: adminModulePath, Version: releaseVersion,
				Replace: &debug.Module{Path: "../goadmin"},
			}}},
			want: adminDevelopmentVersion,
		},
		{
			name: "versioned replacement identifies compiled source",
			info: debug.BuildInfo{Deps: []*debug.Module{{
				Path: adminModulePath, Version: releaseVersion,
				Replace: &debug.Module{Path: adminModulePath, Version: "v0.6.2-rc.1"},
			}}},
			want: "v0.6.2-rc.1",
		},
		{
			name: "main module release",
			info: debug.BuildInfo{Main: debug.Module{Path: adminModulePath, Version: releaseVersion}},
			want: releaseVersion,
		},
		{
			name: "main module checkout",
			info: debug.BuildInfo{Main: debug.Module{Path: adminModulePath, Version: "(devel)"}},
			want: adminDevelopmentVersion,
		},
		{
			name: "missing dependency never falls back to host version",
			info: debug.BuildInfo{Main: debug.Module{Path: "example.com/host", Version: "v9.0.0"}},
			want: adminDevelopmentVersion,
		},
		{
			name: "missing build metadata",
			want: adminDevelopmentVersion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAdminVersion(&tt.info); got != tt.want {
				t.Fatalf("resolveAdminVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
