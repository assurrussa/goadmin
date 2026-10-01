package files_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/http/handlers/files"
	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/shared"
)

const uploadDomain = integrationroles.PermissionDomainUploads

func TestAPI_RegisterGroupRoutes(t *testing.T) {
	app := fiber.New()
	adminAppTest := adminappt.NewAppTest(t)

	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionRead)).
		Return(nil).Times(1)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionCreate)).
		Return(nil).Times(1)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionDelete)).
		Return(nil).Times(1)

	h := files.NewHandler(adminAppTest.App, &testHandler{}, &testFileRepo{})
	assert.NotPanics(t, func() {
		h.RegisterGroupRoutes(app)
	})

	routes := app.GetRoutes()
	assert.NotEmpty(t, routes, "should have routes registered")
	assert.Len(t, routes, 2)
}

type testHandler struct{}

func (h *testHandler) RegisterGroupRoutesWithGuards(_ string, router fiber.Router, _ uploadhost.UploadRouteGuards) {
	router.Get("test", func(_ fiber.Ctx) error { return nil })
	router.Get("test2", func(_ fiber.Ctx) error { return nil })
}

func TestAPI_ReadOnlyCannotMutateFiles(t *testing.T) {
	app := fiber.New()
	adminAppTest := adminappt.NewAppTest(t)

	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionRead)).
		Return(func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	for _, action := range []integrationroles.PermissionAction{
		integrationroles.PermissionActionCreate,
		integrationroles.PermissionActionDelete,
	} {
		adminAppTest.MockPermissionGuard.EXPECT().
			AdminGuard(integrationroles.NewPermissionKey(uploadDomain, action)).
			Return(func(c fiber.Ctx) error { return c.SendStatus(http.StatusForbidden) })
	}

	uploads := uploadhost.NewFiberUploadHandler(uploadhost.NewUploadHandler(nil, nil, nil, nil, nil, nil))
	files.NewHandler(adminAppTest.App, uploads, files.NewManagerScopedFileRepository(&testFileRepo{})).RegisterGroupRoutes(app)

	for _, tc := range []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/files", http.StatusNoContent},
		{http.MethodPost, "/files", http.StatusForbidden},
		{http.MethodPost, "/files/rich-text", http.StatusForbidden},
		{http.MethodOptions, "/files/tus", http.StatusForbidden},
		{http.MethodPost, "/files/tus", http.StatusForbidden},
		{http.MethodHead, "/files/tus/upload-1", http.StatusForbidden},
		{http.MethodPatch, "/files/tus/upload-1", http.StatusForbidden},
		{http.MethodPost, "/files/tus/upload-1/complete", http.StatusForbidden},
		{http.MethodDelete, "/files/1", http.StatusForbidden},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			response, err := app.Test(httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, nil))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
			require.Equal(t, tc.want, response.StatusCode)
		})
	}
}

func TestAPI_GetFileHidesFileOwnedByAnotherAdmin(t *testing.T) {
	app := fiber.New()
	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionRead)).
		Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionCreate)).
		Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionDelete)).
		Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)

	const adminID = int64(12)
	app.Use(func(c fiber.Ctx) error {
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: adminID})
		return c.Next()
	})

	foreignManagerID := int64(99)
	fileRepo := files.NewManagerScopedFileRepository(&testFileRepo{
		files: []uploadhost.File{testFile(42, &foreignManagerID)},
	})
	uploader := &testTaskUploader{file: testFile(42, &foreignManagerID)}
	uploads := uploadhost.NewFiberUploadHandler(uploadhost.NewUploadHandler(uploader, fileRepo, nil, nil, testUploadActor, nil))
	files.NewHandler(adminAppTest.App, uploads, fileRepo).RegisterGroupRoutes(app)

	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/files/file/42", nil))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusNotFound, response.StatusCode)
	require.Zero(t, uploader.getCalls, "foreign file must be rejected before calling the upload service")

	deleteRequest := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/files/42?confirm=true", nil)
	deleteResponse, err := app.Test(deleteRequest)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, deleteResponse.Body.Close()) })
	require.Equal(t, http.StatusNotFound, deleteResponse.StatusCode)
	require.Zero(t, uploader.deleteCalls, "foreign file must be rejected before enqueueing deletion")
}

