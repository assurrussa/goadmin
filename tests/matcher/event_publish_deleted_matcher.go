package testsmatcher

import (
	"strings"

	uploadhost "github.com/assurrussa/gouploads/host"
	"go.uber.org/mock/gomock"
)

var _ gomock.Matcher = EventPublishDeletedMatcher{}

// EventPublishDeletedMatcher is intended to be used only in tests.
type EventPublishDeletedMatcher struct {
	*BaseMatcher[uploadhost.FileDeletedEvent]
}

func NewEventPublishDeletedMatcher(name string, expected uploadhost.FileDeletedEvent) *EventPublishDeletedMatcher {
	return &EventPublishDeletedMatcher{
		NewBaseMatcher[uploadhost.FileDeletedEvent](name, expected, checkEventPublishDeletedFields),
	}
}

func checkEventPublishDeletedFields(
	expected uploadhost.FileDeletedEvent,
	got uploadhost.FileDeletedEvent,
	unequalFields []string,
) ([]string, bool) {
	if got.EventType != expected.EventType {
		unequalFields = append(unequalFields, "EventType")
	}
	if got.FileID != expected.FileID {
		unequalFields = append(unequalFields, "FileID")
	}
	if got.Status != expected.Status {
		unequalFields = append(unequalFields, "Status")
	}
	if got.Error != "" && !strings.Contains(got.Error, expected.Error) {
		unequalFields = append(unequalFields, "Error")
	}
	if got.FilePath != expected.FilePath {
		unequalFields = append(unequalFields, "FilePath")
	}

	return unequalFields, len(unequalFields) == 0
}
