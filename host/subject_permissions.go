package host

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/assurrussa/goauth/postgres"
	"github.com/jackc/pgx/v5/stdlib"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

var ErrSubjectPermissionDenied = errors.New("goadmin subject permission denied")

type subjectPermissionGuard interface {
	SubjectGuardCheck(
		ctx context.Context,
		subjectID string,
		key integrationroles.PermissionKey,
		opts ...integrationroles.PermissionGuardOption,
	) bool
}

// SubjectPermissionChecker authorizes a canonical subject without exposing
// goauth role repositories or session claims to embedded features.
type SubjectPermissionChecker struct {
	guard        subjectPermissionGuard
	rolesGuard   *integrationroles.GuardService
	rolesService *integrationroles.Service
	db           *sql.DB
}

// NewSubjectPermissionChecker builds the shared roles guard used by goadmin
// and transport-neutral embedded features.
func NewSubjectPermissionChecker(
	ctx context.Context,
	db StoragePgsqlClient,
	_ StoragePgsqlTxManager,
	lg Logger,
) (*SubjectPermissionChecker, error) {
	if ctx == nil {
		return nil, errors.New("subject permission checker context is required")
	}
	if db == nil || db.DB() == nil || db.DB().Pool() == nil {
		return nil, errors.New("subject permission checker PostgreSQL pool is required")
	}
	sqlDB := stdlib.OpenDBFromPool(db.DB().Pool())
	stableRoles, err := postgres.NewRBAC(sqlDB, nil)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("build subject permission RBAC adapter: %w", err)
	}
	rolesService := integrationroles.NewRoleService(stableRoles)
	listSubjectRoles := integrationroles.MustListSubjectRolesUseCase(rolesService)
	guard, err := integrationroles.NewGuardService(listSubjectRoles, rolesService, lg)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("build subject permission guard: %w", err)
	}
	return &SubjectPermissionChecker{
		guard: guard, rolesGuard: guard, rolesService: rolesService, db: sqlDB,
	}, nil
}

func (c *SubjectPermissionChecker) CheckPermission(
	ctx context.Context,
	subjectID string,
	key PermissionKey,
) error {
	if c == nil || c.guard == nil || ctx == nil || subjectID == "" || key.IsZero() {
		return ErrSubjectPermissionDenied
	}
	if !c.guard.SubjectGuardCheck(ctx, subjectID, key) {
		return ErrSubjectPermissionDenied
	}
	return nil
}

func (c *SubjectPermissionChecker) validForInstall() bool {
	return c != nil && c.guard != nil && c.rolesGuard != nil && c.rolesService != nil
}

func subjectPermissionCheckerFromRoles(
	rolesService *integrationroles.RoleService,
	lg Logger,
) (*SubjectPermissionChecker, error) {
	listSubjectRoles := integrationroles.MustListSubjectRolesUseCase(rolesService)
	guard, err := integrationroles.NewGuardService(
		listSubjectRoles,
		rolesService,
		lg,
	)
	if err != nil {
		return nil, err
	}

	return &SubjectPermissionChecker{guard: guard, rolesGuard: guard, rolesService: rolesService}, nil
}
