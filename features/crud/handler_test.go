//nolint:goconst,lll // Self-contained HTTP fixtures retain literal requests and expectations for audit.
package crud //nolint:testpackage // Exercises actual resource route registration and DTO boundaries.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goinertia"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/hosttest"
	coreinertia "github.com/assurrussa/goadmin/infrastructure/core/inertia"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type testRow struct {
	Name     string `json:"name"`
	Secret   string `json:"secret"`
	ReadOnly string `json:"readOnly"`
}
type testCreate struct {
	Name string `json:"name"`
}
type testUpdate struct {
	Name     string  `json:"name"`
	Stock    int64   `json:"stock"`
	Active   bool    `json:"active"`
	Nullable *string `json:"nullable"`
}

func definition() Definition[testRow, testCreate, testUpdate] {
	p := func(action host.PermissionAction) host.PermissionKey {
		return host.NewPermissionKey("crudtest", action)
	}
	row := Record[testRow]{ID: "42", Revision: "v1", Data: testRow{Name: "Alice", Secret: "private-token", ReadOnly: "protected"}}
	return Definition[testRow, testCreate, testUpdate]{Key: "people", BasePath: "/people", Title: "People", Permissions: Permissions{Read: p(host.PermissionActionRead), Create: p(host.PermissionActionCreate), Update: p(host.PermissionActionUpdate), Delete: p(host.PermissionActionDelete)}, Fields: []Field{{Key: "name", Label: "Name", Type: "text", Read: true, Create: true, Update: true, Required: true}, {Key: "readOnly", Label: "Read only", Type: "text", Read: true}}, Columns: []datagrid.Column{{Key: "name", Label: "Name", Sortable: true, Filterable: true}}, List: func(context.Context, host.Actor, datagrid.Filtered) ([]Record[testRow], int, error) {
		return []Record[testRow]{row}, 1, nil
	}, Get: func(context.Context, host.Actor, string) (Record[testRow], error) { return row, nil }, LoadSelected: func(context.Context, host.Actor, []string) ([]Record[testRow], error) {
		return []Record[testRow]{row}, nil
	}, Create: func(context.Context, host.Actor, testCreate) (Record[testRow], error) { return row, nil }, Update: func(context.Context, host.Actor, string, string, testUpdate) (Record[testRow], error) {
		return row, nil
	}, Delete: func(context.Context, host.Actor, string, string) error { return nil }, Authorize: func(context.Context, host.Actor, Operation, Record[testRow]) error { return nil }}
}

func mounted(t *testing.T, d Definition[testRow, testCreate, testUpdate], preview bool, deny bool, flash ...bool) *fiber.App {
	t.Helper()
	return mountedPolicy(t, d, preview, deny, nil, flash...)
}

