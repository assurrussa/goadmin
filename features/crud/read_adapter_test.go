package crud //nolint:testpackage // Exercise the runtime's resource builder.

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/hosttest"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func TestReadAdapterBindsPerRuntime(t *testing.T) {
	d := exampleEvents()
	var access []ReadOnlyReader
	adapter := ReadOnlyAdapter{BasePath: "/agent/events", Build: func(_ *host.App, reader ReadOnlyReader) (*host.Extension, error) {
		access = append(access, reader)
		return host.NewExtension(), nil
	}}
	module, err := NewReadOnlyWithAdapter(d, adapter)
	require.NoError(t, err)
	require.Equal(t, []string{d.BasePath, adapter.BasePath}, module.Descriptor().RouteNamespaces)
	f := feature[eventRow, struct{}, struct{}]{d: cloneDefinition(d.definition()), mode: readOnlyResource, adapter: &adapter}
	for _, enabled := range []bool{true, false} {
		app := hosttest.NewApp(t)
		app.MockRoleService.EXPECT().
			AdminCan(gomock.Any(), int64(9), d.Permission.Domain, d.Permission.Action).Return(enabled).AnyTimes()
		ext, buildErr := f.Build(host.Menu{})(host.WrapApp(app.App))
		require.NoError(t, buildErr)
		require.Len(t, ext.Handlers(), 1)
	}
	actor := host.Actor{AdminID: 9, SubjectID: "adapter-test-subject"}
	rows, total, err := access[0].List(t.Context(), actor, datagrid.Filters{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	record, err := access[0].Get(t.Context(), actor, "9")
	require.NoError(t, err)
	require.Equal(t, rows[0], record)
	require.NotContains(t, record.Data, "private")
	_, err = access[1].Get(t.Context(), actor, "9")
	require.ErrorIs(t, err, ErrDenied)
	_, err = NewReadOnlyWithAdapter(d, ReadOnlyAdapter{})
	require.Error(t, err)
}
