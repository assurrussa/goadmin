package shared_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/models"
	adminshared "github.com/assurrussa/goadmin/shared"
)

func TestGetAdminAuth(t *testing.T) {
	t.Run("returns nil if no admin in context", func(t *testing.T) {
		ctx := context.Background()
		if got := adminshared.GetAdminAuth(ctx); got != nil {
			t.Errorf("expected nil, got %#v", got)
		}
	})

	t.Run("returns admin if present in context", func(t *testing.T) {
		admin := &models.SessionAdmin{
			ID:       42,
			Username: "testuser",
			Name:     "Test",
			LastName: "User",
			Email:    "test@example.com",
			Roles:    []string{"admin"},
			Permissions: map[string][]string{
				"blog": {"read", "write"},
			},
		}
		//nolint:staticcheck //it's need
		ctx := context.WithValue(context.Background(), session.AuthAdminKey.String(), admin)
		got := adminshared.GetAdminAuth(ctx)
		if !reflect.DeepEqual(got, admin) {
			t.Errorf("expected %#v, got %#v", admin, got)
		}
	})
}

func TestMustGetAdminAuth(t *testing.T) {
	assert.Panics(t, func() {
		adminshared.MustGetAdminAuth(context.Background())
	})
	assert.NotPanics(t, func() {
		admin := &models.SessionAdmin{
			ID:       42,
			Username: "testuser",
			Name:     "Test",
			LastName: "User",
			Email:    "test@example.com",
			Roles:    []string{"admin"},
			Permissions: map[string][]string{
				"blog": {"read", "write"},
			},
		}
		//nolint:staticcheck //it's need
		ctx := context.WithValue(context.Background(), session.AuthAdminKey.String(), admin)
		adminshared.MustGetAdminAuth(ctx)
	})
}
