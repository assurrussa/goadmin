package host_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
)

const minimumFirstAdminSetupTokenLength = 32

func TestNewFirstAdminSetupToken(t *testing.T) {
	t.Parallel()

	t.Run("empty disables static bootstrap", func(t *testing.T) {
		t.Parallel()

		_, err := adminhost.NewFirstAdminSetupToken("  ")
		require.NoError(t, err)
	})

	t.Run("short value is rejected", func(t *testing.T) {
		t.Parallel()

		_, err := adminhost.NewFirstAdminSetupToken("too-short")
		require.EqualError(t, err, "first admin setup token must contain at least 32 bytes")
	})

	t.Run("raw value is not retained", func(t *testing.T) {
		t.Parallel()

		raw := strings.Repeat("s", minimumFirstAdminSetupTokenLength)
		token, err := adminhost.NewFirstAdminSetupToken(raw)
		require.NoError(t, err)
		require.NotContains(t, fmt.Sprintf("%#v", token), raw)
	})
}
