//go:build integration

package host //nolint:testpackage // exercises the host runtime against its public migrations and native upload transport.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/assurrussa/goauth/postgres"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	goadminmigrations "github.com/assurrussa/goadmin/migrations"
	"github.com/assurrussa/goadmin/models"
	adminshared "github.com/assurrussa/goadmin/shared"
	pgtests "github.com/assurrussa/goadmin/tests"
)

const (
	nativeAudioContext  = "integration-audio"
	nativeAudioCategory = "audio"
)

func TestLocalNativeTUSAudioAfterPublicMigrationsIntegration(t *testing.T) {
	ctx := t.Context()
	// Only prepare the disposable database and canonical auth schema. The public
	// migration entrypoint, not copied SQL or the legacy test runner, owns all
	// admin/upload tables required by the actual TUS completion below.
	db, _, cleanup := pgtests.PrepareDB(ctx, t, "LocalNativeTUSAudio", pgtests.WithDatabasePathFilesMigration())
	t.Cleanup(func() { cleanup(context.Background()) })
	engine, ok := db.DB().(outbox.StoragePgsqlDBPgxEnginePool)
	require.True(t, ok)
	sqlDB := stdlib.OpenDBFromPool(engine.Pool())
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	t.Cleanup(func() {
		require.NoError(t, goadminmigrations.Reset(context.Background(), goadminmigrations.DatabaseConfig{},
			sqlDB, postgres.ConfirmResetAuthState))
	})
	require.NoError(t, goadminmigrations.Migrate(ctx, goadminmigrations.DatabaseConfig{}, sqlDB))

	queue := &uploadTestQueue{}
	cfg := LocalUploadsConfig{Root: t.TempDir(), BaseURL: "https://admin.test/uploads"}
	tx := outbox.PgsqlTrxNew(db.DB())
	runtime, err := NewLocalUploads(cfg, db, tx, queue, nil, DiscardLogger())
	require.NoError(t, err)
	owner := models.SessionAdmin{ID: 42, UUID: AdminUUID(uploadhost.NewUserID()), SessionID: "native-audio-session"}
	app := nativeAudioApp(runtime, owner)
	wav := nativeAudioWAV()
	created := nativeAudioRequest(t, app, http.MethodPost, "/files/tus", nil, map[string]string{
		"Upload-Length": strconv.Itoa(len(wav)),
		"Upload-Metadata": nativeAudioMetadata(map[string]string{
			"filename": "sample.wav", "context": nativeAudioContext, "file_type": nativeAudioCategory,
			"entity_type": uploadhost.ObjectTypeAdmin.String(), "entity_id": strconv.FormatInt(owner.ID, 10),
		}),
	})
	require.Equal(t, http.StatusCreated, created.status, string(created.body))
	location := created.header.Get("Location")
	require.True(t, strings.HasPrefix(location, "/files/tus/"), location)
	const firstChunk = 3 // "RIF" is not enough to decide that the private bytes are WAV.
	patched := nativeAudioRequest(t, app, http.MethodPatch, location, wav[:firstChunk], map[string]string{
		"Content-Type": "application/offset+octet-stream", "Upload-Offset": "0",
	})
	require.Equal(t, http.StatusNoContent, patched.status, string(patched.body))
	require.Equal(t, strconv.Itoa(firstChunk), patched.header.Get("Upload-Offset"))
	session, err := runtime.Uploads.TusStore.Get(ctx, path.Base(location))
	require.NoError(t, err)
	require.Empty(t, session.MimeType, "a short prefix must remain unapproved until enough bytes arrive")
	require.Equal(t, uploadhost.TusStatusActive, session.Status)

	patched = nativeAudioRequest(t, app, http.MethodPatch, location, wav[firstChunk:], map[string]string{
		"Content-Type": "application/offset+octet-stream", "Upload-Offset": strconv.Itoa(firstChunk),
	})
	require.Equal(t, http.StatusNoContent, patched.status, string(patched.body))
	require.Equal(t, strconv.Itoa(len(wav)), patched.header.Get("Upload-Offset"))
	completed := nativeAudioComplete(t, app, location)
	require.Positive(t, completed.ID)
	require.Equal(t, "7", completed.FileType)
	require.Equal(t, "audio/wav", completed.MimeType)
	require.Equal(t, int64(len(wav)), completed.Size)
	require.Equal(t, uploadhost.FileUploadTaskStatusQueued.String(), completed.Status)
	require.Empty(t, completed.URL)
	require.Len(t, queue.jobs, 1)
	require.Equal(t, "finalize_original_file", queue.jobs[0].name)
	queuedFile, err := runtime.Repositories.FileRepo.GetByID(ctx, completed.ID)
	require.NoError(t, err)
	stagingPath := queuedFile.GetFullPath()
	require.FileExists(t, filepath.Join(cfg.Root, stagingPath))

	session, err = runtime.Uploads.TusStore.Get(ctx, path.Base(location))
	require.NoError(t, err)
	require.Equal(t, uploadhost.TusStatusReady, session.Status)
	require.Equal(t, "audio/wav", session.MimeType)
	require.NotEmpty(t, session.FinalizationKey)
	var savedID int64
	var bindingHash string
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT file_id, binding_hash FROM upload_finalizations WHERE finalization_key = $1::uuid",
		session.FinalizationKey).Scan(&savedID, &bindingHash))
	require.Equal(t, completed.ID, savedID)
	require.Len(t, bindingHash, 64)

	// Reconstruct both service and handler: retries must reuse PostgreSQL's
	// durable handoff, not an in-memory cache, and must not enqueue another job.
	restarted, err := NewLocalUploads(cfg, db, tx, queue, nil, DiscardLogger())
	require.NoError(t, err)
	app = nativeAudioApp(restarted, owner)
	repeated := nativeAudioComplete(t, app, location)
	require.Equal(t, completed, repeated)
	require.Len(t, queue.jobs, 1)

	handled := false
	for _, job := range restarted.Jobs {
		if job.Name() == queue.jobs[0].name {
			require.NoError(t, job.Handle(ctx, queue.jobs[0].payload))
			handled = true
		}
	}
	require.True(t, handled, "the queued job must be the real original-only finalizer")
	require.Len(t, queue.jobs, 2)
	require.Equal(t, "deleted_file", queue.jobs[1].name, "successful finalization schedules staging cleanup")
	var cleanupPayload struct {
		FileID   int64  `json:"fileId"`
		FilePath string `json:"filepath"`
	}
	require.NoError(t, json.Unmarshal([]byte(queue.jobs[1].payload), &cleanupPayload))
	require.Zero(t, cleanupPayload.FileID, "staging cleanup must not delete the final file record")
	require.Equal(t, stagingPath, cleanupPayload.FilePath)
	expectedJobs := append([]uploadQueuedJob(nil), queue.jobs...)
	final, err := restarted.Repositories.FileRepo.GetByID(ctx, completed.ID)
	require.NoError(t, err)
	require.True(t, IsFinalizedUpload(final))
	require.Equal(t, uploadhost.FileTypeAudio, final.FileType)
	require.Equal(t, "audio/wav", final.MimeType)
	require.Equal(t, int64(len(wav)), final.Size)
	require.Equal(t, "sample.wav", final.OriginalFileName)
	require.NotNil(t, final.ManagerID)
	require.Equal(t, owner.ID, *final.ManagerID)
	require.Equal(t, uploadhost.ObjectTypeAdmin, final.ObjectType)
	require.NotNil(t, final.ObjectID)
	require.Equal(t, uploadhost.ObjectID(owner.ID), *final.ObjectID)
	require.Zero(t, final.GetWidth())
	require.Zero(t, final.GetHeight())
	require.True(t, strings.HasPrefix(final.GetFullPath(), "media/v1/"), final.GetFullPath())
	reader, err := restarted.Storage.Open(ctx, final.GetFullPath())
	require.NoError(t, err)
	stored, readErr := io.ReadAll(reader)
	require.NoError(t, reader.Close())
	require.NoError(t, readErr)
	require.Equal(t, wav, stored, "original-only processing must preserve every uploaded byte")
	require.NotEqual(t, stagingPath, final.GetFullPath())
	for _, job := range restarted.Jobs {
		if job.Name() == queue.jobs[1].name {
			require.NoError(t, job.Handle(ctx, queue.jobs[1].payload))
			require.NoError(t, job.Handle(ctx, queue.jobs[1].payload), "staging cleanup is retryable")
		}
		if job.Name() == queue.jobs[0].name {
			require.NoError(t, job.Handle(ctx, queue.jobs[0].payload), "finalizer replay must not enqueue more work")
		}
	}
	require.NoFileExists(t, filepath.Join(cfg.Root, stagingPath))
	reader, err = restarted.Storage.Open(ctx, final.GetFullPath())
	require.NoError(t, err)
	stored, readErr = io.ReadAll(reader)
	require.NoError(t, reader.Close())
	require.NoError(t, readErr)
	require.Equal(t, wav, stored, "staging cleanup must preserve the final artifact")

	repeated = nativeAudioComplete(t, app, location)
	require.Equal(t, completed.ID, repeated.ID)
	require.Equal(t, "7", repeated.FileType)
	require.Equal(t, "audio/wav", repeated.MimeType)
	require.Equal(t, int64(len(wav)), repeated.Size)
	require.Equal(t, uploadhost.FileUploadTaskStatusCompleted.String(), repeated.Status)
	require.Equal(t, cfg.BaseURL+"/"+final.GetFullPath(), repeated.URL)
	require.Equal(t, expectedJobs, queue.jobs, "completion and job retries must not enqueue duplicate work")
	var fileCount, finalizationCount int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT (SELECT count(*) FROM files), (SELECT count(*) FROM upload_finalizations)").Scan(&fileCount, &finalizationCount))
	require.Equal(t, 1, fileCount)
	require.Equal(t, 1, finalizationCount)
}

