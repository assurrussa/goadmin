package crud

import (
	"context"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

// ReadOnlyAdapter reserves a separate route namespace for a host-owned
// transport. Authenticate the actor before calling the resource reader.
type ReadOnlyAdapter struct {
	BasePath string
	Build    func(*host.App, ReadOnlyReader) (*host.Extension, error)
}

// ReadOnlyReader applies the resource's current permission, scoped repository,
// row policy and field projection to every transport.
type ReadOnlyReader interface {
	List(ctx context.Context, actor host.Actor, query datagrid.Filters) ([]ReadOnlyRecord[map[string]any], int, error)
	Get(ctx context.Context, actor host.Actor, id string) (ReadOnlyRecord[map[string]any], error)
}

func (s reader[R, C, U]) List(
	ctx context.Context, actor host.Actor, query datagrid.Filters,
) ([]ReadOnlyRecord[map[string]any], int, error) {
	page, err := s.list(ctx, actor, query)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]ReadOnlyRecord[map[string]any], len(page.Rows))
	for index, row := range page.Rows {
		rows[index] = ReadOnlyRecord[map[string]any]{ID: row.ID, Data: row.Data}
	}
	return rows, page.Total, nil
}

func (s reader[R, C, U]) Get(ctx context.Context, actor host.Actor, id string) (ReadOnlyRecord[map[string]any], error) {
	row, err := s.get(ctx, actor, id)
	return ReadOnlyRecord[map[string]any]{ID: row.ID, Data: row.Data}, err
}
