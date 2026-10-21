package externalconsumer_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/assurrussa/goadmin/host"
)

type directoryPermissionStub struct{}

func (directoryPermissionStub) CheckPermission(context.Context, string, host.PermissionKey) error {
	return host.ErrAdminDirectoryDenied
}

func TestAdminDirectoryUsesOnlySupportedPublicTypes(t *testing.T) {
	directory, err := host.NewAdminDirectory(new(sql.DB), directoryPermissionStub{}, host.NewPermissionKey("cms-team", "manage"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = directory.Search(t.Context(), host.Actor{}, "query", 20)
	if !errors.Is(err, host.ErrAdminDirectoryDenied) {
		t.Fatal(err)
	}
	if err = directory.LockActive(t.Context(), nil, ""); !errors.Is(err, host.ErrAdminDirectoryInvalid) {
		t.Fatal(err)
	}
}