func TestAPI_GetFileAllowsManagerOwnedFile(t *testing.T) {
	app := fiber.New()
	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionRead)).
		Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionCreate)).
		Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionDelete)).
		Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)

	const adminID = int64(12)
	app.Use(func(c fiber.Ctx) error {
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: adminID})
		return c.Next()
	})

	managerID := adminID
	fileRepo := files.NewManagerScopedFileRepository(&testFileRepo{
		files: []uploadhost.File{testFile(42, &managerID)},
	})
	uploader := &testTaskUploader{file: testFile(42, &managerID)}
	uploads := uploadhost.NewFiberUploadHandler(uploadhost.NewUploadHandler(uploader, fileRepo, nil, nil, testUploadActor, nil))
	files.NewHandler(adminAppTest.App, uploads, fileRepo).RegisterGroupRoutes(app)

	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/files/file/42", nil))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, 1, uploader.getCalls)
}

func TestAPI_FileReadsFailClosedWithoutSessionOrActiveOwnership(t *testing.T) {
	app := fiber.New()
	adminAppTest := adminappt.NewAppTest(t)
	for _, action := range []integrationroles.PermissionAction{
		integrationroles.PermissionActionRead,
		integrationroles.PermissionActionCreate,
		integrationroles.PermissionActionDelete,
	} {
		adminAppTest.MockPermissionGuard.EXPECT().
			AdminGuard(integrationroles.NewPermissionKey(uploadDomain, action)).
			Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)
	}

	adminID := int64(12)
	foreignID := int64(99)
	deleted := testFile(42, &adminID)
	deleted.DeletedAt = sql.NullTime{Valid: true}
	fileRepo := files.NewManagerScopedFileRepository(&testFileRepo{files: []uploadhost.File{
		testFile(41, &adminID), deleted, testFile(43, nil), testFile(44, &foreignID),
	}})
	uploader := &testTaskUploader{file: testFile(41, &adminID)}
	app.Use(func(c fiber.Ctx) error {
		if c.Get("X-Test-Admin") == "yes" {
			c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: adminID})
		}
		return c.Next()
	})
	uploads := uploadhost.NewFiberUploadHandler(uploadhost.NewUploadHandler(uploader, fileRepo, nil, nil, testUploadActor, nil))
	files.NewHandler(adminAppTest.App, uploads, fileRepo).RegisterGroupRoutes(app)

	for _, tc := range []struct {
		method        string
		path          string
		authenticated bool
	}{
		{http.MethodGet, "/files/file/41", false},
		{http.MethodGet, "/files/tasks/41", false},
		{http.MethodDelete, "/files/41?confirm=true", false},
		{http.MethodGet, "/files/file/42", true},
		{http.MethodGet, "/files/tasks/42", true},
		{http.MethodGet, "/files/upload/tasks/42", true},
		{http.MethodGet, "/files/file/43", true},
		{http.MethodGet, "/files/file/44", true},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, nil)
			if tc.authenticated {
				req.Header.Set("X-Test-Admin", "yes")
			}
			response, err := app.Test(req)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
			require.Equal(t, http.StatusNotFound, response.StatusCode)
		})
	}
	require.Zero(t, uploader.getCalls, "denied task lookup must not reach upload service")
	require.Zero(t, uploader.deleteCalls, "denied delete must not reach upload service")

	response, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/files?entity_type=admin&entity_id=99", nil,
	))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusOK, response.StatusCode)
	var body struct {
		Files []json.RawMessage `json:"files"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Empty(t, body.Files, "anonymous list must not expose files")
}

type inaccurateManagerLister struct {
	*testFileRepo
	foreign uploadhost.File
}

func (r inaccurateManagerLister) ListByManager(
	_ context.Context, _ int64, _ uploadhost.ListFilters,
) ([]uploadhost.File, int, error) {
	return []uploadhost.File{r.foreign}, 1, nil
}

func TestManagerScopedFileRepositoryRejectsInaccurateManagerResult(t *testing.T) {
	const adminID = int64(12)
	foreignID := int64(99)
	repo := files.NewManagerScopedFileRepository(inaccurateManagerLister{
		testFileRepo: &testFileRepo{},
		foreign:      testFile(44, &foreignID),
	})
	app := fiber.New()
	app.Get("/files", func(c fiber.Ctx) error {
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: adminID})
		_, _, err := repo.List(c, uploadhost.ListFilters{ObjectType: "admin", ObjectID: 99, Limit: 50})
		return err
	})

	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/files", nil))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusInternalServerError, response.StatusCode)
}

func TestAPI_ListFilesFiltersFilesOwnedByAnotherAdmin(t *testing.T) {
	app := fiber.New()
	adminAppTest := adminappt.NewAppTest(t)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionRead)).
		Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionCreate)).
		Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)
	adminAppTest.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(uploadDomain, integrationroles.PermissionActionDelete)).
		Return(func(c fiber.Ctx) error { return c.Next() }).Times(1)

	const adminID = int64(12)
	app.Use(func(c fiber.Ctx) error {
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: adminID})
		return c.Next()
	})

	foreignManagerID := int64(99)
	currentManagerID := adminID
	rows := make([]uploadhost.File, 0, 102)
	for id := int64(1); id <= 101; id++ {
		rows = append(rows, testFile(id, &foreignManagerID))
	}
	rows = append(rows, testFile(143, &currentManagerID))
	fileRepo := files.NewManagerScopedFileRepository(&testFileRepo{
		files: rows,
	})
	uploads := uploadhost.NewFiberUploadHandler(
		uploadhost.NewUploadHandler(&testTaskUploader{}, fileRepo, nil, nil, testUploadActor, nil),
	)
	files.NewHandler(adminAppTest.App, uploads, fileRepo).RegisterGroupRoutes(app)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/files?entity_type=admin&entity_id=99", nil)
	response, err := app.Test(request)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusOK, response.StatusCode)

	var body struct {
		Files []struct {
			ID int64 `json:"id"`
		} `json:"files"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Len(t, body.Files, 1)
	require.Equal(t, int64(143), body.Files[0].ID)
}

