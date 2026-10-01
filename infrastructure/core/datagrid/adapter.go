package datagrid

import "context"

// RepositoryAdapter универсальный адаптер для любого репозитория.
type RepositoryAdapter[T any] struct {
	getListFunc func(ctx context.Context, filters Filtered) ([]T, int, error)
}

// NewRepositoryAdapter создает новый универсальный адаптер.
func NewRepositoryAdapter[T any](
	getListFunc func(ctx context.Context, filters Filtered) ([]T, int, error),
) *RepositoryAdapter[T] {
	return &RepositoryAdapter[T]{
		getListFunc: getListFunc,
	}
}

// GetList реализует интерфейс Repository[T].
func (r *RepositoryAdapter[T]) GetList(ctx context.Context, filters Filtered) ([]T, int, error) {
	return r.getListFunc(ctx, filters)
}

// NewRepositoryAdapterFromRepo создает адаптер из существующего репозитория с методом GetList.
func NewRepositoryAdapterFromRepo[T any, R any](
	repo R,
	getListMethod func(R, context.Context, Filtered) ([]T, int, error),
) *RepositoryAdapter[T] {
	return &RepositoryAdapter[T]{
		getListFunc: func(ctx context.Context, filters Filtered) ([]T, int, error) {
			return getListMethod(repo, ctx, filters)
		},
	}
}
