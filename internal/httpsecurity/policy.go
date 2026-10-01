// Package httpsecurity holds transport policies with no framework dependencies.
package httpsecurity

import (
	"encoding/json"
	"errors"
	"strings"
)

// ValidRequestID accepts bounded correlation identifiers, never arbitrary header text.
func ValidRequestID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range []byte(id) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':' {
			continue
		}
		return false
	}
	return true
}

// ValidateTLSFiles rejects a partially configured TLS identity. Both empty is
// allowed for a listener explicitly served behind a TLS-terminating proxy.
func ValidateTLSFiles(cert, key string) error {
	if (cert == "") != (key == "") {
		return errors.New("TLS certificate and key must be configured together")
	}
	return nil
}

// AccessLog deliberately has no raw URL, query, headers, cookies, body or error
// message. Framework error strings can contain credentials and SQL arguments.
type AccessLog struct {
	Time      string `json:"time"`
	ID        string `json:"id"`
	Status    int    `json:"status"`
	Latency   string `json:"latency"`
	RemoteIP  string `json:"remote_ip"` //nolint:tagliatelle // preserved snake_case log keys for log ingestion compatibility
	Method    string `json:"method"`
	Host      string `json:"host"`
	Path      string `json:"path"`
	UserAgent string `json:"user_agent"` //nolint:tagliatelle // preserved snake_case log keys for log ingestion compatibility
	Failed    bool   `json:"failed"`
	BytesIn   int    `json:"bytes_in"`  //nolint:tagliatelle // preserved snake_case log keys for log ingestion compatibility
	BytesOut  int    `json:"bytes_out"` //nolint:tagliatelle // preserved snake_case log keys for log ingestion compatibility
}

func (r AccessLog) MarshalJSON() ([]byte, error) {
	type plain AccessLog
	r.Path, _, _ = strings.Cut(r.Path, "?")
	r.Path, _, _ = strings.Cut(r.Path, "#")
	if len(r.UserAgent) > 512 {
		r.UserAgent = r.UserAgent[:512]
	}
	return json.Marshal(plain(r))
}
