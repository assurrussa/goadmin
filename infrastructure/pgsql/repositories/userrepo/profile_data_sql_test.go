package userrepo //nolint:testpackage // exercises real database/sql conversions at the managed repository boundary

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/admintx"
	authcore "github.com/assurrussa/goadmin/internal/auth"
)

func TestManagedProfileDataSQLRoundTrip(t *testing.T) {
	t.Parallel()
	const payload = `{"role":"reader","permissions":{"users":["read"]}}`
	for _, tc := range []struct {
		name string
		data driver.Value
	}{
		{name: "bytes", data: []byte(payload)},
		{name: "string", data: payload},
		{name: "sql-null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			connector := &profileDataConnector{data: tc.data}
			db := sql.OpenDB(connector)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			repo := Must(NewOptions(&fakeProjectionClient{db: &fakeProjectionDB{}}, fakeProjectionTxManager{}))
			ctx := admintx.WithExecutor(t.Context(), db)
			profile, err := repo.GetByID(ctx, 42)
			require.NoError(t, err)
			require.Equal(t, int64(42), profile.ID)
			if tc.data == nil {
				require.Nil(t, profile.Data)
			} else {
				require.Equal(t, &authcore.ProfileData{Role: "reader", Permissions: map[string][]string{"users": {"read"}}}, profile.Data)
			}
			require.NoError(t, repo.Update(ctx, profile.ID, profile))
			require.Len(t, connector.written, 4)
			if tc.data == nil {
				require.Nil(t, connector.written[1].Value)
			} else {
				value, ok := connector.written[1].Value.(string)
				require.True(t, ok)
				require.JSONEq(t, payload, value)
			}
		})
	}
}

type profileDataConnector struct {
	data    driver.Value
	written []driver.NamedValue
}

func (c *profileDataConnector) Connect(context.Context) (driver.Conn, error) {
	return &profileDataConn{connector: c}, nil
}

func (*profileDataConnector) Driver() driver.Driver { return profileDataDriver{} }

type profileDataDriver struct{}

func (profileDataDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type profileDataConn struct{ connector *profileDataConnector }

func (*profileDataConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}

func (*profileDataConn) Close() error { return nil }

func (*profileDataConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}

func (c *profileDataConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &profileDataRows{data: c.connector.data}, nil
}

func (c *profileDataConn) ExecContext(_ context.Context, _ string, args []driver.NamedValue) (driver.Result, error) {
	c.connector.written = append([]driver.NamedValue(nil), args...)
	return driver.RowsAffected(1), nil
}

type profileDataRows struct {
	data driver.Value
	done bool
}

func (*profileDataRows) Columns() []string { return []string{"id", "data"} }

func (*profileDataRows) Close() error { return nil }

func (r *profileDataRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0], values[1] = int64(42), r.data
	return nil
}
