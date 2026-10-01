package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
)

func TestSMTPSenderDeliversMessage(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	received := make(chan string, 1)
	go func() {
		defer serverConn.Close()
		_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(serverConn)
		_, _ = fmt.Fprint(serverConn, "220 test SMTP\r\n")
		var body strings.Builder
		inData := false
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				received <- "read: " + readErr.Error()
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					_, _ = fmt.Fprint(serverConn, "250 queued\r\n")
					continue
				}
				body.WriteString(line)
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO "):
				_, _ = fmt.Fprint(serverConn, "250 localhost\r\n")
			case strings.HasPrefix(line, "MAIL FROM:"):
				_, _ = fmt.Fprint(serverConn, "250 sender\r\n")
			case strings.HasPrefix(line, "RCPT TO:"):
				_, _ = fmt.Fprint(serverConn, "250 recipient\r\n")
			case line == "DATA\r\n":
				inData = true
				_, _ = fmt.Fprint(serverConn, "354 continue\r\n")
			case line == "QUIT\r\n":
				_, _ = fmt.Fprint(serverConn, "221 goodbye\r\n")
				received <- body.String()
				return
			default:
				received <- "unexpected command: " + line
				return
			}
		}
	}()

	sink := &smtpSender{addr: "smtp.local:1025", from: "admin@example.test", dial: func(context.Context, string, string) (net.Conn, error) {
		return clientConn, nil
	}}
	delivery := goauth.NotificationDelivery{ID: "delivery-1", Notification: goauth.Notification{To: "user@example.test", Template: "password_reset", Data: map[string]string{"reset_url": "https://admin.test/auth/reset-password?token=abc"}}}
	if err := sink.SendNotification(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-received:
		if !strings.Contains(body, "Open this link to reset your password: https://admin.test/auth/reset-password?token=abc\r\n") {
			t.Fatalf("SMTP server received %q", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SMTP server did not receive message")
	}
}

func TestSMTPSenderReportsRecipientRejection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	go func() {
		reader := bufio.NewReader(serverConn)
		_, _ = fmt.Fprint(serverConn, "220 test SMTP\r\n")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO "), strings.HasPrefix(line, "MAIL FROM:"):
				_, _ = fmt.Fprint(serverConn, "250 accepted\r\n")
			case strings.HasPrefix(line, "RCPT TO:"):
				_, _ = fmt.Fprint(serverConn, "550 rejected\r\n")
				return
			}
		}
	}()
	sink := &smtpSender{addr: "smtp.local:1025", from: "admin@example.test", dial: func(context.Context, string, string) (net.Conn, error) {
		return clientConn, nil
	}}
	err := sink.send(context.Background(), "user@example.test", []byte("Subject: Test\r\n\r\nhello\r\n"))
	if err == nil || !strings.Contains(err.Error(), "SMTP recipient") {
		t.Fatalf("want SMTP recipient error, got %v", err)
	}
}

func TestSMTPSenderRejectsInvalidRecipientAndTemplate(t *testing.T) {
	sender := &smtpSender{addr: "smtp.local:1025", from: "admin@example.test", dial: func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("invalid notification must not dial SMTP")
		return nil, nil
	}}
	for _, notification := range []goauth.Notification{
		{To: "user@example.test\r\nBcc: other@example.test", Template: "password_reset"},
		{To: "user@example.test", Template: "unknown"},
		{To: "user@example.test", Template: "password_reset", Data: map[string]string{"reset_url": "https://admin.test/\r\ninjected"}},
	} {
		if err := sender.SendNotification(t.Context(), goauth.NotificationDelivery{Notification: notification}); err == nil {
			t.Fatal("invalid notification was accepted")
		}
	}
}

func TestSMTPSenderCancellationInterruptsSMTP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, server := net.Pipe()
	defer server.Close()
	dialed := make(chan struct{})
	sender := &smtpSender{addr: "smtp.local:1025", from: "admin@example.test", dial: func(context.Context, string, string) (net.Conn, error) { close(dialed); return client, nil }}
	done := make(chan error, 1)
	go func() {
		done <- sender.SendNotification(ctx, goauth.NotificationDelivery{Notification: goauth.Notification{To: "user@example.test", Template: "password_reset_success"}})
	}()
	<-dialed
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled SMTP succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("SMTP did not stop on cancellation")
	}
}
