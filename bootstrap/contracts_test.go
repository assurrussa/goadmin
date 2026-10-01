package bootstrap_test

import (
	"testing"

	"github.com/assurrussa/goadmin/bootstrap"
	adminrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	userrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/userrepo"
)

func TestProjectionAndCanonicalAuthContractsCompile(t *testing.T) {
	t.Helper()

	var _ bootstrap.AdminRepo = (*adminrepo.Repo)(nil)
	var _ bootstrap.UserRepo = (*userrepo.Repo)(nil)
}
