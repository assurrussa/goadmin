package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/gonotify/transport"
	"github.com/assurrussa/gonotify/transport/notifyhub"
)

const (
	testRecipient           = "recipient@example.test"
	testNotificationsModule = "notifications"
	testAuthModule          = "authmail"
	testMailFrom            = "admin@example.test"
	testUnknown             = "unknown"
	testExpired             = "expired"
)

func authDelivery() goauth.NotificationDelivery {
	expires := time.Now().Add(time.Minute)
	return goauth.NotificationDelivery{
		ID: "cbd80ba4-d2a1-4912-bebc-43be72372d86", ValidUntil: expires,
		EncryptedEvent: goauth.EncryptedEvent{
			ID: "cbd80ba4-d2a1-4912-bebc-43be72372d86", ValidUntil: expires,
			Envelope: goauth.EncryptedEnvelope{DeleteAfter: expires.Add(time.Hour)},
		},
		Notification: goauth.Notification{
			To: testRecipient, Template: "password_reset",
			Data: map[string]string{"reset_url": "https://admin.example.test/reset?token=synthetic-secret"},
		},
	}
}

func TestMailIntegrationsConfiguration(t *testing.T) {
	t.Setenv("NOTIFYHUB_URL", "")
	t.Setenv("NOTIFYHUB_PROJECT_KEY", "")
	t.Setenv("AUTH_MAIL_FROM", "")
	result, err := newMailIntegrations(nil)
	if err != nil || result.client != nil || result.auth != nil {
		t.Fatalf("disabled mail should not require configuration: %+v / %v", result, err)
	}
	for _, selected := range []map[string]bool{{testAuthModule: true}, {testNotificationsModule: true}} {
		if _, err := newMailIntegrations(selected); err == nil {
			t.Fatal("selected mail must require a gateway")
		}
	}
	t.Setenv("NOTIFYHUB_URL", "https://notify.example.test/prefix")
	t.Setenv("NOTIFYHUB_PROJECT_KEY", "synthetic-key")
	result, err = newMailIntegrations(map[string]bool{testNotificationsModule: true})
	if err != nil || result.client == nil || result.auth != nil {
		t.Fatalf("ordinary notifications should not require auth sender: %v", err)
	}
	if _, err := newMailIntegrations(map[string]bool{testAuthModule: true}); err == nil {
		t.Fatal("auth mail must require a sender")
	}
	t.Setenv("AUTH_MAIL_FROM", testMailFrom)
	result, err = newMailIntegrations(map[string]bool{testAuthModule: true, testNotificationsModule: true})
	if err != nil || result.auth == nil || result.auth.client != result.client {
		t.Fatalf("auth and ordinary mail must share one client: %v", err)
	}
	for _, url := range []string{
		"http://remote.example.test", "https://notify.example.test/v1/notifications",
		"https://notify.example.test/prefix/v1/confidential-email/", "https://user:secret@notify.example.test",
		"https://notify.example.test?secret=secret", "https://notify.example.test#secret",
	} {
		t.Setenv("NOTIFYHUB_URL", url)
		if _, err := newMailIntegrations(map[string]bool{testAuthModule: true}); err == nil {
			t.Fatal("invalid gateway configuration was accepted")
		}
	}
	t.Setenv("NOTIFYHUB_URL", "https://notify.example.test")
	for _, key := range []string{"", " synthetic-key", "synthetic-key ", "synthetic\nkey"} {
		t.Setenv("NOTIFYHUB_PROJECT_KEY", key)
		if _, err := newMailIntegrations(map[string]bool{testNotificationsModule: true}); err == nil {
			t.Fatal("invalid credential was accepted")
		}
	}
	t.Setenv("NOTIFYHUB_PROJECT_KEY", "synthetic-key")
	for _, from := range []string{"", "Admin <admin@example.test>", "admin@example.test\r\nBcc: other@example.test"} {
		t.Setenv("AUTH_MAIL_FROM", from)
		if _, err := newMailIntegrations(map[string]bool{testAuthModule: true}); err == nil {
			t.Fatal("invalid sender was accepted")
		}
	}
}

