//nolint:testpackage // Exercises the private path validator against ambiguous URL encodings.
package authrecovery

import (
	"context"
	"testing"
)

const testResetPath = "/auth/reset-password"

func TestNormalizeRequiresAllLegacyAliases(t *testing.T) {
	scope := func(ctx context.Context) (context.Context, error) { return ctx, nil }
	valid := Routes{
		ForgotPath: "/auth/forgot-password", ResetPaths: []string{testResetPath, "/reset-password"},
		LoginURL: "/auth/login", Context: scope,
	}
	for _, bad := range []Routes{
		{ForgotPath: valid.ForgotPath, ResetPaths: []string{testResetPath}, Context: scope},
		{ForgotPath: valid.ForgotPath, ResetPaths: valid.ResetPaths},
		{ForgotPath: valid.ForgotPath, ResetPaths: valid.ResetPaths, Context: scope, LoginURL: "//attacker.invalid"},
	} {
		if _, err := Normalize([]Routes{bad}); err == nil {
			t.Fatal("accepted unsafe route configuration")
		}
	}
	normalized, err := Normalize([]Routes{valid, {
		ForgotPath: "/auth/sso/forgot-password", ResetPaths: []string{"/auth/sso/reset-password"}, Context: scope,
	}})
	if err != nil {
		t.Fatal(err)
	}
	valid.ResetPaths[0] = "/changed"
	if normalized[0].ResetPaths[0] != testResetPath {
		t.Fatal("configuration was not snapshotted")
	}
}

func TestDefaultRoutesCanBeNormalizedAgain(t *testing.T) {
	routes, err := Normalize(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(routes); err != nil {
		t.Fatal(err)
	}
}

func TestRejectAmbiguousRecoveryPaths(t *testing.T) {
	for _, p := range []string{"//x", "/x/../y", "/x/:id", "/x/*", "/x%2fy", "/x?scope=sso", "/x#sso", "/x\\y"} {
		if validPath(p) {
			t.Fatalf("accepted %q", p)
		}
	}
}