type nativeAudioStrategy struct{ owner models.SessionAdmin }

func (s nativeAudioStrategy) CanUpload(_ context.Context, req UploadContext) error {
	if req.UserID != s.owner.ID || req.UserUUID != uploadhost.UserID(s.owner.UUID) ||
		req.SessionID != s.owner.SessionID || req.Metadata["entity_type"] != uploadhost.ObjectTypeAdmin.String() ||
		req.Metadata["entity_id"] != strconv.FormatInt(s.owner.ID, 10) {
		return errors.New("audio upload is outside the authenticated owner's object")
	}
	return nil
}

func (nativeAudioStrategy) GetConfig(context.Context, UploadContext) *FileUploadConfig {
	return &FileUploadConfig{
		MaxFileSize: 1 << 20, UploadDir: nativeAudioCategory, AllowedExtensions: []string{".wav"},
		AllowedMimeTypes: map[string][]string{".wav": {"audio/wav"}},
	}
}

func (nativeAudioStrategy) GetAfterJobs(context.Context, UploadContext) ([]FileEventAfterJob, error) {
	return nil, nil
}

func nativeAudioApp(runtime LocalUploadsRuntime, owner models.SessionAdmin) *fiber.App {
	app := fiber.New()
	// Match the admin session context consumed by bootstrap's native handler.
	// Authentication itself is covered by the assembly integration tests.
	app.Use(func(c fiber.Ctx) error {
		c.Locals(adminsession.AuthAdminKey.String(), &owner)
		return c.Next()
	})
	handler := uploadhost.NewFiberUploadHandler(uploadhost.NewUploadHandler(
		runtime.Uploads.Service, runtime.Repositories.FileRepo, runtime.Uploads.TusStore, DiscardLogger(),
		func(ctx context.Context, metadata map[string]string) (uploadhost.UploadContext, error) {
			admin := adminshared.GetAdminAuth(ctx)
			if admin == nil {
				return uploadhost.UploadContext{}, errors.New("admin session is required")
			}
			return uploadhost.UploadContext{
				UserID: admin.ID, UserUUID: uploadhost.UserID(admin.UUID), SessionID: admin.SessionID, Metadata: metadata,
			}, nil
		},
		func(relativePath string) string {
			return uploadhost.ComposeFileURL("https://admin.test/uploads", "", relativePath)
		},
	))
	handler.RegisterStrategy(nativeAudioContext, nativeAudioStrategy{owner: owner})
	handler.RegisterGroupRoutes("/files", app)
	return app
}