//nolint:gocognit // One wire test checks shared configuration and immutable replay together.
func TestNotifyHubSharedClientAndImmutableRetry(t *testing.T) {
	delivery := authDelivery()
	var authCalls, ordinaryCalls int
	var firstBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Error("mail did not reuse the project credential")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/prefix/v1/confidential-email":
			authCalls++
			if r.Header.Get("Idempotency-Key") != "goadmin:auth:"+delivery.ID {
				t.Error("auth identity changed")
			}
			var payload struct {
				Event     string                 `json:"event"`
				ExpiresAt int64                  `json:"expires_at"` //nolint:tagliatelle // NotifyHub HTTP wire contract.
				Email     transport.EmailMessage `json:"email"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Error(err)
			}
			if payload.ExpiresAt != delivery.ValidUntil.Unix() || payload.Event != "goadmin.password_reset" {
				t.Error("auth identity or expiry changed")
			}
			if len(payload.Email.To) != 1 || payload.Email.To[0] != delivery.Notification.To ||
				!strings.Contains(payload.Email.Text, "synthetic-secret") {
				t.Error("auth payload changed")
			}
			if authCalls == 1 {
				firstBody = string(body)
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, `{"id":"auth-1","status":"retry","expires_at":%d}`, delivery.ValidUntil.Unix())
			} else {
				if string(body) != firstBody {
					t.Error("retry changed payload or expiry")
				}
				_, _ = fmt.Fprintf(w, `{"id":"auth-1","status":"accepted","duplicate":true,"expires_at":%d}`, delivery.ValidUntil.Unix())
			}
		case "/prefix/v1/notifications":
			ordinaryCalls++
			if strings.Contains(string(body), "synthetic-secret") {
				t.Error("ordinary queue received a credential")
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(transport.Receipt{ID: "ordinary-1", Deliveries: []transport.DeliveryReceipt{
				{ID: "delivery-1", Channel: "email", Recipient: testRecipient, Status: "queued"},
			}})
		default:
			t.Error("unexpected gateway path")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("NOTIFYHUB_URL", server.URL+"/prefix")
	t.Setenv("NOTIFYHUB_PROJECT_KEY", "synthetic-key")
	t.Setenv("AUTH_MAIL_FROM", testMailFrom)
	mail, err := newMailIntegrations(map[string]bool{testAuthModule: true, testNotificationsModule: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := mail.auth.SendNotification(t.Context(), delivery); !errors.Is(err, errAuthMailUnaccepted) {
		t.Fatalf("temporary rejection was not returned: %v", err)
	}
	if err := mail.auth.SendNotification(t.Context(), delivery); err != nil {
		t.Fatal(err)
	}
	_, err = mail.client.Submit(t.Context(), transport.Request{
		IdempotencyKey: "ordinary-message-1", Event: "admin.notice",
		Email: &transport.EmailMessage{
			From: testMailFrom, To: []string{testRecipient},
			Subject: "Notice", Text: "Ordinary notice",
		},
	})
	if err != nil || authCalls != 2 || ordinaryCalls != 1 {
		t.Fatalf("unexpected attempts or fallback: auth=%d ordinary=%d error=%v", authCalls, ordinaryCalls, err)
	}
}

func TestNotifyHubAuthOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   int
		status string
		want   error
	}{
		{"accepted", 200, "accepted", nil},
		{"simulated", 200, "simulated", errAuthMailUnaccepted},
		{"dispatching", 202, "dispatching", errAuthMailUnknown},
		{testUnknown, 409, testUnknown, errAuthMailUnknown},
		{"rejected", 422, "failed", errAuthMailRejected},
		{"temporary", 503, "retry", errAuthMailUnaccepted},
		{"unauthorized", 401, "", errAuthMailUnaccepted},
		{"quota", 429, "", errAuthMailUnaccepted},
		{"non-json", 200, "invalid", errAuthMailUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			delivery := authDelivery()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1/confidential-email" {
					t.Error("auth used the ordinary queue")
				}
				w.WriteHeader(tc.code)
				if tc.status == "invalid" {
					_, _ = io.WriteString(w, "synthetic-secret")
					return
				}
				_, _ = fmt.Fprintf(w, `{"id":"auth-1","status":%q,"code":"synthetic-secret","expires_at":%d}`,
					tc.status, delivery.ValidUntil.Unix())
			}))
			defer server.Close()
			client, err := notifyhub.New(notifyhub.Config{BaseURL: server.URL, ProjectKey: "synthetic-key"})
			if err != nil {
				t.Fatal(err)
			}
			sender := &notifyHubSender{client: client, from: testMailFrom}
			err = sender.SendNotification(t.Context(), delivery)
			if !errors.Is(err, tc.want) || calls.Load() != 1 {
				t.Fatalf("got %v, want %v; calls=%d", err, tc.want, calls.Load())
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("secret leaked through diagnostics")
			}
		})
	}
}

type confidentialSenderFunc func(context.Context, notifyhub.ConfidentialEmailRequest) (notifyhub.ConfidentialEmailReceipt, error)

func (f confidentialSenderFunc) SendConfidentialEmail(
	ctx context.Context, req notifyhub.ConfidentialEmailRequest,
) (notifyhub.ConfidentialEmailReceipt, error) {
	return f(ctx, req)
}

func TestNotifyHubAuthRejectsInvalidDelivery(t *testing.T) {
	sender := &notifyHubSender{client: confidentialSenderFunc(func(
		context.Context, notifyhub.ConfidentialEmailRequest,
	) (notifyhub.ConfidentialEmailReceipt, error) {
		t.Fatal("invalid delivery contacted gateway")
		return notifyhub.ConfidentialEmailReceipt{}, nil
	})}
	for _, mutate := range []func(*goauth.NotificationDelivery){
		func(d *goauth.NotificationDelivery) { d.ID = "" },
		func(d *goauth.NotificationDelivery) { d.EncryptedEvent.ID = "other" },
		func(d *goauth.NotificationDelivery) { d.ValidUntil = time.Time{} },
		func(d *goauth.NotificationDelivery) { d.ValidUntil = d.ValidUntil.Add(time.Second) },
		func(d *goauth.NotificationDelivery) { d.EncryptedEvent.Envelope.DeleteAfter = time.Time{} },
		func(d *goauth.NotificationDelivery) {
			d.Notification.To = "recipient@example.test\r\nBcc: other@example.test"
		},
		func(d *goauth.NotificationDelivery) { d.Notification.Template = testUnknown },
		func(d *goauth.NotificationDelivery) { d.Notification.Data["reset_url"] = "" },
		func(d *goauth.NotificationDelivery) {
			d.Notification.Data["reset_url"] = "https://admin.example.test/\r\ninjected"
		},
		func(d *goauth.NotificationDelivery) {
			d.Notification.Template = "email_challenge"
			d.Notification.Data["code"] = "bad"
		},
	} {
		delivery := authDelivery()
		mutate(&delivery)
		if err := sender.SendNotification(t.Context(), delivery); !errors.Is(err, errAuthMailInvalid) {
			t.Fatalf("invalid delivery: %v", err)
		}
	}
}

func TestNotifyHubAuthCancellationTimeoutAndExpiry(t *testing.T) {
	for _, scenario := range []string{"cancel", "timeout", testExpired} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			started := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				started <- struct{}{}
				<-r.Context().Done()
			}))
			defer server.Close()
			client, err := notifyhub.New(notifyhub.Config{
				BaseURL: server.URL, ProjectKey: "synthetic-key", Timeout: 30 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}
			sender := &notifyHubSender{client: client, from: testMailFrom}
			delivery := authDelivery()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if scenario == testExpired {
				delivery.ValidUntil = time.Now().Add(-time.Second)
				delivery.EncryptedEvent.ValidUntil = delivery.ValidUntil
			}
			done := make(chan error, 1)
			go func() { done <- sender.SendNotification(ctx, delivery) }()
			if scenario == "cancel" {
				<-started
				cancel()
			}
			select {
			case err := <-done:
				want := errAuthMailUnknown
				if scenario == testExpired {
					want = errAuthMailUnaccepted
				}
				if !errors.Is(err, want) {
					t.Fatalf("got %v, want %v", err, want)
				}
			case <-time.After(time.Second):
				t.Fatal("request did not stop")
			}
			wantCalls := int32(1)
			if scenario == testExpired {
				wantCalls = 0
			}
			if calls.Load() != wantCalls {
				t.Fatalf("unexpected resend/fallback: %d", calls.Load())
			}
		})
	}
}

func TestNotifyHubUnexpectedFailureIsSafeUnknown(t *testing.T) {
	for _, failure := range []error{nil, errors.New("synthetic-secret recipient@example.test synthetic-key")} {
		sender := &notifyHubSender{client: confidentialSenderFunc(func(
			context.Context, notifyhub.ConfidentialEmailRequest,
		) (notifyhub.ConfidentialEmailReceipt, error) {
			return notifyhub.ConfidentialEmailReceipt{Status: notifyhub.StatusQueued}, failure
		})}
		if err := sender.SendNotification(t.Context(), authDelivery()); !errors.Is(err, errAuthMailUnknown) {
			t.Fatalf("unexpected failure exposed or acknowledged: %v", err)
		}
	}
}

func TestAuthMailSupportedTemplates(t *testing.T) {
	for _, template := range []string{
		"password_reset", "email_challenge", "email_change", "password_reset_success", "password_changed", "email_changed",
	} {
		t.Run(template, func(t *testing.T) {
			delivery := authDelivery()
			delivery.Notification.Template = template
			delivery.Notification.Data["code"] = "123456"
			var calls int
			sender := &notifyHubSender{client: confidentialSenderFunc(func(
				_ context.Context, request notifyhub.ConfidentialEmailRequest,
			) (notifyhub.ConfidentialEmailReceipt, error) {
				calls++
				if request.Email.Subject == "" || request.Email.Text == "" || request.Event != "goadmin."+template {
					t.Error("template was not rendered for confidential delivery")
				}
				return notifyhub.ConfidentialEmailReceipt{Status: notifyhub.StatusAccepted}, nil
			}), from: testMailFrom}
			if err := sender.SendNotification(t.Context(), delivery); err != nil || calls != 1 {
				t.Fatalf("template was not accepted: %v; calls=%d", err, calls)
			}
		})
	}
}

func TestAcceptedReceiptWithErrorIsNotSuccess(t *testing.T) {
	sender := &notifyHubSender{client: confidentialSenderFunc(func(
		context.Context, notifyhub.ConfidentialEmailRequest,
	) (notifyhub.ConfidentialEmailReceipt, error) {
		return notifyhub.ConfidentialEmailReceipt{Status: notifyhub.StatusAccepted}, errors.New("synthetic-secret")
	})}
	if err := sender.SendNotification(t.Context(), authDelivery()); !errors.Is(err, errAuthMailUnknown) {
		t.Fatalf("an error acknowledged provider acceptance: %v", err)
	}
}
