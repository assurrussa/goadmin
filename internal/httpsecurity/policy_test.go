package httpsecurity_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/assurrussa/goadmin/internal/httpsecurity"
)

func TestTLSFilesMustBePaired(t *testing.T) {
	for _, tc := range []struct {
		cert, key string
		valid     bool
	}{
		{"", "", true}, {"cert", "key", true}, {"cert", "", false}, {"", "key", false},
	} {
		if got := httpsecurity.ValidateTLSFiles(tc.cert, tc.key) == nil; got != tc.valid {
			t.Errorf("cert=%q key=%q: valid=%v", tc.cert, tc.key, got)
		}
	}
}

func TestRequestIDPolicy(t *testing.T) {
	for _, id := range []string{"request-123", "0abc:trace_id.v2", strings.Repeat("a", 128)} {
		if !httpsecurity.ValidRequestID(id) {
			t.Errorf("rejected %q", id)
		}
	}
	for _, id := range []string{"", "bad id", "bad\nvalue", "bad\tvalue", "bad\"value", "кириллица", strings.Repeat("a", 129)} {
		if httpsecurity.ValidRequestID(id) {
			t.Errorf("accepted %q", id)
		}
	}
}

func TestAccessLogDoesNotContainRecoveryQuery(t *testing.T) {
	raw, err := json.Marshal(httpsecurity.AccessLog{
		Path:      "/auth/reset-password?token=SECRET_SENTINEL#fragment",
		UserAgent: "quote\"slash\\tab\tunicode-☃",
		ID:        "trace",
		Failed:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET_SENTINEL") {
		t.Fatal("query secret in access log")
	}
	var decoded map[string]any
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["path"] != "/auth/reset-password" {
		t.Fatalf("path=%v", decoded["path"])
	}
	if decoded["user_agent"] != "quote\"slash\\tab\tunicode-☃" {
		t.Fatal("JSON escaping corrupted user agent")
	}
	if len(decoded) != 12 {
		t.Fatalf("unexpected injected field: %s", raw)
	}
}

func TestAccessLogBoundsUserAgent(t *testing.T) {
	raw, err := json.Marshal(httpsecurity.AccessLog{UserAgent: strings.Repeat("x", 10000)})
	if err != nil {
		t.Fatal(err)
	}
	var decoded httpsecurity.AccessLog
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.UserAgent) != 512 {
		t.Fatalf("length=%d", len(decoded.UserAgent))
	}
}

func FuzzAccessLog(f *testing.F) {
	f.Add("/auth/reset-password?token=secret", "ua\"", "id")
	f.Fuzz(func(t *testing.T, path, agent, id string) {
		raw, err := json.Marshal(httpsecurity.AccessLog{Path: path, UserAgent: agent, ID: id})
		if err != nil || !json.Valid(raw) {
			t.Fatalf("invalid JSON: %v", err)
		}
		var decoded httpsecurity.AccessLog
		if err = json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(decoded.Path, "?#") {
			t.Fatal("query or fragment not removed")
		}
	})
}
