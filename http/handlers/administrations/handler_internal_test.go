package administrations

import (
	"context"
	"errors"
	"testing"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"

	administrationsmocks "github.com/assurrussa/goadmin/http/handlers/administrations/mocks"
	"github.com/assurrussa/goadmin/internal/pointer"
	"github.com/assurrussa/goadmin/internal/uploadintegration"
	"github.com/assurrussa/goadmin/models"
)

func TestPreparePreview_AssignNewPreview(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	fileRepo := administrationsmocks.NewMockfileRepo(ctrl)
	binder := &previewBinder{MockfileRepo: fileRepo}
	h := &Handler{fileRepo: binder}

	admin := models.Admin{ID: 42}
	preview := uploadhost.File{ID: 7, ManagerID: pointer.To(int64(5))}

	binder.bind = func(request uploadintegration.PreviewBinding) error {
		require.Equal(t, int64(7), request.FileID)
		require.Equal(t, int64(42), request.AdminID)
		require.Equal(t, int64(5), request.ManagerID)
		return nil
	}

	changed, err := h.preparePreview(context.Background(), &admin, &preview)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, admin.Data)
	require.NotNil(t, admin.Data.PreviewFileID)
	require.Equal(t, int64(7), *admin.Data.PreviewFileID)
}

func TestPreparePreview_NoChanges(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	fileRepo := administrationsmocks.NewMockfileRepo(ctrl)
	binder := &previewBinder{MockfileRepo: fileRepo}
	h := &Handler{fileRepo: binder}

	previewID := int64(10)
	admin := models.Admin{
		ID:   99,
		Data: &models.AdminData{PreviewFileID: pointer.To(previewID)},
	}
	preview := uploadhost.File{
		ID:         previewID,
		ManagerID:  pointer.To(int64(5)),
		ObjectType: uploadhost.ObjectTypeAdmin,
		ObjectID:   pointer.To(uploadhost.ObjectID(admin.ID)),
	}

	changed, err := h.preparePreview(context.Background(), &admin, &preview)
	require.NoError(t, err)
	require.False(t, changed)
	require.NotNil(t, admin.Data.PreviewFileID)
	require.Equal(t, previewID, *admin.Data.PreviewFileID)
}

func TestPreparePreview_Remove(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	fileRepo := administrationsmocks.NewMockfileRepo(ctrl)
	binder := &previewBinder{MockfileRepo: fileRepo}
	h := &Handler{fileRepo: binder}

	admin := models.Admin{
		ID:   100,
		Data: &models.AdminData{PreviewFileID: pointer.To(int64(5))},
	}

	changed, err := h.preparePreview(context.Background(), &admin, nil)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, admin.Data)
	require.Nil(t, admin.Data.PreviewFileID)
}

func TestPreparePreview_UpdateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	fileRepo := administrationsmocks.NewMockfileRepo(ctrl)
	binder := &previewBinder{MockfileRepo: fileRepo}
	h := &Handler{fileRepo: binder}

	admin := models.Admin{ID: 1}
	preview := uploadhost.File{ID: 3, ManagerID: pointer.To(int64(5))}

	binder.bind = func(uploadintegration.PreviewBinding) error { return errors.New("binding failed") }

	changed, err := h.preparePreview(context.Background(), &admin, &preview)
	require.Error(t, err)
	require.False(t, changed)
}

type previewBinder struct {
	*administrationsmocks.MockfileRepo
	bind func(uploadintegration.PreviewBinding) error
}

func (b *previewBinder) BindPreview(_ context.Context, request uploadintegration.PreviewBinding) error {
	if b.bind != nil {
		return b.bind(request)
	}
	return nil
}
