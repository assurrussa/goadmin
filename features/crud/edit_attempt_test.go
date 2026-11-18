//nolint:goconst,lll // Literal routes and protocol values document the HTTP contract.
package crud //nolint:testpackage // Exercises resource route registration through the shared HTTP fixture.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

const (
	editAttempt      = "00000000-0000-4000-8000-000000000001"
	otherEditAttempt = "00000000-0000-4000-8000-000000000002"
)

type editAttemptPage struct {
	Component string `json:"component"`
	Props     struct {
		Flash  map[string]string `json:"flash"`
		Errors map[string]string `json:"errors"`
		Old    map[string]any    `json:"old"`
		Values map[string]any    `json:"values"`
		Record struct {
			ID       string `json:"id"`
			Revision string `json:"revision"`
		} `json:"record"`
	} `json:"props"`
}

type editAttemptResponse struct {
	status   int
	location string
	body     []byte
}

func startEditAttemptSession(t *testing.T, app *fiber.App) *http.Cookie {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/start-crud-session", nil)
	res, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	cookies := res.Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

func crudAttemptRequest(t *testing.T, app *fiber.App, cookie *http.Cookie, method, path, id, attempt, body string) editAttemptResponse {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Version", "v1.0")
	req.Header.Set("X-GoAdmin-Record-ID", url.PathEscape(id))
	req.Header.Set("X-GoAdmin-Record-Attempt", attempt)
	if method == http.MethodGet && strings.HasSuffix(strings.SplitN(path, "?", 2)[0], "/edit") {
		req.Header.Set("X-Inertia-Partial-Component", "crud/Form")
		req.Header.Set("X-Inertia-Partial-Data", "errors,flash,old,record,values")
	}
	res, err := app.Test(req)
	require.NoError(t, err)
	defer res.Body.Close()
	encoded, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return editAttemptResponse{status: res.StatusCode, location: res.Header.Get("Location"), body: encoded}
}

func readEditAttemptPage(t *testing.T, res editAttemptResponse) editAttemptPage {
	t.Helper()
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	require.NotContains(t, string(res.body), "private-write-only")
	var page editAttemptPage
	require.NoError(t, json.Unmarshal(res.body, &page))
	return page
}

func TestEditAttemptUsesFreshRecordAndScopedOutcome(t *testing.T) {
	const id = "EU/заказ 50%?"
	for _, name := range []string{"success", "stale", "cas", "validation"} {
		t.Run(name, func(t *testing.T) {
			d := definition()
			d.BasePath = "/nested/people"
			d.Fields = append(d.Fields, Field{Key: "nullable", Label: "Secret", Type: "text", Update: true})
			stored := Record[testRow]{ID: id, Revision: "v1", Data: testRow{Name: "Initial"}}
			if name == "stale" {
				stored.Revision, stored.Data.Name = "v2", "Fresh"
			}
			d.Get = func(_ context.Context, _ host.Actor, gotID string) (Record[testRow], error) {
				result := stored
				result.ID = gotID
				return result, nil
			}
			calls := 0
			d.Update = func(_ context.Context, _ host.Actor, gotID, revision string, input testUpdate) (Record[testRow], error) {
				calls++
				require.Equal(t, id, gotID)
				require.Equal(t, "v1", revision)
				require.NotNil(t, input.Nullable)
				require.Equal(t, "private-write-only", *input.Nullable)
				switch name {
				case "validation":
					return Record[testRow]{}, FieldErrors{"name": {"Already taken"}}
				case "cas":
					stored.Revision, stored.Data.Name = "v2", "Fresh"
					return Record[testRow]{}, ErrConflict
				default:
					stored.Revision, stored.Data.Name = "v2", input.Name
					return stored, nil
				}
			}
			app := mounted(t, d, false, false, true)
			cookie := startEditAttemptSession(t, app)
			path := d.BasePath + "/records/" + url.PathEscape(id)
			write := crudAttemptRequest(t, app, cookie, http.MethodPut, path, id, editAttempt, `{"name":"Draft","nullable":"private-write-only","_revision":"v1"}`)
			require.Equal(t, http.StatusSeeOther, write.status)
			target := write.location
			require.Equal(t, path+"/edit", target)

			for _, wrong := range []struct{ target, attempt string }{
				{target, otherEditAttempt},
				{d.BasePath + "/records/another/edit", editAttempt},
				{path, editAttempt},
			} {
				props := readEditAttemptPage(t, crudAttemptRequest(t, app, cookie, http.MethodGet, wrong.target, id, wrong.attempt, "")).Props
				require.Empty(t, props.Errors)
				require.Empty(t, props.Flash)
				require.Empty(t, props.Old)
			}
			page := readEditAttemptPage(t, crudAttemptRequest(t, app, cookie, http.MethodGet, target, id, editAttempt, ""))
			require.Equal(t, "crud/Form", page.Component)
			require.Equal(t, id, page.Props.Record.ID)
			switch name {
			case "success":
				require.Equal(t, "v2", page.Props.Record.Revision)
				require.Equal(t, "Draft", page.Props.Values["name"])
				require.Equal(t, map[string]string{"recordEditAttempt": editAttempt, "success": "Record updated"}, page.Props.Flash)
				require.Empty(t, page.Props.Errors)
			case "validation":
				require.Equal(t, "v1", page.Props.Record.Revision)
				require.Equal(t, "Initial", page.Props.Values["name"])
				require.Equal(t, map[string]string{"name": "Already taken"}, page.Props.Errors)
				require.Empty(t, page.Props.Flash)
				require.Equal(t, map[string]any{"name": "Draft"}, page.Props.Old)
			default:
				require.Equal(t, "v2", page.Props.Record.Revision)
				require.Equal(t, "Fresh", page.Props.Values["name"])
				require.Equal(t, map[string]string{"form": "Record changed; reload before saving"}, page.Props.Errors)
				require.Equal(t, map[string]string{"recordEditAttempt": editAttempt, "crudEditConflict": "1", "error": "Record changed; reload before saving"}, page.Props.Flash)
				require.Equal(t, map[string]any{"name": "Draft"}, page.Props.Old)
			}
			if name == "stale" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			consumed := readEditAttemptPage(t, crudAttemptRequest(t, app, cookie, http.MethodGet, target, id, editAttempt, ""))
			require.Empty(t, consumed.Props.Flash)
			require.Empty(t, consumed.Props.Errors)
			require.Empty(t, consumed.Props.Old)
		})
	}
}

func TestEditAttemptFallbackNeverCertifiesAnEdit(t *testing.T) {
	for _, name := range []string{"unknown", "readonly", "list", "denied"} {
		t.Run(name, func(t *testing.T) {
			d := definition()
			stored := Record[testRow]{ID: "42", Revision: "v1", Data: testRow{Name: "Initial"}}
			d.Get = func(context.Context, host.Actor, string) (Record[testRow], error) { return stored, nil }
			d.List = func(context.Context, host.Actor, datagrid.Filtered) ([]Record[testRow], int, error) {
				return nil, 0, nil
			}
			d.Authorize = func(_ context.Context, _ host.Actor, op Operation, r Record[testRow]) error {
				if name == "denied" && op == Update {
					return ErrDenied
				}
				if r.Revision == "v2" && (op == Update || name == "list" && op == Read) {
					return ErrDenied
				}
				return nil
			}
			d.Update = func(context.Context, host.Actor, string, string, testUpdate) (Record[testRow], error) {
				if name == "unknown" {
					return Record[testRow]{}, ErrUnknown
				}
				stored.Revision = "v2"
				return stored, nil
			}
			app := mounted(t, d, false, false, true)
			cookie := startEditAttemptSession(t, app)
			requestPath := "/people/records/42?returnTo=%2Fpeople%3Fpage%3D2"
			write := crudAttemptRequest(t, app, cookie, http.MethodPut, requestPath, "42", editAttempt, `{"name":"Draft","_revision":"v1"}`)
			require.Equal(t, http.StatusSeeOther, write.status)
			target := write.location
			if name == "readonly" {
				require.Equal(t, requestPath, target)
			} else {
				require.Equal(t, "/people?page=2", target)
				wrong := readEditAttemptPage(t, crudAttemptRequest(t, app, cookie, http.MethodGet, target, "55", editAttempt, ""))
				require.Empty(t, wrong.Props.Flash)
				require.Empty(t, wrong.Props.Errors)
			}
			page := readEditAttemptPage(t, crudAttemptRequest(t, app, cookie, http.MethodGet, target, "42", editAttempt, ""))
			require.NotContains(t, page.Props.Flash, "recordEditAttempt")
			require.NotContains(t, page.Props.Flash, "crudEditConflict")
			require.Empty(t, page.Props.Old)
			switch name {
			case "unknown":
				require.Equal(t, "Result unknown; check record before retrying", page.Props.Errors["form"])
				require.Empty(t, page.Props.Flash)
			case "denied":
				require.Equal(t, "Record access denied", page.Props.Errors["form"])
				require.Empty(t, page.Props.Flash)
			default:
				require.Empty(t, page.Props.Errors)
				require.Equal(t, "Record updated", page.Props.Flash["success"])
			}
		})
	}
}

func TestEditValuesIncludeOmittedTypedScalars(t *testing.T) {
	d := definition()
	d.Fields = append(d.Fields,
		Field{Key: "stock", Label: "Stock", Type: "number", Read: true, Update: true},
		Field{Key: "active", Label: "Active", Type: "boolean", Read: true, Update: true},
		Field{Key: "count", Label: "Count", Type: "number", Read: true, Update: true},
		Field{Key: "note", Label: "Note", Type: "text", Read: true, Update: true},
	)
	app := mounted(t, d, false, false, true)
	cookie := startEditAttemptSession(t, app)
	edit := readEditAttemptPage(t, crudAttemptRequest(t, app, cookie, http.MethodGet, "/people/records/42/edit", "", "", ""))
	require.Equal(t, map[string]any{
		"name": "Alice", "readOnly": "protected", "stock": float64(0), "active": false, "count": nil, "note": "",
	}, edit.Props.Values)
	show := readEditAttemptPage(t, crudAttemptRequest(t, app, cookie, http.MethodGet, "/people/records/42", "", "", ""))
	require.Equal(t, map[string]any{"name": "Alice", "readOnly": "protected"}, show.Props.Values)
	status, body, _ := request(t, app, http.MethodGet, "/people/data", "")
	require.Equal(t, http.StatusOK, status, body)
	require.NotContains(t, body, `"count":null`)
	require.NotContains(t, body, `"stock":0`)
	require.NotContains(t, body, `"active":false`)
	require.NotContains(t, body, `"note":""`)
}

type customEditRow struct {
	Stock int64  `json:"stock,omitempty"`
	Note  string `json:"note,omitempty"`
}

func (customEditRow) MarshalJSON() ([]byte, error) {
	return []byte(`{"stock":8}`), nil
}

func TestEditValuesRespectCustomRowOmissions(t *testing.T) {
	h := handler[customEditRow, testCreate, testUpdate]{
		d: Definition[customEditRow, testCreate, testUpdate]{
			Fields: []Field{
				{Key: "stock", Type: "number", Read: true, Update: true},
				{Key: "note", Type: "text", Read: true, Update: true},
			},
		},
	}
	out, err := h.project(Record[customEditRow]{ID: "42", Revision: "v1", Data: customEditRow{Note: "suppressed"}}, Update)
	require.NoError(t, err)
	wire, err := json.Marshal(out)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"42","_revision":"v1","stock":8}`, string(wire))
}
