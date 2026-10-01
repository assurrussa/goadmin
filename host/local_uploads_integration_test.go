//go:build integration

package host //nolint:testpackage // verifies the local repository's SQL extension

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"
	"time"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/require"

	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/pointer"
	"github.com/assurrussa/goadmin/models"
	pgtests "github.com/assurrussa/goadmin/tests"
)

func TestLocalPathFileRepoIntegration(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := pgtests.PrepareDB(ctx, t, "TestLocalPathFileRepo")
	defer cleanup(ctx)
	repo, err := uploadhost.NewFileRepo(db, outbox.PgsqlTrxNew(db.DB()))
	require.NoError(t, err)
	local := &localPathFileRepo{FileRepo: repo, db: db}

	owned := pgtests.CreateFile(t)
	owned.ID = 0
	owned.FolderPath = "upload_file"
	owned.FileName = "owned.png"
	owned.URL = "upload_file/owned.png"
	owned.ManagerID = pointer.To[int64](1)
	owned.Slug = "local-owned"
	ownedID, err := repo.Create(ctx, owned)
	require.NoError(t, err)

	foreign := owned
	foreign.ID = 0
	foreign.FileName = "foreign.png"
	foreign.ManagerID = pointer.To[int64](2)
	foreign.Slug = "local-foreign"
	_, err = repo.Create(ctx, foreign)
	require.NoError(t, err)

	adminCtx := context.WithValue(ctx, adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: 1}) //nolint:staticcheck // matches session storage key.
	file, err := local.GetByPath(adminCtx, "upload_file/owned.png")
	require.NoError(t, err)
	require.Equal(t, ownedID, file.ID)
	require.Equal(t, owned.Data.Width, file.Data.Width)
	file, err = local.GetByPath(adminCtx, "upload_file/foreign.png")
	require.NoError(t, err)
	require.Zero(t, file.ID)

	files, total, err := local.ListByManager(ctx, 1, ListFilters{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, files, 1)
	require.Equal(t, ownedID, files[0].ID)
	require.Equal(t, owned.Data.Width, files[0].Data.Width)
	require.NoError(t, repo.DeleteByID(ctx, ownedID))
	files, total, err = local.ListByManager(ctx, 1, ListFilters{Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, files)
}

type (
	uploadQueuedJob struct{ name, payload string }
	uploadTestQueue struct{ jobs []uploadQueuedJob }
)

func (q *uploadTestQueue) Put(_ context.Context, name, payload string, _ time.Time) (outbox.JobID, error) {
	q.jobs = append(q.jobs, uploadQueuedJob{name, payload})
	return outbox.JobIDNil, nil
}

func TestLocalOriginalUploadsAsyncLifecycleIntegration(t *testing.T) {
	ctx := t.Context()
	db, _, cleanup := pgtests.PrepareDB(ctx, t, "TestLocalOriginalUploadsAsyncLifecycle")
	defer cleanup(context.Background())
	tx := outbox.PgsqlTrxNew(db.DB())
	queue := &uploadTestQueue{}
	runtime, err := NewLocalUploads(LocalUploadsConfig{Root: t.TempDir(), BaseURL: "https://admin.test/uploads"}, db, tx, queue, nil, DiscardLogger())
	require.NoError(t, err)
	var body bytes.Buffer
	require.NoError(t, png.Encode(&body, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	user := uploadhost.NewUserID()
	file, err := runtime.Uploads.Service.UploadReader(ctx, ReaderRequest{
		UploaderUUID: user, ManagerID: 1, ObjectType: uploadhost.ObjectTypeAdmin, ObjectID: 1,
		AfterJobs: uploadhost.NewFileEventAfterJobs("test_preview", user, nil),
	}, ReaderUploadInput{OriginalName: "avatar.png", Size: int64(body.Len()), Reader: bytes.NewReader(body.Bytes())})
	require.NoError(t, err)
	require.Equal(t, uploadhost.FileUploadTaskStatusQueued, file.Data.Uploader.Status)
	require.False(t, IsFinalizedUpload(file))
	require.Len(t, queue.jobs, 1)
	require.NotEqual(t, "test_preview", queue.jobs[0].name)
	handled := false
	for _, job := range runtime.Jobs {
		if job.Name() == queue.jobs[0].name {
			require.NoError(t, job.Handle(ctx, queue.jobs[0].payload))
			handled = true
		}
	}
	require.True(t, handled)
	final, err := runtime.Repositories.FileRepo.GetByID(ctx, file.ID)
	require.NoError(t, err)
	require.True(t, IsFinalizedUpload(final), "final file=%+v data=%+v", final, final.Data)
	require.Empty(t, final.URL)
	preview, err := runtime.Repositories.FileLoader.LoadPreview(ctx, file.ID)
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.Contains(t, preview.GetPublicURL(), "https://admin.test/uploads/media/v1/")
	callback := false
	for _, job := range queue.jobs {
		if job.name == "test_preview" {
			require.NoError(t, runtime.Uploads.AfterProcess.HandleAfterProcess(ctx, job.payload, func(_ context.Context, data AfterProcessPayload, file File) error {
				require.Equal(t, user, data.UserID)
				require.True(t, IsFinalizedUpload(file))
				callback = true
				return nil
			}))
		}
	}
	require.True(t, callback)
}
