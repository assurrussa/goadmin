// Package gridworkflow provides the shared, administrator-owned table workflow.
package gridworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

const MaxPayloadBytes = 64 * 1024

var (
	ErrConflict      = errors.New("table preferences changed; reload before saving")
	namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
	viewIDPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type State struct {
	Search       string            `json:"search"`
	Filters      map[string]string `json:"filters"`
	SortBy       string            `json:"sortBy"`
	SortOrder    string            `json:"sortOrder"`
	Limit        int               `json:"limit"`
	Columns      []string          `json:"columns"`
	KnownColumns []string          `json:"knownColumns,omitempty"`
}
type View struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State State  `json:"state"`
}
type Payload struct {
	SchemaVersion int      `json:"schemaVersion"`
	Columns       []string `json:"columns"`
	KnownColumns  []string `json:"knownColumns,omitempty"`
	Views         []View   `json:"views"`
	DefaultViewID string   `json:"defaultViewId,omitempty"`
}
type Preferences struct {
	Owner   string  `json:"owner"`
	Version int64   `json:"version"`
	Payload Payload `json:"payload"`
}

// Store must implement an atomic version comparison on Save. Version zero creates.
type Store interface {
	Load(ctx context.Context, owner int64, namespace string) (Payload, int64, error)
	Save(ctx context.Context, owner int64, namespace string, version int64, payload Payload) (int64, error)
}

type PostgresStore struct{ db outbox.StoragePgsqlDBEngine }

func NewPostgresStore(db outbox.StoragePgsqlDBEngine) *PostgresStore { return &PostgresStore{db: db} }

func (s *PostgresStore) Load(ctx context.Context, owner int64, namespace string) (Payload, int64, error) {
	if s == nil || s.db == nil {
		return Payload{}, 0, errors.New("table preferences database unavailable")
	}
	var row struct {
		Payload []byte `db:"payload"`
		Version int64  `db:"version"`
	}
	err := admintx.Wrap(s.db).ScanOne(ctx, "table_preferences.load", &row,
		`SELECT payload, version FROM goadmin_admin_preferences WHERE admin_id=$1 AND namespace=$2`, owner, namespace)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payload{}, 0, nil
	}
	if err != nil {
		return Payload{}, 0, fmt.Errorf("load table preferences: %w", err)
	}
	var payload Payload
	if err = json.Unmarshal(row.Payload, &payload); err != nil {
		return Payload{}, 0, fmt.Errorf("decode table preferences: %w", err)
	}
	return payload, row.Version, nil
}

func (s *PostgresStore) Save(ctx context.Context, owner int64, namespace string, version int64, payload Payload) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("table preferences database unavailable")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	if len(encoded) > MaxPayloadBytes {
		return 0, errors.New("table preferences exceed 64 KiB")
	}
	var next int64
	if version == 0 {
		err = admintx.Wrap(s.db).ScanOne(ctx, "table_preferences.create", &next,
			`INSERT INTO goadmin_admin_preferences(admin_id,namespace,payload) VALUES ($1,$2,$3::jsonb)
    ON CONFLICT (admin_id,namespace) DO NOTHING RETURNING version`, owner, namespace, string(encoded))
	} else {
		err = admintx.Wrap(s.db).ScanOne(ctx, "table_preferences.update", &next,
			`UPDATE goadmin_admin_preferences SET payload=$3::jsonb,version=version+1,updated_at=NOW()
    WHERE admin_id=$1 AND namespace=$2 AND version=$4 RETURNING version`, owner, namespace, string(encoded), version)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrConflict
	}
	if err != nil {
		return 0, fmt.Errorf("save table preferences: %w", err)
	}
	return next, nil
}

type schema struct {
	columns  []datagrid.Column
	defaults State
}

func newSchema[T any](config datagrid.FilterConfig[T]) schema {
	return schema{
		columns:  config.Columns,
		defaults: State{SortBy: config.DefaultSort, SortOrder: config.DefaultOrder, Limit: config.PageSize},
	}
}

func (s schema) defaultColumns() []string {
	keys := make([]string, 0, len(s.columns))
	for _, c := range s.columns {
		keys = append(keys, c.Key)
	}
	return keys
}

func (s schema) defaultsPayload() Payload {
	return Payload{SchemaVersion: 1, Columns: s.defaultColumns(), KnownColumns: s.defaultColumns(), Views: []View{}}
}

func (s schema) column(key string) (datagrid.Column, bool) {
	for _, c := range s.columns {
		if c.Key == key {
			return c, true
		}
	}
	return datagrid.Column{}, false
}

func (s schema) validateColumns(keys []string) error {
	if len(keys) == 0 || len(keys) > len(s.columns) {
		return errors.New("choose at least one allowed column")
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if _, ok := s.column(key); !ok || seen[key] {
			return errors.New("unknown or duplicate column")
		}
		seen[key] = true
	}
	return nil
}

