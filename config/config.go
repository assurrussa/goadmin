package config

import (
	"path/filepath"
	"time"

	"github.com/assurrussa/goadmin/internal/envs"
)

const (
	AdminCanonicalSessionBackendRedis = "redis"
	AdminCanonicalSessionBackendPGSQL = "pgsql"
)

type Config struct {
	Env                     string        `toml:"app_env" long:"app-env" env:"APP_ENV" value-default:"production" validate:"required,oneof=local development staging production"`                                                //nolint:lll // it's config
	BaseDomain              string        `toml:"app_domain" long:"app-domain" env:"APP_DOMAIN" value-default:"localhost" validate:"required"`                                                                                   //nolint:lll // it's config
	BaseDomainURL           string        `toml:"app_domain_url" long:"app-domain-url" env:"APP_DOMAIN_URL" value-default:"https://localhost"`                                                                                   //nolint:lll // it's config
	Domain                  string        `toml:"domain" long:"server-admin-domain" env:"APP_ADMIN_DOMAIN" value-default:"admin.localhost" validate:"required"`                                                                  //nolint:lll // it's config
	DomainURL               string        `toml:"domain_url" long:"server-admin-domain-url" env:"APP_ADMIN_DOMAIN_URL" value-default:"https://admin.localhost" validate:"required"`                                              //nolint:lll // it's config
	Addr                    string        `toml:"addr" long:"server-admin-addr" env:"SERVER_ADMIN_ADDR" value-default:":8079" validate:"required,hostname_port"`                                                                 //nolint:lll // it's config
	BasicAuth               []string      `toml:"basic_auth" long:"server-admin-basic-auth" env:"SERVER_ADMIN_BASIC_AUTH" value-default:"[]"`                                                                                    //nolint:lll // it's config
	CSRFTokenName           string        `toml:"csrf_token" long:"server-admin-csrf-name" env:"APP_ADMIN_CSRF_TOKEN_NAME" value-default:"csrf_token"`                                                                           //nolint:lll // it's config
	CSRFTokenTTL            time.Duration `toml:"csrf_ttl" long:"server-admin-csrf-ttl" env:"APP_ADMIN_CSRF_TOKEN_TTL" value-default:"1h"`                                                                                       //nolint:lll // it's config
	SessionInactiveTTL      time.Duration `toml:"session_inactive_ttl" long:"server-admin-session-inactive-ttl" env:"APP_ADMIN_SESSION_INACTIVE_TTL" value-default:"30m"`                                                        //nolint:lll // it's config
	SessionTTL              time.Duration `toml:"session_ttl" long:"server-admin-session-ttl" env:"APP_ADMIN_SESSION_TTL" value-default:"24h"`                                                                                   //nolint:lll // it's config
	SessionName             string        `toml:"session_name" long:"server-admin-session-name" env:"APP_ADMIN_SESSION_NAME" value-default:"sess_id"`                                                                            //nolint:lll // it's config
	CanonicalSessionBackend string        `toml:"canonical_session_backend" long:"server-admin-canonical-session-backend" env:"APP_ADMIN_CANONICAL_SESSION_BACKEND" value-default:"pgsql" validate:"required,oneof=redis pgsql"` //nolint:lll // it's config
	PublicRoot              string        `toml:"public_root" long:"server-admin-public-root" env:"GO_ADMIN_PUBLIC_ROOT" value-default:"publicstaticdata"`                                                                       //nolint:lll // it's config
	StaticDataRoot          string        `toml:"static_data_root" long:"server-admin-static-data-root" env:"GO_ADMIN_STATIC_DATA_ROOT" value-default:"publicdata"`                                                              //nolint:lll // it's config
	HotFile                 string        `toml:"hot_file" long:"server-admin-hot-file" env:"GO_ADMIN_HOT_FILE" value-default:"publicstaticdata/hot"`                                                                            //nolint:lll // it's config
	ViewsRoot               string        `toml:"views_root" long:"server-admin-views-root" env:"GO_ADMIN_VIEWS_ROOT" value-default:"views"`                                                                                     //nolint:lll // it's config
}

func (c Config) IsProduction() bool {
	return envs.IsProd(c.Env)
}

func (c Config) IsStaging() bool {
	return envs.IsStage(c.Env)
}

func (c Config) IsDev() bool {
	return envs.IsDev(c.Env)
}

func (c Config) IsLocal() bool {
	return envs.IsLocalOrDev(c.Env)
}

func (c Config) StaticDataPath(elem ...string) string {
	root := c.StaticDataRoot
	if root == "" {
		root = "publicdata"
	}

	if len(elem) == 0 {
		return root
	}

	parts := append([]string{root}, elem...)
	return filepath.Join(parts...)
}