func mountedPolicy(t *testing.T, d Definition[testRow, testCreate, testUpdate], preview bool, deny bool, can func(context.Context, int64, host.PermissionDomain, host.PermissionAction) bool, flash ...bool) *fiber.App {
	t.Helper()
	fixture := hosttest.NewApp(t)
	fixture.MockPermissionGuard.EXPECT().AdminGuard(gomock.Any()).DoAndReturn(func(_ host.PermissionKey, _ ...integrationroles.PermissionGuardOption) fiber.Handler {
		return func(c fiber.Ctx) error {
			if deny {
				return fiber.ErrForbidden
			}
			return c.Next()
		}
	}).AnyTimes()
	if can == nil {
		can = func(context.Context, int64, host.PermissionDomain, host.PermissionAction) bool { return true }
	}
	fixture.MockRoleService.EXPECT().AdminCan(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(can).AnyTimes()
	app := fiber.New()
	if len(flash) > 1 && flash[1] {
		fixture.SessionStore.Storage = failedFlashStorage{fixture.SessionStore.Storage}
		app = fiber.New(fiber.Config{ErrorHandler: fixture.InertiaManager.MiddlewareErrorListener()})
	}
	if len(flash) > 0 && flash[0] {
		goinertia.WithSessionStore(coreinertia.SessionBridge{Store: fixture.SessionStore})(fixture.InertiaManager)
	}
	actor, err := hosttest.WithActor(host.Actor{AdminID: 9, SubjectID: "11111111-1111-4111-8111-111111111111"})
	require.NoError(t, err)
	app.Use(actor)
	if preview {
		app.Use(func(c fiber.Ctx) error {
			c.SetContext(integrationroles.WithSuperRolePreview(c.Context()))
			return c.Next()
		})
	}
	(&handler[testRow, testCreate, testUpdate]{app: host.WrapApp(fixture.App), d: cloneDefinition(d)}).RegisterGroupRoutes(app)
	return app
}

func request(t *testing.T, app *fiber.App, method, path, body string) (status int, responseBody string, location string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	require.NoError(t, err)
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(data), res.Header.Get("Location")
}

func TestConstructorValidation(t *testing.T) {
	d := definition()
	_, err := New(d)
	require.NoError(t, err)
	for _, mutate := range []func(*Definition[testRow, testCreate, testUpdate]){func(d *Definition[testRow, testCreate, testUpdate]) { d.Authorize = nil }, func(d *Definition[testRow, testCreate, testUpdate]) { d.Fields[0].Key = "secretUnknown" }, func(d *Definition[testRow, testCreate, testUpdate]) { d.Fields[0].Type = "html" }, func(d *Definition[testRow, testCreate, testUpdate]) { d.BasePath = "/people/:id" }, func(d *Definition[testRow, testCreate, testUpdate]) { d.Columns[0].Key = "secret" }, func(d *Definition[testRow, testCreate, testUpdate]) {
		d.Fields[0].Type = "custom"
		d.Fields[0].Component = "unbuilt.field"
	}} {
		d = definition()
		mutate(&d)
		_, err = New(d)
		require.Error(t, err)
	}
}

func TestListAndSelectedExportProjectionAndActor(t *testing.T) {
	d := definition()
	listCalls := 0
	d.List = func(_ context.Context, a host.Actor, f datagrid.Filtered) ([]Record[testRow], int, error) {
		listCalls++
		require.Equal(t, int64(9), a.AdminID)
		require.Equal(t, 2, f.GetPage())
		require.Equal(t, 1, f.GetLimit())
		return []Record[testRow]{{ID: "42", Revision: "v1", Data: testRow{Name: "Alice", Secret: "private-token"}}}, 8, nil
	}
	d.LoadSelected = func(_ context.Context, a host.Actor, ids []string) ([]Record[testRow], error) {
		require.Equal(t, int64(9), a.AdminID)
		require.Equal(t, []string{"42"}, ids)
		return []Record[testRow]{{ID: "42", Revision: "v1", Data: testRow{Name: "=formula", Secret: "private-token"}}}, nil
	}
	app := mounted(t, d, false, false)
	status, body, _ := request(t, app, "GET", "/people/data?page=2&limit=1", "")
	require.Equal(t, 200, status)
	require.NotContains(t, body, "private-token")
	require.NotContains(t, body, "secret")
	require.Contains(t, body, `"_revision":"v1"`)
	require.Contains(t, body, `"selectionId":"42"`)
	status, body, _ = request(t, app, "GET", "/people/table-export?scope=selected&ids=42&columns=name", "")
	require.Equal(t, 200, status)
	require.Contains(t, body, "'=formula")
	require.NotContains(t, body, "private-token")
	require.Equal(t, 1, listCalls)
	status, _, _ = request(t, app, "GET", "/people/table-export?columns=secret", "")
	require.Equal(t, 400, status)
}

func TestWritesRejectForgedFieldsMissingAndStaleRevision(t *testing.T) {
	d := definition()
	writes := 0
	d.Update = func(_ context.Context, a host.Actor, id, rev string, input testUpdate) (Record[testRow], error) {
		writes++
		require.Equal(t, int64(9), a.AdminID)
		require.Equal(t, "42", id)
		require.Equal(t, "v1", rev)
		require.Equal(t, "Bob", input.Name)
		return Record[testRow]{ID: id, Revision: "v2", Data: testRow{Name: input.Name}}, nil
	}
	app := mounted(t, d, false, false)
	for _, body := range []string{`{"name":"Bob","_revision":"v1","readOnly":"hacked"}`, `{"name":"Bob","_revision":"v1","secret":"hacked"}`, `{"name":"Bob"}`} {
		status, got, _ := request(t, app, "PUT", "/people/records/42", body)
		require.Equal(t, 422, status, got)
	}
	status, _, _ := request(t, app, "PUT", "/people/records/42", `{"name":"Bob","_revision":"stale"}`)
	require.Equal(t, 409, status)
	require.Zero(t, writes)
	status, _, location := request(t, app, "PUT", "/people/records/42", `{"name":"Bob","_revision":"v1"}`)
	require.Equal(t, 303, status)
	require.Equal(t, "/people/records/42/edit", location)
	require.Equal(t, 1, writes)
	status, _, _ = request(t, app, "DELETE", "/people/records/42", `{"_revision":"v1","force":true}`)
	require.Equal(t, 422, status)
}

func TestRecordScopePreviewAndRBAC(t *testing.T) {
	for _, kind := range []string{"scope", "authorize", "preview", "rbac"} {
		t.Run(kind, func(t *testing.T) {
			d := definition()
			calls := 0
			d.Delete = func(context.Context, host.Actor, string, string) error { calls++; return nil }
			if kind == "scope" {
				d.Get = func(context.Context, host.Actor, string) (Record[testRow], error) {
					return Record[testRow]{}, ErrNotFound
				}
			}
			if kind == "authorize" {
				d.Authorize = func(_ context.Context, _ host.Actor, op Operation, _ Record[testRow]) error {
					if op == Delete {
						return ErrDenied
					}
					return nil
				}
			}
			app := mounted(t, d, kind == "preview", kind == "rbac")
			status, _, _ := request(t, app, "DELETE", "/people/records/42", `{"_revision":"v1"}`)
			if kind == "scope" {
				require.Equal(t, 404, status)
			} else {
				require.Equal(t, 403, status)
			}
			require.Zero(t, calls)
		})
	}
}

func TestSanitizedErrorsAndCallbackCAS(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{{errors.New("private database password"), 500}, {ErrConflict, 409}, {ErrDenied, 403}, {ErrNotFound, 404}, {FieldErrors{"name": {"Name is already taken"}, "secret": {"private"}}, 422}} {
		d := definition()
		d.Update = func(context.Context, host.Actor, string, string, testUpdate) (Record[testRow], error) {
			return Record[testRow]{}, tc.err
		}
		app := mounted(t, d, false, false)
		status, body, _ := request(t, app, "PUT", "/people/records/42", `{"name":"Bob","_revision":"v1"}`)
		require.Equal(t, tc.status, status)
		require.NotContains(t, body, "private")
		var decoded any
		require.NoError(t, json.Unmarshal([]byte(body), &decoded))
	}
}

func TestCustomActionRechecksRevisionAndRecordAuthorization(t *testing.T) {
	d := definition()
	calls := 0
	d.Actions = []Action[testRow]{{Key: "publish", Label: "Publish", Permission: host.NewPermissionKey("crudtest", "publish"), Run: func(_ context.Context, a host.Actor, id, revision string) error {
		calls++
		require.Equal(t, int64(9), a.AdminID)
		require.Equal(t, "42", id)
		require.Equal(t, "v1", revision)
		return nil
	}}}
	app := mounted(t, d, false, false)
	status, _, _ := request(t, app, "POST", "/people/records/42/actions/publish", `{"_revision":"stale"}`)
	require.Equal(t, 409, status)
	require.Zero(t, calls)
	status, _, _ = request(t, app, "POST", "/people/records/42/actions/publish", `{"_revision":"v1","params":{}}`)
	require.Equal(t, 422, status)
	require.Zero(t, calls)
	status, _, location := request(t, app, "POST", "/people/records/42/actions/publish", `{"_revision":"v1"}`)
	require.Equal(t, 303, status)
	require.Equal(t, "/people/records/42", location)
	require.Equal(t, 1, calls)
	d.Authorize = func(_ context.Context, _ host.Actor, op Operation, _ Record[testRow]) error {
		if op == Operation("publish") {
			return ErrDenied
		}
		return nil
	}
	status, _, _ = request(t, mounted(t, d, false, false), "POST", "/people/records/42/actions/publish", `{"_revision":"v1"}`)
	require.Equal(t, 403, status)
	require.Equal(t, 1, calls)
}

func TestSelectedExportRejectsUnexpectedCallbackRows(t *testing.T) {
	d := definition()
	d.LoadSelected = func(context.Context, host.Actor, []string) ([]Record[testRow], error) {
		return []Record[testRow]{{ID: "unrequested", Revision: "v1", Data: testRow{Name: "sensitive"}}}, nil
	}
	status, body, _ := request(t, mounted(t, d, false, false), "GET", "/people/table-export?scope=selected&ids=42", "")
	require.Equal(t, 500, status)
	require.NotContains(t, body, "sensitive")
}

func TestCreateAndDeleteRedirects(t *testing.T) {
	d := definition()
	app := mounted(t, d, false, false)
	status, _, location := request(t, app, "POST", "/people", `{"name":"Alice"}`)
	require.Equal(t, 303, status)
	require.Equal(t, "/people/records/42/edit", location)
	status, _, location = request(t, app, "DELETE", "/people/records/42", `{"_revision":"v1"}`)
	require.Equal(t, 303, status)
	require.Equal(t, "/people", location)
}

func TestInertiaMutationErrorsRedirectWithoutExposingRawError(t *testing.T) {
	d := definition()
	d.Update = func(context.Context, host.Actor, string, string, testUpdate) (Record[testRow], error) {
		return Record[testRow]{}, errors.New("private connection details")
	}
	app := mounted(t, d, false, false)
	req := httptest.NewRequestWithContext(t.Context(), "PUT", "/people/records/42", strings.NewReader(`{"name":"retained","_revision":"v1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("Referer", "/people/records/42/edit")
	res, err := app.Test(req)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, 303, res.StatusCode)
	require.NotContains(t, string(body), "private")
}

func TestReturnToIsCarriedOnlyForSameResourceList(t *testing.T) {
	app := mounted(t, definition(), false, false)
	for _, tc := range []struct{ query, want string }{
		{"returnTo=%2Fpeople%3Fpage%3D3", "/people/records/42/edit?returnTo=%2Fpeople%3Fpage%3D3"},
		{"returnTo=https%3A%2F%2Fevil.example%2Fpeople", "/people/records/42/edit"},
		{"returnTo=%2F%2Fevil.example%2Fpeople", "/people/records/42/edit"},
		{"returnTo=%2Fother", "/people/records/42/edit"},
	} {
		status, _, location := request(t, app, "PUT", "/people/records/42?"+tc.query, `{"name":"Alice","_revision":"v1"}`)
		require.Equal(t, 303, status)
		require.Equal(t, tc.want, location)
	}
}

func TestOpaqueIDsRoundTripAcrossHTTPRoutes(t *testing.T) {
	for _, id := range []string{"space id", "percent%id", "slash/id", "Привет", "%2F", "a?b", "a#b", "a+b", `a\b`, "a/../b", "a%252Fb", "a:b@c", "a!b", `a"b`} {
		t.Run(id, func(t *testing.T) {
			d := definition()
			r := Record[testRow]{ID: id, Revision: "v1", Data: testRow{Name: "Alice"}}
			d.Get = func(_ context.Context, _ host.Actor, got string) (Record[testRow], error) {
				require.Equal(t, id, got)
				return r, nil
			}
			d.Update = func(_ context.Context, _ host.Actor, got, revision string, _ testUpdate) (Record[testRow], error) {
				require.Equal(t, id, got)
				require.Equal(t, "v1", revision)
				return r, nil
			}
			d.Delete = func(_ context.Context, _ host.Actor, got, revision string) error {
				require.Equal(t, id, got)
				require.Equal(t, "v1", revision)
				return nil
			}
			d.Actions = []Action[testRow]{{Key: "publish", Label: "Publish", Permission: host.NewPermissionKey("crudtest", "publish"), Run: func(_ context.Context, _ host.Actor, got, _ string) error { require.Equal(t, id, got); return nil }}}
			app := mounted(t, d, false, false)
			path := "/people/records/" + url.PathEscape(id)
			for _, method := range []string{"GET", "PUT", "DELETE"} {
				body := ""
				switch method {
				case "PUT":
					body = `{"name":"Alice","_revision":"v1"}`
				case "DELETE":
					body = `{"_revision":"v1"}`
				}
				status, _, location := request(t, app, method, path, body)
				if method == "GET" {
					require.Equal(t, 200, status)
				} else {
					require.Equal(t, 303, status)
					if method == "PUT" {
						require.Equal(t, path+"/edit", location)
					}
				}
			}
			status, _, location := request(t, app, "POST", path+"/actions/publish", `{"_revision":"v1"}`)
			require.Equal(t, 303, status)
			require.Equal(t, path, location)
		})
	}
}

func previewPage(t *testing.T, app *fiber.App, path string) map[string]any {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), "GET", path, nil)
	req.Header.Set("X-Inertia", "true")
	response, err := app.Test(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	var page map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
	props, ok := page["props"].(map[string]any)
	require.True(t, ok)
	return props
}

func TestPreviewSuppressesMutationMetadata(t *testing.T) {
	d := definition()
	d.Actions = []Action[testRow]{{Key: "publish", Label: "Publish", Permission: host.NewPermissionKey("crudtest", "publish"), Run: func(context.Context, host.Actor, string, string) error { return nil }}}
	app := mounted(t, d, true, false)
	for _, path := range []string{"/people", "/people/records/42"} {
		props := previewPage(t, app, path)
		require.Equal(t, map[string]any{"create": false, "update": false, "delete": false}, props["permissions"])
		resource, ok := props["resource"].(map[string]any)
		require.True(t, ok)
		require.Empty(t, resource["actions"])
		if path != "/people" {
			record, ok := props["record"].(map[string]any)
			require.True(t, ok)
			require.Empty(t, record["actions"])
		}
	}
	status, body, _ := request(t, app, "GET", "/people/data", "")
	require.Equal(t, 200, status)
	var grid map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &grid))
	require.NotContains(t, body, `"key":"edit"`)
	require.NotContains(t, body, `"key":"delete"`)
	require.NotContains(t, body, `"key":"publish"`)
	for _, path := range []string{"/people/create", "/people/records/42/edit"} {
		status, _, _ = request(t, app, "GET", path, "")
		require.Equal(t, 403, status)
	}
}