type nativeAudioHTTPResult struct {
	status int
	header http.Header
	body   []byte
}

func nativeAudioRequest(
	t *testing.T, app *fiber.App, method, target string, body []byte, headers map[string]string,
) nativeAudioHTTPResult {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(body))
	request.Header.Set("Tus-Resumable", "1.0.0")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	responseBody, readErr := io.ReadAll(response.Body)
	require.NoError(t, response.Body.Close())
	require.NoError(t, readErr)
	return nativeAudioHTTPResult{status: response.StatusCode, header: response.Header, body: responseBody}
}

type nativeAudioFileResponse struct {
	ID       int64  `json:"id"`
	FileType string `json:"fileType"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	Status   string `json:"status"`
	URL      string `json:"url"`
}

func nativeAudioComplete(t *testing.T, app *fiber.App, location string) nativeAudioFileResponse {
	t.Helper()
	response := nativeAudioRequest(t, app, http.MethodPost, location+"/complete", nil, nil)
	require.Equal(t, http.StatusAccepted, response.status, string(response.body))
	var decoded struct {
		File nativeAudioFileResponse `json:"file"`
	}
	require.NoError(t, json.Unmarshal(response.body, &decoded))
	return decoded.File
}

func nativeAudioMetadata(metadata map[string]string) string {
	parts := make([]string, 0, len(metadata))
	for _, key := range []string{"filename", "context", "file_type", "entity_type", "entity_id"} {
		parts = append(parts, key+" "+base64.StdEncoding.EncodeToString([]byte(metadata[key])))
	}
	return strings.Join(parts, ",")
}

func nativeAudioWAV() []byte {
	// 100 ms of deterministic mono 16-bit PCM at 8 kHz, generated without
	// external media tools or a binary fixture copied from GoUploads internals.
	const dataSize = 800 * 2
	wav := make([]byte, 44+dataSize)
	copy(wav[:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], 1)
	binary.LittleEndian.PutUint32(wav[24:28], 8000)
	binary.LittleEndian.PutUint32(wav[28:32], 16000)
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], dataSize)
	for index := range 800 {
		binary.LittleEndian.PutUint16(wav[44+index*2:], uint16(index*31))
	}
	return wav
}
