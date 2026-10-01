package bootstrap //nolint:testpackage // exercises route registration order

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/gofiber/fiber/v3"

	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/models"
)

const (
	testMediaFolder      = "media/v1/test"
	testAvatarFilename   = "avatar.png"
	testHTMLFilename     = "page.html"
	testSVGFilename      = "image.svg"
	testUnknownFilename  = "unknown"
	testCodeFilename     = "code.js"
	testDocumentFilename = "document.xml"
	testForeignFilename  = "foreign.png"
	testOrphanFilename   = "orphan.png"
	testAuthorizedHeader = "yes"
	testPrivateBody      = "private"
)

func TestRegisterStaticRoutes(t *testing.T) {
	root := t.TempDir()
	uploadsRoot := filepath.Join(root, "uploads")
	customRoot := filepath.Join(root, "custom")
	for _, dir := range []string{filepath.Join(uploadsRoot, testMediaFolder), customRoot} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(uploadsRoot, testMediaFolder,
		testAvatarFilename), []byte(testPrivateBody), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		testHTMLFilename, testSVGFilename, testCodeFilename, testDocumentFilename,
		testUnknownFilename, testForeignFilename, testOrphanFilename,
	} {
		if err := os.WriteFile(filepath.Join(uploadsRoot, testMediaFolder, name), []byte("active"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(customRoot, "host.txt"), []byte("host"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	registerStaticRoutes(app, fstest.MapFS{
		"dist/js/app.js":   &fstest.MapFile{Data: []byte("entry")},
		"dist/css/app.css": &fstest.MapFile{Data: []byte("style")},
		"dist/js/page.js":  &fstest.MapFile{Data: []byte("chunk")},
	}, []StaticRoute{{Path: "/host", Root: customRoot}}, []func(*fiber.App){
		func(app *fiber.App) { app.Get("/host/health", func(c fiber.Ctx) error { return c.SendString("ok") }) },
	}, uploadsRoot, staticPathRepo{files: map[string]uploadhost.File{
		testAvatarFilename: {
			ID: 1, ManagerID: staticAdminID(1), Data: staticFinalData(testAvatarFilename),
			FolderPath: testMediaFolder, FileName: testAvatarFilename,
		},
		testHTMLFilename: {
			ID: 2, ManagerID: staticAdminID(1), Data: staticFinalData(testHTMLFilename),
			FolderPath: testMediaFolder, FileName: testHTMLFilename,
		},
		testSVGFilename: {
			ID: 3, ManagerID: staticAdminID(1), Data: staticFinalData(testSVGFilename),
			FolderPath: testMediaFolder, FileName: testSVGFilename,
		},
		testCodeFilename: {
			ID: 4, ManagerID: staticAdminID(1), Data: staticFinalData(testCodeFilename),
			FolderPath: testMediaFolder, FileName: testCodeFilename,
		},
		testDocumentFilename: {
			ID: 5, ManagerID: staticAdminID(1), Data: staticFinalData(testDocumentFilename),
			FolderPath: testMediaFolder, FileName: testDocumentFilename,
		},
		testUnknownFilename: {
			ID: 6, ManagerID: staticAdminID(1), Data: staticFinalData(testUnknownFilename),
			FolderPath: testMediaFolder, FileName: testUnknownFilename,
		},
		testForeignFilename: {
			ID: 7, ManagerID: staticAdminID(2), Data: staticFinalData(testForeignFilename),
			FolderPath: testMediaFolder, FileName: testForeignFilename,
		},
	}}, func(c fiber.Ctx) error {
		if c.Get("X-Test-Deny-Upload-Read") == testAuthorizedHeader {
			return c.SendStatus(fiber.StatusForbidden)
		}
		return c.Next()
	}, func(c fiber.Ctx) error {
		if c.Get("X-Test-Admin") != testAuthorizedHeader {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
		return c.Next()
	})

	assertPublicStaticRoutes(t, app)
	assertProtectedUploadRoutes(t, app)
}

type staticPathRepo struct {
	FileRepo
	files map[string]uploadhost.File
}

func (r staticPathRepo) GetByPath(_ context.Context, path string) (uploadhost.File, error) {
	return r.files[strings.TrimPrefix(path, "media/v1/test/")], nil
}

func staticAdminID(id int64) *int64 { return &id }
func staticFinalData(name string) *uploadhost.FileData {
	return &uploadhost.FileData{Presets: map[uploadhost.PresetName]uploadhost.FilePreset{"main": {
		RelativePath: "media/v1/test/" + name,
		URL:          "https://admin.test/uploads/" + name,
	}}}
}

type staticListRepo struct {
	FileRepo
	files []uploadhost.File
}

func (r staticListRepo) List(_ context.Context, filters uploadhost.ListFilters) ([]uploadhost.File, int, error) {
	if filters.Offset >= len(r.files) {
		return nil, len(r.files), nil
	}
	end := min(filters.Offset+filters.Limit, len(r.files))
	return r.files[filters.Offset:end], len(r.files), nil
}

func TestFindUploadByPathScansHostRepositoryPages(t *testing.T) {
	t.Parallel()
	files := make([]uploadhost.File, 257)
	for i := range files[:256] {
		files[i] = uploadhost.File{ID: int64(i + 1), FolderPath: "upload_file", FileName: "other.png"}
	}
	files[256] = uploadhost.File{ID: 257, FolderPath: "upload_file", FileName: testAvatarFilename}
	file, err := findUploadByPath(context.Background(), staticListRepo{files: files}, "upload_file/"+testAvatarFilename)
	if err != nil || file.ID != 257 {
		t.Fatalf("paginated host lookup: file ID = %d, error = %v", file.ID, err)
	}
}

func TestStaticUploadRechecksOwnershipAndDeletionForEachRequest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, testMediaFolder), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, testMediaFolder, testAvatarFilename),
		[]byte(testPrivateBody), 0o600); err != nil {
		t.Fatal(err)
	}
	ownerID := int64(1)
	foreignID := int64(2)
	files := map[string]uploadhost.File{
		testAvatarFilename: {
			ID: 1, ManagerID: &ownerID, Data: staticFinalData(testAvatarFilename),
			FolderPath: testMediaFolder, FileName: testAvatarFilename,
		},
	}
	app := fiber.New()
	registerStaticRoutes(app, nil, nil, nil, root, staticPathRepo{files: files}, nil, func(c fiber.Ctx) error {
		if c.Get("X-Test-Admin") != testAuthorizedHeader {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: ownerID})
		return c.Next()
	})

	request := func(method, rangeHeader string) staticTestResponse {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), method, "/uploads/media/v1/test/"+testAvatarFilename, nil)
		req.Header.Set("X-Test-Admin", testAuthorizedHeader)
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return staticTestResponse{status: resp.StatusCode, header: resp.Header, body: body}
	}

	if got := request(http.MethodGet, ""); got.status != http.StatusOK || string(got.body) != testPrivateBody {
		t.Fatalf("initial owner read: status = %d, body = %q", got.status, got.body)
	}
	for _, tc := range []struct {
		name string
		file uploadhost.File
	}{
		{name: "owner changed", file: uploadhost.File{
			ID: 1, ManagerID: &foreignID,
			Data: staticFinalData(testAvatarFilename), FolderPath: testMediaFolder,
			FileName: testAvatarFilename,
		}},
		{name: "unowned", file: uploadhost.File{
			ID: 1, Data: staticFinalData(testAvatarFilename),
			FolderPath: testMediaFolder, FileName: testAvatarFilename,
		}},
		{name: "soft deleted", file: uploadhost.File{
			ID: 1, ManagerID: &ownerID, Data: staticFinalData(testAvatarFilename),
			FolderPath: testMediaFolder, FileName: testAvatarFilename, DeletedAt: sql.NullTime{Valid: true},
		}},
		{name: "legacy record", file: uploadhost.File{
			ID: 1, ManagerID: &ownerID,
			FolderPath: testMediaFolder, FileName: testAvatarFilename,
		}},
		{name: "queued record", file: uploadhost.File{
			ID: 1, ManagerID: &ownerID,
			FolderPath: testMediaFolder, FileName: testAvatarFilename, Data: staticQueuedData(),
		}},

		{name: "path lookup mismatch", file: uploadhost.File{ID: 1, ManagerID: &ownerID, FileName: "other.png"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files[testAvatarFilename] = tc.file
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				got := request(method, "bytes=0-1")
				if got.status != http.StatusNotFound || string(got.body) == testPrivateBody {
					t.Fatalf("%s with %s: status = %d, body = %q", tc.name, method, got.status, got.body)
				}
			}
		})
	}
	delete(files, testAvatarFilename)
	if got := request(http.MethodGet, ""); got.status != http.StatusNotFound || string(got.body) == testPrivateBody {
		t.Fatalf("orphaned disk file: status = %d, body = %q", got.status, got.body)
	}
}

