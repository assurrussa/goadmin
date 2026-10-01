package host

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/assurrussa/goadmin/internal/admintx"
	adminshared "github.com/assurrussa/goadmin/shared"
)

type postgresFileRepository struct {
	FileRepository
	db StoragePgsqlClient
}

// NewPostgresFileRepository adds indexed delivery and transactional preview
// binding to a host's existing file store. Both use the core PostgreSQL pool.
func NewPostgresFileRepository(repo FileRepository, db StoragePgsqlClient) (FileRepository, error) {
	if repo == nil || db == nil {
		return nil, errors.New("file repository and core database are required")
	}
	return &postgresFileRepository{FileRepository: repo, db: db}, nil
}

func (r *postgresFileRepository) GetByID(ctx context.Context, id int64) (File, error) {
	if _, ok := admintx.Executor(ctx); !ok {
		return r.FileRepository.GetByID(ctx, id)
	}
	var file File
	err := admintx.Wrap(r.db.DB()).ScanOne(ctx, "file.read", &file, "SELECT * FROM files WHERE id=$1", id)
	return file, err
}

func (r *postgresFileRepository) GetByPath(ctx context.Context, relative string) (File, error) {
	actor := adminshared.GetAdminAuth(ctx)
	if actor == nil || actor.ID <= 0 {
		return File{}, nil
	}
	folder, name := path.Split(relative)
	folder = strings.TrimSuffix(folder, "/")
	if folder == "" || name == "" || path.Clean(path.Join(folder, name)) != relative {
		return File{}, nil
	}
	var ids []int64
	const query = `SELECT id FROM files
 WHERE manager_id=$1 AND folder_path=$2 AND filename=$3 AND deleted_at IS NULL LIMIT 2`
	if err := admintx.Wrap(r.db.DB()).ScanAll(ctx, "file.path", &ids, query, actor.ID, folder, name); err != nil {
		return File{}, fmt.Errorf("find file path: %w", err)
	}
	if len(ids) != 1 {
		return File{}, nil
	}
	return r.GetByID(ctx, ids[0])
}

func (r *postgresFileRepository) BindPreview(ctx context.Context, request PreviewBinding) error {
	return BindPostgresPreview(ctx, r.db, request)
}
