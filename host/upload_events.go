package host

import (
	uploadhost "github.com/assurrussa/gouploads/host"

	"github.com/assurrussa/goadmin/internal/uploadintegration"
)

// NewUploadEventPublisher bridges standalone upload notifications to the admin stream.
func NewUploadEventPublisher(stream EventStream) uploadhost.EventPublisher {
	return uploadintegration.Publisher{Stream: stream}
}

// IsFinalizedUpload reports whether a recorded artifact is ready for admin delivery.
func IsFinalizedUpload(file File) bool { return uploadintegration.Finalized(file) }
