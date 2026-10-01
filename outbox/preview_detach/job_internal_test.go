package adminpreview_test

import (
	"context"
	"errors"
	"testing"

	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	tests "github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	adminpreview "github.com/assurrussa/goadmin/outbox/preview_detach"
	adminpreviewmocks "github.com/assurrussa/goadmin/outbox/preview_detach/mocks"
)

type TestSuite struct {
	suite.Suite

	adminMock                        *adminpreviewmocks.MockadminRepository
	eventFileAfterProcessServiceMock *adminpreviewmocks.MockeventFileAfterProcessService
	sessionUpdater                   *adminpreviewmocks.MocksessionUpdater

	job *adminpreview.Job
}

func NewTestSuite(t *testing.T) (context.Context, context.CancelFunc, *TestSuite) {
	t.Helper()

	return tests.NewSuite[*TestSuite](t, func(t *testing.T, _ context.Context) *TestSuite {
		t.Helper()

		log := logger.Discard()

		ctrl := gomock.NewController(t)
		adminMock := adminpreviewmocks.NewMockadminRepository(ctrl)
		eventFileAfterProcessServiceMock := adminpreviewmocks.NewMockeventFileAfterProcessService(ctrl)
		sessionUpdaterMock := adminpreviewmocks.NewMocksessionUpdater(ctrl)

		job := adminpreview.Must(adminpreview.NewOptions(
			eventFileAfterProcessServiceMock,
			adminMock,
			sessionUpdaterMock,
			log,
		))

		return &TestSuite{
			job:                              job,
			adminMock:                        adminMock,
			eventFileAfterProcessServiceMock: eventFileAfterProcessServiceMock,
			sessionUpdater:                   sessionUpdaterMock,
		}
	})
}

func TestJob_MustInit(t *testing.T) {
	assert.Panics(t, func() {
		adminpreview.Must(adminpreview.NewOptions(nil, nil, nil, nil))
	})
}

func TestJobHandle_Success(t *testing.T) {
	ctx, _, ts := NewTestSuite(t)

	ts.Equal(adminpreview.JobName, ts.job.Name())

	fileID := int64(123)
	admin := models.Admin{
		ID: 1234,
		Data: &models.AdminData{
			PreviewFileID: &fileID,
		},
	}

	userID := identity.NewUserID()
	sessionID := "session-123"
	payloadObj := uploadhost.NewAfterProcessPayload(uploadhost.UserID(userID),
		uploadhost.UserTypeAdmin, fileID, "testEventJob", map[string]any{
			"sessionId": sessionID, //nolint:goconst // required
		})
	payload, err := uploadhost.MarshalAfterProcessPayload(payloadObj)
	ts.Require().NoError(err)

	ts.eventFileAfterProcessServiceMock.EXPECT().HandleAfterProcess(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, fn uploadhost.AfterProcessFunc) error {
			return fn(ctx, payloadObj, uploadhost.File{})
		}).Times(1)
	ts.adminMock.EXPECT().GetByUUID(ctx, userID).Return(admin, nil).Times(1)
	ts.adminMock.EXPECT().UpdatePreview(ctx, admin, int64(0)).Return(nil).Times(1)
	ts.sessionUpdater.EXPECT().UpdatePreview(ctx, sessionID, int64(0)).Return(nil).Times(1)

	err = ts.job.Handle(ctx, payload)
	ts.Require().NoError(err)
}

func TestJobHandle_DetachDoesNotClearNewerPreview(t *testing.T) {
	ctx, _, ts := NewTestSuite(t)
	fileID := int64(123)
	newerPreviewID := int64(456)
	file := uploadhost.File{ID: fileID}
	admin := models.Admin{
		ID:   1234,
		Data: &models.AdminData{PreviewFileID: &newerPreviewID},
	}
	userID := identity.NewUserID()
	payloadObj := uploadhost.NewAfterProcessPayload(uploadhost.UserID(userID),
		uploadhost.UserTypeAdmin, fileID, "testEventJob", map[string]any{
			"sessionId": "session-123",
		})
	payload, err := uploadhost.MarshalAfterProcessPayload(payloadObj)
	ts.Require().NoError(err)
	ts.eventFileAfterProcessServiceMock.EXPECT().HandleAfterProcess(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, fn uploadhost.AfterProcessFunc) error {
			return fn(ctx, payloadObj, file)
		}).Times(1)
	ts.adminMock.EXPECT().GetByUUID(ctx, userID).Return(admin, nil).Times(1)
	ts.adminMock.EXPECT().UpdatePreview(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	ts.sessionUpdater.EXPECT().UpdatePreview(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	ts.Require().NoError(ts.job.Handle(ctx, payload))
}

func TestJobHandle_ErrorUpdatePreview(t *testing.T) {
	ctx, _, ts := NewTestSuite(t)

	errExpect := errors.New("expected error")
	fileID := int64(123)
	file := uploadhost.File{
		ID:         fileID,
		FileName:   "testname.png",
		FolderPath: "path/foo",
	}
	admin := models.Admin{
		ID: 1234,
		Data: &models.AdminData{
			PreviewFileID: &fileID,
		},
	}

	userID := identity.NewUserID()
	payloadObj := uploadhost.NewAfterProcessPayload(uploadhost.UserID(userID),
		uploadhost.UserTypeAdmin, fileID, "testEventJob", map[string]any{
			"sessionId": "session-456",
		})
	payload, err := uploadhost.MarshalAfterProcessPayload(payloadObj)
	ts.Require().NoError(err)

	ts.eventFileAfterProcessServiceMock.EXPECT().HandleAfterProcess(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, fn uploadhost.AfterProcessFunc) error {
			return fn(ctx, payloadObj, file)
		}).Times(1)
	ts.adminMock.EXPECT().GetByUUID(ctx, userID).Return(admin, nil).Times(1)
	ts.adminMock.EXPECT().UpdatePreview(ctx, admin, int64(0)).Return(errExpect).Times(1)
	ts.sessionUpdater.EXPECT().UpdatePreview(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err = ts.job.Handle(ctx, payload)
	ts.Require().ErrorIs(err, errExpect)
}

func TestJobHandle_ErrorGetByUUID(t *testing.T) {
	ctx, _, ts := NewTestSuite(t)

	errExpect := errors.New("expected error")
	fileID := int64(123)
	file := uploadhost.File{
		ID:         fileID,
		FileName:   "testname.png",
		FolderPath: "path/foo",
	}
	admin := models.Admin{
		ID: 1234,
		Data: &models.AdminData{
			PreviewFileID: nil,
		},
	}

	userID := identity.NewUserID()
	payloadObj := uploadhost.NewAfterProcessPayload(uploadhost.UserID(userID),
		uploadhost.UserTypeAdmin, fileID, "testEventJob", map[string]any{
			"sessionId": "session-456",
		})
	payload, err := uploadhost.MarshalAfterProcessPayload(payloadObj)
	ts.Require().NoError(err)

	ts.eventFileAfterProcessServiceMock.EXPECT().HandleAfterProcess(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, fn uploadhost.AfterProcessFunc) error {
			return fn(ctx, payloadObj, file)
		}).Times(1)
	ts.adminMock.EXPECT().GetByUUID(ctx, userID).Return(admin, errExpect).Times(1)
	ts.sessionUpdater.EXPECT().UpdatePreview(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err = ts.job.Handle(ctx, payload)
	ts.Require().ErrorIs(err, errExpect)
}
