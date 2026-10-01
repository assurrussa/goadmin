//nolint:testpackage // injects independent clocks without widening the public host options.
package csrf

import (
	"context"
	"testing"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/identity"
)

func clockTestService(t *testing.T, at time.Time) *Service {
	t.Helper()
	opts := NewOptions("admin.test", "synthetic-clock-test-secret-at-least-32-bytes",
		[]string{"https://admin.test"}, logger.Discard(), WithTokenTTL(time.Minute))
	svc, err := newWithClock(opts, func() time.Time { return at })
	require.NoError(t, err)
	return svc
}

func TestProofAllowsBoundedClockSkewBetweenNodes(t *testing.T) {
	issuedAt := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	issuer := clockTestService(t, issuedAt)
	req := Request{SessionID: "clock-bound-browser", UserID: identity.NewUserID()}
	token, err := issuer.Create(context.Background(), req)
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		skew  time.Duration
		valid bool
	}{
		{"same-clock", 0, true},
		{"one-second-behind", -time.Second, true},
		{"exact-skew-limit", -proofClockSkew, true},
		{"outside-skew-limit", -proofClockSkew - time.Millisecond, false},
		{"one-second-ahead", time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verifier := clockTestService(t, issuedAt.Add(tc.skew))
			claims, valid, err := verifier.Check(context.Background(), req, token.Token)
			require.Equal(t, tc.valid, valid)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.True(t, issuedAt.Equal(claims.IssuedAt.Time))
			require.True(t, issuedAt.Add(time.Minute).Equal(claims.ExpiresAt.Time))
		})
	}
}

func TestProofExpiryHasAnExplicitSkewBoundary(t *testing.T) {
	issuedAt := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	req := Request{SessionID: "clock-bound-browser"}
	token, err := clockTestService(t, issuedAt).Create(context.Background(), req)
	require.NoError(t, err)
	expiresAt := issuedAt.Add(time.Minute)
	for _, tc := range []struct {
		name  string
		after time.Duration
		valid bool
	}{
		{"before-expiry", -time.Millisecond, true},
		{"at-expiry-within-leeway", 0, true},
		{"last-millisecond-of-leeway", proofClockSkew - time.Millisecond, true},
		{"at-expiry-plus-leeway", proofClockSkew, false},
		{"after-leeway", proofClockSkew + time.Millisecond, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, valid, err := clockTestService(t, expiresAt.Add(tc.after)).Check(context.Background(), req, token.Token)
			require.Equal(t, tc.valid, valid)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
