package adminpreviewattach_test

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
	adminpreviewattach "github.com/assurrussa/goadmin/outbox/preview_attach"
	adminpreviewmocks "github.com/assurrussa/goadmin/outbox/preview_attach/mocks"
)

const testMediaFolder = "media/v1/test"

type TestSuite struct {
	suite.Suite

	adminMock                        *adminpreviewmocks.MockadminRepository
	eventFileAfterProcessServiceMock *adminpreviewmocks.MockeventFileAfterProcessService
	sessionUpdater                   *adminpreviewmocks.MocksessionUpdater

	job *adminpreviewattach.Job
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

		job := adminpreviewattach.Must(adminpreviewattach.NewOptions(
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
		adminpreviewattach.Must(adminpreviewattach.NewOptions(nil, nil, nil, nil))
	})
}

func TestJobHandle_Success(t *testing.T) {
	ctx, _, ts := NewTestSuite(t)

	ts.Equal(adminpreviewattach.JobName, ts.job.Name())

	fileID := int64(123)
	file := uploadhost.File{
		ID:         fileID,
		FileName:   "testname.png", //nolint:goconst // required
		FolderPath: testMediaFolder,
		Data:       finalPreviewData(),
	}
	admin := models.Admin{
		ID: 1234,
		Data: &models.AdminData{
			PreviewFileID: nil,
		},
	}

	userID := identity.NewUserID()
	sessionID := "session-123"
	payloadObj := uploadhost.NewAfterProcessPayload(uploadhost.UserID(userID),
		uploadhost.UserTypeAdmin, fileID, "testEventJob", map[string]any{
			"sessionId":             sessionID, //nolint:goconst // required
			"expectedPreviewFileId": "0",
		})
	payload, err := uploadhost.MarshalAfterProcessPayload(payloadObj)
	ts.Require().NoError(err)

	ts.eventFileAfterProcessServiceMock.EXPECT().HandleAfterProcess(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, fn uploadhost.AfterProcessFunc) error {
			return fn(ctx, payloadObj, file)
		}).Times(1)
	ts.adminMock.EXPECT().GetByUUID(ctx, userID).Return(admin, nil).Times(1)
	ts.adminMock.EXPECT().UpdatePreview(ctx, admin, file.ID).Return(nil).Times(1)
	ts.sessionUpdater.EXPECT().UpdatePreview(ctx, sessionID, file.ID).Return(nil).Times(1)

	err = ts.job.Handle(ctx, payload)
	ts.Require().NoError(err)
}

func TestJobHandle_DoesNotApplyStaleAttach(t *testing.T) {
	ctx, _, ts := NewTestSuite(t)
	fileID := int64(123)
	newerPreviewID := int64(456)
	file := uploadhost.File{ID: fileID, FolderPath: testMediaFolder, FileName: "testname.png", Data: finalPreviewData()}
	admin := models.Admin{
		ID:   1234,
		Data: &models.AdminData{PreviewFileID: &newerPreviewID},
	}
	userID := identity.NewUserID()
	payloadObj := uploadhost.NewAfterProcessPayload(uploadhost.UserID(userID),
		uploadhost.UserTypeAdmin, fileID, "testEventJob", map[string]any{
			"sessionId":             "session-123",
			"expectedPreviewFileId": "0",
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
		FolderPath: testMediaFolder,
		Data:       finalPreviewData(),
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
			"sessionId": "session-789",
		})
	payload, err := uploadhost.MarshalAfterProcessPayload(payloadObj)
	ts.Require().NoError(err)

	ts.eventFileAfterProcessServiceMock.EXPECT().HandleAfterProcess(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, fn uploadhost.AfterProcessFunc) error {
			return fn(ctx, payloadObj, file)
		}).Times(1)
	ts.adminMock.EXPECT().GetByUUID(ctx, userID).Return(admin, nil).Times(1)
	ts.adminMock.EXPECT().UpdatePreview(ctx, admin, file.ID).Return(errExpect).Times(1)
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
		FolderPath: testMediaFolder,
		Data:       finalPreviewData(),
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
			"sessionId": "session-789",
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

func finalPreviewData() *uploadhost.FileData {
	return &uploadhost.FileData{Presets: map[uploadhost.PresetName]uploadhost.FilePreset{
		"main": {RelativePath: "media/v1/test/testname.png"},
	}}
}

func TestJobHandleRejectsUnfinalizedPreview(t *testing.T) {
	ctx, _, ts := NewTestSuite(t)
	data := uploadhost.NewAfterProcessPayload(uploadhost.NewUserID(), uploadhost.UserTypeAdmin, 123, "preview", nil)
	payload, err := uploadhost.MarshalAfterProcessPayload(data)
	ts.Require().NoError(err)
	ts.eventFileAfterProcessServiceMock.EXPECT().HandleAfterProcess(gomock.Any(),
		gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context,
			_ string, fn uploadhost.AfterProcessFunc,
		) error {
			return fn(ctx, data, uploadhost.File{ID: 123, FolderPath: "staging/v1/tus", FileName: "source.png"})
		})
	ts.Require().ErrorContains(ts.job.Handle(ctx, payload), "preview file is not finalized")
}
