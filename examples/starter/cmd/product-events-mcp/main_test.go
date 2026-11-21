package main

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

	"example.com/goadmin-starter/internal/productjournal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMain(m *testing.M) {
	if origin := os.Getenv("STARTER_TEST_MCP_STDIO_ORIGIN"); origin != "" {
		if run(context.Background(), origin) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestStdioJournal(t *testing.T) {
	const secret, access = "fixture-device-secret", "fixture-access-token"
	const reason = "Operator entry\n  keep spacing 🙂"
	var approved, revoked atomic.Bool
	var pairCalls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Cookie") != "" {
			t.Error("expected a cookieless POST")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case agentPath + "pair":
			pairCalls.Add(1)
			_ = json.NewEncoder(w).Encode(pairing{Code: "123456", DeviceSecret: secret, VerificationURL: "/verify", ExpiresIn: 300})
		case agentPath + "token":
			var in map[string]string
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in["device_secret"] != secret {
				t.Error("missing exchange proof")
			}
			if !approved.Load() {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"`+access+`","expires_in":900}`)
		case agentPath + "search", agentPath + "get":
			if r.Header.Get("Authorization") != "Bearer "+access || revoked.Load() {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			row := journalRow{ID: "12", ProductID: "9", Action: "unpublish", CreatedAt: "2026-01-01 00:00:00 UTC"}
			if strings.HasSuffix(r.URL.Path, "/search") {
				var in productjournal.SearchInput
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ProductID != "9" || in.Action != "unpublish" {
					t.Error("search query did not reach the journal")
				}
				if page, size := in.Pagination(); page != 1 || size != 25 {
					t.Errorf("default page and size: got %d, %d", page, size)
				}
				_ = json.NewEncoder(w).Encode(searchOutput{Rows: []journalRow{row}, Total: 1})
				return
			}
			var in productjournal.GetInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
			}
			switch in.EventID {
			case "12":
				_ = json.NewEncoder(w).Encode(eventDetail{journalRow: row, Reason: new(reason)})
			case "13":
				row.ID = "13"
				_ = json.NewEncoder(w).Encode(eventDetail{journalRow: row})
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
	cmd := exec.CommandContext(ctx, os.Args[0])
	cmd.Env = append(os.Environ(), "STARTER_TEST_MCP_STDIO_ORIGIN="+gateway.URL)
	cmd.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 2 {
		t.Fatalf("tools/list: %v, %v", tools, err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.OutputSchema == nil {
			t.Fatalf("missing read-only output contract for %s", tool.Name)
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"get_product_event", "search_product_events"}) {
		t.Fatal(names)
	}
	call := func(name string, input any, problem string) map[string]any {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: input})
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
	const search, get = "search_product_events", "get_product_event"
	query := productjournal.SearchInput{ProductID: "9", Action: "unpublish"}
	if res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: search, Arguments: map[string]any{"product_id": "9", "admin_id": 3}}); err != nil || !res.IsError {
		t.Fatal("unexpected arguments were accepted", err)
	}
	call(search, productjournal.SearchInput{ProductID: "9", Page: new(0)}, "invalid_input")
	call(search, productjournal.SearchInput{ProductID: "9", PageSize: new(0)}, "invalid_input")
	call(search, productjournal.SearchInput{ProductID: "9", PageSize: new(101)}, "invalid_input")
	call(search, productjournal.SearchInput{ProductID: "Product 9"}, "invalid_input")
	call(get, productjournal.GetInput{EventID: "0"}, "invalid_input")
	call(search, productjournal.SearchInput{ProductID: "9", Page: new(math.MaxInt/2 + 1)}, "invalid_input")
	if pairCalls.Load() != 0 {
		t.Fatal("invalid journal input started a connection request")
	}
	need := call(search, query, "authorization_required")
	pending := call(search, query, "authorization_pending")
	if pairCalls.Load() != 1 || need["code"] != "123456" || need["verification_url"] != gateway.URL+"/verify" || pending["code"] != need["code"] {
		t.Fatal("connection instructions changed or a second pairing was created")
	}
	approved.Store(true)
	result := call(search, query, "")
	rows := result["rows"].([]any)
	row := rows[0].(map[string]any)
	if result["total"] != float64(1) || row["id"] != "12" {
		t.Fatal(result)
	}
	if _, listedReason := row["reason"]; listedReason {
		t.Fatal("search returned a reason field")
	}
	if detail := call(get, productjournal.GetInput{EventID: "12"}, ""); detail["reason"] != reason || detail["id"] != row["id"] {
		t.Fatal(detail)
	}
	if r, ok := call(get, productjournal.GetInput{EventID: "13"}, "")["reason"]; !ok || r != nil {
		t.Fatal("missing historical null")
	}
	call(get, productjournal.GetInput{EventID: "404"}, "not_found")
	revoked.Store(true)
	call(get, productjournal.GetInput{EventID: "12"}, "authorization_expired")
	call(get, productjournal.GetInput{EventID: "12"}, "authorization_required")
	if pairCalls.Load() != 2 {
		t.Fatal("new pairing was not started after revocation")
	}
}

func TestOriginAndPairingLink(t *testing.T) {
	for _, origin := range []string{"https://localhost:8443", "http://127.0.0.1:8080", "http://[::1]:8080"} {
		if _, err := newGateway(origin); err != nil {
			t.Error(err)
		}
	}
	for _, origin := range []string{"", "file:///tmp", "http://example.test", "http://localhost/path", "https://user@example.test", "https://example.test?token=x"} {
		if _, err := newGateway(origin); err == nil {
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
			_ = json.NewEncoder(w).Encode(pairing{Code: "ABC", DeviceSecret: "private", VerificationURL: foreign.URL, ExpiresIn: 300})
		}))
		g, err := newGateway(server.URL)
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
				if r.URL.Path != agentPath+"token" {
					t.Error("unexpected request")
				}
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			gateway, err := newGateway(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			gateway.pending = pairing{Code: "123456", DeviceSecret: "proof"}
			gateway.validUntil = time.Now().Add(time.Minute)
			token, problem := gateway.credential(t.Context())
			if token != "" || gateway.pending != (pairing{}) || problem == nil || problem.Error != test.outcome {
				t.Fatal("terminal authorization attempt did not reset pairing")
			}
		})
	}
}