func assertPublicStaticRoutes(t *testing.T, app *fiber.App) {
	t.Helper()
	for _, path := range []string{"/public/dist/js/app.js", "/public/dist/css/app.css"} {
		resp := staticResponse(t, app, path, false)
		if resp.status != fiber.StatusOK {
			t.Fatalf("%s: status = %d", path, resp.status)
		}
		if got := resp.header.Get("Cache-Control"); got != "no-cache, must-revalidate" {
			t.Fatalf("%s: cache policy = %q", path, got)
		}
	}
	resp := staticResponse(t, app, "/public/dist/js/page.js", false)
	if resp.header.Get("Cache-Control") == "no-cache, must-revalidate" {
		t.Fatal("hashed chunk unexpectedly has entry cache policy")
	}
	if resp := staticResponse(t, app, "/host/host.txt", false); resp.status != fiber.StatusOK {
		t.Fatalf("host static route status = %d", resp.status)
	}
	if resp := staticResponse(t, app, "/host/health", false); resp.status != fiber.StatusOK {
		t.Fatalf("host public route status = %d", resp.status)
	}
}

func TestEmbeddedEntrypointsIgnoreUnreliableModificationDates(t *testing.T) {
	app := fiber.New()
	registerStaticRoutes(app, fstest.MapFS{
		"dist/js/app.js":   &fstest.MapFile{Data: []byte("current entry")},
		"dist/css/app.css": &fstest.MapFile{Data: []byte("current style")},
	}, nil, nil, "", nil, nil)

	for _, entry := range []struct{ path, body string }{
		{"/public/dist/js/app.js", "current entry"},
		{"/public/dist/css/app.css", "current style"},
	} {
		t.Run(entry.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, entry.path, nil)
			// embed.FS has a zero ModTime, even when the compiled bytes change.
			req.Header.Set("If-Modified-Since", "Wed, 30 Sep 2026 10:10:00 GMT")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK || string(body) != entry.body {
				t.Fatalf("revalidation returned status %d, body %q", resp.StatusCode, body)
			}
			if got := resp.Header.Get("Last-Modified"); got != "" {
				t.Fatalf("unreliable Last-Modified = %q", got)
			}
		})
	}
}

