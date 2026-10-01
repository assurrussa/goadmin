//nolint:testpackage // Exercises the private authenticated encryption codec.
package browserstate

import (
	"bytes"
	"strings"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"
)

func TestEncryptedCredentialsAreBoundToOpaqueSessionAndVersion(t *testing.T) {
	keys, err := goauth.NewKeyRing("test", goauth.Key{ID: "test", Material: bytes.Repeat([]byte{4}, 32)})
	require.NoError(t, err)
	c := codec{keys: keys}
	r := Record{Version: 1, Tokens: goauth.TokenPair{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh"}}
	raw, err := c.encode("opaque-1", r)
	require.NoError(t, err)
	require.NotContains(t, string(raw), r.Tokens.AccessToken)
	require.NotContains(t, string(raw), r.Tokens.RefreshToken)
	decoded, err := c.decode("opaque-1", raw)
	require.NoError(t, err)
	require.Equal(t, r, decoded)
	_, err = c.decode("opaque-2", raw)
	require.Error(t, err)
	_, err = c.decode("opaque-1", []byte(strings.Replace(string(raw), `"version":1`, `"version":2`, 1)))
	require.Error(t, err)
}
