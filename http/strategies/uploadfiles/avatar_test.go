package uploadfiles_test

import (
	"context"
	"encoding/json"
	"testing"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/http/strategies/uploadfiles"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	adminpreviewattach "github.com/assurrussa/goadmin/outbox/preview_attach"
)

type avatarAdminRepoStub struct {
	admin models.Admin
	err   error
}

func (s avatarAdminRepoStub) GetByID(context.Context, int64) (models.Admin, error) {
	return s.admin, s.err
}

func TestAvatarStrategyAfterJobsCapturesExpectedPreview(t *testing.T) {
	userID := identity.NewUserID()
	previewID := int64(42)
	strategy := uploadfiles.NewAvatarStrategy(avatarAdminRepoStub{
		admin: models.Admin{ID: 10, UUID: userID, Data: &models.AdminData{PreviewFileID: &previewID}},
	})
	const testSessionSentinel = "BROWSER_SESSION_SENTINEL"
	jobs, err := strategy.GetAfterJobs(context.Background(), uploadhost.UploadContext{
		UserID: 10, UserUUID: uploadhost.UserID(userID), SessionID: testSessionSentinel,
		Metadata: map[string]string{"entity_id": "10"},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, adminpreviewattach.JobName, jobs[0].JobName)
	require.Equal(t, "42", jobs[0].Meta["expectedPreviewFileId"])
	require.NotContains(t, jobs[0].Meta, "sessionId")
	raw, err := json.Marshal(jobs)
	require.NoError(t, err)
	require.NotContains(t, string(raw), testSessionSentinel)
}
