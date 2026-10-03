package auth_test

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	authcore "github.com/assurrussa/goadmin/internal/auth"
)

var (
	_ sql.Scanner   = (*authcore.ProfileData)(nil)
	_ driver.Valuer = authcore.ProfileData{}
)

func TestProfileDataSQLCodec(t *testing.T) {
	t.Parallel()
	const payload = `{"role":"reader","permissions":{"users":["read"]}}`
	want := authcore.ProfileData{Role: "reader", Permissions: map[string][]string{"users": {"read"}}}
	for _, input := range []any{payload, []byte(payload)} {
		var data authcore.ProfileData
		require.NoError(t, data.Scan(input))
		require.Equal(t, want, data)
		encoded, err := data.Value()
		require.NoError(t, err)
		text, ok := encoded.(string)
		require.True(t, ok)
		require.JSONEq(t, payload, text)
		require.NoError(t, data.Scan(`{}`))
		require.Equal(t, authcore.ProfileData{}, data)
	}
	data := want
	for _, input := range []any{123, []byte(`{"role":`), `{"role":123}`} {
		require.Error(t, data.Scan(input))
		require.Equal(t, want, data, "a failed scan must not leave a partially decoded value")
	}
	for _, input := range []any{nil, "null"} {
		data = want
		require.NoError(t, data.Scan(input))
		require.Equal(t, authcore.ProfileData{}, data)
	}
	var absent *authcore.ProfileData
	require.Error(t, absent.Scan(payload))
	value, err := driver.DefaultParameterConverter.ConvertValue(absent)
	require.NoError(t, err)
	require.Nil(t, value)
}

func TestProfileDataNativePGXCodec(t *testing.T) {
	t.Parallel()
	const payload = `{"role":"reader","permissions":{"users":["read"]}}`
	for _, format := range []int16{pgtype.TextFormatCode, pgtype.BinaryFormatCode} {
		mapper := pgtype.NewMap()
		var data *authcore.ProfileData
		encoded, err := mapper.Encode(pgtype.JSONBOID, format, payload, nil)
		require.NoError(t, err)
		require.NoError(t, mapper.Scan(pgtype.JSONBOID, format, encoded, &data))
		require.Equal(t, "reader", data.Role)
		encoded, err = mapper.Encode(pgtype.JSONBOID, format, data, nil)
		require.NoError(t, err)
		var roundTrip *authcore.ProfileData
		require.NoError(t, mapper.Scan(pgtype.JSONBOID, format, encoded, &roundTrip))
		require.Equal(t, data, roundTrip)
		require.NoError(t, mapper.Scan(pgtype.JSONBOID, format, nil, &roundTrip))
		require.Nil(t, roundTrip)
	}
}
