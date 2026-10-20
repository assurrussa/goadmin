package adminmiddleware

import (
	_ "embed"
	"html/template"
	"mime"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/services/adminservice"
)

//go:embed browser-recovery.js
var browserRecoveryScript string

//go:embed browser-recovery.css
var browserRecoveryStyles string

// BrowserAuthRecoveryStyles serves the independent recovery screen under strict CSP.
func BrowserAuthRecoveryStyles(c fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.Type("css").SendString(browserRecoveryStyles)
}

// BrowserAuthRecoveryScript is registered before static files and auth gates.
// Keeping this tiny asset independent of custom UI bundles also supports dev.
func BrowserAuthRecoveryScript(c fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.Type("js").SendString(browserRecoveryScript)
}

func isBrowserRecoveryNavigation(c fiber.Ctx) bool {
	if c.Method() != fiber.MethodGet || c.Get("X-Inertia") != "" ||
		c.Get("X-Requested-With") != "" || len(c.Request().Header.PeekAll("Authorization")) != 0 {
		return false
	}
	if mode := c.Get("Sec-Fetch-Mode"); mode != "" && mode != "navigate" {
		return false
	}
	if dest := c.Get("Sec-Fetch-Dest"); dest != "" && dest != "document" {
		return false
	}
	html := false
	for _, part := range strings.Split(c.Get(fiber.HeaderAccept), ",") {
		media, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		if raw, exists := params["q"]; exists {
			quality, qualityErr := strconv.ParseFloat(raw, 64)
			if qualityErr != nil || !(quality > 0 && quality <= 1) {
				continue
			}
		}
		if media == "application/json" || strings.HasSuffix(media, "+json") {
			return false
		}
		html = html || media == "text/html"
	}
	return html
}

func safeRecoverySignIn(target string) string {
	if target == "" || len(target) > 2048 || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") ||
		strings.ContainsAny(target, "\\\r\n\t") {
		return ""
	}
	u, err := url.Parse(target)
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || u.Fragment != "" ||
		u.EscapedPath() != u.Path || strings.ContainsAny(u.Path, ":%") ||
		u.Path == "" || path.Clean(u.Path) != u.Path {
		return ""
	}
	// Fiber's default routing is case-insensitive and non-strict. Reject native
	// loop destinations independent of spelling, not just their literal URL.
	if canonical := strings.ToLower(u.Path); canonical == "/auth/login" || canonical == "/auth/session/recover" {
		return ""
	}
	return target
}

var browserRecoveryPage = template.Must(template.New("recovery").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Проверяем вход</title><link rel="stylesheet" href="/public/auth-recovery.css">
<script src="/public/auth-recovery.js" defer></script></head>
<body><main aria-labelledby="auth-recovery-title"><span class="eyebrow">ПОДТВЕРЖДЕНИЕ ДОСТУПА</span>
<h1 id="auth-recovery-title">Проверяем вход</h1><p id="auth-recovery-status" role="status" aria-live="polite">` +
	`Проверяем вашу сессию. После подтверждения вернём вас на страницу автоматически.</p>
<p class="note">Несохранённые действия не отправляются повторно. Удалять cookies не требуется.</p>
<div class="actions"><button id="auth-recovery-retry" type="button" hidden>Повторить проверку</button>
<button id="auth-recovery-continue" type="button" hidden>Продолжить</button></div>
{{if .}}<p><a id="auth-recovery-sign-in" href="{{.}}">Войти заново</a></p>{{end}}
<noscript>` +
	`Для автоматического восстановления включите JavaScript или воспользуйтесь ссылкой «Войти заново».</noscript>
</main></body></html>`))

func sendBrowserRecoveryDocument(c fiber.Ctx) error {
	var body strings.Builder
	if err := browserRecoveryPage.Execute(&body, safeRecoverySignIn(adminservice.ExternalAdminRecoverySignIn(c))); err != nil {
		return err
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Vary(fiber.HeaderAccept, "X-Inertia", "Sec-Fetch-Mode", "Sec-Fetch-Dest")
	c.Response().Header.Del(fiber.HeaderLocation)
	c.Response().Header.Del("X-Inertia-Location")
	c.Response().Header.Del("X-Inertia")
	return c.Status(fiber.StatusUnauthorized).Type("html").SendString(body.String())
}
