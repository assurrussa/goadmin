package sso //nolint:testpackage // Tests exercise internal binding and recovery invariants.

import "testing"

func TestSafeReturnPath(t *testing.T) {
	const origin = testAdminOrigin
	for _, tc := range []struct{ in, want string }{
		{testReturnTarget, testReturnTarget},
		{origin + testReturnTarget, testReturnTarget},
		{"https://attacker.invalid/", "/"},
		{"//attacker.invalid/", "/"},
		{"/%2fattacker.invalid/", "/"},
		{"/%252fattacker.invalid/", "/"},
		{"/\\attacker.invalid/", "/"},
		{"/reports%0d%0aLocation:evil", "/"},
		{"/auth/login?redirectback=/reports", "/"},
		{"/sso/open", "/"},
		{"", "/"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			if got := safeReturnPath(tc.in, origin); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
