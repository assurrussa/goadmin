package shared

import (
	"context"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	"github.com/assurrussa/goadmin/models"
)

func MustGetAdminAuth(ctx context.Context) *models.SessionAdmin {
	return adminmiddleware.MustGetAdminAuth(ctx)
}

func GetAdminAuth(ctx context.Context) *models.SessionAdmin {
	return adminmiddleware.GetAdminAuth(ctx)
}
