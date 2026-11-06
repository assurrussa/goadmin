package crud

import (
	"context"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type repository[T any] func(context.Context, datagrid.Filtered) ([]T, int, error)

func (r repository[T]) GetList(ctx context.Context, f datagrid.Filtered) ([]T, int, error) {
	return r(ctx, f)
}
