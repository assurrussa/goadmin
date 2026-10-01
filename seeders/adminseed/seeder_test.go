//go:build integration

package adminseed_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	adminhost "github.com/assurrussa/goadmin/host"
	gosharedtests "github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/seeders/adminseed"
	"github.com/assurrussa/goadmin/tests"
)

const seedPassword = "Q7!river-snow-2026"

type TestRepoSuite struct {
	suite.Suite

	db      outbox.StoragePgsqlClient
	cleanUp func(context.Context)
	auth    *adminhost.AuthAdapter
	seed    *adminseed.Seed
}

func NewTestSeedSuite(t *testing.T, opts ...tests.OptionDatabase) (context.Context, context.CancelFunc, *TestRepoSuite) {
	return gosharedtests.NewSuite[*TestRepoSuite](t, func(t *testing.T, ctx context.Context) *TestRepoSuite {
		db, _, cleanUp := tests.PrepareDB(ctx, t, "TestAdminSeedSuite", opts...)
		tx := adminhost.NewTxManager(db)
		auth, err := adminhost.NewAuthAdapter(adminhost.AuthAdapterConfig{
			Database: db, TxManager: tx, Runtime: tests.AuthRuntimeConfig(t),
			NotificationSender: tests.AuthNotificationSender(),
		})
		require.NoError(t, err)
		seed := adminhost.NewAdminSeed(auth, adminseed.Account{
			Email:    "seed-admin@example.test",
			Username: "seed-admin",
			Name:     "Seed Admin",
			LastName: "Test",
			Password: seedPassword,
		})

		return &TestRepoSuite{db: db, cleanUp: cleanUp, auth: auth, seed: seed}
	})
}

func (s *TestRepoSuite) close(ctx context.Context) {
	s.Require().NoError(s.auth.Close())
	s.cleanUp(ctx)
}

func TestSeedIntegration_Handle(t *testing.T) {
	ctx, _, ts := NewTestSeedSuite(t)
	defer ts.close(ctx)

	require.Equal(t, "adminseed", ts.seed.Name())

	assert.Equal(t, 0, countRows(t, ctx, ts.db, "administrations"))
	require.NoError(t, integrationroles.NewSeed(ts.db).Handle(ctx))
	require.NoError(t, ts.seed.Handle(ctx))
	// A repeated trusted seed is idempotent and proves the same credential.
	require.NoError(t, ts.seed.Handle(ctx))

	assert.Equal(t, 1, countRows(t, ctx, ts.db, "administrations"))
	assert.GreaterOrEqual(t, countRows(t, ctx, ts.db, "auth_roles"), 1)
	assert.Equal(t, 1, countRows(t, ctx, ts.db, "auth_subject_roles"))
	assert.Equal(t, 1, countRows(t, ctx, ts.db, "auth_subjects"))
	assert.Equal(t, 1, countRows(t, ctx, ts.db, "auth_identifiers"))
	assert.Equal(t, 1, countRows(t, ctx, ts.db, "auth_local_credentials"))

	type identityRow struct {
		SubjectID string     `db:"subject_id"`
		Email     string     `db:"display_value"`
		Verified  *time.Time `db:"verified_at"`
		PHC       string     `db:"password_phc"`
	}
	query := outbox.BuilderDollar().
		Select("i.subject_id", "i.display_value", "i.verified_at", "c.password_phc").
		From("auth_identifiers i").
		Join("auth_local_credentials c ON c.subject_id = i.subject_id").
		Where("i.scheme = 'email' AND i.is_primary").
		Limit(1)
	var identity identityRow
	require.NoError(t, ts.db.DB().ScanOnex(ctx, "seed.identity", &identity, query))
	assert.Equal(t, "seed-admin@example.test", identity.Email)
	assert.NotNil(t, identity.Verified)
	assert.True(t, strings.HasPrefix(identity.PHC, "$argon2id$"))
	assert.NotContains(t, identity.PHC, seedPassword)
}

func TestSeedIntegration_Handle_RequireExistingSuperAdminRole(t *testing.T) {
	ctx, _, ts := NewTestSeedSuite(t)
	defer ts.close(ctx)

	err := ts.seed.Handle(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "super admin role")
	assert.Equal(t, 0, countRows(t, ctx, ts.db, "administrations"))
	assert.Equal(t, 0, countRows(t, ctx, ts.db, "auth_subjects"))
}

func countRows(t *testing.T, ctx context.Context, db outbox.StoragePgsqlClient, table string) int {
	t.Helper()
	query := outbox.BuilderDollar().Select("count(*)").From(table)
	var count int
	require.NoError(t, db.DB().ScanOnex(ctx, "seed.count", &count, query))

	return count
}
