package identity_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/identity"
)

func TestUserIDStorageAndWireContract(t *testing.T) {
	const text = "123e4567-e89b-42d3-a456-426614174116"
	id := identity.UserID(uuid.MustParse(text))
	encoded, err := json.Marshal(id)
	require.NoError(t, err)
	require.Equal(t, `"`+text+`"`, string(encoded))
	var decoded identity.UserID
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, id, decoded)
	value, err := id.Value()
	require.NoError(t, err)
	require.Equal(t, text, value)
	for _, value := range []any{text, []byte(text)} {
		var scanned identity.UserID
		require.NoError(t, scanned.Scan(value))
		require.Equal(t, id, scanned)
	}
	require.Error(t, decoded.Scan("invalid"))
	require.Error(t, decoded.UnmarshalText([]byte("invalid")))
	require.ErrorIs(t, identity.UserIDNil.Validate(), identity.ErrUserIDZero)
	require.Nil(t, identity.UserIDNil.AsPointer())
	require.False(t, id.Matches(uuid.UUID(id)))
	require.True(t, id.Matches(decoded))
	generated := identity.NewUserID()
	require.NoError(t, generated.Validate())
	require.Equal(t, uuid.Version(4), uuid.UUID(generated).Version())
}
