package uploadfiles

import (
	"context"
	"strings"

	uploadhost "github.com/assurrussa/gouploads/host"
)

// GenericStrategy handles standard file uploads without specific business logic side-effects.
type GenericStrategy struct{}

func NewGenericStrategy() *GenericStrategy {
	return &GenericStrategy{}
}

func (s *GenericStrategy) CanUpload(_ context.Context, _ uploadhost.UploadContext) error {
	// Allow all authenticated admins to upload generic files.
	// Basic auth check is performed by the handler/middleware.
	return nil
}

func (s *GenericStrategy) GetConfig(
	_ context.Context,
	req uploadhost.UploadContext,
) *uploadhost.FileUploadConfig {
	objectType := req.Metadata["entity_type"]
	objectID := req.Metadata["entity_id"]
	fileTypeStr := req.Metadata["file_type"]
	fileType := uploadhost.GetFileTypeString(strings.ToLower(strings.TrimSpace(fileTypeStr)))

	config := defaultFileUploadConfig(objectType, objectID)

	// Apply default rules based on file type
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
		config.AllowedExtensions = []string{extJPG, extJPEG, extPNG, extGIF, extWebP}
		config.AllowedMimeTypes = map[string][]string{
			extJPG:  {mimeImageJPEG},
			extJPEG: {mimeImageJPEG},
			extPNG:  {mimeImagePNG},
			extGIF:  {mimeImageGIF},
			extWebP: {mimeImageWebP},
		}
	case uploadhost.FileTypeUnknown, uploadhost.FileTypePdf, uploadhost.FileTypeDocx,
		uploadhost.FileTypeLink, uploadhost.FileTypeText:
		// These types retain the generic policy configured above.
	}

	return config
}

func (s *GenericStrategy) GetAfterJobs(
	_ context.Context,
	_ uploadhost.UploadContext,
) ([]uploadhost.FileEventAfterJob, error) {
	// No specific jobs for generic uploads
	return nil, nil
}
