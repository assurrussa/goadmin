package sso

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"

	browser "github.com/assurrussa/gossoclient"
)

const SchemaName = "goadmin_sso"

// Runtime borrows the canonical auth database and never closes its pool.
// Native linking, membership and RBAC remain the host's responsibility.
type Runtime struct {
	Config     Config
	Client     *browser.Client
	Authority  *Authority
	intents    intentStore
	csrfCookie string
	db         *sql.DB
}

// NewRuntime is opt-in. A disabled configuration performs no database access,
// discovery or credential validation. Enabled construction requires an already
// migrated store; no migrations or secret generation occur on this path.
func NewRuntime(ctx context.Context, cfg Config, db *sql.DB) (*Runtime, error) {
	if !cfg.Enabled {
		return nil, nil //nolint:nilnil // Disabled integration deliberately has no runtime or dependencies.
	}
	if ctx == nil || db == nil {
		return nil, errors.New("admin SSO database and context are required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	store, err := browser.NewPostgresStoreWithSchema(ctx, db, cfg.schema())
	if err != nil {
		return nil, errors.New("admin SSO durable schema is not ready; apply application migrations")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(cfg.StorageKey)
	if err != nil {
		return nil, errors.New("invalid admin SSO storage key")
	}
	opts := []browser.Option{
		browser.WithIssuer(cfg.Issuer), browser.WithClientID(cfg.ClientID),
		browser.WithOrigin(cfg.Origin), browser.WithCallback(cfg.Origin + callbackPath),
		browser.WithStore(store), browser.WithStorageKey(key), browser.WithClientSecret(cfg.ClientSecret),
		browser.WithCurrentSessionLogoutEndpoint(cfg.Issuer + "/oauth2/logout"),
	}
	if cfg.IssuerCAFile != "" {
		opts = append(opts, browser.WithIssuerCAFile(cfg.IssuerCAFile))
	}
	if cfg.ProjectID == "" {
		opts = append(opts, browser.WithAuthHubClientBoundProject())
	} else {
		opts = append(opts, browser.WithProjectID(cfg.ProjectID))
	}
	// Every mounted route must also enforce Config.AllowsRequest, including the
	// optional socket-peer restriction. Headers cannot enable either boundary.
	if cfg.HTTPSReverseProxy || cfg.TrustedProxyCIDRs != "" {
		opts = append(opts, browser.WithHTTPSReverseProxy())
	}
	client, err := browser.NewWithOptions(ctx, opts...)
	if err != nil {
		return nil, errors.New("admin SSO provider initialization failed")
	}
	authority := &Authority{Client: client, AllowsRequest: cfg.AllowsRequest, Allows: func(issuer, subject string) bool {
		return issuer == cfg.Issuer && subject != ""
	}}
	return &Runtime{
		Config: cfg, Client: client, Authority: authority,
		db: db, intents: postgresIntents{db: db, table: `"` + cfg.schema() + `".sso_link_intents`},
	}, nil
}

// Migrate runs only from the application's explicit migration workflow.
// Search path is transaction-local, not a mutation of shared pooled sessions.
func Migrate(ctx context.Context, db *sql.DB) error { return MigrateSchema(ctx, db, SchemaName) }

// MigrateSchema extends a dedicated existing schema without replacing sessions or links.
func MigrateSchema(ctx context.Context, db *sql.DB, schema string) error {
	if !schemaPattern.MatchString(schema) {
		return errors.New("invalid admin SSO schema")
	}
	if ctx == nil || db == nil {
		return errors.New("admin SSO migration database and context are required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Preserve the earlier host migration fence during rolling adoption.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(710241002)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS "`+schema+`"`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `SET LOCAL search_path = "`+schema+`"`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, browser.Schema()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, LinkSchema()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, returnSchema()); err != nil {
		return err
	}
	return tx.Commit()
}
