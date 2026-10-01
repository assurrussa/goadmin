package filerepo

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	uploadhost "github.com/assurrussa/gouploads/host"

	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
)

const (
	tableName = "files"
)

var (
	columns = []string{
		"id", "user_id", "manager_id", "object_type", "object_id", "original_filename", //nolint:goconst // required
		"filename", "folder_path", "provider", "size", "mime_type", //nolint:goconst // required
		"moderate", "block_cause", "name", "description", "file_type", "position", "is_primary", "url", //nolint:goconst // required
		"slug", "locale", "data", "created_at", "updated_at", "deleted_at", "published_at",
	}
	allowedSortFields = map[string]bool{
		"id":         true,
		"name":       true,
		"rate":       true,
		"created_at": true,
	}
	adoptedFields = map[string]datagrid.FieldMapping{
		"id":            datagrid.NewFieldMapping("id"),                                        // точное совпадение
		"name":          datagrid.NewFieldMappingWithOp("name", datagrid.OpLike),               // поиск по подстроке
		"description":   datagrid.NewFieldMappingWithOp("description", datagrid.OpLike),        // поиск по подстроке
		"slug":          datagrid.NewFieldMapping("slug"),                                      // точное совпадение
		"url":           datagrid.NewFieldMapping("url"),                                       // точное совпадение
		"type":          datagrid.NewFieldMapping("type"),                                      // точное совпадение
		"category":      datagrid.NewFieldMapping("category"),                                  // точное совпадение
		"category_text": datagrid.NewFieldMapping("category_text"),                             // точное совпадение
		"body":          datagrid.NewFieldMappingWithOp("body", datagrid.OpLike),               // поиск по подстроке
		"locale":        datagrid.NewFieldMapping("locale"),                                    // точное совпадение
		"createdAt":     datagrid.NewFieldMapping("created_at"),                                // точное совпадение
		"publishedAt":   datagrid.NewFieldMapping("published_at"),                              // точное совпадение
		"createdAtFrom": datagrid.NewFieldMappingWithOp("created_at", datagrid.OpGreaterEqual), // дата от
		"createdAtTo":   datagrid.NewFieldMappingWithOp("created_at", datagrid.OpLessEqual),    // дата до
	}
)

// ListFilters фильтры для списка файлов.
type ListFilters struct {
	Limit      int
	Offset     int
	FileType   string
	ObjectType string
	ObjectID   int64
}

func (r *Repo) GetList(ctx context.Context, filters datagrid.Filtered) ([]uploadhost.File, int, error) {
	const op = "exercise.repo.GetList"

	sqlBuilderCount := outbox.BuilderDollar().
		Select("count(id) as total").
		From(tableName)

	sqlBuilderList := outbox.BuilderDollar().
		Select(columns...).
		From(tableName)

	sqlBuilderCount = datagrid.SQLWherex(sqlBuilderCount, filters, adoptedFields)

	// Применяем фильтры
	fields := filters.GetFields()
	fileType := getField[string](fields, "file_type")
	if fileType != "" {
		sqlBuilderCount = sqlBuilderCount.Where(squirrel.Like{"mime_type": fileType + "%"})
		sqlBuilderList = sqlBuilderList.Where(squirrel.Like{"mime_type": fileType + "%"})
	}

	objectType := getField[string](fields, "object_type")
	if fields["object_type"] != "" {
		sqlBuilderCount = sqlBuilderCount.Where(squirrel.Eq{"object_type": objectType})
		sqlBuilderList = sqlBuilderList.Where(squirrel.Eq{"object_type": objectType})
	}

	objectID := getField[int](fields, "object_id")
	if objectID > 0 {
		sqlBuilderCount = sqlBuilderCount.Where(squirrel.Eq{"object_id": objectID})
		sqlBuilderList = sqlBuilderList.Where(squirrel.Eq{"object_id": objectID})
	}

	var count int
	if err := r.pgsql.DB().ScanOnex(ctx, op, &count, sqlBuilderCount); err != nil {
		return nil, 0, fmt.Errorf("%s: error creating account: %w", op, outbox.ErrorTransform(err))
	}

	sqlBuilderList = datagrid.SQLBuilderx(sqlBuilderList, filters, allowedSortFields)
	sqlBuilderList = datagrid.SQLWherex(sqlBuilderList, filters, adoptedFields)

	var data []uploadhost.File
	if err := r.pgsql.DB().ScanAllx(ctx, op, &data, sqlBuilderList); err != nil {
		return nil, 0, fmt.Errorf("%s: error get list: %w", op, outbox.ErrorTransform(err))
	}

	return data, count, nil
}

// List возвращает список файлов с фильтрами.
func (r *Repo) List(ctx context.Context, filters uploadhost.ListFilters) ([]uploadhost.File, int, error) {
	return r.repo.List(ctx, filters)
}

// Create создает новый файл в базе данных.
func (r *Repo) Create(ctx context.Context, file uploadhost.File) (int64, error) {
	return r.repo.Create(ctx, file)
}

func (r *Repo) Update(ctx context.Context, id int64, file uploadhost.File) error {
	return r.repo.Update(ctx, id, file)
}

// GetByID получает файл по ID.
func (r *Repo) GetByID(ctx context.Context, id int64) (uploadhost.File, error) {
	return r.repo.GetByID(ctx, id)
}

// DeleteByID удаляет файл по ID (soft delete).
func (r *Repo) DeleteByID(ctx context.Context, id int64) error {
	return r.repo.DeleteByID(ctx, id)
}

// GetByObjectType возвращает файлы по типу объекта.
func (r *Repo) GetByObjectType(ctx context.Context, objectType string, objectID int64) ([]uploadhost.File, error) {
	return r.repo.GetByObjectType(ctx, objectType, objectID)
}

// ClearPrimary resets the primary flag for files attached to an entity.
func (r *Repo) ClearPrimary(ctx context.Context, objectType string, objectID int64, excludeID int64) error {
	return r.repo.ClearPrimary(ctx, objectType, objectID, excludeID)
}

// SetPrimary marks the specified file as primary and updates its binding.
func (r *Repo) SetPrimary(ctx context.Context, id int64, objectType string, objectID int64) error {
	return r.repo.SetPrimary(ctx, id, objectType, objectID)
}

// UpdatePosition обновляет позицию файла.
func (r *Repo) UpdatePosition(ctx context.Context, id int64, position int) error {
	return r.repo.UpdatePosition(ctx, id, position)
}

// CleanupExpiredFiles удаляет строки, где deleted_at старше, к примеру 60 минут.
// Работает батчами со SKIP LOCKED, чтобы не блокировать рабочие транзакции.
// batchSize: количество строк за раз (например, 100).
// minutes: кол-во минут прошедших с момента удаления.
func (r *Repo) CleanupExpiredFiles(ctx context.Context, batchSize, minutes int) (int64, error) {
	return r.repo.CleanupExpiredFiles(ctx, batchSize, minutes)
}

func getField[T any](data map[string]any, key string) T {
	var empty T
	val, ok := data[key].(T)
	if !ok {
		return empty
	}

	return val
}
