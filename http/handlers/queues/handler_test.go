package queues //nolint:testpackage // transactional retry tests exercise the package handler directly

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	queuesmocks "github.com/assurrussa/goadmin/http/handlers/queues/mocks"
	"github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/models"
)

type retryTxContextKey struct{}

const (
	testJobQueue = "mail"
	testJobName  = "send"
)

type queueAuditStub struct {
	records []adminactionauditrepo.Record
	err     error
}

func (s *queueAuditStub) RecordAdminAction(_ context.Context, record adminactionauditrepo.Record) error {
	if s.err != nil {
		return s.err
	}
	s.records = append(s.records, record)
	return nil
}

func TestRetryFailedJobReadsDeletesAndCreatesInsideOneTransaction(t *testing.T) {
	adminAppTest := adminappt.NewAppTest(t)
	jobsRepo := queuesmocks.NewMockjobsRepository(adminAppTest.Ctrl)
	failedRepo := queuesmocks.NewMockjobsFailedRepository(adminAppTest.Ctrl)
	audit := &queueAuditStub{}
	handler := NewHandler(adminAppTest.App, jobsRepo, failedRepo, audit)
	id := outbox.NewJobID()
	failedJob := outbox.JobFailedModel{
		ID: id, Queue: testJobQueue, Name: testJobName, Payload: `{"to":"user@example.test"}`,
	}
	txContext := context.WithValue(context.Background(), retryTxContextKey{}, true)

	txCall := adminAppTest.MockTransaction.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, fn func(context.Context) error) error {
			return fn(txContext)
		},
	)
	getCall := failedRepo.EXPECT().GetByID(txContext, id).Return(failedJob, nil)
	deleteCall := failedRepo.EXPECT().Delete(txContext, id).Return(int64(1), nil)
	createCall := jobsRepo.EXPECT().Create(txContext, gomock.Any()).DoAndReturn(
		func(_ context.Context, job outbox.JobModel) (outbox.JobID, error) {
			require.Equal(t, testJobQueue, job.Queue)
			require.Equal(t, failedJob.Payload, job.Payload)
			require.Equal(t, 0, job.Attempts)

			return job.ID, nil
		},
	)
	gomock.InOrder(txCall, getCall, deleteCall, createCall)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 9})
		return c.Next()
	})
	app.Post("/jobs-failed/:id/retry", handler.RetryFailedJob)
	req := httptest.NewRequestWithContext(
		context.Background(), http.MethodPost, "/jobs-failed/"+id.String()+"/retry", nil,
	)
	req.Header.Set("Referer", "/queues/jobs-failed")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, fiber.StatusFound, resp.StatusCode)
	require.Len(t, audit.records, 1)
	require.Equal(t, adminactionauditrepo.ActionQueueFailedRetry, audit.records[0].Action)
	require.Equal(t, id.String(), audit.records[0].TargetID)
	require.NotContains(t, audit.records[0].TargetID, "user@example.test")
}

func TestRetryFailedJobReturnsNotFoundWhenConcurrentDeleteWins(t *testing.T) {
	adminAppTest := adminappt.NewAppTest(t)
	jobsRepo := queuesmocks.NewMockjobsRepository(adminAppTest.Ctrl)
	failedRepo := queuesmocks.NewMockjobsFailedRepository(adminAppTest.Ctrl)
	audit := &queueAuditStub{err: context.Canceled}
	handler := NewHandler(adminAppTest.App, jobsRepo, failedRepo, audit)
	id := outbox.NewJobID()
	failedJob := outbox.JobFailedModel{ID: id, Queue: testJobQueue, Name: testJobName, Payload: "sensitive"}

	adminAppTest.MockTransaction.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
	)
	failedRepo.EXPECT().GetByID(gomock.Any(), id).Return(failedJob, nil)
	failedRepo.EXPECT().Delete(gomock.Any(), id).Return(int64(0), nil)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 9})
		return c.Next()
	})
	app.Post("/jobs-failed/:id/retry", handler.RetryFailedJob)
	req := httptest.NewRequestWithContext(
		context.Background(), http.MethodPost, "/jobs-failed/"+id.String()+"/retry", nil,
	)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestRetryFailedJobAbortsTransactionWhenAuditInsertFails(t *testing.T) {
	adminAppTest := adminappt.NewAppTest(t)
	jobsRepo := queuesmocks.NewMockjobsRepository(adminAppTest.Ctrl)
	failedRepo := queuesmocks.NewMockjobsFailedRepository(adminAppTest.Ctrl)
	auditErr := errors.New("audit insert failed")
	audit := &queueAuditStub{err: auditErr}
	handler := NewHandler(adminAppTest.App, jobsRepo, failedRepo, audit)
	id := outbox.NewJobID()
	failedJob := outbox.JobFailedModel{ID: id, Queue: testJobQueue, Name: testJobName, Payload: "sensitive"}
	var txError error
	adminAppTest.MockTransaction.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			txError = fn(ctx)
			return txError
		},
	)
	failedRepo.EXPECT().GetByID(gomock.Any(), id).Return(failedJob, nil)
	failedRepo.EXPECT().Delete(gomock.Any(), id).Return(int64(1), nil)
	jobsRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(outbox.NewJobID(), nil)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 9})
		return c.Next()
	})
	app.Post("/jobs-failed/:id/retry", handler.RetryFailedJob)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/jobs-failed/"+id.String()+"/retry", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	require.ErrorIs(t, txError, auditErr)
	require.Empty(t, audit.records)
}

func TestQueueGridsMaskPayloadInPreviewAndSerializedItem(t *testing.T) {
	adminAppTest := adminappt.NewAppTest(t)
	jobsRepo := queuesmocks.NewMockjobsRepository(adminAppTest.Ctrl)
	failedRepo := queuesmocks.NewMockjobsFailedRepository(adminAppTest.Ctrl)
	jobsRepo.EXPECT().GetList(gomock.Any(), gomock.Any()).Return(
		[]outbox.JobModel{{ID: outbox.NewJobID(), Payload: `{"secret":"active-secret"}`}}, 1, nil,
	)
	failedRepo.EXPECT().GetList(gomock.Any(), gomock.Any()).Return(
		[]outbox.JobFailedModel{{ID: outbox.NewJobID(), Payload: `{"secret":"failed-secret"}`}}, 1, nil,
	)
	handler := NewHandler(adminAppTest.App, jobsRepo, failedRepo, &queueAuditStub{})
	app := fiber.New()
	app.Get("/jobs/data", handler.jobsGrid.HandleData)
	app.Get("/jobs-failed/data", handler.jobsFailedGrid.HandleData)

	for _, path := range []string{"/jobs/data", "/jobs-failed/data"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		rawBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		var body struct {
			Data []struct {
				Item   map[string]any `json:"item"`
				Values map[string]any `json:"values"`
			} `json:"data"`
		}
		require.Equal(t, fiber.StatusOK, resp.StatusCode)
		require.NoError(t, json.Unmarshal(rawBody, &body))
		require.Len(t, body.Data, 1)
		require.Equal(t, "Скрыто", body.Data[0].Item["payload"])
		require.Equal(t, "Скрыто", body.Data[0].Values["payloadPreview"])
		require.NotContains(t, string(rawBody), "active-secret")
		require.NotContains(t, string(rawBody), "failed-secret")
	}
}
