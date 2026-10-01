package profileavatar_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminprofileavatar "github.com/assurrussa/goadmin/http/handlers/profileavatar"
	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	adminpreviewdetach "github.com/assurrussa/goadmin/outbox/preview_detach"
)

func TestDeleteQueuesFileDeletionAndPreviewDetach(t *testing.T) {
	userID := identity.NewUserID()
	service := &deleteServiceStub{}
	const testSessionSentinel = "BROWSER_SESSION_SENTINEL"
	app := newTestApp(t, service, &models.SessionAdmin{
		ID: 1, UUID: userID, PreviewID: 96, SessionID: testSessionSentinel,
	})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/auth/profile/avatar", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, response.StatusCode)
	require.Equal(t, int64(96), service.req.FileID)
	require.Equal(t, uploadhost.UserID(userID), service.req.UserRequestID)
	require.Len(t, service.req.AfterJobs, 1)
	assert.Equal(t, adminpreviewdetach.JobName, service.req.AfterJobs[0].JobName)
	assert.Equal(t, uploadhost.UserID(userID), service.req.AfterJobs[0].UserID)
	assert.NotContains(t, service.req.AfterJobs[0].Meta, "sessionId")
	raw, err := json.Marshal(service.req)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), testSessionSentinel)
	assert.Equal(t, deleteResponse{Status: "pending", ID: 96}, decodeResponse(t, response))
}

func TestDeleteWithoutPreviewIsIdempotent(t *testing.T) {
	service := &deleteServiceStub{}
	app := newTestApp(t, service, &models.SessionAdmin{ID: 1, UUID: identity.NewUserID()})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/auth/profile/avatar", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Zero(t, service.calls)
	assert.Equal(t, deleteResponse{Status: "deleted"}, decodeResponse(t, response))
}

func TestDeleteRejectsUnauthenticatedRequest(t *testing.T) {
	service := &deleteServiceStub{}
	app := newTestApp(t, service, nil)
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/auth/profile/avatar", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	assert.Zero(t, service.calls)
	assert.Equal(t, deleteResponse{Status: "failed", Error: "unauthorized"}, decodeResponse(t, response))
}

func TestDeleteReturnsJSONErrorWhenQueueingFails(t *testing.T) {
	service := &deleteServiceStub{err: errors.New("outbox unavailable")}
	app := newTestApp(t, service, &models.SessionAdmin{ID: 1, UUID: identity.NewUserID(), PreviewID: 96})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/auth/profile/avatar", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, response.StatusCode)
	assert.Equal(t, deleteResponse{Status: "failed", Error: "failed to delete avatar"}, decodeResponse(t, response))
}

func newTestApp(t *testing.T, service *deleteServiceStub, admin *models.SessionAdmin) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		if admin != nil {
			c.Locals(adminsession.AuthAdminKey.String(), admin)
		}
		return c.Next()
	})
	adminprofileavatar.NewHandler(service, logger.Discard()).RegisterGroupRoutes(app)
	return app
}

func decodeResponse(t *testing.T, response *http.Response) deleteResponse {
	t.Helper()
	defer response.Body.Close()
	var result deleteResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	return result
}

type deleteResponse struct {
	Status string `json:"status"`
	ID     int64  `json:"id,omitempty"`
	Error  string `json:"error,omitempty"`
}

type deleteServiceStub struct {
	req   uploadhost.DeleteRequest
	err   error
	calls int
}

func (s *deleteServiceStub) DeleteFile(_ context.Context, req uploadhost.DeleteRequest) error {
	s.calls++
	s.req = req
	return s.err
}
