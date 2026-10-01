package host_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
)

func TestCorePublicFSIncludesBrowserAuthClient(t *testing.T) {
	t.Parallel()

	assets := adminhost.CorePublicFS()
	entry, err := assets.ReadFile("dist/js/app.js")
	require.NoError(t, err, "clean consumers need the built client embedded in the module")
	for _, contract := range []string{"x-goadmin-auth-error", "reauthentication_required", "authentication_retry_required"} {
		require.Contains(t, string(entry), contract, "embedded client must handle the server auth failure contract")
	}
}
