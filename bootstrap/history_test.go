package bootstrap //nolint:testpackage // verifies private response-history policy

import (
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"

	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	"github.com/assurrussa/goadmin/models"
)

type historyResponseTestCase struct {
	name, path, target string
	authenticated      bool
	encrypt, clear     bool
}

func TestAdminHistoryResponseFlags(t *testing.T) {
	const loginPath = "/auth/login"
	for _, test := range []historyResponseTestCase{
		{"protected roles", "/roles", "/roles", true, true, false},
		{"protected profile", "/auth/profile", "/auth/profile", true, true, false},
		{"authenticated login", loginPath, loginPath, true, true, false},
		{"anonymous login", loginPath, loginPath, false, false, true},
		{"anonymous login query", loginPath, loginPath + "?redirectback=%2Froles", false, false, true},
		{"anonymous registration", "/auth/register", "/auth/register", false, false, false},
		{"other anonymous page", "/other", "/other", false, false, false},
	} {
		for _, inertiaRequest := range []bool{false, true} {
			kind := "document"
			if inertiaRequest {
				kind = "inertia"
			}
			t.Run(test.name+"/"+kind, func(t *testing.T) {
				assertHistoryResponseFlags(t, test, inertiaRequest)
			})
		}
	}
}

func assertHistoryResponseFlags(t *testing.T, test historyResponseTestCase, inertiaRequest bool) {
	t.Helper()
	const templateName = "app.gohtml"
	manager := goinertia.New("https://admin.test", goinertia.WithFS(fstest.MapFS{
		templateName: &fstest.MapFile{Data: []byte(`<div data-page="{{ marshal .page }}"></div>`)},
	}))
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		if test.authenticated {
			c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
		}
		return c.Next()
	})
	app.Use(manager.Middleware(), adminHistoryMiddleware(manager))
	app.Get(test.path, func(c fiber.Ctx) error {
		return manager.Render(c, "HistoryTestPage", map[string]any{})
	})
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, test.target, nil)
	if inertiaRequest {
		request.Header.Set("X-Inertia", "true")
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("page status = %d", response.StatusCode)
	}
	var page struct {
		EncryptHistory bool `json:"encryptHistory"`
		ClearHistory   bool `json:"clearHistory"`
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !inertiaRequest {
		match := regexp.MustCompile(`data-page="([^"]+)"`).FindSubmatch(body)
		if len(match) != 2 {
			t.Fatal("missing document page payload")
		}
		body = []byte(html.UnescapeString(string(match[1])))
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if page.EncryptHistory != test.encrypt || page.ClearHistory != test.clear {
		t.Fatalf("history flags = %+v, want encrypt=%t clear=%t", page, test.encrypt, test.clear)
	}
}
