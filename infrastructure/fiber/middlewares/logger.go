package middlewares

import (
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	mLogger "github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"github.com/assurrussa/goadmin/internal/httpsecurity"
)

// FieldLog is retained for source compatibility. Arbitrary template tags are
// intentionally not evaluated: they can expose query strings or credentials.
type FieldLog struct {
	//nolint:unused // fields retained for struct shape compatibility
	name, val string
}

type RequestLoggerConfig struct {
	Skip   func(c fiber.Ctx) bool
	Fields []FieldLog
	Logger logger.Logger
	// Stream is used by standalone middleware when Logger is nil. The caller owns it.
	Stream io.Writer
}

func NewRequestLogger(opts ...RequestLoggerConfig) fiber.Handler {
	var cfg RequestLoggerConfig
	if len(opts) > 0 {
		cfg = opts[0]
	}
	var mu sync.Mutex
	return mLogger.New(mLogger.Config{
		Format: "${latency}", DisableColors: true, Stream: cfg.Stream,
		LoggerFunc: func(c fiber.Ctx, data *mLogger.Data, fc *mLogger.Config) error {
			if cfg.Skip != nil && cfg.Skip(c) {
				return nil
			}
			record := httpsecurity.AccessLog{
				Time: time.Now().UTC().Format(time.RFC3339Nano), ID: requestid.FromContext(c),
				Status: c.Response().StatusCode(), Latency: data.Stop.Sub(data.Start).String(),
				RemoteIP: c.IP(), Method: c.Method(), Host: c.Hostname(), Path: c.Path(),
				UserAgent: c.Get(fiber.HeaderUserAgent), Failed: data.ChainErr != nil,
				BytesIn: c.Request().Header.ContentLength(), BytesOut: c.Response().Header.ContentLength(),
			}
			if cfg.Logger != nil {
				// The host logger owns JSON encoding, level filtering and its sink. Never
				// log ChainErr.Error(): access logs are not a secret-safe diagnostic channel.
				cfg.Logger.InfoContext(c, "http request", slog.Any("http", record))
				return nil
			}
			raw, err := json.Marshal(record)
			if err != nil {
				return err
			}
			raw = append(raw, '\n')
			mu.Lock()
			defer mu.Unlock()
			_, err = fc.Stream.Write(raw)
			return err
		},
	})
}
