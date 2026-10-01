package uploadfiles

import (
	"context"
	"strings"

	uploadhost "github.com/assurrussa/gouploads/host"
)

// RichTextStrategy handles uploads from the rich text editor.
type RichTextStrategy struct{}

func NewRichTextStrategy() *RichTextStrategy {
	return &RichTextStrategy{}
}

func (s *RichTextStrategy) CanUpload(_ context.Context, _ uploadhost.UploadContext) error {
	// Allow all authenticated admins.
	return nil
}

func (s *RichTextStrategy) GetConfig(
	_ context.Context,
	req uploadhost.UploadContext,
) *uploadhost.FileUploadConfig {
	objectType := req.Metadata["entity_type"]
	objectID := req.Metadata["entity_id"]
	fileTypeStr := req.Metadata["file_type"]
	fileType := uploadhost.GetFileTypeString(strings.ToLower(strings.TrimSpace(fileTypeStr)))

	config := defaultRichTextUploadConfig(objectType, objectID)

	switch fileType {
	case uploadhost.FileTypeVideo:
		config.MaxFileSize = 50 * 1024 * 1024 // 50MB
		config.AllowedExtensions = []string{extMP4, extWebM}
		config.AllowedMimeTypes = map[string][]string{
			extMP4:  {mimeVideoMP4},
			extWebM: {mimeVideoWebM},
		}
	case uploadhost.FileTypeImage:
		config.MaxFileSize = 10 * 1024 * 1024 // 10MB
	case uploadhost.FileTypeUnknown, uploadhost.FileTypePdf, uploadhost.FileTypeDocx,
		uploadhost.FileTypeLink, uploadhost.FileTypeText:
		// These types retain the rich-text policy configured above.
	}

	return config
}

func (s *RichTextStrategy) GetAfterJobs(
	_ context.Context,
	_ uploadhost.UploadContext,
) ([]uploadhost.FileEventAfterJob, error) {
	return nil, nil
}
