//nolint:goconst,lll // Contract fixtures retain explicit wire values for review.
package crud //nolint:testpackage // Constructor and actual HTTP checks share private route setup.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/hosttest"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type scalarDTO[T any] struct {
	Value T `json:"value"`
}

func scalarDefinition[T any](kind string) Definition[scalarDTO[T], scalarDTO[T], scalarDTO[T]] {
	permission := func(action host.PermissionAction) host.PermissionKey { return host.NewPermissionKey("scalars", action) }
	var value T
	r := Record[scalarDTO[T]]{ID: "42", Revision: "v1", Data: scalarDTO[T]{Value: value}}
	return Definition[scalarDTO[T], scalarDTO[T], scalarDTO[T]]{
		Key: "scalars", BasePath: "/scalars", Title: "Scalars", Permissions: Permissions{Read: permission("read"), Create: permission("create"), Update: permission("update"), Delete: permission("delete")},
		Fields:             []Field{{Key: "value", Label: "Value", Type: kind, Read: true, Create: true, Update: true, Options: []Option{{Value: "known", Label: "Known"}}, Component: "host.fields/value"}},
		ClientRequirements: []host.ClientExtensionRequirement{{Key: "host.fields", APIVersion: 1, Fingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		Columns:            []datagrid.Column{{Key: "value", Label: "Value", Sortable: true}},
		List: func(context.Context, host.Actor, datagrid.Filtered) ([]Record[scalarDTO[T]], int, error) {
			return []Record[scalarDTO[T]]{r}, 1, nil
		},
		Get: func(context.Context, host.Actor, string) (Record[scalarDTO[T]], error) { return r, nil },
		LoadSelected: func(context.Context, host.Actor, []string) ([]Record[scalarDTO[T]], error) {
			return []Record[scalarDTO[T]]{r}, nil
		},
		Create: func(_ context.Context, _ host.Actor, input scalarDTO[T]) (Record[scalarDTO[T]], error) {
			r.Data = input
			return r, nil
		},
		Update: func(_ context.Context, _ host.Actor, _, _ string, input scalarDTO[T]) (Record[scalarDTO[T]], error) {
			r.Data = input
			return r, nil
		},
		Delete:    func(context.Context, host.Actor, string, string) error { return nil },
		Authorize: func(context.Context, host.Actor, Operation, Record[scalarDTO[T]]) error { return nil },
	}
}

func mountScalar[T any](t *testing.T, d Definition[scalarDTO[T], scalarDTO[T], scalarDTO[T]]) *fiber.App {
	t.Helper()
	_, err := New(d)
	require.NoError(t, err)
	fixture := hosttest.NewApp(t)
	fixture.MockPermissionGuard.EXPECT().AdminGuard(gomock.Any()).Return(func(c fiber.Ctx) error { return c.Next() }).AnyTimes()
	fixture.MockRoleService.EXPECT().AdminCan(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
	app := fiber.New()
	actor, err := hosttest.WithActor(host.Actor{AdminID: 9, SubjectID: "11111111-1111-4111-8111-111111111111"})
	require.NoError(t, err)
	app.Use(actor)
	(&handler[scalarDTO[T], scalarDTO[T], scalarDTO[T]]{app: host.WrapApp(fixture.App), d: cloneDefinition(d)}).RegisterGroupRoutes(app)
	return app
}

type (
	namedText    string
	namedNumber  int64
	namedBoolean bool
)

func TestConstructorEnforcesFlatDeclaredGoKinds(t *testing.T) {
	for _, check := range []struct {
		name string
		run  func() error
	}{
		{"string number", func() error { _, err := New(scalarDefinition[string]("number")); return err }},
		{"integer text", func() error { _, err := New(scalarDefinition[int]("text")); return err }},
		{"boolean select", func() error { _, err := New(scalarDefinition[bool]("select")); return err }},
		{"custom map", func() error { _, err := New(scalarDefinition[map[string]string]("custom")); return err }},
		{"custom slice", func() error { _, err := New(scalarDefinition[[]string]("custom")); return err }},
		{"custom raw JSON", func() error { _, err := New(scalarDefinition[json.RawMessage]("custom")); return err }},
		{"date object", func() error { _, err := New(scalarDefinition[time.Time]("date")); return err }},
		{"complex number", func() error { _, err := New(scalarDefinition[complex64]("number")); return err }},
	} {
		t.Run(check.name, func(t *testing.T) { require.Error(t, check.run()) })
	}
	_, err := New(scalarDefinition[*namedText]("text"))
	require.NoError(t, err)
	_, err = New(scalarDefinition[namedNumber]("number"))
	require.NoError(t, err)
	_, err = New(scalarDefinition[*namedBoolean]("boolean"))
	require.NoError(t, err)
	_, err = New(scalarDefinition[*float32]("custom"))
	require.NoError(t, err)
	// The checks apply to create and update DTOs, not just the readable row.
	bad := scalarDefinition[string]("text")
	bad.Fields[0].Type = "number"
	bad.Fields[0].Read = false
	_, err = New(bad)
	require.Error(t, err)
}

func scalarRequest[T any](t *testing.T, kind, raw string, want int) {
	t.Helper()
	app := mountScalar(t, scalarDefinition[T](kind))
	status, body, _ := request(t, app, "PUT", "/scalars/records/42", `{"value":`+raw+`,"_revision":"v1"}`)
	require.Equal(t, want, status, body)
}

func TestScalarInputShapesDatesAndSafeNumericRange(t *testing.T) {
	for _, raw := range []string{`{}`, `[]`, `28`, `true`, `null`} {
		scalarRequest[string](t, "text", raw, 422)
	}
	for _, raw := range []string{`"yes"`, `1`, `{}`, `null`} {
		scalarRequest[bool](t, "boolean", raw, 422)
	}
	scalarRequest[bool](t, "boolean", `false`, 303)
	scalarRequest[*bool](t, "boolean", `null`, 303)
	scalarRequest[string](t, "date", `"2024-02-29"`, 303)
	scalarRequest[string](t, "date", `""`, 303)
	for _, raw := range []string{`"0000-01-01"`, `"2026-02-29"`, `"2026-11-31"`, `"2026-1-01"`, `true`} {
		scalarRequest[string](t, "date", raw, 422)
	}
	for _, raw := range []string{`9007199254740992`, `-9007199254740992`, `9007199254740991.1`, `1e309`, `1e-1000000000`, `"123"`, `true`, `{}`, `null`} {
		scalarRequest[float64](t, "number", raw, 422)
	}
	scalarRequest[int64](t, "number", `9007199254740991`, 303)
	scalarRequest[uint64](t, "number", `9007199254740992`, 422)
	scalarRequest[namedNumber](t, "number", `17`, 303)
	scalarRequest[float64](t, "number", `12.5`, 303)
	scalarRequest[float64](t, "number", `0e-1000000000`, 303)
	scalarRequest[*namedNumber](t, "custom", `null`, 303)
	scalarRequest[namedNumber](t, "custom", `[]`, 422)
}

func TestOptionalSelectBlankAndExplicitNullableValues(t *testing.T) {
	scalarRequest[string](t, "select", `""`, 303)
	scalarRequest[string](t, "select", `"known"`, 303)
	scalarRequest[string](t, "select", `"unknown"`, 422)
	scalarRequest[*string](t, "select", `null`, 303)
	scalarRequest[string](t, "select", `null`, 422)
	d := scalarDefinition[*string]("select")
	d.Fields[0].Required = true
	app := mountScalar(t, d)
	for _, raw := range []string{`null`, `""`} {
		status, _, _ := request(t, app, "PUT", "/scalars/records/42", `{"value":`+raw+`,"_revision":"v1"}`)
		require.Equal(t, 422, status)
	}
	d = scalarDefinition[*string]("text")
	calls := 0
	d.Update = func(_ context.Context, _ host.Actor, _, _ string, input scalarDTO[*string]) (Record[scalarDTO[*string]], error) {
		calls++
		require.Nil(t, input.Value)
		return Record[scalarDTO[*string]]{ID: "42", Revision: "v2", Data: input}, nil
	}
	status, _, _ := request(t, mountScalar(t, d), "PUT", "/scalars/records/42", `{"value":null,"_revision":"v1"}`)
	require.Equal(t, 303, status)
	require.Equal(t, 1, calls)
}

func TestReadProjectionRejectsUnsafeScalarNumbers(t *testing.T) {
	d := scalarDefinition[float64]("number")
	d.List = func(context.Context, host.Actor, datagrid.Filtered) ([]Record[scalarDTO[float64]], int, error) {
		return []Record[scalarDTO[float64]]{{ID: "42", Revision: "v1", Data: scalarDTO[float64]{Value: math.MaxFloat64}}}, 1, nil
	}
	status, body, _ := request(t, mountScalar(t, d), "GET", "/scalars/data", "")
	require.Equal(t, 500, status)
	require.NotContains(t, body, fmt.Sprint(math.MaxFloat64))
}

func TestCreateMetadataUsesCurrentRBACAndRecordGate(t *testing.T) {
	d := definition()
	allowed := true
	d.Authorize = func(_ context.Context, _ host.Actor, op Operation, _ Record[testRow]) error {
		if op == Create && !allowed {
			return ErrDenied
		}
		return nil
	}
	app := mounted(t, d, false, false)
	read := func(app *fiber.App, want bool) {
		status, body, _ := request(t, app, "GET", "/people/data", "")
		require.Equal(t, 200, status)
		require.Contains(t, body, fmt.Sprintf(`"creatable":%t`, want))
	}
	read(app, true)
	allowed = false
	read(app, false)
	allowed = true
	read(app, true)
	read(mountedPolicy(t, definition(), false, false, func(_ context.Context, _ int64, _ host.PermissionDomain, action host.PermissionAction) bool {
		return action != host.PermissionActionCreate
	}), false)
	read(mounted(t, definition(), true, false), false)
}

func TestDeleteAndUnknownListDestinationsRetainOnlySafeListQuery(t *testing.T) {
	for _, tc := range []struct{ returnTo, want string }{{"/people?page=3&search=Alice", "/people?page=3&search=Alice"}, {"https://evil.example/people?page=3", "/people"}, {"//evil.example/people?page=3", "/people"}, {"/other?page=3", "/people"}, {"/people?search=%zz", "/people"}} {
		for _, unknown := range []bool{false, true} {
			d := definition()
			if unknown {
				d.Delete = func(context.Context, host.Actor, string, string) error { return ErrUnknown }
			}
			app := mounted(t, d, false, false, true)
			path := "/people/records/42?returnTo=" + url.QueryEscape(tc.returnTo)
			if !unknown {
				status, _, location := request(t, app, "DELETE", path, `{"_revision":"v1"}`)
				require.Equal(t, 303, status)
				require.Equal(t, tc.want, location)
			} else {
				response := inertiaMutation(t, app, "DELETE", path, `{"_revision":"v1"}`)
				require.Equal(t, 303, response.StatusCode)
				require.Equal(t, tc.want, response.Header.Get("Location"))
				require.NoError(t, response.Body.Close())
			}
		}
	}
}

func inertiaMutation(t *testing.T, app *fiber.App, method, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Inertia", "true")
	response, err := app.Test(req)
	require.NoError(t, err)
	return response
}

func TestDataEndpointsRetainTypedResourceOutcomes(t *testing.T) {
	for _, tc := range []struct {
		err     error
		status  int
		message string
	}{{ErrDenied, 403, "Record access denied"}, {ErrNotFound, 404, "Record not found"}, {ErrConflict, 409, "Record changed; reload before saving"}, {ErrUnknown, 503, "Result unknown; check record before retrying"}, {errors.New("private database failure"), 500, "Unable to complete resource request"}} {
		for _, method := range []string{"GET", "POST"} {
			d := definition()
			d.List = func(context.Context, host.Actor, datagrid.Filtered) ([]Record[testRow], int, error) {
				return nil, 0, errors.Join(tc.err, errors.New("private detail"))
			}
			status, body, location := request(t, mounted(t, d, false, false), method, "/people/data", `{}`)
			require.Equal(t, tc.status, status)
			require.Contains(t, body, tc.message)
			require.NotContains(t, body, "private")
			require.Empty(t, location)
		}
	}
	d := definition()
	d.Authorize = func(_ context.Context, _ host.Actor, op Operation, _ Record[testRow]) error {
		if op == Read {
			return ErrDenied
		}
		return nil
	}
	status, _, _ := request(t, mounted(t, d, false, false), "GET", "/people/data", "")
	require.Equal(t, 403, status)
}

func TestTwoResourcesShareRequiredCustomClientExtension(t *testing.T) {
	one := scalarDefinition[string]("custom")
	two := scalarDefinition[string]("custom")
	two.Key = "second"
	two.BasePath = "/second"
	_, err := New(one)
	require.NoError(t, err)
	_, err = New(two)
	require.NoError(t, err)
	registry := host.NewRegistry(feature[scalarDTO[string], scalarDTO[string], scalarDTO[string]]{d: one}, feature[scalarDTO[string], scalarDTO[string], scalarDTO[string]]{d: two})
	requirements, err := registry.ClientExtensionRequirements()
	require.NoError(t, err)
	require.Len(t, requirements, 2)
}

func TestConstructorChecksCreateAndUpdateKindsIndependently(t *testing.T) {
	base := scalarDefinition[string]("text")
	wrongCreate := Definition[scalarDTO[string], scalarDTO[int], scalarDTO[string]]{
		Key: base.Key, BasePath: base.BasePath, Title: base.Title, Permissions: base.Permissions, Fields: base.Fields, Columns: base.Columns,
		List: base.List, Get: base.Get, LoadSelected: base.LoadSelected, Update: base.Update, Delete: base.Delete, Authorize: base.Authorize,
		Create: func(context.Context, host.Actor, scalarDTO[int]) (Record[scalarDTO[string]], error) {
			return Record[scalarDTO[string]]{}, nil
		},
	}
	_, err := New(wrongCreate)
	require.ErrorContains(t, err, "field kind")
	wrongUpdate := Definition[scalarDTO[string], scalarDTO[string], scalarDTO[bool]]{
		Key: base.Key, BasePath: base.BasePath, Title: base.Title, Permissions: base.Permissions, Fields: base.Fields, Columns: base.Columns,
		List: base.List, Get: base.Get, LoadSelected: base.LoadSelected, Create: base.Create, Delete: base.Delete, Authorize: base.Authorize,
		Update: func(context.Context, host.Actor, string, string, scalarDTO[bool]) (Record[scalarDTO[string]], error) {
			return Record[scalarDTO[string]]{}, nil
		},
	}
	_, err = New(wrongUpdate)
	require.ErrorContains(t, err, "field kind")
}
