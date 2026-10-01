//go:build integration

package adminactionauditrepo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	auditrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/tests"
)

func TestAuditWriterParticipatesInTransaction(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(ctx, t, "AdminActionAudit")
	t.Cleanup(func() { cleanup(ctx) })
	tx := adminhost.NewTxManager(db)
	writer := auditrepo.Must(auditrepo.NewOptions(db))

	err := tx.RunInTx(ctx, func(txCtx context.Context) error {
		if err := writer.RecordAdminAction(txCtx, validAuditRecord()); err != nil {
			return err
		}
		return writer.RecordAdminAction(txCtx, auditrepo.Record{
			ActorAdminID: 9,
			Action:       strings.Repeat("a", 101),
			TargetType:   "queue_job",
			TargetID:     "job-1",
		})
	})
	require.Error(t, err, "the database constraint must reject the second audit event")
	require.EqualValues(t, 0, auditCount(t, db))

	require.NoError(t, tx.RunInTx(ctx, func(txCtx context.Context) error {
		return writer.RecordAdminAction(txCtx, validAuditRecord())
	}))
	require.EqualValues(t, 1, auditCount(t, db))
}

func validAuditRecord() auditrepo.Record {
	return auditrepo.Record{
		ActorAdminID: 9,
		Action:       auditrepo.ActionQueueJobDeleted,
		TargetType:   "queue_job",
		TargetID:     "job-1",
	}
}

func auditCount(t *testing.T, db outbox.StoragePgsqlClient) int64 {
	t.Helper()
	var count int64
	query := outbox.BuilderDollar().Select("COUNT(*)").From("admin_action_audit")
	require.NoError(t, db.DB().Getx(context.Background(), "adminactionaudit.count", &count, query))
	return count
}
