package mcp //nolint:testpackage // Exercise the unexported MCP transport and grant state.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/hosttest"
	roles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func TestScopedReadsUseCurrentRoleAndProjection(t *testing.T) {
	const origin = "https://admin.example.test"
	resource := testResource()
	fixture := hosttest.NewApp(t)
	owner := host.Actor{AdminID: 9, SubjectID: "operator"}
	other := host.Actor{AdminID: 10, SubjectID: "unrelated"}
	type scopeKey struct{}
	policy := context.WithValue(t.Context(), scopeKey{}, "resolved")

	var listCalls, getCalls int
	row := Record{ID: "12", Data: map[string]any{
		testGroupField: "9", testTitleField: "Locked reading", "notes": "Operator notes", "internal": "private value",
	}}
	reader := ReadFuncs{
		ListFunc: func(ctx context.Context, actor host.Actor, filters datagrid.Filters) ([]Record, int, error) {
			listCalls++
			if actor != owner || ctx.Value(scopeKey{}) != "resolved" || filters.Fields[testGroupField] != "9" ||
				filters.Search != "today" || filters.Page != 2 || filters.Limit != 3 {
				t.Errorf("reader received incorrect authority or filters: actor=%v, filters=%v", actor, filters)
			}
			return []Record{row}, 4, nil
		},
		GetFunc: func(ctx context.Context, actor host.Actor, id string) (Record, error) {
			getCalls++
			if actor != owner || ctx.Value(scopeKey{}) != "resolved" {
				t.Errorf("reader did not receive the resolved request scope for actor %v", actor)
			}
			if id != row.ID {
				return Record{}, crud.ErrNotFound
			}
			return row, nil
		},
	}
	h := newHandler(host.WrapApp(fixture.App), Config{Origin: origin}, resource, reader)
	acting := owner
	var sessionErr error
	h.resolveRead = func(context.Context, host.ReadSessionBinding) (context.Context, host.Actor, error) {
		return policy, acting, sessionErr
	}
	permissionAllowed := true
	fixture.MockRoleService.EXPECT().AdminCan(gomock.Any(), owner.AdminID, resource.Permission.Domain, resource.Permission.Action).
		DoAndReturn(func(context.Context, int64, host.PermissionDomain, host.PermissionAction) bool {
			return permissionAllowed
		}).AnyTimes()

	app := fiber.New()
	h.registerDevice(app)
	code, secret, ok := h.grants.start(netip.MustParseAddr("127.0.0.1"))
	if !ok || !h.grants.decide(code, true, owner, host.ReadSessionBinding{}) {
		t.Fatal("could not approve fixture grant")
	}
	token, _, status := h.grants.exchange(secret)
	if status != http.StatusOK {
		t.Fatal("could not exchange fixture grant")
	}

	call := func(route, body string, want int) string {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, origin+testResourcePath+route, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s returned %d, want %d: %s", route, resp.StatusCode, want, raw)
		}
		if strings.Contains(string(raw), "private value") {
			t.Fatalf("%s exposed an undeclared field", route)
		}
		return string(raw)
	}
	const query = `{"group":"9","query":"today","page":2,"page_size":3}`
	const id = `{"reading_id":"12"}`
	call("/search", `{"group":"9","actor":"other"}`, http.StatusUnprocessableEntity)
	call("/get", `{"reading_id":"9223372036854775808"}`, http.StatusUnprocessableEntity)
	if raw := call("/search", query, http.StatusOK); strings.Contains(raw, "Operator notes") || !strings.Contains(raw, `"total":4`) {
		t.Fatalf("unexpected search projection: %s", raw)
	}
	if raw := call("/get", id, http.StatusOK); !strings.Contains(raw, `"notes":"Operator notes"`) {
		t.Fatalf("unexpected detail projection: %s", raw)
	}
	call("/get", `{"reading_id":"13"}`, http.StatusNotFound)
	permissionAllowed = false
	call("/search", query, http.StatusForbidden)
	permissionAllowed = true
	policy = roles.WithRolePreview(policy, map[host.PermissionKey]bool{})
	call("/get", id, http.StatusForbidden)
	policy = context.WithValue(t.Context(), scopeKey{}, "resolved")
	acting = other
	call("/search", query, http.StatusUnauthorized)
	acting = owner
	sessionErr = host.ErrExternalRecoveryRequired
	call("/get", id, http.StatusConflict)
	sessionErr = host.ErrReadSessionEnded
	call("/get", id, http.StatusUnauthorized)
	sessionErr = nil
	if !h.grants.revoke(code, owner) {
		t.Fatal("owner could not revoke the credential")
	}
	call("/search", query, http.StatusUnauthorized)
	if listCalls != 1 || getCalls != 2 {
		t.Fatalf("denied or malformed requests reached the reader: calls=%d,%d", listCalls, getCalls)
	}
}

func TestResourceDeclarationLimitsRoutesAndValues(t *testing.T) {
	resource := testResource()
	resource.Search = nil
	cfg := Config{Origin: "http://localhost:8080"}
	deadReader := ReadFuncs{}
	for _, incomplete := range []Reader{deadReader, &deadReader, (*ReadFuncs)(nil)} {
		if _, err := Module(cfg, resource, incomplete); err == nil {
			t.Fatal("enabled detail operation without a reader")
		}
	}
	adapter, err := Adapter(cfg, resource)
	if err != nil || adapter.BasePath != resource.Path {
		t.Fatal(adapter.BasePath, err)
	}
	resource.Detail.Fields[0].Key = "id"
	extension, err := adapter.Build(nil, deadReader)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	extension.RegisterPublic(app)
	result, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, cfg.Origin+resource.Path+"/search", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusNotFound {
		t.Fatal("disabled search route was mounted")
	}
	result, err = app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, cfg.Origin+resource.Path+"/describe", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	var description description
	if err := json.NewDecoder(result.Body).Decode(&description); err != nil || result.StatusCode != http.StatusOK ||
		description.validate() != nil || description.Search != nil || description.Detail.Fields[0].Key != testGroupField {
		t.Fatal("invalid or mutated discovery contract", err)
	}
	rows := []Record{
		{ID: "1", Data: map[string]any{testGroupField: map[string]any{"internal": "hidden"}, testTitleField: "display"}},
		{ID: "1", Data: map[string]any{testGroupField: "1"}},
		{ID: "1", Data: map[string]any{testGroupField: "0", testTitleField: "display"}},
	}
	for _, row := range rows {
		if _, ok := project(row, description.Detail.Fields); ok {
			t.Fatal("invalid host projection was exposed")
		}
	}
}
