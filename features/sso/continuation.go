package sso

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
)

const continuationScript = "window.location.replace('/sso/open');"

func writeContinuation(w http.ResponseWriter) {
	hash := sha256.Sum256([]byte(continuationScript))
	w.Header().Del("Location")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	policy := "default-src 'none'; script-src 'sha256-" + base64.StdEncoding.EncodeToString(hash[:]) +
		"'; base-uri 'none'; frame-ancestors 'none'"
	w.Header().Set("Content-Security-Policy", policy)
	_, _ = w.Write([]byte(`<!doctype html><html lang="ru"><meta charset="utf-8"><title>Admin · Вход</title><script>` +
		continuationScript + `</script><p>Продолжаем вход…</p><a href="/sso/open">Продолжить вход</a></html>`))
}
