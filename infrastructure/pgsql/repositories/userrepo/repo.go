package userrepo

import (
	"errors"
	"fmt"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
)

type Options struct {
	pgsql      outbox.StoragePgsqlClient
	trxManager outbox.StoragePgsqlTxManager
}

func NewOptions(pgsql outbox.StoragePgsqlClient, trxManager outbox.StoragePgsqlTxManager) Options {
	return Options{
		pgsql:      pgsql,
		trxManager: trxManager,
	}
}

func (o Options) Validate() error {
	switch {
	case o.pgsql == nil:
		return errors.New("field `pgsql` did not pass the test: required")
	case o.trxManager == nil:
		return errors.New("field `trxManager` did not pass the test: required")
	default:
		return nil
	}
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
