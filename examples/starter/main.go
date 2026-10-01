package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/assurrussa/goauth"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/migrations"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pgDSN, err := databaseDSN()
	if err != nil {
		return err
	}
	selected := map[string]bool{}
	for _, key := range strings.Split(os.Getenv("ADMIN_MODULES"), ",") {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		switch key {
		case "access", "jobs", "uploads", "queues", "notifications", "realtime", "authmail":
			selected[key] = true
		default:
			return fmt.Errorf("unknown admin module %q", key)
		}
	}
	baseURL, err := required("ADMIN_URL")
	if err != nil {
		return err
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return errors.New("ADMIN_URL must be an absolute http(s) URL")
	}
	// Staging uses embedded templates/assets while keeping HTTP cookies usable on localhost.
	adminEnv := "staging"
	if parsedURL.Scheme == "https" {
		adminEnv = "production"
	}
	csrfSecret, err := required("CSRF_SECRET")
	if err != nil {
		return err
	}
	if len(csrfSecret) < 32 {
		return errors.New("CSRF_SECRET must contain at least 32 bytes")
	}
	setupRaw, err := required("FIRST_ADMIN_SETUP_TOKEN")
	if err != nil {
		return err
	}
	setupToken, err := host.NewFirstAdminSetupToken(setupRaw)
	if err != nil {
		return err
	}

	signing, err := keyRing("GOAUTH_SIGNING_KEY")
	if err != nil {
		return err
	}
	token, err := keyRing("GOAUTH_TOKEN_KEY")
	if err != nil {
		return err
	}
	envelope, err := keyRing("GOAUTH_ENVELOPE_KEY")
	if err != nil {
		return err
	}

	if os.Getenv("STARTER_MIGRATE") == "1" {
		if err := migrations.Migrate(ctx, migrations.DatabaseConfig{DSN: pgDSN}, nil); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return nil
	}

	lg := host.DiscardLogger()
	db, err := host.NewPgsqlClient(ctx, host.PgsqlConfig{DSN: pgDSN}, lg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	tx := host.NewTxManager(db)
	uploadRoot := os.Getenv("UPLOAD_ROOT")
	if uploadRoot == "" {
		uploadRoot = "/data/uploads"
	}
	const csrfCookieName = "csrf_token"
	adminInput := host.AdminConfigInput{
		Env: adminEnv, BaseDomain: parsedURL.Hostname(), BaseDomainURL: baseURL,
		Domain: parsedURL.Hostname(), DomainURL: baseURL, Addr: ":8080",
		CSRFTokenName: csrfCookieName, CSRFTokenTTL: 24 * time.Hour,
		SessionInactiveTTL: 24 * time.Hour, SessionTTL: 7 * 24 * time.Hour,
		SessionName: "goadmin_session", StaticDataRoot: filepath.Dir(uploadRoot),
	}
	adminCfg := host.BuildConfig(adminInput, host.PathDefaults{})
	adminCfg.CanonicalSessionBackend = "pgsql"
	deps := host.Dependencies{Database: db, TxManager: tx, Logger: lg}
	if os.Getenv("APP_ADMIN_CANONICAL_SESSION_BACKEND") == "redis" {
		redisDSN, readErr := required("REDIS_DSN")
		if readErr != nil {
			return readErr
		}
		redis, createErr := host.NewRedisClient(ctx, host.RedisConfig{DSN: redisDSN})
		if createErr != nil {
			return createErr
		}
		defer func() { _ = redis.Close() }()
		deps.SessionRedis = redis
		adminCfg.CanonicalSessionBackend = "redis"
	}
	cfg := host.Config{
		Admin: adminCfg, Server: host.ServerConfig{Addr: ":8080", AllowOrigins: []string{baseURL}}, FirstAdminSetupToken: setupToken,
		CSRF: host.CSRFConfig{AppDomain: parsedURL.Hostname(), SecretKey: csrfSecret, AllowedOrigins: []string{baseURL}},
		Auth: goauth.Config{Signing: goauth.SigningConfig{Issuer: baseURL, Audience: "goadmin", Keys: signing}, TokenHMACKeys: token, OutboxAEADKeys: envelope, URLBuilder: goauth.URLBuilderFunc(func(_ context.Context, token string) (string, error) {
			return baseURL + "/auth/reset-password?token=" + url.QueryEscape(token), nil
		})},
	}
	modules := make([]host.Module, 0, len(selected)+1)
	if selected["access"] {
		modules = append(modules, host.AccessModule())
	}
	var stream host.EventStream
	if selected["realtime"] {
		stream = host.NewEventStream()
		modules = append(modules, host.RealtimeModule(stream))
	}
	var queue host.OutboxRuntime
	if selected["jobs"] {
		queue, err = host.NewOutboxRuntime(tx, db, lg, host.OutboxConfig{Workers: 2})
		if err != nil {
			return err
		}
		modules = append(modules, host.JobsModule(host.JobsConfig{Owned: queue}))
	}
	if selected["uploads"] {
		if queue == nil {
			return errors.New("uploads requires explicit jobs module")
		}
		uploads, createErr := host.NewLocalUploads(host.LocalUploadsConfig{Root: uploadRoot, BaseURL: baseURL + "/uploads"}, db, tx, queue, stream, lg)
		if createErr != nil {
			return createErr
		}
		modules = append(modules, host.UploadsModule(host.UploadsConfig{Repository: uploads.Repositories.FileRepo, Loader: uploads.Repositories.FileLoader, Uploads: uploads.Uploads, Jobs: uploads.Jobs, URLs: host.URLConfigs{FilesBaseURL: baseURL + "/uploads"}}))
	}
	if selected["queues"] {
		modules = append(modules, host.QueuesModule())
	}
	if selected["notifications"] {
		notifyURL, configErr := required("NOTIFYHUB_URL")
		if configErr != nil {
			return configErr
		}
		projectKey, configErr := required("NOTIFYHUB_PROJECT_KEY")
		if configErr != nil {
			return configErr
		}
		manager, createErr := host.NewNotificationManager(host.NotificationConfig{
			BaseURL: notifyURL, ProjectKey: projectKey,
		})
		if createErr != nil {
			return fmt.Errorf("configure notifications: %w", createErr)
		}
		modules = append(modules, host.NotificationsModule(manager))
	}
	if selected["authmail"] {
		sender, createErr := newSMTPSender()
		if createErr != nil {
			return createErr
		}
		modules = append(modules, host.AuthMailModule(host.AuthMailConfig{Sender: sender}))
	}
	var runtime *host.Runtime
	modules = append(modules, host.DefinedFeatureModule(host.ModuleDescriptor{Key: "starter.health", Routes: []string{"GET /healthz"}}, healthFeature{ready: func(ctx context.Context) error {
		if runtime == nil {
			return errors.New("admin is not ready")
		}
		return runtime.Readiness(ctx)
	}}))
	runtime, err = host.New(ctx, cfg, deps, modules...)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	return runtime.Run(ctx)
}

func required(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func databaseDSN() (string, error) {
	if override := strings.TrimSpace(os.Getenv("DATABASE_DSN")); override != "" {
		return override, nil
	}
	user, err := required("POSTGRES_USER")
	if err != nil {
		return "", err
	}
	password, ok := os.LookupEnv("POSTGRES_PASSWORD")
	if !ok || password == "" {
		return "", errors.New("POSTGRES_PASSWORD is required")
	}
	database, err := required("POSTGRES_DB")
	if err != nil {
		return "", err
	}
	dsn := url.URL{
		Scheme: "postgres", User: url.UserPassword(user, password), Host: "postgres:5432",
		Path: "/" + database, RawQuery: "sslmode=disable",
	}
	return dsn.String(), nil
}

func keyRing(name string) (goauth.KeyRing, error) {
	raw, err := required(name)
	if err != nil {
		return goauth.KeyRing{}, err
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return goauth.KeyRing{}, fmt.Errorf("%s: base64 decode: %w", name, err)
	}
	return goauth.NewKeyRing("v1", goauth.Key{ID: "v1", Material: key})
}
