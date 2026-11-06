//nolint:goconst // Explicit field names and wire values keep the write-only contract fixtures readable.
package crud //nolint:testpackage // Reuses the actual HTTP resource fixture and typed DTOs.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func TestOptionalWriteOnlyUpdateRequiresPointer(t *testing.T) {
	for _, field := range []Field{
		{Key: "name", Label: "Name", Type: "text", Update: true},
		{Key: "stock", Label: "Stock", Type: "number", Update: true},
		{Key: "active", Label: "Active", Type: "boolean", Update: true},
	} {
		t.Run(field.Type, func(t *testing.T) {
			d := definition()
			d.Fields = []Field{d.Fields[1], field}
			d.Columns = []datagrid.Column{{Key: "readOnly", Label: "Read only", Sortable: true}}
			_, err := New(d)
			require.ErrorContains(t, err, "optional write-only update fields require pointer DTO fields")
			d.Fields[1].Required = true
			_, err = New(d)
			require.NoError(t, err, "required write-only scalars remain supported")
		})
	}
	d := definition()
	d.Fields = append(d.Fields, Field{Key: "nullable", Label: "Secret", Type: "text", Update: true})
	_, err := New(d)
	require.NoError(t, err)
}

func TestWriteOnlyUpdatePreservesOmittedAndNullButReplacesExplicitEmpty(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		present          bool
	}{
		{"omitted", `{"name":"Bob","_revision":"v1"}`, "existing-secret", false},
		{"null", `{"name":"Bob","nullable":null,"_revision":"v1"}`, "existing-secret", false},
		{"empty", `{"name":"Bob","nullable":"","_revision":"v1"}`, "", true},
		{"replacement", `{"name":"Bob","nullable":"new-secret","_revision":"v1"}`, "new-secret", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := definition()
			d.Fields = append(d.Fields, Field{Key: "nullable", Label: "Secret", Type: "text", Update: true})
			_, err := New(d)
			require.NoError(t, err)
			stored := "existing-secret"
			calls := 0
			d.Update = func(_ context.Context, _ host.Actor, id, _ string, input testUpdate) (Record[testRow], error) {
				calls++
				require.Equal(t, tc.present, input.Nullable != nil)
				if input.Nullable != nil {
					stored = *input.Nullable
				}
				return Record[testRow]{ID: id, Revision: "v2", Data: testRow{Name: input.Name}}, nil
			}
			status, body, _ := request(t, mounted(t, d, false, false), "PUT", "/people/records/42", tc.body)
			require.Equal(t, 303, status, body)
			require.Equal(t, 1, calls)
			require.Equal(t, tc.want, stored)
		})
	}
}

func TestRequiredWriteOnlyUpdateRejectsOmittedButAcceptsExplicitZero(t *testing.T) {
	for _, tc := range []struct {
		field Field
		body  string
	}{
		{
			Field{Key: "stock", Label: "Stock", Type: "number", Update: true, Required: true},
			`{"name":"Bob","stock":0,"_revision":"v1"}`,
		},
		{
			Field{Key: "active", Label: "Active", Type: "boolean", Update: true, Required: true},
			`{"name":"Bob","active":false,"_revision":"v1"}`,
		},
	} {
		t.Run(tc.field.Type, func(t *testing.T) {
			d := definition()
			d.Fields = append(d.Fields, tc.field)
			_, err := New(d)
			require.NoError(t, err)
			calls := 0
			d.Update = func(_ context.Context, _ host.Actor, id, _ string, input testUpdate) (Record[testRow], error) {
				calls++
				require.Zero(t, input.Stock)
				require.False(t, input.Active)
				return Record[testRow]{ID: id, Revision: "v2", Data: testRow{Name: input.Name}}, nil
			}
			app := mounted(t, d, false, false)
			status, body, _ := request(t, app, "PUT", "/people/records/42", `{"name":"Bob","_revision":"v1"}`)
			require.Equal(t, 422, status, body)
			require.Zero(t, calls)
			status, body, _ = request(t, app, "PUT", "/people/records/42", tc.body)
			require.Equal(t, 303, status, body)
			require.Equal(t, 1, calls)
		})
	}
}