func (s schema) validateState(state State) error {
	if len(state.Search) > 4096 || !utf8.ValidString(state.Search) {
		return errors.New("invalid search")
	}
	if state.Limit != 10 && state.Limit != 25 && state.Limit != 50 && state.Limit != 100 {
		return errors.New("invalid page size")
	}
	if state.SortOrder != "asc" && state.SortOrder != "desc" {
		return errors.New("invalid sort order")
	}
	col, ok := s.column(state.SortBy)
	if (!ok || !col.Sortable) && state.SortBy != s.defaults.SortBy {
		return errors.New("invalid sort column")
	}
	if err := s.validateSelection(state.Columns, state.KnownColumns); err != nil {
		return err
	}
	for key, value := range state.Filters {
		col, ok := s.column(key)
		if !ok || !col.Filterable {
			return errors.New("unknown filter")
		}
		if len(value) > 4096 || !utf8.ValidString(value) {
			return errors.New("invalid filter value")
		}
		if value == "" {
			continue
		}
		switch col.Type {
		case "number":
			if _, err := strconv.Atoi(value); err != nil {
				return errors.New("invalid number filter")
			}
		case "boolean":
			if _, err := strconv.ParseBool(value); err != nil {
				return errors.New("invalid boolean filter")
			}
		case "date":
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return errors.New("invalid date filter")
			}
		}
	}
	return nil
}

func (s schema) validate(payload Payload) error {
	if payload.SchemaVersion != 1 {
		return errors.New("unsupported preferences schema")
	}
	if len(payload.Views) > 20 {
		return errors.New("at most 20 saved views are allowed")
	}
	if err := s.validateSelection(payload.Columns, payload.KnownColumns); err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, view := range payload.Views {
		if !viewIDPattern.MatchString(view.ID) || ids[view.ID] {
			return errors.New("invalid or duplicate view ID")
		}
		ids[view.ID] = true
		if strings.TrimSpace(view.Name) == "" || utf8.RuneCountInString(view.Name) > 80 || !utf8.ValidString(view.Name) {
			return errors.New("invalid view name")
		}
		if err := s.validateState(view.State); err != nil {
			return err
		}
	}
	if payload.DefaultViewID != "" && !ids[payload.DefaultViewID] {
		return errors.New("default view is missing")
	}
	return nil
}

func (s schema) validateSelection(visible, known []string) error {
	if err := s.validateColumns(visible); err != nil {
		return err
	}
	if len(known) == 0 {
		return nil // Legacy payloads establish a baseline when next saved.
	}
	if err := s.validateColumns(known); err != nil {
		return err
	}
	for _, key := range visible {
		if !slices.Contains(known, key) {
			return errors.New("visible column is missing from the schema snapshot")
		}
	}
	return nil
}

func (s schema) reconcileColumns(keys, known []string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := s.column(key); ok && !slices.Contains(out, key) {
			out = append(out, key)
		}
	}
	if len(known) > 0 {
		for _, key := range s.defaultColumns() {
			if !slices.Contains(known, key) && !slices.Contains(out, key) {
				out = append(out, key)
			}
		}
	}
	if len(out) == 0 {
		return s.defaultColumns()
	}
	return out
}

// Reconcile schema changes without writing or changing the stored version.
// Each view owns its snapshot, so updating table preferences cannot hide new
// columns from a view that has not been applied yet. Missing legacy snapshots
// adopt today's baseline without guessing which omitted columns were hidden.
func (s schema) reconcile(p Payload) Payload {
	if p.SchemaVersion != 1 {
		return s.defaultsPayload()
	}
	currentColumns := s.defaultColumns()
	p.Columns = s.reconcileColumns(p.Columns, p.KnownColumns)
	p.KnownColumns = slices.Clone(currentColumns)
	p.Views = slices.Clone(p.Views)
	if p.Views == nil {
		p.Views = []View{}
	}
	if len(p.Views) > 20 {
		p.Views = p.Views[:20]
	}
	for i := range p.Views {
		st := &p.Views[i].State
		st.Columns = s.reconcileColumns(st.Columns, st.KnownColumns)
		st.KnownColumns = slices.Clone(currentColumns)
		fields := map[string]string{}
		for k, v := range st.Filters {
			if c, ok := s.column(k); ok && c.Filterable {
				fields[k] = v
			}
		}
		st.Filters = fields
		if c, ok := s.column(st.SortBy); !ok || !c.Sortable {
			st.SortBy = s.defaults.SortBy
		}
		if st.SortOrder != "asc" && st.SortOrder != "desc" {
			st.SortOrder = s.defaults.SortOrder
		}
		if st.Limit != 10 && st.Limit != 25 && st.Limit != 50 && st.Limit != 100 {
			st.Limit = s.defaults.Limit
		}
	}
	found := false
	for _, v := range p.Views {
		found = found || v.ID == p.DefaultViewID
	}
	if !found {
		p.DefaultViewID = ""
	}
	return p
}
