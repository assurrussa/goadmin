//go:build integration

package users //nolint:testpackage // Acceptance exercises the actual shared HTTP command and managed transaction.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/userrepo"
	"github.com/assurrussa/goadmin/internal/admintx"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/gridworkflow"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/internal/tablecommands"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/tests"
)

const workflowPassword = "Q7!user-workflow-2026" //nolint:gosec // Synthetic credential used only in isolated test databases.

type workflowFixture struct {
	db      outbox.StoragePgsqlClient
	adapter *authcore.Adapter
	admins  *adminrepo.Repo
	users   *userrepo.Repo
	audit   *adminactionauditrepo.Repo
	harness *adminappt.App
	account goauth.Account
	id      int64
}

func newWorkflowFixture(t *testing.T) *workflowFixture {
	t.Helper()
	ctx := t.Context()
	db, _, cleanup := tests.PrepareDB(ctx, t, "UserCommandConcurrency")
	tx := outbox.PgsqlTrxNew(db.DB())
	admins := adminrepo.Must(adminrepo.NewOptions(db, tx))
	adapter, err := authcore.New(authcore.Config{
		Database: db, Memberships: admins, Runtime: tests.AuthRuntimeConfig(t),
		NotificationSender: tests.AuthNotificationSender(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close()); cleanup(context.Background()) })
	account, err := adapter.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{
		Email: "command-target@example.test", Password: workflowPassword,
	})
	require.NoError(t, err)
	var id int64
	require.NoError(t, db.DB().ScanOne(ctx, "test.workflow.user", &id,
		`INSERT INTO users(subject_id,uuid,created_at,updated_at) VALUES($1,$2,NOW(),NOW()) RETURNING id`,
		account.Subject.ID, identity.NewUserID()))
	f := &workflowFixture{
		db: db, adapter: adapter, admins: admins, users: userrepo.Must(userrepo.NewOptions(db, tx)),
		audit:   adminactionauditrepo.Must(adminactionauditrepo.NewOptions(db)),
		harness: adminappt.NewAppTest(t), account: account, id: id,
	}
	f.harness.App.SetCommandTransaction(func(ctx context.Context, fn func(context.Context) error) error {
		return f.managed(ctx, fn)
	})
	return f
}

func (f *workflowFixture) managed(ctx context.Context, fn func(context.Context) error) error {
	return f.adapter.Runtime().InAuthTransaction(ctx, func(txCtx context.Context) error {
		executor, err := f.adapter.Runtime().SQLExecutor(txCtx)
		if err != nil {
			return err
		}
		return fn(admintx.WithExecutor(txCtx, executor, f.db.DB().Pool()))
	})
}

func (f *workflowFixture) httpApp(audit adminactionauditrepo.Writer) *fiber.App {
	f.harness.App.SetGridWorkflowDependencies(f.db.DB(), audit)
	handler := NewHandler(f.harness.App, f.users, f.adapter.Runtime())
	app := fiber.New(fiber.Config{ErrorHandler: f.harness.App.HTTPManager().MiddlewareErrorListener()})
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 9, SubjectID: f.account.Subject.ID})
		return c.Next()
	})
	app.Post("/users/bulk", gridworkflow.APIHandler(handler.Bulk))
	return app
}

type workflowHTTPResult struct {
	results []tablecommands.Result
	err     error
}

