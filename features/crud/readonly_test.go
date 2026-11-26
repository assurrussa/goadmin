//nolint:goconst // The SDK, HTTP and in-process fixtures share literal contract examples.
package crud //nolint:testpackage // Exercise the same internal reader as the HTTP adapter.

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/hosttest"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type eventRow struct {
	Label   string `json:"label"`
	Owner   int64  `json:"owner"`
	Private string `json:"private"`
}

func exampleEvents() ReadOnlyDefinition[eventRow] {
	event := func(a host.Actor) ReadOnlyRecord[eventRow] {
		return ReadOnlyRecord[eventRow]{
			ID:   strconv.FormatInt(a.AdminID, 10),
			Data: eventRow{Label: "Created", Owner: a.AdminID, Private: "private-value"},
		}
	}
	return ReadOnlyDefinition[eventRow]{
		Key: "events", BasePath: "/event-history", Title: "Events", Permission: host.NewPermissionKey("event_history", "read"),
		Fields:  []Field{{Key: "label", Label: "Action", Type: "text", Read: true}, {Key: "private", Label: "Private", Type: "text"}},
		Columns: []datagrid.Column{{Key: "label", Label: "Action", Type: "text", Sortable: true, Filterable: true}},
		List: func(_ context.Context, a host.Actor, _ datagrid.Filtered) ([]ReadOnlyRecord[eventRow], int, error) {
			return []ReadOnlyRecord[eventRow]{event(a)}, 1, nil
		},
		Get: func(_ context.Context, a host.Actor, id string) (ReadOnlyRecord[eventRow], error) {
			row := event(a)
			if row.ID != id {
				return ReadOnlyRecord[eventRow]{}, ErrNotFound
			}
			return row, nil
		},
		Authorize: func(_ context.Context, a host.Actor, r ReadOnlyRecord[eventRow]) error {
			if a.AdminID != r.Data.Owner {
				return ErrDenied
			}
			return nil
		},
	}
}

func TestReadOnlyConstructor(t *testing.T) {
	d := exampleEvents()
	_, err := NewReadOnly(d)
	require.NoError(t, err)
	for _, flag := range []Field{
		{Key: "label", Create: true}, {Key: "label", Update: true}, {Key: "label", Required: true},
	} {
		d = exampleEvents()
		d.Fields[0] = flag
		_, err = NewReadOnly(d)
		require.ErrorContains(t, err, "read-only field \"label\"")
	}
	d = exampleEvents()
	d.List = nil
	_, err = NewReadOnly(d)
	require.Error(t, err)
	d = exampleEvents()
	d.Get = nil
	_, err = NewReadOnly(d)
	require.Error(t, err)
	d = exampleEvents()
	d.Authorize = nil
	_, err = NewReadOnly(d)
	require.Error(t, err)
	d = exampleEvents()
	d.Permission = host.PermissionKey{}
	_, err = NewReadOnly(d)
	require.Error(t, err)
}

func TestReadOnlyReader(t *testing.T) {
	d := exampleEvents()
	calls := 0
	list := d.List
	d.List = func(ctx context.Context, a host.Actor, q datagrid.Filtered) ([]ReadOnlyRecord[eventRow], int, error) {
		calls++
		require.NotImplements(t, (*fiber.Ctx)(nil), ctx)
		return list(ctx, a, q)
	}
	def := cloneDefinition(d.definition())
	s := reader[eventRow, struct{}, struct{}]{
		d: &def, mode: readOnlyResource,
		can: func(_ context.Context, id int64, domain host.PermissionDomain, action host.PermissionAction) bool {
			return (id == 9 || id == 10) && host.NewPermissionKey(domain, action) == d.Permission
		},
	}
	a := host.Actor{AdminID: 9, SubjectID: "user-a"}
	b := host.Actor{AdminID: 10, SubjectID: "user-b"}
	ctx := t.Context()
	description, err := s.describe(ctx, a)
	require.NoError(t, err)
	require.Equal(t, 100, description.MaxPageSize)
	require.Equal(t, d.Fields[:1], description.Fields)
	description.Columns[0].Key = "mutated"
	require.Equal(t, "label", s.d.Columns[0].Key)
	for _, actor := range []host.Actor{a, b} {
		page, e := s.list(ctx, actor, datagrid.Filters{Limit: 100})
		require.NoError(t, e)
		require.Len(t, page.Rows, 1)
		require.Equal(t, 1, page.Total)
		r := page.Rows[0]
		require.Equal(t, strconv.FormatInt(actor.AdminID, 10), r.ID)
		require.Equal(t, map[string]any{"label": "Created"}, r.Data)
		require.Empty(t, r.Revision)
		require.Empty(t, r.Actions)
		got, e := s.get(ctx, actor, r.ID)
		require.NoError(t, e)
		require.Equal(t, r, got)
	}
	_, err = s.get(ctx, b, "9")
	require.ErrorIs(t, err, ErrNotFound)
	for _, denied := range []host.Actor{{}, {AdminID: 11, SubjectID: "user-c"}} {
		_, err = s.describe(ctx, denied)
		require.ErrorIs(t, err, ErrDenied)
		_, err = s.get(ctx, denied, "9")
		require.ErrorIs(t, err, ErrDenied)
		page, e := s.list(ctx, denied, datagrid.Filters{})
		require.ErrorIs(t, e, ErrDenied)
		require.Zero(t, page)
	}
	preview := integrationroles.WithRolePreview(ctx, map[host.PermissionKey]bool{})
	_, err = s.describe(preview, a)
	require.ErrorIs(t, err, ErrDenied)
	for _, q := range []datagrid.Filters{
		{Limit: 101},
		{Limit: 100000},
		{Page: -1},
		{SortBy: "private"},
		{SortBy: "id"},
		{SortOrder: "ASC"},
		{Fields: map[string]any{"private": "secret"}},
		{Fields: map[string]any{"label": []string{"a"}}},
		{Fields: map[string]any{"label": float64(12)}},
	} {
		page, e := s.list(ctx, a, q)
		require.ErrorIs(t, e, ErrInvalidQuery)
		require.Zero(t, page)
	}
	require.Equal(t, 2, calls)
	// An incorrectly scoped host callback yields an authorized row followed by
	// a foreign one. Neither the partial rows nor the untrusted count escape.
	baseList := s.d.List
	s.d.List = func(ctx context.Context, a host.Actor, q datagrid.Filtered) ([]Record[eventRow], int, error) {
		rows, _, e := baseList(ctx, a, q)
		return append(rows, Record[eventRow]{ID: "10", Data: eventRow{Owner: 10}}), 999, e
	}
	page, err := s.list(ctx, a, datagrid.Filters{})
	require.ErrorIs(t, err, ErrDenied)
	require.Zero(t, page)
}