func TestUnknownOutcomeIsExplicitAndNeverReplayed(t *testing.T) {
	d := definition()
	calls := 0
	d.Update = func(context.Context, host.Actor, string, string, testUpdate) (Record[testRow], error) {
		calls++
		return Record[testRow]{}, errors.Join(ErrUnknown, errors.New("private connection state"))
	}
	app := mounted(t, d, false, false)
	status, body, location := request(t, app, "PUT", "/people/records/42", `{"name":"Alice","_revision":"v1"}`)
	require.Equal(t, 503, status)
	require.Empty(t, location)
	require.Contains(t, body, "Result unknown; check record before retrying")
	require.NotContains(t, body, "private")
	require.Equal(t, 1, calls)
}

func TestRecordIDsMatchSharedSelectedExportTransport(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"a,b", " padded", "padded ", "tab\tid", strings.Repeat("x", 256)} {
		require.False(t, validRecord(Record[testRow]{ID: id, Revision: "1"}), id)
	}
	for _, id := range []string{"a b", "a/b", "100%", "запись"} {
		require.True(t, validRecord(Record[testRow]{ID: id, Revision: "1"}), id)
	}
}

func TestUnknownOutcomeRedirectSurvivesRemovedRecord(t *testing.T) {
	for _, method := range []string{"DELETE", "POST", "PUT"} {
		t.Run(method, func(t *testing.T) {
			d := definition()
			removed := false
			originalGet := d.Get
			d.Get = func(ctx context.Context, a host.Actor, id string) (Record[testRow], error) {
				if removed {
					return Record[testRow]{}, ErrNotFound
				}
				return originalGet(ctx, a, id)
			}
			d.List = func(context.Context, host.Actor, datagrid.Filtered) ([]Record[testRow], int, error) {
				return nil, 0, nil
			}
			write := func(context.Context, host.Actor, string, string) error { removed = true; return ErrUnknown }
			d.Delete = write
			d.Update = func(ctx context.Context, a host.Actor, id, rev string, _ testUpdate) (Record[testRow], error) {
				return Record[testRow]{}, write(ctx, a, id, rev)
			}
			d.Actions = []Action[testRow]{{Key: "publish", Label: "Publish", Permission: host.NewPermissionKey("crudtest", "publish"), Run: write}}
			app := mounted(t, d, false, false, true)
			path := "/people/records/42"
			body := `{"_revision":"v1"}`
			if method == "POST" {
				path += "/actions/publish"
			}
			if method == "PUT" {
				body = `{"name":"Alice","_revision":"v1"}`
			}
			req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Inertia", "true")
			response, err := app.Test(req)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, 303, response.StatusCode)
			require.Equal(t, "/people", response.Header.Get("Location"))
			require.True(t, removed)
			follow := httptest.NewRequestWithContext(t.Context(), "GET", response.Header.Get("Location"), nil)
			follow.Header.Set("X-Inertia", "true")
			follow.Header.Set("X-Inertia-Version", "v1.0")
			for _, cookie := range response.Cookies() {
				follow.AddCookie(cookie)
			}
			page, err := app.Test(follow)
			require.NoError(t, err)
			defer page.Body.Close()
			require.Equal(t, 200, page.StatusCode)
			encoded, err := io.ReadAll(page.Body)
			require.NoError(t, err)
			require.Contains(t, string(encoded), "Result unknown; check record before retrying")
			require.Contains(t, string(encoded), `"form"`)
			require.NotContains(t, string(encoded), "Record action completed")
			require.NotContains(t, string(encoded), "Record updated")
		})
	}
}

