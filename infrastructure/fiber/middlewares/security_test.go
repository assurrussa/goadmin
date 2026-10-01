package middlewares_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"github.com/assurrussa/goadmin/infrastructure/fiber/middlewares"
	"github.com/assurrussa/goadmin/internal/httpsecurity"
)

func TestAccessLogExcludesSecretsAndSharesAuditRequestID(t *testing.T) {
	var logs bytes.Buffer
	var auditID string
	app := fiber.New()
	app.Use(middlewares.NewRequestLogger(middlewares.RequestLoggerConfig{Stream: &logs}))
	app.Use(middlewares.NewRequestID())
	app.Get("/auth/reset-password", func(c fiber.Ctx) error {
		auditID = requestid.FromContext(c)
		return errors.New("INTERNAL_SECRET_SENTINEL")
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/auth/reset-password?token=QUERY_SECRET_SENTINEL", nil)
	req.Header.Set(fiber.HeaderXRequestID, "correlation-123")
	req.Header.Set(fiber.HeaderUserAgent, "browser-\"quoted\"-\\-tab\tvalue")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var record httpsecurity.AccessLog
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record); err != nil {
		t.Fatalf("invalid log JSON: %v", err)
	}
	if record.ID != "correlation-123" || auditID != record.ID || resp.Header.Get(fiber.HeaderXRequestID) != record.ID {
		t.Fatal("request log, audit context and response correlation differ")
	}
	if record.Status != 500 || !record.Failed || record.Path != "/auth/reset-password" {
		t.Fatalf("incorrect access record: %+v", record)
	}
	if strings.Contains(logs.String(), "SECRET_SENTINEL") {
		t.Fatal("secret leaked into access log")
	}
}

func TestRequestIDRejectsInvalidClientAndGeneratorValues(t *testing.T) {
	app := fiber.New()
	app.Use(middlewares.NewRequestID(middlewares.ConfigRequestID{Generator: func() string { return "invalid generator\n" }}))
	app.Get("/", func(c fiber.Ctx) error {
		id := requestid.FromContext(c)
		if !httpsecurity.ValidRequestID(id) || c.Locals("requestid") != id {
			return c.SendStatus(500)
		}
		return c.SendStatus(204)
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.Header.Set(fiber.HeaderXRequestID, strings.Repeat("x", 129))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || !httpsecurity.ValidRequestID(resp.Header.Get(fiber.HeaderXRequestID)) {
		t.Fatal("unsafe request ID accepted")
	}
}
