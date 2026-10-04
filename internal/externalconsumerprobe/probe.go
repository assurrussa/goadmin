package externalconsumerprobe

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultProbeModule      = "example.com/goadminprobe"
	DefaultModulePath       = "github.com/assurrussa/goadmin"
	DefaultGoauthModulePath = "github.com/assurrussa/goauth"
	LocalModuleVersion      = "v0.0.0-local"
)

type Config struct {
	ProbeModule          string
	ModulePath           string
	Version              string
	LocalPath            string
	GoauthModulePath     string
	GoauthVersion        string
	GoauthLocalPath      string
	GouploadsLocalPath   string
	GonotifyLocalPath    string
	GowebsocketLocalPath string
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Version) == "" && strings.TrimSpace(c.LocalPath) == "" {
		return errors.New("external consumer probe: version or local path is required")
	}
	if strings.TrimSpace(c.Version) != "" && strings.TrimSpace(c.LocalPath) != "" {
		return errors.New("external consumer probe: version and local path are mutually exclusive")
	}
	if strings.TrimSpace(c.GoauthVersion) != "" && strings.TrimSpace(c.GoauthLocalPath) != "" {
		return errors.New("external consumer probe: goauth version and local path are mutually exclusive")
	}

	if strings.TrimSpace(c.LocalPath) == "" && (strings.TrimSpace(c.GoauthLocalPath) != "" ||
		strings.TrimSpace(c.GouploadsLocalPath) != "" || strings.TrimSpace(c.GonotifyLocalPath) != "" ||
		strings.TrimSpace(c.GowebsocketLocalPath) != "") {
		return errors.New("external consumer probe: local dependencies require a local target")
	}

	return nil
}

func (c Config) BuildGoMod() (string, error) {
	cfg := c.normalized()
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	var builder strings.Builder
	_, _ = builder.WriteString("module ")
	_, _ = builder.WriteString(cfg.ProbeModule)
	_, _ = builder.WriteString("\n\ngo 1.27.0\n\ntoolchain go1.27.1\n\nrequire ")
	_, _ = builder.WriteString(cfg.ModulePath)
	_, _ = builder.WriteString(" ")
	_, _ = builder.WriteString(cfg.targetVersion())
	_, _ = builder.WriteString("\n")

	switch {
	case cfg.GoauthVersion != "":
		_, _ = builder.WriteString("require ")
		_, _ = builder.WriteString(cfg.GoauthModulePath)
		_, _ = builder.WriteString(" ")
		_, _ = builder.WriteString(cfg.GoauthVersion)
		_, _ = builder.WriteString("\n")
	case cfg.GoauthLocalPath != "":
		_, _ = builder.WriteString("require ")
		_, _ = builder.WriteString(cfg.GoauthModulePath)
		_, _ = builder.WriteString(" ")
		_, _ = builder.WriteString(LocalModuleVersion)
		_, _ = builder.WriteString("\n")
	}

	if cfg.LocalPath != "" {
		_, _ = builder.WriteString("\nreplace ")
		_, _ = builder.WriteString(cfg.ModulePath)
		_, _ = builder.WriteString(" => ")
		_, _ = builder.WriteString(strconv.Quote(filepath.Clean(cfg.LocalPath)))
		_, _ = builder.WriteString("\n")
	}

	if cfg.GoauthLocalPath != "" {
		_, _ = builder.WriteString("replace ")
		_, _ = builder.WriteString(cfg.GoauthModulePath)
		_, _ = builder.WriteString(" => ")
		_, _ = builder.WriteString(strconv.Quote(filepath.Clean(cfg.GoauthLocalPath)))
		_, _ = builder.WriteString("\n")
	}

	for _, dep := range []struct{ module, path string }{
		{"github.com/assurrussa/gouploads", cfg.GouploadsLocalPath},
		{"github.com/assurrussa/gonotify", cfg.GonotifyLocalPath},
		{"github.com/assurrussa/gowebsocket", cfg.GowebsocketLocalPath},
	} {
		if dep.path != "" {
			_, _ = builder.WriteString("replace " + dep.module + " => " + strconv.Quote(filepath.Clean(dep.path)) + "\n")
		}
	}

	return builder.String(), nil
}

func (c Config) BuildProbeTest() (string, error) {
	cfg := c.normalized()
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	content := strings.ReplaceAll(runnableProbeTest, DefaultModulePath, cfg.ModulePath)
	content = strings.ReplaceAll(content, DefaultGoauthModulePath, cfg.GoauthModulePath)

	return content, nil
}

