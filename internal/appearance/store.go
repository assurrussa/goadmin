package appearance

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/adminpreferences"
	"github.com/assurrussa/goadmin/internal/admintx"
)

type BrandStore struct{ db outbox.StoragePgsqlDBEngine }

func NewBrandStore(db outbox.StoragePgsqlDBEngine) *BrandStore { return &BrandStore{db: db} }
func (s *BrandStore) Version(ctx context.Context) (int64, error) {
	var v int64
	err := admintx.Wrap(s.db).ScanOne(ctx, "branding.version", &v, `SELECT version FROM goadmin_branding WHERE singleton=true`)
	return v, err
}

func (s *BrandStore) Load(ctx context.Context) (Snapshot, error) {
	var row struct {
		Version   int64  `db:"version"`
		Name      string `db:"name"`
		Accent    string `db:"accent_color"`
		AccentEnd string `db:"accent_color_end"`
		Light     []byte `db:"logo_light"`
		Dark      []byte `db:"logo_dark"`
	}
	err := admintx.Wrap(s.db).ScanOne(ctx, "branding.load", &row,
		`SELECT version,name,accent_color,accent_color_end,logo_light,logo_dark FROM goadmin_branding WHERE singleton=true`)
	return Snapshot{
		Version: row.Version, Name: row.Name, AccentColor: row.Accent, AccentColorEnd: row.AccentEnd,
		LogoLight: string(row.Light), LogoDark: string(row.Dark),
	}, err
}

func (s *BrandStore) Save(ctx context.Context, version int64, next Snapshot) (int64, error) {
	var v int64
	err := admintx.Wrap(s.db).ScanOne(ctx, "branding.save", &v,
		`UPDATE goadmin_branding SET version=version+1,name=$2,accent_color=$3,logo_light=$4,logo_dark=$5,
accent_color_end=$6,updated_at=NOW()
WHERE singleton=true AND version=$1 RETURNING version`,
		version, next.Name, next.AccentColor, []byte(next.LogoLight), []byte(next.LogoDark), next.AccentColorEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, adminpreferences.ErrConflict
	}
	return v, err
}
