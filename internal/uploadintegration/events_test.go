package uploadintegration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/assurrussa/gowebsocket/eventstream"
	inmem "github.com/assurrussa/gowebsocket/eventstream/inmem"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/uploadintegration"
)

func TestUploadEventsSurviveStreamJSONSnapshots(t *testing.T) {
	status := uploadhost.NewFileUploadStatusEvent(42, uploadhost.FileUploadTaskStatusCompleted)
	deleted := uploadhost.NewFileDeletedEvent(42, "https://admin.test/file.png", uploadhost.FileDeleteStatusCompleted)
	processed := uploadhost.NewEventAfterProcess(42, "https://admin.test/file.png", uploadhost.StatusCompleted, "avatar")
	for _, event := range []uploadhost.Event{&status, &deleted, &processed} {
		t.Run(event.EventName(), func(t *testing.T) {
			stream := inmem.New()
			t.Cleanup(func() { require.NoError(t, stream.Close()) })
			user := uploadhost.NewUserID()
			events, err := stream.Subscribe(t.Context(), eventstream.UserID(user))
			require.NoError(t, err)
			require.NoError(t, (uploadintegration.Publisher{Stream: stream}).Publish(t.Context(), user, event))
			select {
			case got := <-events:
				require.NoError(t, got.Validate())
				require.Equal(t, eventstream.EventID(event.EventID()), got.EventID())
				expected, marshalErr := json.Marshal(event)
				require.NoError(t, marshalErr)
				actual, marshalErr := json.Marshal(got)
				require.NoError(t, marshalErr)
				require.JSONEq(t, string(expected), string(actual))
			case <-time.After(time.Second):
				t.Fatal("upload event did not survive stream snapshot")
			}
		})
	}
}

func TestUploadEventBridgePreservesIdentityAndJSON(t *testing.T) {
	stream := &captureStream{}
	user := uploadhost.NewUserID()
	event := uploadhost.NewEventAfterProcess(42, "https://admin.test/uploads/media/v1/file.png",
		uploadhost.StatusCompleted, "avatar")
	require.NoError(t, (uploadintegration.Publisher{Stream: stream}).Publish(context.Background(), user, event))
	got := stream.event
	require.Equal(t, eventstream.UserID(user), stream.user)
	require.Equal(t, eventstream.EventID(event.EventID()), got.EventID())
	expected, err := json.Marshal(event)
	require.NoError(t, err)
	actual, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(actual))
	processor := uploadintegration.Processor[*uploadhost.EventAfterProcess]{}
	decoded, err := processor.ReverseAdapt(expected)
	require.NoError(t, err)
	adapted, err := processor.Adapt(decoded)
	require.NoError(t, err)
	body, ok := adapted.([]byte)
	require.True(t, ok)
	require.JSONEq(t, string(expected), string(body))
}

type captureStream struct {
	user  eventstream.UserID
	event eventstream.Event
}

func (*captureStream) Close() error { return nil }
func (*captureStream) Subscribe(context.Context, eventstream.UserID) (<-chan eventstream.Event, error) {
	return make(chan eventstream.Event), nil
}

func (s *captureStream) Publish(_ context.Context, user eventstream.UserID, event eventstream.Event) error {
	s.user = user
	s.event = event
	return nil
}
