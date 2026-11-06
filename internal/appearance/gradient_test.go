package appearance //nolint:testpackage // Exercises the private request transition function.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGradientRequestTransitions(t *testing.T) {
	const start = "#123456"
	end := "#aabbcc"
	empty := ""
	old := Snapshot{Version: 1, AccentColor: start, AccentColorEnd: "#654321", LogoLight: "image"}
	for _, tc := range []struct {
		name    string
		request SaveBrandingRequest
		end     string
		invalid bool
	}{
		{"replace", SaveBrandingRequest{AccentColor: start, AccentColorEnd: &end}, "#AABBCC", false},
		{"legacy preserves", SaveBrandingRequest{AccentColor: start}, "#654321", false},
		{"explicit solid", SaveBrandingRequest{AccentColor: start, AccentColorEnd: &empty}, "", false},
		{"empty start clears", SaveBrandingRequest{}, "", false},
		{"end needs start", SaveBrandingRequest{AccentColorEnd: &end}, "", true},
		{"reset", SaveBrandingRequest{Reset: true}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.request.apply(old)
			if tc.invalid {
				require.ErrorIs(t, err, ErrInvalid)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.end, got.AccentColorEnd)
			require.Equal(t, tc.end, got.Public().AccentColorEnd)
			require.Equal(t, tc.end, got.Document().Payload.AccentColorEnd)
			if !tc.request.Reset {
				require.Equal(t, "image", got.LogoLight)
			}
		})
	}
	bad := "#oops"
	_, err := (SaveBrandingRequest{AccentColor: start, AccentColorEnd: &bad}).apply(old)
	require.ErrorIs(t, err, ErrInvalid)
}
