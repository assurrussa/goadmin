package filerepo

import (
	"fmt"

	uploadhost "github.com/assurrussa/gouploads/host"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
)

//go:generate options-gen -out-filename=repo_options.gen.go -from-struct=Options
type Options struct {
	pgsql      outbox.StoragePgsqlClient    `option:"mandatory" validate:"required"`
	trxManager outbox.StoragePgsqlTxManager `option:"mandatory" validate:"required"`
	repo       *uploadhost.FileRepo         `option:"mandatory" validate:"required"`
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