func assertProtectedUploadRoutes(t *testing.T, app *fiber.App) {
	t.Helper()
	if resp := staticResponse(t, app, "/uploads/media/v1/test/avatar.png", false); resp.status != fiber.StatusUnauthorized {
		t.Fatalf("unauthenticated upload status = %d", resp.status)
	}
	resp := staticResponse(t, app, "/uploads/media/v1/test/avatar.png", true)
	if resp.status != fiber.StatusOK {
		t.Fatalf("authenticated upload status = %d", resp.status)
	}
	if string(resp.body) != testPrivateBody {
		t.Fatalf("upload body = %q", resp.body)
	}
	if got := resp.header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("image sniffing policy = %q", got)
	}
	if got := resp.header.Get("Content-Disposition"); got != "" {
		t.Fatalf("image disposition = %q", got)
	}
	for _, name := range []string{
		testHTMLFilename, testSVGFilename, testCodeFilename,
		testDocumentFilename, testUnknownFilename,
	} {
		resp := staticResponse(t, app, "/uploads/media/v1/test/"+name, true)
		if resp.status != fiber.StatusOK {
			t.Fatalf("%s: status = %d", name, resp.status)
		}
		if got := resp.header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("%s: sniffing policy = %q", name, got)
		}
		if got := resp.header.Get("Content-Disposition"); got != "attachment" {
			t.Fatalf("%s: disposition = %q", name, got)
		}
	}
	for _, path := range []string{
		"/uploads/" + testForeignFilename, "/uploads/" + testOrphanFilename,
		"/uploads/%2e%2e/avatar.png", "/uploads//avatar.png",
	} {
		if resp := staticResponse(t, app, path, true); resp.status != fiber.StatusNotFound {
			t.Fatalf("%s: unauthorized path status = %d", path, resp.status)
		}
	}
	if resp := staticResponse(t, app, "/tmp/avatar.png", true); resp.status != fiber.StatusNotFound {
		t.Fatalf("temporary files status = %d", resp.status)
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/uploads/media/v1/test/avatar.png", nil)
	req.Header.Set("X-Test-Admin", testAuthorizedHeader)
	req.Header.Set("X-Test-Deny-Upload-Read", testAuthorizedHeader)
	deniedResp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = deniedResp.Body.Close() })
	if deniedResp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("owner without uploads.read status = %d", deniedResp.StatusCode)
	}
}

type staticTestResponse struct {
	status int
	header http.Header
	body   []byte
}

func staticResponse(t *testing.T, app *fiber.App, path string, authenticated bool) staticTestResponse {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
	if authenticated {
		req.Header.Set("X-Test-Admin", testAuthorizedHeader)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return staticTestResponse{status: resp.StatusCode, header: resp.Header, body: body}
}

func staticQueuedData() *uploadhost.FileData {
	data := staticFinalData(testAvatarFilename)
	data.Uploader.Status = uploadhost.FileUploadTaskStatusQueued
	return data
}
