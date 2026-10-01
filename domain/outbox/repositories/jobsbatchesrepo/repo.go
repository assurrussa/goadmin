package jobsbatchesrepo

import (
	"fmt"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
)

//go:generate options-gen -out-filename=repo_options.gen.go -from-struct=Options
type Options struct {
	pgsql outbox.StoragePgsqlClient `option:"mandatory" validate:"required"`
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
		return nil, fmt.Errorf("validate repo jobsrepo: %w", err)
	}

	return &Repo{opts}, nil
}
