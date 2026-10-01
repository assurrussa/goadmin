package host

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/Masterminds/squirrel"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/assurrussa/outbox/outbox"
	querybuilder "github.com/assurrussa/outbox/shared/query_builder"

	"github.com/assurrussa/goadmin/internal/uploadintegration"
	adminshared "github.com/assurrussa/goadmin/shared"
)

// LocalUploadsConfig contains local-disk upload settings for a clean admin host.
type LocalUploadsConfig struct {
	Root    string
	BaseURL string
}

// LocalUploadsRuntime groups dependencies and jobs required by the admin host.
// Register Jobs before starting the host-owned outbox worker.
type LocalUploadsRuntime struct {
	Repositories Repositories
	Uploads      Uploads
	Storage      uploadhost.Storage
	Jobs         []outbox.Job
}

// NewLocalUploads builds the original-only async upload runtime. It owns no workers.
func NewLocalUploads(cfg LocalUploadsConfig, db StoragePgsqlClient, tx StoragePgsqlTxManager,
	queue OutboxPutter, stream EventStream, lg Logger,
) (LocalUploadsRuntime, error) {
	if strings.TrimSpace(cfg.Root) == "" {
		return LocalUploadsRuntime{}, errors.New("local uploads root is required")
	}
	if lg == nil {
		lg = DiscardLogger()
	}
	runtime, err := uploadhost.NewOriginalRuntime(uploadhost.StorageConfig{
		Driver: uploadhost.StorageDriverLocal,
		Local:  uploadhost.StorageLocalConfig{Root: filepath.Clean(cfg.Root), BaseURL: cfg.BaseURL},
		Public: uploadhost.StoragePublicConfig{Prefix: "media/v1"},
	}, uploadhost.OriginalRuntimeDeps{
		Database: db, Transaction: tx, Outbox: queue,
		Logger: lg, Events: NewUploadEventPublisher(stream),
	})
	if err != nil {
		return LocalUploadsRuntime{}, fmt.Errorf("create local uploads runtime: %w", err)
	}
	return LocalUploadsRuntime{
		Repositories: Repositories{FileRepo: &localPathFileRepo{
			FileRepo: runtime.Files,
			db:       db,
		}, FileLoader: &localFileLoader{repo: runtime.Files, baseURL: cfg.BaseURL}},

		Uploads: Uploads{
			Service: runtime.Uploader, TusStore: runtime.TusStore,
			AfterProcess: &localAfterProcess{
				tx: tx, repo: runtime.Files, eventStream: stream,
				lg: lg, baseURL: cfg.BaseURL,
			},
		},
		Storage: runtime.Storage, Jobs: runtime.Jobs,
	}, nil
}

// localPathFileRepo adds an indexed path lookup without widening the host FileRepository contract.
type localPathFileRepo struct {
	*uploadhost.FileRepo
	db StoragePgsqlClient
}

func (r *localPathFileRepo) GetByPath(ctx context.Context, relativePath string) (File, error) {
	admin := adminshared.GetAdminAuth(ctx)
	if admin == nil || admin.ID <= 0 {
		return File{}, nil
	}
	folder, name := path.Split(relativePath)
	folder = strings.TrimSuffix(folder, "/")
	if folder == "" || name == "" || path.Clean(path.Join(folder, name)) != relativePath {
		return File{}, nil
	}
	query := querybuilder.BuilderDollar().Select("id").From("files").Where(squirrel.Eq{
		"manager_id": admin.ID, "folder_path": folder, "filename": name, "deleted_at": nil,
	}).Limit(2)
	var rows []struct {
		ID int64 `db:"id"`
	}
	if err := r.db.DB().ScanAllx(ctx, "localPathFileRepo.GetByPath", &rows, query); err != nil {
		return File{}, fmt.Errorf("find upload path: %w", err)
	}
	if len(rows) != 1 {
		return File{}, nil
	}
	return r.GetByID(ctx, rows[0].ID)
}

