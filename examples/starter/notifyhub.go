package main

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/gonotify/transport"
	"github.com/assurrussa/gonotify/transport/notifyhub"
)

var (
	errAuthMailInvalid    = errors.New("invalid auth mail delivery")
	errAuthMailUnknown    = errors.New("auth mail provider outcome is unknown")
	errAuthMailRejected   = errors.New("auth mail was rejected")
	errAuthMailUnaccepted = errors.New("auth mail was not accepted")
)

type mailIntegrations struct {
	client *notifyhub.Client
	auth   *notifyHubSender
}

// Construction validates one connection/key for both modules without sending.
func newMailIntegrations(selected map[string]bool) (mailIntegrations, error) {
	var result mailIntegrations
	if !selected["notifications"] && !selected["authmail"] {
		return result, nil
	}
	baseURL, err := required("NOTIFYHUB_URL")
	if err != nil {
		return result, err
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1/confidential-email") {
		return result, errors.New("NOTIFYHUB_URL must be a gateway origin or deployment prefix")
	}
	// Preserve the exact credential so the SDK rejects accidental whitespace.
	client, err := notifyhub.New(notifyhub.Config{
		BaseURL: baseURL, ProjectKey: os.Getenv("NOTIFYHUB_PROJECT_KEY"), Timeout: 10 * time.Second,
	})
	if err != nil {
		return result, fmt.Errorf("configure NotifyHub: %w", err)
	}
	result.client = client
	if selected["authmail"] {
		from, err := required("AUTH_MAIL_FROM")
		if err != nil {
			return mailIntegrations{}, err
		}
		if !bareMailAddress(from) {
			return mailIntegrations{}, errors.New("AUTH_MAIL_FROM must be a bare email address")
		}
		result.auth = &notifyHubSender{client: client, from: from}
	}
	return result, nil
}

type confidentialEmailSender interface {
	SendConfidentialEmail(
		ctx context.Context, request notifyhub.ConfidentialEmailRequest,
	) (notifyhub.ConfidentialEmailReceipt, error)
}

// GoAuth remains the sole durable owner of plaintext-bearing auth events, stored
// encrypted. Never use ordinary Submit or a fallback for these notifications.
type notifyHubSender struct {
	client confidentialEmailSender
	from   string
}

func (s *notifyHubSender) SendNotification(ctx context.Context, delivery goauth.NotificationDelivery) error {
	event := delivery.EncryptedEvent
	if delivery.ID == "" || delivery.ID != event.ID || delivery.ValidUntil.IsZero() ||
		!delivery.ValidUntil.Equal(event.ValidUntil) || event.Envelope.DeleteAfter.Before(delivery.ValidUntil) ||
		!bareMailAddress(delivery.Notification.To) {
		return errAuthMailInvalid
	}
	subject, body, err := authMailContent(delivery.Notification)
	if err != nil {
		return err
	}
	// The key, rendered content and original expiry stay identical on worker
	// retries. Unknown outcomes only reconcile Hub metadata, never redispatch.
	receipt, err := s.client.SendConfidentialEmail(ctx, notifyhub.ConfidentialEmailRequest{
		IdempotencyKey: "goadmin:auth:" + delivery.ID,
		Event:          "goadmin." + delivery.Notification.Template,
		ExpiresAt:      delivery.ValidUntil,
		Email:          transport.EmailMessage{From: s.from, To: []string{delivery.Notification.To}, Subject: subject, Text: body},
	})
	if err == nil && receipt.Status == notifyhub.StatusAccepted {
		return nil
	}
	var failure *notifyhub.ConfidentialEmailError
	if errors.As(err, &failure) {
		switch failure.Outcome {
		case notifyhub.OutcomeRejected:
			return errAuthMailRejected
		case notifyhub.OutcomeNotSent, notifyhub.OutcomeRetryable, notifyhub.OutcomeSimulated:
			return errAuthMailUnaccepted
		case notifyhub.OutcomeUnknown:
			return errAuthMailUnknown
		}
	}
	// Never expose arbitrary transport error text or interpret it as non-delivery.
	return errAuthMailUnknown
}

func authMailContent(notification goauth.Notification) (subject, body string, err error) {
	switch notification.Template {
	case "password_reset":
		value := notification.Data["reset_url"]
		if value == "" || strings.ContainsAny(value, "\r\n") {
			return "", "", errAuthMailInvalid
		}
		return "Reset your admin password", "Open this link to reset your password: " + value, nil
	case "email_challenge", "email_change":
		value := notification.Data["code"]
		if len(value) != 6 || strings.Trim(value, "0123456789") != "" {
			return "", "", errAuthMailInvalid
		}
		return "Confirm your admin email", "Your confirmation code is: " + value, nil
	case "password_reset_success", "password_changed":
		return "Your admin password was changed", "Your admin password was changed.", nil
	case "email_changed":
		return "Your admin email was changed", "Your admin email was changed.", nil
	default:
		return "", "", errAuthMailInvalid
	}
}

func bareMailAddress(value string) bool {
	if strings.ContainsAny(value, "\r\n") {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

var _ goauth.NotificationSender = (*notifyHubSender)(nil)
