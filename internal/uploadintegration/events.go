package uploadintegration

import (
	"context"
	"encoding/json"
	"fmt"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/assurrussa/gowebsocket/eventstream"
)

// Event adapts identity while preserving the original notification JSON.
type Event struct{ uploadhost.Event }

func (e Event) EventID() eventstream.EventID { return eventstream.EventID(e.Event.EventID()) }
func (e Event) MarshalJSON() ([]byte, error) { return json.Marshal(e.Event) }

// UnmarshalJSON restores the concrete upload event when the stream snapshots
// and independently decodes it for each subscriber.
func (e *Event) UnmarshalJSON(data []byte) error {
	var header struct {
		EventType string `json:"eventType"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return fmt.Errorf("decode upload event type: %w", err)
	}
	var event uploadhost.Event
	switch header.EventType {
	case uploadhost.EventTypeUploadStatus:
		event = &uploadhost.FileUploadStatusEvent{}
	case uploadhost.EventTypeAfterProcess:
		event = &uploadhost.EventAfterProcess{}
	case uploadhost.EventTypeDeleted:
		event = &uploadhost.FileDeletedEvent{}
	default:
		return fmt.Errorf("unsupported upload event type: %w", eventstream.ErrInvalidEvent)
	}
	if err := json.Unmarshal(data, event); err != nil {
		return fmt.Errorf("decode upload event: %w", err)
	}
	e.Event = event
	return nil
}

type Publisher struct{ Stream eventstream.EventStream }

func (p Publisher) Publish(ctx context.Context, userID uploadhost.UserID, event uploadhost.Event) error {
	if p.Stream == nil {
		return nil
	}
	return p.Stream.Publish(ctx, eventstream.UserID(userID), Event{event})
}

// Processor is the WebSocket adapter for upload notifications.
type Processor[T uploadhost.Event] struct{}

func (Processor[T]) Adapt(event eventstream.Event) (any, error) { return json.Marshal(event) }
func (Processor[T]) ReverseAdapt(message []byte) (eventstream.Event, error) {
	var event T
	if err := json.Unmarshal(message, &event); err != nil {
		return nil, fmt.Errorf("decode upload event: %w", err)
	}
	return Event{event}, nil
}
