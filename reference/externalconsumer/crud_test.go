package externalconsumer_test

import (
	"context"
	"testing"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

// This consumer uses only supported public imports and typed business callbacks.
func TestCRUDUsesOnlySupportedPublicTypes(t *testing.T) {
	type row struct {
		Name string `json:"name"`
	}
	type create struct {
		Name string `json:"name"`
	}
	type update struct {
		Name string `json:"name"`
	}
	record := crud.Record[row]{ID: "one", Revision: "v1", Data: row{Name: "Example"}}
	p := func(action host.PermissionAction) host.PermissionKey {
		return host.NewPermissionKey("consumer.records", action)
	}
	module, err := crud.New(crud.Definition[row, create, update]{
		Key: "consumer", BasePath: "/consumer", Title: "Records",
		Permissions: crud.Permissions{Read: p("read"), Create: p("create"), Update: p("update"), Delete: p("delete")},
		Fields:      []crud.Field{{Key: "name", Label: "Name", Type: "text", Read: true, Create: true, Update: true}},
		Columns:     []datagrid.Column{{Key: "name", Label: "Name", Sortable: true}},
		List: func(context.Context, host.Actor, datagrid.Filtered) ([]crud.Record[row], int, error) {
			return []crud.Record[row]{record}, 1, nil
		},
		Get: func(context.Context, host.Actor, string) (crud.Record[row], error) { return record, nil },
		LoadSelected: func(context.Context, host.Actor, []string) ([]crud.Record[row], error) {
			return []crud.Record[row]{record}, nil
		},
		Create: func(context.Context, host.Actor, create) (crud.Record[row], error) { return record, nil },
		Update: func(context.Context, host.Actor, string, string, update) (crud.Record[row], error) {
			return record, nil
		},
		Delete:    func(context.Context, host.Actor, string, string) error { return nil },
		Authorize: func(context.Context, host.Actor, crud.Operation, crud.Record[row]) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if module.Descriptor().Key != "crud.consumer" {
		t.Fatalf("unexpected module: %#v", module.Descriptor())
	}
}
