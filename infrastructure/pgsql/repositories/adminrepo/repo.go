package adminrepo

import (
	"context"
	"fmt"

	"github.com/assurrussa/goauth"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
)

//go:generate options-gen -out-filename=repo_options.gen.go -from-struct=Options
type Options struct {
	pgsql      outbox.StoragePgsqlClient    `option:"mandatory" validate:"required"`
	trxManager outbox.StoragePgsqlTxManager `option:"mandatory" validate:"required"`
}

// HasAdminMembership is the goauth Runtime realm gate. Membership remains a
// host projection in administrations and is never inferred from Subject data.
func (r *Repo) HasAdminMembership(ctx context.Context, subjectID goauth.SubjectID) (bool, error) {
	if err := subjectID.Validate(); err != nil {
		return false, fmt.Errorf("validate admin subject id: %w", err)
	}
	var exists bool
	err := admintx.Wrap(r.pgsql.DB()).ScanOne(ctx, "admin.membership", &exists, `
		SELECT EXISTS (
			SELECT 1 FROM administrations
			WHERE subject_id = $1 AND deleted_at IS NULL
		)`, subjectID)
	if err != nil {
		return false, fmt.Errorf("check admin membership: %w", err)
	}

	return exists, nil
}

type Repo struct {
	Options
}

func Must(opts Options) *Repo {
	repo, err := New(opts)
	if err != nil {
		panic(fmt.Errorf("fatal user repo: %w", err))
	}

	return repo
}

func New(opts Options) (*Repo, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validate options: %w", err)
	}

	return &Repo{Options: opts}, nil
}