func TestExtensionRedirectPersistsConflictAndSuccessWithoutCoreMiddleware(t *testing.T) {
	for _, conflict := range []bool{true, false} {
		t.Run(strconv.FormatBool(conflict), func(t *testing.T) {
			d := definition()
			if conflict {
				d.Update = func(context.Context, host.Actor, string, string, testUpdate) (Record[testRow], error) {
					return Record[testRow]{}, ErrConflict
				}
			}
			app := mounted(t, d, false, false, true)
			req := httptest.NewRequestWithContext(t.Context(), "PUT", "/people/records/42", strings.NewReader(`{"name":"unsaved draft","_revision":"v1"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Inertia", "true")
			response, err := app.Test(req)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, 303, response.StatusCode)
			follow := httptest.NewRequestWithContext(t.Context(), "GET", response.Header.Get("Location"), nil)
			follow.Header.Set("X-Inertia", "true")
			for _, cookie := range response.Cookies() {
				follow.AddCookie(cookie)
			}
			page, err := app.Test(follow)
			require.NoError(t, err)
			defer page.Body.Close()
			require.Equal(t, 200, page.StatusCode)
			body, err := io.ReadAll(page.Body)
			require.NoError(t, err)
			if conflict {
				require.Contains(t, string(body), "Record changed; reload before saving")
				require.Contains(t, string(body), "unsaved draft")
				require.NotContains(t, string(body), "Record updated")
			} else {
				require.Equal(t, 1, strings.Count(string(body), "Record updated"))
			}
		})
	}
}

func TestNumericInvalidInputPersistsSafeErrorRedirect(t *testing.T) {
	app := mounted(t, definition(), false, false, true)
	req := httptest.NewRequestWithContext(t.Context(), "PUT", "/people/records/42", strings.NewReader(`{"name":28,"_revision":"v1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Inertia", "true")
	response, err := app.Test(req)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, 303, response.StatusCode, string(body))
}

func TestNumericBooleanAndNullDraftPersistAcrossConflict(t *testing.T) {
	d := definition()
	d.Fields = append(d.Fields, Field{Key: "stock", Label: "Stock", Type: "number", Update: true}, Field{Key: "active", Label: "Active", Type: "boolean", Update: true}, Field{Key: "nullable", Label: "Nullable", Type: "text", Update: true})
	d.Update = func(_ context.Context, _ host.Actor, _, _ string, input testUpdate) (Record[testRow], error) {
		require.Equal(t, int64(28), input.Stock)
		require.True(t, input.Active)
		require.Nil(t, input.Nullable)
		return Record[testRow]{}, ErrConflict
	}
	app := mounted(t, d, false, false, true)
	req := httptest.NewRequestWithContext(t.Context(), "PUT", "/people/records/42", strings.NewReader(`{"name":"retained","stock":28,"active":true,"nullable":null,"_revision":"v1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Inertia", "true")
	response, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, 303, response.StatusCode)
	follow := httptest.NewRequestWithContext(t.Context(), "GET", response.Header.Get("Location"), nil)
	follow.Header.Set("X-Inertia", "true")
	for _, cookie := range response.Cookies() {
		follow.AddCookie(cookie)
	}
	page, err := app.Test(follow)
	require.NoError(t, err)
	defer page.Body.Close()
	require.Equal(t, 200, page.StatusCode)
	var decoded struct {
		Props struct {
			Errors map[string]string `json:"errors"`
			Old    map[string]any    `json:"old"`
		} `json:"props"`
	}
	require.NoError(t, json.NewDecoder(page.Body).Decode(&decoded))
	require.Equal(t, "Record changed; reload before saving", decoded.Props.Errors["form"])
	require.InDelta(t, float64(28), decoded.Props.Old["stock"], 0)
	require.Equal(t, true, decoded.Props.Old["active"])
	require.Contains(t, decoded.Props.Old, "nullable")
	require.Nil(t, decoded.Props.Old["nullable"])
}

type failedFlashStorage struct{ fiber.Storage }

func (failedFlashStorage) SetWithContext(context.Context, string, []byte, time.Duration) error {
	return errors.New("private storage failure")
}

func (failedFlashStorage) Set(string, []byte, time.Duration) error {
	return errors.New("private storage failure")
}

func TestPersistenceFailureCannotBecomeCoreSuccessRedirect(t *testing.T) {
	app := mounted(t, definition(), false, false, true, true)
	req := httptest.NewRequestWithContext(t.Context(), "PUT", "/people/records/42", strings.NewReader(`{"name":"confirmed write","_revision":"v1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("Referer", "http://localhost/people/records/42/edit")
	response, err := app.Test(req)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, 503, response.StatusCode)
	require.Empty(t, response.Header.Get("Location"))
	require.Contains(t, string(body), "Result unknown; check record before retrying")
	require.NotContains(t, string(body), "private")
}
