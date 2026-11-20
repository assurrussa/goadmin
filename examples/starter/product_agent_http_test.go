package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestAgentDeviceRoutesAreSeparateFromOperatorApproval(t *testing.T) {
	const origin = "http://localhost:8080"
	app := fiber.New()
	h := journalAgent{grants: newJournalGrants(), origin: origin}
	h.registerDevice(app)
	request := func(path, body string, headers map[string]string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, origin+agentPath+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}
	for _, headers := range []map[string]string{{"Origin": origin}, {"Cookie": "goadmin_session=unused"}, {"Sec-Fetch-Site": "same-origin"}} {
		if result := request("/pair", "{}", headers); result.StatusCode != 403 {
			t.Fatalf("browser request returned %d", result.StatusCode)
		}
	}
	if result := request("/connect", "{}", nil); result.StatusCode != 404 {
		t.Fatal("approval was publicly mounted")
	}
	if result := request("/get", `{"event_id":"1"}`, nil); result.StatusCode != 401 {
		t.Fatalf("unpaired lookup returned %d", result.StatusCode)
	}
	if result := request("/pair", `{"actor":"9"}`, nil); result.StatusCode != 400 {
		t.Fatalf("unrecognized input returned %d", result.StatusCode)
	}
	pair := request("/pair", "{}", nil)
	if pair.StatusCode != 200 || pair.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("pairing status %d", pair.StatusCode)
	}
	var data struct {
		Code   string
		Secret string `json:"device_secret"`
		URL    string `json:"verification_url"`
	}
	if err := json.NewDecoder(pair.Body).Decode(&data); err != nil || data.Code == "" || data.Secret == "" || data.URL != origin+connectPath {
		t.Fatal("incomplete device response", err)
	}
	if r := request("/token", `{"device_secret":"`+data.Code+`"}`, nil); r.StatusCode != 410 {
		t.Fatal("human code accepted as device credential")
	}
	if r := request("/token", `{"device_secret":"`+data.Secret+`"}`, nil); r.StatusCode != 202 {
		t.Fatal("device polling did not await approval")
	}
}