const runnableProbeTest = `package probe

import (
	"context"
	"errors"
	"testing"

	goauth "github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"

	_ "github.com/assurrussa/goadmin/features/access"
	_ "github.com/assurrussa/goadmin/features/authmail"
	_ "github.com/assurrussa/goadmin/features/jobs"
	adminuploads "github.com/assurrussa/goadmin/features/uploads"
	_ "github.com/assurrussa/goadmin/features/queues"
	_ "github.com/assurrussa/goadmin/features/notifications"
	_ "github.com/assurrussa/goadmin/features/realtime"
	_ "github.com/assurrussa/goadmin/features/operations"
	_ "github.com/assurrussa/goadmin/features/users"
	adminhost "github.com/assurrussa/goadmin/host"
	_ "github.com/assurrussa/goadmin/hosttest"
	adminmigrations "github.com/assurrussa/goadmin/migrations"
	_ "github.com/assurrussa/goadmin/toolkit/datagrid"
	_ "github.com/assurrussa/goadmin/toolkit/formvalidator"
)

func TestHostAuthAdapterAndMigrationContract(t *testing.T) {
	_, err := adminhost.NewAuthAdapter(adminhost.AuthAdapterConfig{
		Runtime: goauth.Config{}, NotificationSender: goauth.NotificationSenderFunc(nil),
		NotificationWorker: postgres.NotificationWorkerConfig{},
	})
	if err == nil {
		t.Fatal("missing PostgreSQL dependencies must fail closed")
	}
	err = adminmigrations.Reset(
		context.Background(),
		adminmigrations.DatabaseConfig{},
		nil,
		postgres.ResetConfirmation("wrong"),
	)
	if !errors.Is(err, postgres.ErrResetConfirmationRequired) {
		t.Fatalf("unexpected reset validation error: %v", err)
	}

	_, err = adminhost.New(context.Background(), adminhost.Config{}, adminhost.Dependencies{})
	if err == nil {
		t.Fatal("missing admin PostgreSQL must fail closed")
	}
	_ = (*adminhost.Runtime).App
	_ = adminmigrations.Migrate
}

type audioStrategy struct{}

func (audioStrategy) CanUpload(context.Context, adminhost.UploadContext) error {
	return errors.New("compile-only strategy refuses runtime uploads")
}

func (audioStrategy) GetConfig(context.Context, adminhost.UploadContext) *adminhost.FileUploadConfig {
	return &adminhost.FileUploadConfig{
		MaxFileSize: 50 * 1024 * 1024,
		AllowedExtensions: []string{".mp3", ".wav"},
		AllowedMimeTypes: map[string][]string{
			".mp3": {"audio/mpeg"},
			".wav": {"audio/wav", "audio/x-wav", "audio/vnd.wave"},
		},
		UploadDir: "audio",
	}
}

func (audioStrategy) GetAfterJobs(context.Context, adminhost.UploadContext) ([]adminhost.FileEventAfterJob, error) {
	return nil, nil
}

func TestCustomUploadStrategyContract(t *testing.T) {
	var strategy adminhost.UploadStrategy = audioStrategy{}
	module := adminuploads.New(adminuploads.Config{
		Strategies: map[string]adminhost.UploadStrategy{"meditation-audio": strategy},
	})
	if module.Descriptor().Key != "uploads" {
		t.Fatal("custom strategies must use the canonical uploads module")
	}
}
`

func (c Config) normalized() Config {
	cfg := c
	cfg.ProbeModule = strings.TrimSpace(cfg.ProbeModule)
	cfg.ModulePath = strings.TrimSpace(cfg.ModulePath)
	cfg.Version = strings.TrimSpace(cfg.Version)
	cfg.LocalPath = strings.TrimSpace(cfg.LocalPath)
	cfg.GoauthModulePath = strings.TrimSpace(cfg.GoauthModulePath)
	cfg.GoauthVersion = strings.TrimSpace(cfg.GoauthVersion)
	cfg.GoauthLocalPath = strings.TrimSpace(cfg.GoauthLocalPath)
	cfg.GouploadsLocalPath = strings.TrimSpace(cfg.GouploadsLocalPath)
	cfg.GonotifyLocalPath = strings.TrimSpace(cfg.GonotifyLocalPath)
	cfg.GowebsocketLocalPath = strings.TrimSpace(cfg.GowebsocketLocalPath)

	if cfg.ProbeModule == "" {
		cfg.ProbeModule = DefaultProbeModule
	}
	if cfg.ModulePath == "" {
		cfg.ModulePath = DefaultModulePath
	}
	if cfg.GoauthModulePath == "" {
		cfg.GoauthModulePath = DefaultGoauthModulePath
	}

	return cfg
}

func (c Config) targetVersion() string {
	if strings.TrimSpace(c.Version) != "" {
		return strings.TrimSpace(c.Version)
	}

	return LocalModuleVersion
}
