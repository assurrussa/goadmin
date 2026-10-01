package host //nolint:testpackage // exercises module preflight before dependencies are constructed

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testReportRoute       = "GET /reports"
	testReportNamespace   = "/reports"
	testFooParameterRoute = "GET /foo/:id"
	testFooRoute          = "GET /foo"
)

func TestBuiltInNamespacesRejectFeatureRoutesBeforeConstruction(t *testing.T) {
	for _, tc := range []struct {
		name, route string
		module      Module
	}{
		{"role mutation", "POST /roles", AccessModule()},
		{"role parameter", "PUT /roles/:name", AccessModule()},
		{"admin mutation", "DELETE /admins/42", AccessModule()},
		{"permission descendant", "GET /permissions/data", AccessModule()},
		{"notifications mutation", "POST /notifications/read-all", NotificationsModule(nil)},
		{"queue wildcard", "DELETE /queues/jobs/*", QueuesModule()},
		{"file wildcard", "POST /files/*", UploadsModule(UploadsConfig{})},
		{"upload delivery", "GET /uploads/foo", UploadsModule(UploadsConfig{})},
		{"upload delivery head", "HEAD /uploads/foo", UploadsModule(UploadsConfig{})},
		{"avatar", "DELETE /auth/profile/avatar", UploadsModule(UploadsConfig{})},
		{"realtime parameter", "GET /:socket", RealtimeModule(nil)},
		{"mail", "POST /auth/profile/settings/email/confirm", AuthMailModule(AuthMailConfig{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			feature := Module{
				descriptor: ModuleDescriptor{Key: testModuleReports, Routes: []string{tc.route}},
				configure:  func(*assembly) error { called = true; return nil },
			}
			_, err := New(t.Context(), Config{}, Dependencies{}, feature, tc.module, JobsModule(JobsConfig{}))
			require.ErrorContains(t, err, "claimed by")
			require.False(t, called)
		})
	}
}

func TestCoreStaticNamespacesRejectFeatureRoutesBeforeConstruction(t *testing.T) {
	for _, route := range []string{
		"GET /public/custom.js", "HEAD /public/dist/js/app.js", "GET /test/theme", "POST /test/buttons",
	} {
		t.Run(route, func(t *testing.T) {
			called := false
			feature := Module{
				descriptor: ModuleDescriptor{Key: testModuleReports, Routes: []string{route}},
				configure:  func(*assembly) error { called = true; return nil },
			}
			_, err := New(t.Context(), Config{}, Dependencies{}, feature)
			require.ErrorContains(t, err, "claimed by core")
			require.False(t, called)
		})
	}
}

func TestBuiltInStaticNamespaceBoundaries(t *testing.T) {
	for _, route := range []string{"GET /publicity/custom.js", "GET /testing/theme", "GET /uploads-extra/foo"} {
		_, _, err := validateModules([]Module{
			UploadsModule(UploadsConfig{}), JobsModule(JobsConfig{}),
			{descriptor: ModuleDescriptor{Key: testModuleReports, Routes: []string{route}}},
		})
		require.NoError(t, err)
	}
}

func TestRouteClaimIntersections(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		conflict            bool
	}{
		{"named parameters", testFooParameterRoute, "GET /foo/:name", true},
		{"parameter and literal", testFooParameterRoute, "GET /foo/create", true},
		{"embedded parameter", "GET /foo/file-:id", "GET /foo/file-42", true},
		{"wildcard descendant", "GET /foo/*", "GET /foo/bar/baz", true},
		{"wildcard base", "GET /foo/*", testFooRoute, true},
		{"plus descendant", "GET /foo/+", "GET /foo/bar/baz", true},
		{"trailing slash", "GET /foo/", testFooRoute, true},
		{"implicit head", testFooRoute, "HEAD /foo", true},
		{"all methods", "* /foo", "PATCH /foo", true},
		{"different methods", testFooParameterRoute, "POST /foo/:id", false},
		{"different depth", testFooParameterRoute, "GET /foo/:id/edit", false},
		{"different prefix", testFooParameterRoute, "GET /foobar/:id", false},
		{"different suffix", "GET /foo/:id/edit", "GET /foo/:name/delete", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := validateModules([]Module{
				{descriptor: ModuleDescriptor{Key: testModuleOne, Routes: []string{tc.first}}},
				{descriptor: ModuleDescriptor{Key: testModuleTwo, Routes: []string{tc.second}}},
			})
			if tc.conflict {
				require.ErrorContains(t, err, "claimed by")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNamespaceClaims(t *testing.T) {
	for _, tc := range []struct {
		name     string
		other    ModuleDescriptor
		conflict bool
	}{
		{"same namespace", ModuleDescriptor{RouteNamespaces: []string{"/roles"}}, true},
		{"nested namespace", ModuleDescriptor{RouteNamespaces: []string{"/roles/custom"}}, true},
		{"parent namespace", ModuleDescriptor{RouteNamespaces: []string{"/"}}, true},
		{"parameter namespace crossing", ModuleDescriptor{Routes: []string{"POST /:resource/:id"}}, true},
		{"boundary", ModuleDescriptor{Routes: []string{"POST /roles-custom"}}, false},
		{"separate namespace", ModuleDescriptor{RouteNamespaces: []string{testReportNamespace}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.other.Key = testModuleReports
			_, _, err := validateModules([]Module{AccessModule(), {descriptor: tc.other}})
			if tc.conflict {
				require.ErrorContains(t, err, "claimed by")
			} else {
				require.NoError(t, err)
			}
		})
	}
	d := ModuleDescriptor{Key: testModuleReports, Routes: []string{testReportRoute}, RouteNamespaces: []string{testReportNamespace}}
	module := DefinedFeatureModule(d, nil)
	d.Routes[0], d.RouteNamespaces[0] = "GET /changed", "/changed"
	cloned := module.Descriptor()
	cloned.Routes[0], cloned.RouteNamespaces[0] = "GET /changed", "/changed"
	require.Equal(t, []string{testReportRoute}, module.Descriptor().Routes)
	require.Equal(t, []string{testReportNamespace}, module.Descriptor().RouteNamespaces)
}

func TestInvalidRouteClaims(t *testing.T) {
	for _, path := range []string{"roles", "/roles/*", "/:resource", "/roles?x=1", "/roles//nested", "/roles path"} {
		_, _, err := validateModules([]Module{{descriptor: ModuleDescriptor{Key: testModuleReports, RouteNamespaces: []string{path}}}})
		require.ErrorContains(t, err, "invalid route namespace")
	}
	for _, route := range []string{"get /reports", "INVALID /reports", "GET  /reports", "GET /reports?x=1", "GET reports"} {
		_, _, err := validateModules([]Module{{descriptor: ModuleDescriptor{Key: testModuleReports, Routes: []string{route}}}})
		require.ErrorContains(t, err, "invalid route claim")
	}
}
