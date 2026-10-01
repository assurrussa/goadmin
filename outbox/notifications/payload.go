package notificationsjob

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	notifyjob "github.com/assurrussa/gonotify/interfaces/outbox/notifications"
	"github.com/assurrussa/gonotify/transport"

	"github.com/assurrussa/goadmin/infrastructure/notify"
	"github.com/assurrussa/goadmin/models"
)

type Recipient struct {
	AdminID        int64  `json:"adminId"`
	Name           string `json:"name,omitempty"`
	Email          string `json:"email,omitempty"`
	TelegramChatID int64  `json:"telegramChatId,omitempty"`
}

type Payload struct {
	DispatchID string                       `json:"dispatchId,omitempty"`
	EmailFrom  string                       `json:"emailFrom,omitempty"`
	Title      string                       `json:"title"`
	Message    string                       `json:"message"`
	Level      notify.NotificationLevel     `json:"level"`
	Payload    models.NotificationPayload   `json:"payload,omitempty"`
	Channels   []notify.NotificationChannel `json:"channels,omitempty"`
	Recipients []Recipient                  `json:"recipients"`
}

// DeliveryRequests returns immutable, per-recipient gateway requests. A key is
// created by the producer once and remains unchanged across worker retries.
func DeliveryRequests(payload Payload) ([]transport.Request, error) {
	requests := make([]transport.Request, 0, len(payload.Recipients)*len(payload.Channels))
	seen := make(map[string]bool)
	for _, channel := range payload.Channels {
		if channel != notify.ChannelEmail && channel != notify.ChannelTelegram {
			return nil, fmt.Errorf("unsupported notification channel %q", channel)
		}
		for _, recipient := range payload.Recipients {
			key := "goadmin:" + payload.DispatchID + ":" + strconv.FormatInt(recipient.AdminID, 10) + ":" + string(channel)
			if seen[key] {
				continue
			}
			seen[key] = true
			req := transport.Request{Event: "admin.notification", IdempotencyKey: key}
			switch channel {
			case notify.ChannelEmail:
				if recipient.Email == "" {
					continue
				}
				if payload.EmailFrom == "" {
					return nil, errors.New("email sender is required")
				}
				req.Email = &transport.EmailMessage{
					From: payload.EmailFrom, To: []string{recipient.Email}, Subject: payload.Title, Text: payload.Message,
				}
			case notify.ChannelTelegram:
				if recipient.TelegramChatID == 0 {
					continue
				}
				req.Telegram = &transport.TelegramMessage{
					ChatID: strconv.FormatInt(recipient.TelegramChatID, 10), Text: payload.Title + "\n\n" + payload.Message,
				}
			}
			if payload.DispatchID == "" {
				return nil, errors.New("notification dispatch id is required for external delivery")
			}
			if err := notifyjob.ValidateRequest(req); err != nil {
				return nil, err
			}
			requests = append(requests, req)
		}
	}
	return requests, nil
}

func MarshalPayload(payload Payload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func UnmarshalPayload(raw string) (Payload, error) {
	var payload Payload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Payload{}, err
	}
	if payload.Payload == nil {
		payload.Payload = models.NotificationPayload{}
	}
	return payload, nil
}
