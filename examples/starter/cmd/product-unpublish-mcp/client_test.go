package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientReconcilesOnlyTheOriginalReceipt(t *testing.T) {
	const key = "d48ddead-f89e-4f34-80d6-065a2bfd254d"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer local-device" {
			t.Error("missing device credential")
		}
		switch call {
		case 1:
			if r.URL.Path != actionPath+"/propose" {
				t.Errorf("unexpected proposal route %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2, 3:
			if r.URL.Path != actionPath+"/status" {
				t.Errorf("unexpected recovery route %s", r.URL.Path)
			}
			var in lookup
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.RequestID != key {
				t.Error("lookup did not use the original UUID")
			}
			result := outcome{RequestID: key, Status: "succeeded", EventID: "42"}
			if call == 2 {
				result = outcome{RequestID: key, Status: "pending", ReviewURL: "https://other.invalid/review"}
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				t.Error(err)
			}
		default:
			t.Error("unexpected extra dispatch")
		}
	}))
	t.Cleanup(server.Close)
	g, err := newClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	g.token, g.until = "local-device", time.Now().Add(time.Minute)
	if out := g.call(t.Context(), "propose", key, proposal{key, "3", "Reason"}); out.Error != "outcome_unknown" {
		t.Fatalf("indeterminate response: %+v", out)
	}
	if out := g.call(t.Context(), "status", key, lookup{key}); out.Error != "outcome_unknown" || out.ReviewURL != "" {
		t.Fatalf("unexpected review URL escaped: %+v", out)
	}
	if out := g.call(t.Context(), "status", key, lookup{key}); out.Status != "succeeded" || out.EventID != "42" {
		t.Fatalf("unexpected receipt: %+v", out)
	}
}
