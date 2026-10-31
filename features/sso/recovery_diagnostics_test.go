package sso //nolint:testpackage // Exercises the private redacted recovery failure boundary.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/assurrussa/gologger"
	browser "github.com/assurrussa/gossoclient"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func TestRecoveryFailureDiagnosticsAreRedactedAndPreserveResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stage  recoveryStage
		err    error
		status int
		code   string
	}{
		{"intent denied", recoveryIntentStage, browser.ErrDenied, http.StatusForbidden, ssoRecoveryReauthentication},
		{"native denied", recoveryNativeStage, browser.ErrDenied, http.StatusForbidden, ssoRecoveryReauthentication},
		{
			"provider denied", recoveryProviderStage, fmt.Errorf("private-raw-error: %w", browser.ErrDenied),
			http.StatusForbidden, ssoRecoveryReauthentication,
		},
		{"provider unavailable", recoveryProviderStage, browser.ErrUnavailable, http.StatusServiceUnavailable, ssoRecoveryUnavailable},
		{
			"unknown failure", recoveryCommitStage, errors.New("private-raw-error"),
			http.StatusServiceUnavailable, ssoRecoveryUnavailable,
		},
		{"CSRF unavailable", recoveryCSRFCreateStage, browser.ErrUnavailable, http.StatusServiceUnavailable, ssoRecoveryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			logger, err := gologger.New(gologger.Config{Level: "warn", JSON: true}, gologger.WithWriter(&output))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, logger.Close()) })
			app := fiber.New()
			app.Get("/recover", func(c fiber.Ctx) error {
				// Ambient request attributes must not be copied to this diagnostic event.
				c.SetContext(gologger.WithValue(c.Context(), slog.String("private-context", "private-identity")))
				return writeRecoveryFailure(c, logger, tc.stage, tc.err)
			})
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/recover?private-query=private-value", nil)
			request.Header.Set("Authorization", "Bearer private-token")
			request.Header.Set("Cookie", "session=private-cookie")
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, tc.status, response.StatusCode)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			var payload ssoRecoveryResponse
			require.NoError(t, json.Unmarshal(body, &payload))
			require.Equal(t, ssoRecoveryResponse{Status: tc.status, Code: tc.code}, payload)
			var event map[string]any
			decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
			decoder.UseNumber()
			require.NoError(t, decoder.Decode(&event))
			require.Len(t, event, 6, "only time, level, message and the three fixed diagnostic fields")
			require.Equal(t, "admin authentication recovery failed", event["msg"])
			require.Equal(t, string(tc.stage), event["recovery_stage"])
			require.Equal(t, json.Number(strconv.Itoa(tc.status)), event["status"])
			require.Equal(t, tc.code, event["code"])
			require.NotContains(t, output.String(), "private-")
			require.NotContains(t, string(body), "private-")
		})
	}
}