func TestReadOnlyHTTP(t *testing.T) {
	d := exampleEvents()
	var wrapped *host.App
	type localKey struct{}
	list := d.List
	lastLimit := 0
	d.List = func(ctx context.Context, a host.Actor, q datagrid.Filtered) ([]ReadOnlyRecord[eventRow], int, error) {
		require.NotImplements(t, (*fiber.Ctx)(nil), ctx)
		require.Equal(t, "from-middleware", ctx.Value(localKey{}))
		current, ok := wrapped.CurrentActor(ctx)
		require.True(t, ok)
		require.Equal(t, a, current)
		lastLimit = q.GetLimit()
		return list(ctx, a, q)
	}
	f := hosttest.NewApp(t)
	f.MockPermissionGuard.EXPECT().AdminGuard(d.Permission).DoAndReturn(func(
		_ host.PermissionKey, _ ...integrationroles.PermissionGuardOption,
	) fiber.Handler {
		return func(c fiber.Ctx) error { return c.Next() }
	}).AnyTimes()
	f.MockRoleService.EXPECT().AdminCan(gomock.Any(), int64(9), d.Permission.Domain, d.Permission.Action).Return(true).AnyTimes()
	app := fiber.New()
	auth, err := hosttest.WithActor(host.Actor{AdminID: 9, SubjectID: "11111111-1111-4111-8111-111111111111"})
	require.NoError(t, err)
	app.Use(auth)
	app.Use(func(c fiber.Ctx) error {
		c.Locals(localKey{}, "from-middleware")
		c.SetContext(integrationroles.WithRolePreview(c.Context(), map[host.PermissionKey]bool{d.Permission: true}))
		return c.Next()
	})
	wrapped = host.WrapApp(f.App)
	h := handler[eventRow, struct{}, struct{}]{app: wrapped, d: cloneDefinition(d.definition()), mode: readOnlyResource}
	h.RegisterGroupRoutes(app)
	for _, path := range []string{"/event-history", "/event-history/records/9"} {
		props := previewPage(t, app, path)
		require.Equal(t, map[string]any{"create": false, "update": false, "delete": false}, props["permissions"])
		wire, e := json.Marshal(props)
		require.NoError(t, e)
		require.NotContains(t, string(wire), "private-value")
		require.NotContains(t, string(wire), "\"private\"")
		require.NotContains(t, string(wire), "\"revision\"")
		if rec, ok := props["record"].(map[string]any); ok {
			require.Equal(t, "9", rec["id"])
			require.Empty(t, rec["actions"])
		}
	}
	status, body, _ := request(t, app, http.MethodGet, "/event-history/data?limit=1000", "")
	require.Equal(t, 200, status, body)
	require.Equal(t, 25, lastLimit)
	require.Contains(t, body, "\"key\":\"view\"")
	require.Contains(t, body, "\"selectionId\":\"9\"")
	require.Contains(t, body, "/table-preferences")
	require.NotContains(t, body, "/table-export")
	require.NotContains(t, body, "\"_revision\"")
	status, body, _ = request(t, app, http.MethodPost, "/event-history/data", `{"limit":100}`)
	require.Equal(t, 200, status, body)
	require.Equal(t, 100, lastLimit)
	lastLimit = 0
	status, _, _ = request(t, app, http.MethodPost, "/event-history/data", `{"limit":101}`)
	require.Equal(t, 400, status)
	require.Zero(t, lastLimit)
	for _, route := range []struct{ method, path string }{
		{"GET", "/create"},
		{"GET", "/table-export"},
		{"POST", ""},
		{"GET", "/records/9/edit"},
		{"PUT", "/records/9"},
		{"DELETE", "/records/9"},
		{"POST", "/records/9/actions/publish"},
	} {
		status, _, _ = request(t, app, route.method, "/event-history"+route.path, "")
		require.Contains(t, []int{404, 405}, status, route)
	}
}

func TestEditableCRUDRetainsPOSTPageLimit(t *testing.T) {
	d := definition()
	d.List = func(_ context.Context, _ host.Actor, q datagrid.Filtered) ([]Record[testRow], int, error) {
		require.Equal(t, 1000, q.GetLimit())
		return nil, 0, nil
	}
	app := mounted(t, d, false, false)
	status, body, _ := request(t, app, http.MethodPost, "/people/data", `{"limit":1000}`)
	require.Equal(t, http.StatusOK, status, body)
}
