//go:build integration

package uploadintegration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/internal/admintx"
	"github.com/assurrussa/goadmin/internal/uploadintegration"
	"github.com/assurrussa/goadmin/tests"
)

func TestPreviewBindingConcurrentOwnershipAndMetadataPreservation(t *testing.T) {
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "AtomicPreviewBinding")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanup(ctx)
	})
	tx := adminhost.NewTxManager(db)
	cfg := tests.AuthRuntimeConfig(t)
	cfg.NotificationDelivery = goauth.NotificationDeliveryDisabled
	adapter, err := adminhost.NewAuthAdapter(adminhost.AuthAdapterConfig{Database: db, TxManager: tx, Runtime: cfg})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close()) })
	repo, err := uploadhost.NewFileRepo(db, tx)
	require.NoError(t, err)
	file := tests.CreateFile(t)
	file.ID = 0
	file.ObjectType = uploadhost.ObjectType("quarantine")
	file.ObjectID = nil
	file.FolderPath = "media/v1/preview"
	file.Data.Presets = map[uploadhost.PresetName]uploadhost.FilePreset{
		"main": {RelativePath: file.FolderPath + "/" + file.FileName},
	}
	file.Data.Uploader.Status = uploadhost.FileUploadTaskStatusCompleted
	fileID, err := repo.Create(t.Context(), file)
	require.NoError(t, err)
	canonicalDB := adapter.Runtime().Database()
	_, err = canonicalDB.ExecContext(t.Context(), `UPDATE files SET data=data || '{"customMetadata":{"doNotDrop":true}}'::jsonb
WHERE id=$1`, fileID)
	require.NoError(t, err)
	var initialData string
	require.NoError(t, canonicalDB.QueryRowContext(t.Context(),
		"SELECT data::text FROM files WHERE id=$1", fileID).Scan(&initialData))
	request := uploadintegration.PreviewBinding{
		FileID: fileID, ManagerID: *file.ManagerID,
		ExpectedObjectType: file.ObjectType, ExpectedObjectID: nil, AdminID: 17,
	}
	require.ErrorContains(t, uploadintegration.BindPreview(t.Context(), db, request), "managed admin transaction")

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, target := range []int64{17, 18} {
		workers.Go(func() {
			<-start
			binding := request
			binding.AdminID = target
			results <- adapter.Runtime().InAuthTransaction(t.Context(), func(ctx context.Context) error {
				executor, err := adapter.Runtime().SQLExecutor(ctx)
				if err != nil {
					return err
				}
				return uploadintegration.BindPreview(admintx.WithExecutor(ctx, executor, db.DB().Pool()), db, binding)
			})
		})
	}
	close(start)
	workers.Wait()
	close(results)
	var success, conflict int
	for err := range results {
		if err == nil {
			success++
			continue
		}
		require.ErrorIs(t, err, uploadintegration.ErrPreviewBindingConflict)
		conflict++
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	bound, err := repo.GetByID(t.Context(), fileID)
	require.NoError(t, err)
	require.Equal(t, uploadhost.ObjectTypeAdmin, bound.ObjectType)
	require.Contains(t, []int64{17, 18}, bound.ObjectID.Int64())
	require.Equal(t, file.ManagerID, bound.ManagerID)
	require.Equal(t, file.FileName, bound.FileName)
	require.Equal(t, file.Data.Presets, bound.Data.Presets)
	var finalData string
	require.NoError(t, canonicalDB.QueryRowContext(t.Context(), "SELECT data::text FROM files WHERE id=$1", fileID).Scan(&finalData))
	require.JSONEq(t, initialData, finalData, "binding must preserve opaque upload metadata")

	// Failure after the linkage write must restore both ownership and metadata.
	file.ID = 0
	file.Slug += "-rollback"
	rollbackID, err := repo.Create(t.Context(), file)
	require.NoError(t, err)
	request.FileID = rollbackID
	injected := errors.New("preview projection failed")
	err = adapter.Runtime().InAuthTransaction(t.Context(), func(ctx context.Context) error {
		executor, err := adapter.Runtime().SQLExecutor(ctx)
		if err != nil {
			return err
		}
		if err = uploadintegration.BindPreview(admintx.WithExecutor(ctx, executor, db.DB().Pool()), db, request); err != nil {
			return err
		}
		return injected
	})
	require.ErrorIs(t, err, injected)
	rolledBack, err := repo.GetByID(t.Context(), rollbackID)
	require.NoError(t, err)
	require.Equal(t, file.ObjectType, rolledBack.ObjectType)
	require.Nil(t, rolledBack.ObjectID)
	require.Equal(t, file.Data.Presets, rolledBack.Data.Presets)
}
