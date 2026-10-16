//go:build integration

package host //nolint:testpackage // verifies assembled PostgreSQL sessions and canonical HTTP boundaries.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	logger "github.com/assurrussa/gologger"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/identity"
)

func TestAssemblyAdministrationFlashRoundTrip(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	var logs bytes.Buffer
	log, err := logger.New(logger.Config{Level: "error", JSON: true}, logger.WithWriter(&logs))
	require.NoError(t, err)
	deps.Logger = log
	runtime, err := New(t.Context(), cfg, deps, AccessModule())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	const (
		email           = "flash-operator@example.test"
		createPath      = "/admins/create"
		nullPreviewJSON = `,"previewId":null`
	)
	original, err := runtime.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: email, Password: assemblyPassword,
		Profile: goauth.BasicProfile{GivenName: "Original", FamilyName: "Operator", Username: "original"},
	})
	require.NoError(t, err)
	admin, err := runtime.Auth.admins.ProvisionAccount(t.Context(), original, identity.NewUserID())
	require.NoError(t, err)
	roles, err := runtime.Auth.Runtime().RBAC(nil)
	require.NoError(t, err)
	super, err := roles.UpsertRole(t.Context(), rbac.Role{Slug: "super_admin", Name: "Super Admin", System: true})
	require.NoError(t, err)
	require.NoError(t, roles.ReplaceSubjectRoles(t.Context(), original.Subject.ID, []int64{super.ID}))
	cookies := make(map[string]*http.Cookie)
	response, body := assemblyGet(t, runtime, "/auth/login", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, assemblyAdminOrigin+"/auth/login",
		strings.NewReader(`{"email":"`+email+`","password":"`+assemblyPassword+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", assemblyAdminOrigin)
	request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, body = assemblyRequest(t, runtime, request, cookies)
	require.Equal(t, http.StatusFound, response.StatusCode, string(body))
	response, body = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	original, err = runtime.Auth.Runtime().GetAccount(t.Context(), original.Subject.ID)
	require.NoError(t, err)
	var passwordPHC string
	require.NoError(t, deps.Database.DB().Pool().QueryRow(t.Context(),
		"SELECT password_phc FROM auth_local_credentials WHERE subject_id=$1", original.Subject.ID).Scan(&passwordPHC))
	editPath := "/admins/" + strconv.FormatInt(admin.ID, 10) + "/edit"
	updatePath := "/admins/" + strconv.FormatInt(admin.ID, 10)
	for _, tc := range []struct {
		name, method, path, referer, email, nameValue, previewJSON string
		previewValue                                               int64
		changedEmail                                               bool
	}{
		{
			name: "changed email absent preview", method: http.MethodPut, path: updatePath, referer: editPath,
			email: "crafted@example.test", nameValue: "Rejected edit", changedEmail: true,
		},
		{
			name: "changed email null preview", method: http.MethodPut, path: updatePath, referer: editPath,
			email: "crafted@example.test", nameValue: "Rejected edit", previewJSON: nullPreviewJSON, changedEmail: true,
		},
		{
			name: "update validation absent preview", method: http.MethodPut, path: updatePath, referer: editPath, email: email,
		},
		{
			name: "update validation null preview", method: http.MethodPut, path: updatePath, referer: editPath,
			email: email, previewJSON: nullPreviewJSON,
		},
		{
			name: "update validation scalar preview", method: http.MethodPut, path: updatePath, referer: editPath,
			email: email, previewJSON: `,"previewId":73`, previewValue: 73,
		},
		{
			name: "store validation absent preview", method: http.MethodPost, path: createPath, referer: createPath,
			email: email,
		},
		{
			name: "store validation null preview", method: http.MethodPost, path: createPath, referer: createPath,
			email: email, previewJSON: nullPreviewJSON,
		},
		{
			name: "store validation scalar preview", method: http.MethodPost, path: createPath, referer: createPath,
			email: email, previewJSON: `,"previewId":73`, previewValue: 73,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			payload := fmt.Sprintf(`{"username":"submitted","name":%q,"lastName":"Submitted","email":%q%s}`,
				tc.nameValue, tc.email, tc.previewJSON)
			request := httptest.NewRequestWithContext(t.Context(), tc.method, assemblyAdminOrigin+tc.path, strings.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Inertia", "true")
			request.Header.Set("Origin", assemblyAdminOrigin)
			request.Header.Set("Referer", assemblyAdminOrigin+tc.referer)
			request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
			response, body := assemblyRequest(t, runtime, request, cookies)
			require.Equal(t, http.StatusSeeOther, response.StatusCode, string(body))
			require.Equal(t, assemblyAdminOrigin+tc.referer, response.Header.Get("Location"))
			response, body = assemblyGet(t, runtime, tc.referer, cookies)
			require.Equal(t, http.StatusOK, response.StatusCode, string(body))
			var page struct {
				Props struct {
					Old    map[string]json.RawMessage `json:"old"`
					Flash  map[string]string          `json:"flash"`
					Errors map[string]string          `json:"errors"`
				} `json:"props"`
			}
			require.NoError(t, json.Unmarshal(body, &page))
			require.NotContains(t, logs.String(), "gob:", "flash persistence must serialize successfully")
			if tc.changedEmail {
				require.Equal(t, "Bad request", page.Props.Flash["error"])
			} else {
				require.NotEmpty(t, page.Props.Flash["warning"])
				require.Contains(t, page.Props.Errors, "name")
			}
			require.NotContains(t, string(body), "gob:")
			require.NotContains(t, page.Props.Old, "password")
			if tc.previewValue == 0 {
				require.NotContains(t, page.Props.Old, "previewId")
			} else {
				var preview int64
				require.NoError(t, json.Unmarshal(page.Props.Old["previewId"], &preview))
				require.Equal(t, tc.previewValue, preview)
			}
			current, err := runtime.Auth.Runtime().GetAccount(t.Context(), original.Subject.ID)
			require.NoError(t, err)
			require.Equal(t, original.PrimaryEmail, current.PrimaryEmail)
			require.Equal(t, original.Profile, current.Profile)
			var currentPHC string
			require.NoError(t, deps.Database.DB().Pool().QueryRow(t.Context(),
				"SELECT password_phc FROM auth_local_credentials WHERE subject_id=$1", original.Subject.ID).Scan(&currentPHC))
			//nolint:testifylint // A failed equality assertion must not print credential hashes.
			require.True(t, passwordPHC == currentPHC, "password credential must remain unchanged")
		})
	}
}