func runWorkflowBulk(ctx context.Context, app *fiber.App, id int64) workflowHTTPResult {
	body := fmt.Sprintf(`{"action":"delete","ids":["%d"]}`, id)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/users/bulk", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response, err := app.Test(req, fiber.TestConfig{Timeout: 15 * time.Second})
	if err != nil {
		return workflowHTTPResult{err: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return workflowHTTPResult{err: fmt.Errorf("bulk status %d", response.StatusCode)}
	}
	var payload struct {
		Results []tablecommands.Result `json:"results"`
	}
	err = json.NewDecoder(response.Body).Decode(&payload)
	return workflowHTTPResult{results: payload.Results, err: err}
}

func assertWorkflowResult(t *testing.T, r workflowHTTPResult, status string) {
	t.Helper()
	require.NoError(t, r.err)
	require.Len(t, r.results, 1)
	require.Equal(t, status, r.results[0].Status)
}

func (f *workflowFixture) assertState(t *testing.T, status goauth.SubjectStatus, deleted bool, auditCount int) {
	t.Helper()
	account, err := f.adapter.Runtime().GetAccount(t.Context(), f.account.Subject.ID)
	require.NoError(t, err)
	require.Equal(t, status, account.Subject.Status)
	projection, err := f.users.GetByID(t.Context(), f.id)
	require.NoError(t, err)
	require.Equal(t, deleted, projection.DeletedAt.Valid)
	var count int
	require.NoError(t, f.db.DB().ScanOne(t.Context(), "test.workflow.audit_count", &count,
		`SELECT count(*) FROM admin_action_audit WHERE target_type='user' AND target_id=$1`, strconv.FormatInt(f.id, 10)))
	require.Equal(t, auditCount, count)
}

// A real audit insert is held before commit, after the actual command has locked
// the subject and changed both canonical state and user projection.
type workflowAuditGate struct {
	db      outbox.StoragePgsqlClient
	writer  adminactionauditrepo.Writer
	entered chan int
	release chan struct{}
	once    sync.Once
}

func (g *workflowAuditGate) RecordAdminAction(ctx context.Context, r adminactionauditrepo.Record) error {
	if err := g.writer.RecordAdminAction(ctx, r); err != nil {
		return err
	}
	var pid int
	if err := admintx.Wrap(g.db.DB()).ScanOne(ctx, "test.workflow.audit_pid", &pid, "SELECT pg_backend_pid()"); err != nil {
		return err
	}
	g.once.Do(func() { g.entered <- pid })
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func workflowWaitBlocked(t *testing.T, ctx context.Context, db outbox.StoragePgsqlClient, blocker int) {
	t.Helper()
	for {
		var pid int
		require.NoError(t, db.DB().ScanOne(ctx, "test.workflow.blocked", &pid, `SELECT COALESCE((SELECT pid FROM pg_stat_activity
   WHERE $1=ANY(pg_blocking_pids(pid)) AND wait_event_type='Lock' AND query LIKE '%auth_subjects%' LIMIT 1),0)`, blocker))
		if pid != 0 {
			t.Logf("PostgreSQL confirmed subject-lock waiter pid=%d blocked by transaction pid=%d", pid, blocker)
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("actual competing subject query never entered a PostgreSQL lock wait")
			return
		default:
		}
	}
}

func TestWorkflowBulkDeleteVersusProvisionAccountBothOrders(t *testing.T) {
	t.Run("user command locks first and provisioning is denied", func(t *testing.T) {
		f := newWorkflowFixture(t)
		ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
		defer cancel()
		gate := &workflowAuditGate{db: f.db, writer: f.audit, entered: make(chan int, 1), release: make(chan struct{})}
		var release sync.Once
		defer release.Do(func() { close(gate.release) })
		app := f.httpApp(gate)
		bulkDone := make(chan workflowHTTPResult, 1)
		go func() { bulkDone <- runWorkflowBulk(ctx, app, f.id) }()
		var blocker int
		select {
		case blocker = <-gate.entered:
		case <-ctx.Done():
			t.Fatal("user command did not reach audit with subject lock")
		}
		provisionDone := make(chan error, 1)
		go func() {
			_, err := f.admins.ProvisionAccount(ctx, f.account, identity.NewUserID())
			provisionDone <- err
		}()
		workflowWaitBlocked(t, ctx, f.db, blocker)
		release.Do(func() { close(gate.release) })
		assertWorkflowResult(t, <-bulkDone, "succeeded")
		require.ErrorIs(t, <-provisionDone, goauth.ErrMembershipDenied)
		f.assertState(t, goauth.SubjectStatusDisabled, true, 1)
		member, err := f.admins.HasAdminMembership(ctx, f.account.Subject.ID)
		require.NoError(t, err)
		require.False(t, member)
	})
	t.Run("provisioning locks first and bulk skips active administrator", func(t *testing.T) {
		f := newWorkflowFixture(t)
		ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
		defer cancel()
		entered, release := make(chan int, 1), make(chan struct{})
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })
		provisionDone := make(chan error, 1)
		go func() {
			provisionDone <- f.managed(ctx, func(txCtx context.Context) error {
				if _, err := f.admins.ProvisionAccount(txCtx, f.account, identity.NewUserID()); err != nil {
					return err
				}
				var pid int
				if err := admintx.Wrap(f.db.DB()).ScanOne(txCtx, "test.workflow.provision_pid", &pid, "SELECT pg_backend_pid()"); err != nil {
					return err
				}
				entered <- pid
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}()
		var blocker int
		select {
		case blocker = <-entered:
		case <-ctx.Done():
			t.Fatal("ProvisionAccount did not complete under the managed transaction")
		}
		app := f.httpApp(f.audit)
		bulkDone := make(chan workflowHTTPResult, 1)
		go func() { bulkDone <- runWorkflowBulk(ctx, app, f.id) }()
		workflowWaitBlocked(t, ctx, f.db, blocker)
		releaseOnce.Do(func() { close(release) })
		require.NoError(t, <-provisionDone)
		assertWorkflowResult(t, <-bulkDone, "skipped")
		f.assertState(t, goauth.SubjectStatusActive, false, 0)
		// An already committed active membership also remains ineligible.
		assertWorkflowResult(t, runWorkflowBulk(ctx, app, f.id), "skipped")
		f.assertState(t, goauth.SubjectStatusActive, false, 0)
	})
}

func TestWorkflowHTTPCommandRollsBackStatusProjectionAndAudit(t *testing.T) {
	f := newWorkflowFixture(t)
	app := f.httpApp(f.audit)
	_, err := f.db.DB().Exec(t.Context(), "test.workflow.reject_audit",
		`CREATE FUNCTION reject_user_audit() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'deliberate audit failure'; END $$;
 CREATE TRIGGER reject_user_audit BEFORE INSERT ON admin_action_audit FOR EACH ROW EXECUTE FUNCTION reject_user_audit();`)
	require.NoError(t, err)
	assertWorkflowResult(t, runWorkflowBulk(t.Context(), app, f.id), "unknown")
	f.assertState(t, goauth.SubjectStatusActive, false, 0)
	_, err = f.db.DB().Exec(t.Context(), "test.workflow.allow_audit",
		`DROP TRIGGER reject_user_audit ON admin_action_audit; DROP FUNCTION reject_user_audit();`)
	require.NoError(t, err)
	assertWorkflowResult(t, runWorkflowBulk(t.Context(), app, f.id), "succeeded")
	f.assertState(t, goauth.SubjectStatusDisabled, true, 1)
	var action string
	require.NoError(t, f.db.DB().ScanOne(t.Context(), "test.workflow.audit_action", &action,
		`SELECT action FROM admin_action_audit`))
	require.Equal(t, "user.soft_deleted", action)
}
