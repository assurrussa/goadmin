package mcp //nolint:testpackage // Exercise the unexported MCP transport and grant state.

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/assurrussa/goadmin/host"
)

func TestMain(m *testing.M) {
	if origin := os.Getenv("GOADMIN_TEST_MCP_STDIO_ORIGIN"); origin != "" {
		if err := RunStdio(context.Background(), origin, testResourcePath); err != nil {
			_, _ = os.Stderr.WriteString(err.Error())
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestStdioResource(t *testing.T) { //nolint:gocognit,gocyclo // One subprocess exercises the full credential lifecycle.
	const secret, access = "fixture-device-secret", "fixture-access-token"
	const reason = "Operator entry\n  keep spacing 🙂"
	var approved, refresh, revoked atomic.Bool
	var pairCalls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Cookie") != "" {
			t.Error("expected a cookieless POST")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case testResourcePath + "/describe":
			_ = json.NewEncoder(w).Encode(testResource().description())
		case testResourcePath + "/pair":
			pairCalls.Add(1)
			_ = json.NewEncoder(w).Encode(gatewayPairing{
				Code: "ABCDE-FGHIJ", DeviceSecret: secret,
				VerificationURL: "http://" + r.Host + testResourcePath + "/connect", ExpiresIn: 300,
			})
		case testResourcePath + "/token":
			var in map[string]string
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in["device_secret"] != secret {
				t.Error("missing exchange proof")
			}
			if !approved.Load() {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"`+access+`","expires_in":900}`)
		case testResourcePath + "/search", testResourcePath + "/get":
			if r.Header.Get("Authorization") != "Bearer "+access || revoked.Load() {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if refresh.Load() {
				w.WriteHeader(http.StatusConflict)
				return
			}
			row := map[string]any{"id": "12", testGroupField: "9", testTitleField: "Readings", "private": access}
			var input map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			resource := testResource()
			if strings.HasSuffix(r.URL.Path, "/search") {
				filters, ok := resource.Search.filters(input)
				if !ok || filters.Fields[testGroupField] != "9" || filters.Search != "reading" {
					t.Error("search parameters did not reach the resource")
				}
				if filters.Page != 1 || filters.Limit != 25 {
					t.Errorf("default page and size: got %d, %d", filters.Page, filters.Limit)
				}
				row["notes"] = "detail only"
				_ = json.NewEncoder(w).Encode(searchOutput{Rows: []map[string]any{row}, Total: 1})
				return
			}
			id, _ := resource.Detail.recordID(input)
			switch id {
			case "12":
				row["notes"] = reason
				_ = json.NewEncoder(w).Encode(row)
			case "13":
				row["id"] = id
				_ = json.NewEncoder(w).Encode(row)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			t.Error("unexpected route")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(gateway.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0]) //nolint:gosec // Start this test binary as a real stdio child.
	cmd.Env = append(os.Environ(), "GOADMIN_TEST_MCP_STDIO_ORIGIN="+gateway.URL)
	cmd.Stderr = os.Stderr
	client := sdk.NewClient(&sdk.Implementation{Name: "stdio-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 2 {
		t.Fatalf("tools/list: %v, %v", tools, err)
	}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.OutputSchema == nil {
			t.Fatalf("missing read-only output contract for %s", tool.Name)
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"get_reading", "search_readings"}) {
		t.Fatal(names)
	}
	call := func(name string, input any, problem string) map[string]any {
		t.Helper()
		res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: input})
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(res)
		if err != nil || strings.Contains(string(wire), secret) || strings.Contains(string(wire), access) {
			t.Fatal("tool result contained a credential or failed to serialize")
		}
		payload, ok := res.StructuredContent.(map[string]any)
		if !ok || res.IsError != (problem != "") || (problem != "" && payload["error"] != problem) {
			t.Fatalf("unexpected tool result %s", wire)
		}
		return payload
	}
	const search, get = "search_readings", "get_reading"
	query := map[string]any{testGroupParam: "9", "query": "reading"}
	detail := func(id string) map[string]string { return map[string]string{"reading_id": id} }
	for _, invalid := range []map[string]any{
		{testGroupParam: "9", "admin_id": 3},
		{testGroupParam: "9", "page": 0},
		{testGroupParam: "9", "page_size": 0},
		{testGroupParam: "9", "page_size": 101},
		{testGroupParam: "Group 9"},
		{testGroupParam: "9223372036854775808"},
		{testGroupParam: "9", "page": math.MaxInt/2 + 1},
	} {
		call(search, invalid, "invalid_input")
	}
	call(get, detail("0"), "invalid_input")
	if pairCalls.Load() != 0 {
		t.Fatal("invalid journal input started a connection request")
	}
	need := call(search, query, "authorization_required")
	pending := call(search, query, "authorization_pending")
	if pairCalls.Load() != 1 || need["code"] != "ABCDE-FGHIJ" ||
		need["verification_url"] != gateway.URL+testResourcePath+"/connect" || pending["code"] != need["code"] {
		t.Fatal("connection instructions changed or a second pairing was created")
	}
	approved.Store(true)
	result := call(search, query, "")
	rows, ok := result["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("unexpected rows: %v", result)
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected row: %v", rows[0])
	}
	if result["total"] != float64(1) || row["id"] != "12" {
		t.Fatal(result)
	}
	if _, listedReason := row["notes"]; listedReason {
		t.Fatal("search returned a reason field")
	}
	if detail := call(get, detail("12"), ""); detail["notes"] != reason || detail["id"] != row["id"] {
		t.Fatal(detail)
	}
	if r, ok := call(get, detail("13"), "")["notes"]; !ok || r != nil {
		t.Fatal("missing historical null")
	}
	call(get, detail("404"), "not_found")
	refresh.Store(true)
	refreshProblem := call(get, detail("12"), "browser_refresh_required")
	if refreshProblem["verification_url"] != gateway.URL+testResourcePath+"/connect" {
		t.Fatal("refresh instructions did not identify the admin page")
	}
	refresh.Store(false)
	call(get, detail("12"), "")
	if pairCalls.Load() != 1 {
		t.Fatal("temporary browser recovery replaced the grant")
	}
	revoked.Store(true)
	call(get, detail("12"), "authorization_expired")
	call(get, detail("12"), "authorization_required")
	if pairCalls.Load() != 2 {
		t.Fatal("new pairing was not started after revocation")
	}
}

func TestOriginAndPairingLink(t *testing.T) {
	for _, origin := range []string{"https://localhost:8443", "http://127.0.0.1:8080", "http://[::1]:8080"} {
		if _, err := newGateway(origin, testResourcePath); err != nil {
			t.Error(err)
		}
	}
	for _, origin := range []string{
		"", "file:///tmp", "http://example.test", "http://localhost/path", "https://user@example.test", "https://example.test?token=x",
	} {
		if _, err := newGateway(origin, testResourcePath); err == nil {
			t.Fatalf("accepted invalid configured origin %q", origin)
		}
	}
	foreign := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unexpected redirect") }))
	defer foreign.Close()
	for _, redirect := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirect {
				http.Redirect(w, r, foreign.URL, http.StatusTemporaryRedirect)
				return
			}
			_ = json.NewEncoder(w).Encode(gatewayPairing{
				Code: "ABC", DeviceSecret: "private", VerificationURL: foreign.URL, ExpiresIn: 300,
			})
		}))
		g, err := newGateway(server.URL, testResourcePath)
		if err != nil {
			t.Fatal(err)
		}
		_, p := g.credential(t.Context())
		server.Close()
		if p == nil || p.Error != "unavailable" || p.Code != "" || p.VerificationURL != "" {
			t.Fatal("unexpected verification origin")
		}
	}
}

func TestTerminalExchangeClearsPendingCode(t *testing.T) {
	for _, test := range []struct {
		status  int
		outcome string
	}{
		{http.StatusForbidden, "denied"},
		{http.StatusGone, "authorization_expired"},
	} {
		t.Run(test.outcome, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != testResourcePath+"/token" {
					t.Error("unexpected request")
				}
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			gateway, err := newGateway(server.URL, testResourcePath)
			if err != nil {
				t.Fatal(err)
			}
			gateway.pending = gatewayPairing{Code: "123456", DeviceSecret: "proof"}
			gateway.validUntil = time.Now().Add(time.Minute)
			token, problem := gateway.credential(t.Context())
			if token != "" || gateway.pending != (gatewayPairing{}) || problem == nil || problem.Error != test.outcome {
				t.Fatal("terminal authorization attempt did not reset pairing")
			}
		})
	}
}

const (
	testResourcePath = "/agent/readings"
	testGroupField   = "groupId"
	testGroupParam   = "group"
	testTitleField   = "title"
)

func testResource() Resource {
	fields := []Field{
		{Key: testGroupField, Kind: PositiveID, Description: "Owning group number."},
		{Key: testTitleField, Kind: Text, Description: "Reading title."},
	}
	return Resource{
		Path: testResourcePath, Permission: host.NewPermissionKey("readings", "read"),
		Name: "readings", Description: "Find readings for a group.", RecordIDDescription: "Reading ID.",
		Search: &Search{
			ToolName: "search_readings", Description: "List readings for a group.",
			Query: &Parameter{Name: "query", Kind: Text, Description: "Words from the reading."},
			Filters: []Filter{{Parameter: Parameter{
				Name: testGroupParam, Kind: PositiveID, Required: true, Description: "Exact group number.",
			}, FilterKey: testGroupField}},
			Fields: fields,
		},
		Detail: &Detail{
			ToolName: "get_reading", Description: "Read one reading.",
			ID:     Parameter{Name: "reading_id", Kind: PositiveID, Required: true, Description: "Reading ID from search_readings."},
			Fields: append(slices.Clone(fields), Field{Key: "notes", Kind: Text, Description: "Optional notes.", Nullable: true}),
		},
	}
}
