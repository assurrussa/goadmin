package auth //nolint:testpackage // verifies the canonical profile and page boundary.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	authmocks "github.com/assurrussa/goadmin/http/handlers/auth/mocks"
	"github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/models"
)

func TestProfileEditRedirectReloadsCanonicalProfile(t *testing.T) {
	for _, clearOptional := range []bool{false, true} {
		t.Run(map[bool]string{false: "replace", true: "clear optional"}[clearOptional], func(t *testing.T) {
			harness := adminappt.NewAppTest(t)
			repo := authmocks.NewMockadminRepo(harness.Ctrl)
			account := authmocks.NewMockadminAccountService(harness.Ctrl)
			subjectID := goauth.NewSubjectID()
			current := models.Admin{ID: 42, Name: "Before", LastName: "Old", Username: "old", Email: "canonical@example.test"}
			next := models.Admin{ID: 42, Name: "Canonical", LastName: "Family", Username: "normalized", Email: current.Email}
			form := FormProfileEdit{Name: "Submitted", LastName: "Family", Username: "submitted"}
			if clearOptional {
				form.LastName, form.Username, next.LastName, next.Username = "", "", "", ""
			}
			displayName := strings.TrimSpace(form.Name + " " + form.LastName)
			gomock.InOrder(
				repo.EXPECT().GetByID(gomock.Any(), int64(42)).Return(current, nil),
				account.EXPECT().UpdateBasicProfile(gomock.Any(), subjectID, goauth.BasicProfile{
					GivenName: form.Name, FamilyName: form.LastName, Username: form.Username, DisplayName: displayName,
				}).Return(goauth.Account{}, nil),
				repo.EXPECT().GetByID(gomock.Any(), int64(42)).Return(next, nil),
			)
			// No UpdateAdmin expectation: profile ownership stays entirely in GoAuth.
			handler := NewHandler(HandlerOptions{AdminApp: harness.App, AdminRepo: repo, AdminAccountService: account})
			app := fiber.New()
			app.Use(func(c fiber.Ctx) error {
				c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 42, SubjectID: subjectID})
				return c.Next()
			})
			app.Post("/auth/profile/edit", handler.PostProfileEdit)
			app.Get("/auth/profile/edit", handler.ProfileEdit)
			body, err := json.Marshal(form)
			require.NoError(t, err)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/profile/edit", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			response, err := app.Test(req)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusFound, response.StatusCode)
			require.Equal(t, "/auth/profile/edit", response.Header.Get("Location"))
			req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, response.Header.Get("Location"), nil)
			req.Header.Set("X-Inertia", "true")
			response, err = app.Test(req)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			var page struct {
				Component string `json:"component"`
				Props     struct {
					Data models.Admin `json:"data"`
				} `json:"props"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
			require.Equal(t, "profile/EditPage", page.Component)
			require.Equal(t, next.Name, page.Props.Data.Name)
			require.Equal(t, next.LastName, page.Props.Data.LastName)
			require.Equal(t, next.Username, page.Props.Data.Username)
			require.Equal(t, next.Email, page.Props.Data.Email)
		})
	}
}

func TestProfileEditRequiresNameBeforeCanonicalMutation(t *testing.T) {
	for _, name := range []string{"", "   "} {
		t.Run(name, func(t *testing.T) {
			harness := adminappt.NewAppTest(t)
			handler := NewHandler(HandlerOptions{
				AdminApp:  harness.App,
				AdminRepo: authmocks.NewMockadminRepo(harness.Ctrl), AdminAccountService: authmocks.NewMockadminAccountService(harness.Ctrl),
			})
			app := fiber.New()
			var props map[string]any
			app.Post("/auth/profile/edit", func(c fiber.Ctx) error {
				err := handler.PostProfileEdit(c)
				props = harness.InertiaManager.State(c).Props
				return err
			})
			body, err := json.Marshal(FormProfileEdit{Name: name})
			require.NoError(t, err)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/profile/edit", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Referer", "/auth/profile/edit")
			response, err := app.Test(req)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusFound, response.StatusCode)
			require.Contains(t, props["errors"], "name")
		})
	}
}
