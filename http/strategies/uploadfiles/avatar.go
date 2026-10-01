package uploadfiles

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	uploadhost "github.com/assurrussa/gouploads/host"

	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	adminpreviewattach "github.com/assurrussa/goadmin/outbox/preview_attach"
)

type adminRepo interface {
	GetByID(ctx context.Context, id int64) (models.Admin, error)
}

type AvatarStrategy struct {
	adminRepo adminRepo
}

func NewAvatarStrategy(adminRepo adminRepo) *AvatarStrategy {
	return &AvatarStrategy{adminRepo: adminRepo}
}

func (s *AvatarStrategy) CanUpload(_ context.Context, req uploadhost.UploadContext) error {
	if req.UserID <= 0 {
		return errors.New("unauthorized")
	}
	entityIDStr := req.Metadata["entity_id"]
	if entityIDStr == "" {
		return errors.New("entity_id is required")
	}
	entityID, err := strconv.ParseInt(entityIDStr, 10, 64)
	if err != nil {
		return errors.New("invalid entity_id")
	}
	if entityID != req.UserID {
		return errors.New("can only update own avatar")
	}
	return nil
}

func (s *AvatarStrategy) GetConfig(_ context.Context, req uploadhost.UploadContext) *uploadhost.FileUploadConfig {
	objectType := req.Metadata["entity_type"]
	objectID := req.Metadata["entity_id"]
	fileTypeStr := req.Metadata["file_type"]
	fileType := uploadhost.GetFileTypeString(strings.ToLower(strings.TrimSpace(fileTypeStr)))
	config := defaultFileUploadConfig(objectType, objectID)
	config.MaxFileSize = 10 * 1024 * 1024
	switch fileType {
	case uploadhost.FileTypeImage:
		config.AllowedExtensions = []string{extJPG, extJPEG, extPNG, extGIF, extWebP}
		config.AllowedMimeTypes = map[string][]string{
			extJPG: {mimeImageJPEG}, extJPEG: {mimeImageJPEG},
			extPNG: {mimeImagePNG}, extGIF: {mimeImageGIF}, extWebP: {mimeImageWebP},
		}
	default:
		config.AllowedExtensions = []string{extJPG, extJPEG, extPNG, extWebP}
	}
	return config
}

func (s *AvatarStrategy) GetAfterJobs(ctx context.Context, req uploadhost.UploadContext) ([]uploadhost.FileEventAfterJob, error) {
	if req.UserID <= 0 {
		return nil, errors.New("unauthorized")
	}
	adminUser, err := s.adminRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("fetch admin: %w", err)
	}
	if adminUser.ID <= 0 || adminUser.UUID == identity.UserIDNil {
		return nil, errors.New("admin not found")
	}
	objectID, _ := strconv.ParseInt(req.Metadata["entity_id"], 10, 64)
	// The opaque SessionID is a bearer credential, not a background-job address.
	// Authentication reloads the canonical preview on the next request.
	return uploadhost.NewFileEventAfterJobs(adminpreviewattach.JobName, uploadhost.UserID(adminUser.UUID), map[string]any{
		"adminId":               uploadhost.ObjectID(objectID),
		"expectedPreviewFileId": strconv.FormatInt(adminUser.GetPreviewFileID(), 10),
	}), nil
}
