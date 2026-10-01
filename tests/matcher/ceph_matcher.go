package testsmatcher

import (
	"io"
	"reflect"

	uploadhost "github.com/assurrussa/gouploads/host"
	uploadhosttest "github.com/assurrussa/gouploads/hosttest"
	"go.uber.org/mock/gomock"
)

var _ gomock.Matcher = CephMatcher{}

// CephMatcher is intended to be used only in tests.
type CephMatcher struct {
	*BaseMatcher[uploadhosttest.SaveFileInput]
}

func NewCephMatcher(name string, expected uploadhosttest.SaveFileInput) *CephMatcher {
	return &CephMatcher{NewBaseMatcher[uploadhosttest.SaveFileInput](name, expected, checkCephFields)}
}

func checkCephFields(
	expected uploadhosttest.SaveFileInput,
	got uploadhosttest.SaveFileInput,
	unequalFields []string,
) ([]string, bool) {
	if got.Size != expected.Size {
		unequalFields = append(unequalFields, "Size")
	}
	if got.MimeType != expected.MimeType {
		unequalFields = append(unequalFields, "MimeType")
	}

	body, err := io.ReadAll(expected.Reader)
	if err != nil {
		unequalFields = append(unequalFields, "ReadAll")
	}
	if len(body) != int(expected.Size) {
		unequalFields = append(unequalFields, "body file invalid size fule")
	}

	return unequalFields, len(unequalFields) == 0
}

var _ gomock.Matcher = WebhookMatcher{}

// WebhookMatcher is intended to be used only in tests.
type WebhookMatcher struct {
	*BaseMatcher[uploadhost.ListenResizeRequest]
}

func NewWebhookMatcher(name string, expected uploadhost.ListenResizeRequest) *WebhookMatcher {
	return &WebhookMatcher{NewBaseMatcher[uploadhost.ListenResizeRequest](name, expected, checkWebhookFields)}
}

func checkWebhookFields(
	expected uploadhost.ListenResizeRequest,
	got uploadhost.ListenResizeRequest,
	unequalFields []string,
) ([]string, bool) {
	if got.ExternalID != expected.ExternalID {
		unequalFields = append(unequalFields, "ExternalID")
	}
	if got.Status != expected.Status {
		unequalFields = append(unequalFields, "Status")
	}
	if got.Error != expected.Error {
		unequalFields = append(unequalFields, "Error")
	}
	if got.Attempt != expected.Attempt {
		unequalFields = append(unequalFields, "Attempt")
	}
	if got.Timestamp != expected.Timestamp {
		unequalFields = append(unequalFields, "Timestamp")
	}
	if reflect.DeepEqual(got.Metadata, expected.Metadata) {
		unequalFields = append(unequalFields, "request.Metadata")
	}
	if reflect.DeepEqual(got.Artifacts, expected.Artifacts) {
		unequalFields = append(unequalFields, "request.Artifacts")
	}

	return unequalFields, len(unequalFields) == 0
}