type testFileRepo struct {
	files []uploadhost.File
}

func testFile(id int64, managerID *int64) uploadhost.File {
	data := &uploadhost.FileData{}
	data.Uploader.Status = uploadhost.FileUploadTaskStatusFailed

	return uploadhost.File{
		ID:         id,
		ManagerID:  managerID,
		ObjectType: uploadhost.ObjectTypeAdmin,
		ObjectID:   uploadhost.ObjectIDPtr(99),
		Data:       data,
	}
}

func (r *testFileRepo) GetByID(_ context.Context, id int64) (uploadhost.File, error) {
	for _, file := range r.files {
		if file.ID == id {
			return file, nil
		}
	}
	return uploadhost.File{}, nil
}

func (r *testFileRepo) List(_ context.Context, filters uploadhost.ListFilters) ([]uploadhost.File, int, error) {
	files := make([]uploadhost.File, 0, len(r.files))
	for _, file := range r.files {
		if file.ObjectType == uploadhost.ObjectType(filters.ObjectType) &&
			file.ObjectID != nil && file.ObjectID.Int64() == filters.ObjectID {
			files = append(files, file)
		}
	}
	total := len(files)
	if filters.Offset >= total {
		return nil, total, nil
	}
	files = files[filters.Offset:]
	if filters.Limit >= 0 && len(files) > filters.Limit {
		files = files[:filters.Limit]
	}
	return files, total, nil
}

type testTaskUploader struct {
	file        uploadhost.File
	getCalls    int
	deleteCalls int
}

func (u *testTaskUploader) UploadBatch(context.Context, uploadhost.BatchRequest) ([]uploadhost.File, error) {
	return nil, nil
}

func (u *testTaskUploader) UploadSingle(context.Context, uploadhost.SingleRequest) (uploadhost.File, error) {
	return uploadhost.File{}, nil
}

func (u *testTaskUploader) UploadReader(
	context.Context, uploadhost.ReaderRequest, uploadhost.ReaderUploadInput,
) (uploadhost.File, error) {
	return uploadhost.File{}, nil
}

func (u *testTaskUploader) UploadStored(
	context.Context, uploadhost.ReaderRequest, uploadhost.UploadedFile,
) (uploadhost.File, error) {
	return uploadhost.File{}, nil
}

func (u *testTaskUploader) DeleteFile(context.Context, uploadhost.DeleteRequest) error {
	u.deleteCalls++
	return nil
}

func (u *testTaskUploader) GetFile(context.Context, int64) (uploadhost.File, error) {
	u.getCalls++
	return u.file, nil
}

// Test requests supply the same positive admin actor contract as bootstrap.
func testUploadActor(ctx context.Context, metadata map[string]string) (uploadhost.UploadContext, error) {
	actor := shared.GetAdminAuth(ctx)
	if actor == nil {
		return uploadhost.UploadContext{}, nil
	}
	return uploadhost.UploadContext{UserID: actor.ID, Metadata: metadata}, nil
}