// ListByManager keeps the default local repository's admin list pagination
// scoped in SQL, including the total reported to the UI.
func (r *localPathFileRepo) ListByManager(
	ctx context.Context, managerID int64, filters ListFilters,
) ([]File, int, error) {
	if managerID <= 0 {
		return nil, 0, nil
	}
	base := squirrel.Eq{"manager_id": managerID, "deleted_at": nil}
	countQuery := querybuilder.BuilderDollar().Select("count(id)").From("files").Where(base)
	listQuery := querybuilder.BuilderDollar().Select("*").From("files").Where(base)
	if filters.FileType != "" {
		filter := squirrel.Like{"mime_type": filters.FileType + "%"}
		countQuery = countQuery.Where(filter)
		listQuery = listQuery.Where(filter)
	}
	if filters.ObjectType != "" {
		filter := squirrel.Eq{"object_type": filters.ObjectType}
		countQuery = countQuery.Where(filter)
		listQuery = listQuery.Where(filter)
	}
	if filters.ObjectID > 0 {
		filter := squirrel.Eq{"object_id": filters.ObjectID}
		countQuery = countQuery.Where(filter)
		listQuery = listQuery.Where(filter)
	}
	var total int
	if err := r.db.DB().ScanOnex(ctx, "localPathFileRepo.ListByManager.count", &total, countQuery); err != nil {
		return nil, 0, fmt.Errorf("count manager uploads: %w", err)
	}
	if filters.Limit <= 0 {
		return nil, total, nil
	}
	listQuery = listQuery.OrderBy("created_at DESC", "id DESC").Limit(uint64(filters.Limit))
	if filters.Offset > 0 {
		listQuery = listQuery.Offset(uint64(filters.Offset))
	}
	var files []File
	if err := r.db.DB().ScanAllx(ctx, "localPathFileRepo.ListByManager.list", &files, listQuery); err != nil {
		return nil, 0, fmt.Errorf("list manager uploads: %w", err)
	}
	return files, total, nil
}

type localFileLoader struct {
	repo    FileRepository
	baseURL string
}

func (l *localFileLoader) LoadPreview(ctx context.Context, fileID int64) (*File, error) {
	if fileID <= 0 {
		return nil, nil //nolint:nilnil // empty preview is valid for admin profiles.
	}

	file, err := l.repo.GetByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("load preview file: %w", err)
	}
	if !uploadintegration.Finalized(file) {
		return nil, nil //nolint:nilnil // missing preview is rendered as empty.
	}

	file.URL = uploadhost.ComposeFileURL(l.baseURL, "", file.GetFullPath())
	return &file, nil
}

func (l *localFileLoader) LoadPreviewURL(ctx context.Context, fileID int64) (string, error) {
	file, err := l.LoadPreview(ctx, fileID)
	if err != nil || file == nil {
		return "", err
	}

	return file.GetPublicURL(), nil
}

type localAfterProcess struct {
	baseURL     string
	tx          StoragePgsqlTxManager
	repo        *uploadhost.FileRepo
	eventStream EventStream
	lg          Logger
}

func (p *localAfterProcess) HandleAfterProcess(ctx context.Context, payload string, fnCall AfterProcessFunc) error {
	if fnCall == nil {
		return nil
	}

	data, err := uploadhost.UnmarshalAfterProcessPayload(payload)
	if err != nil {
		return fmt.Errorf("unmarshal after process payload: %w", err)
	}
	file, err := p.repo.GetByID(ctx, data.FileID)
	if err != nil {
		return fmt.Errorf("get after process file: %w", err)
	}

	file.URL = uploadhost.ComposeFileURL(p.baseURL, "", file.GetFullPath())
	run := func(ctx context.Context) error {
		if err := fnCall(ctx, data, file); err != nil {
			return fmt.Errorf("run after process callback: %w", err)
		}
		return nil
	}
	if p.tx != nil {
		err = p.tx.RunInTx(ctx, run)
	} else {
		err = run(ctx)
	}
	if err != nil {
		p.publish(ctx, data.UserID, uploadhost.NewEventAfterProcess(file.ID,
			file.GetPublicURL(), uploadhost.StatusFailed, data.EventType))
		return err
	}

	p.publish(ctx, data.UserID, uploadhost.NewEventAfterProcess(file.ID,
		file.GetPublicURL(), uploadhost.StatusCompleted, data.EventType))
	return nil
}

func (p *localAfterProcess) publish(ctx context.Context, userID UserID, event uploadhost.EventAfterProcess) {
	if p.eventStream == nil || userID.IsZero() {
		return
	}
	if err := NewUploadEventPublisher(p.eventStream).Publish(ctx, userID, event); err != nil && p.lg != nil {
		p.lg.WarnContext(ctx, "publish upload after process event")
	}
}

var (
	_ FileLoader          = (*localFileLoader)(nil)
	_ AfterProcessHandler = (*localAfterProcess)(nil)
)
