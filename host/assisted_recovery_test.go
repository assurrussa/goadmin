//nolint:testpackage // Tests private HTTP feature handlers and strict form parsing.
package host

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

type recoveryCommandsStub struct {
	completions int
	token       string
}

func (*recoveryCommandsStub) Initiate(context.Context, goauth.AssistedRecoveryRequest) error {
	return nil
}

func (s *recoveryCommandsStub) Complete(_ context.Context, r goauth.AssistedRecoveryCompletion) error {
	s.completions++
	s.token = r.Token
	return nil
}

func TestAssistedRecoveryPublicCompletionHasFragmentSecretAndSameOriginCSRF(t *testing.T) {
	commands := &recoveryCommandsStub{}
	f := &assistedRecoveryFeature{cfg: AssistedRecoveryConfig{Commands: commands, Origin: "https://admin.example.test"}}
	app := fiber.New()
	app.Get(AssistedRecoveryCompletePath, f.completionPage)
	app.Post(AssistedRecoveryCompletePath, f.complete)
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, AssistedRecoveryCompletePath, nil))
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	require.Contains(t, response.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'")
	require.Contains(t, string(body), "location.hash")
	require.Contains(t, string(body), "history.replaceState")
	require.NotContains(t, string(body), "type=email")
	cookies := response.Cookies()
	require.Len(t, cookies, 1)
	csrf := cookies[0]
	require.True(t, csrf.HttpOnly)
	require.True(t, csrf.Secure)
	require.Equal(t, "__Host-goadmin_recovery", csrf.Name)
	for _, scenario := range []string{
		"missing origin", "foreign origin", "missing cookie", "wrong csrf", "duplicate field", "unknown field", "matching",
	} {
		t.Run(scenario, func(t *testing.T) {
			values := url.Values{
				"csrf": {csrf.Value}, "token": {"synthetic-test-token"},
				"password": {"Synthetic-Password-42"}, "password_confirmation": {"Synthetic-Password-42"},
				"code": {"123456"},
			}
			origin := "https://admin.example.test"
			if scenario == "missing origin" {
				origin = ""
			}
			if scenario == "foreign origin" {
				origin = "https://other.example.test"
			}
			if scenario == "wrong csrf" {
				values.Set("csrf", strings.Repeat("x", 43))
			}
			if scenario == "duplicate field" {
				values.Add("token", "other")
			}
			if scenario == "unknown field" {
				values.Set("subject", "unauthorized-selector")
			}
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				AssistedRecoveryCompletePath, strings.NewReader(values.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", origin)
			if scenario != "missing cookie" {
				request.AddCookie(csrf)
			}
			before := commands.completions
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()
			if scenario == "matching" {
				require.Equal(t, http.StatusOK, response.StatusCode)
				require.Equal(t, before+1, commands.completions)
				require.Equal(t, "synthetic-test-token", commands.token)
			} else {
				require.GreaterOrEqual(t, response.StatusCode, 400)
				require.Equal(t, before, commands.completions)
			}
		})
	}
}

func TestAssistedRecoveryCatalogUsesDedicatedPermissionAndOwnerRouteIsSeparate(t *testing.T) {
	f := &assistedRecoveryFeature{}
	descriptor := f.Descriptor()
	require.Equal(t, "account_recovery:create", descriptor.PermissionDefinitions[0].Key.String())
	require.NotEqual(t, AssistedRecoveryCompletePath, AssistedRecoveryOwnerCompletePath)
	_, err := f.Build(Menu{})(nil)
	require.Error(t, err)
}
