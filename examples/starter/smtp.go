package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
)

// smtpSender delivers notifications claimed by the goauth managed queue.
// The local starter uses plain SMTP only inside its private Compose network.
// Delivery errors return to the worker for its durable retry policy.
type smtpSender struct {
	addr string
	from string
	dial func(context.Context, string, string) (net.Conn, error)
}

func newSMTPSender() (*smtpSender, error) {
	addr, err := required("SMTP_ADDR")
	if err != nil {
		return nil, err
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, fmt.Errorf("SMTP_ADDR: %w", err)
	}
	from, err := required("SMTP_FROM")
	if err != nil {
		return nil, err
	}
	parsed, err := mail.ParseAddress(from)
	if err != nil || parsed.Address != from {
		return nil, errors.New("SMTP_FROM must be a bare email address")
	}
	return &smtpSender{addr: addr, from: from}, nil
}

func (s *smtpSender) SendNotification(ctx context.Context, delivery goauth.NotificationDelivery) error {
	notification := delivery.Notification
	recipient, err := mail.ParseAddress(notification.To)
	if err != nil || recipient.Address != notification.To {
		return errors.New("notification recipient must be a bare email address")
	}
	var subject, body string
	switch notification.Template {
	case "password_reset":
		subject = "Reset your admin password"
		body = "Open this link to reset your password: " + notification.Data["reset_url"]
	case "password_reset_success":
		subject = "Your admin password was reset"
		body = "Your admin password was changed."
	case "email_challenge":
		subject = "Confirm your admin email"
		body = "Your confirmation code is: " + notification.Data["code"]
	default:
		return fmt.Errorf("unsupported notification template %q", notification.Template)
	}
	if strings.ContainsAny(body, "\r\n") {
		// Body line breaks are allowed only for fixed text in this minimal starter.
		return errors.New("notification body contains an unexpected line break")
	}
	message := []byte("From: " + s.from + "\r\nTo: " + notification.To + "\r\nSubject: " + subject + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body + "\r\n")
	return s.send(ctx, notification.To, message)
}

func (s *smtpSender) send(ctx context.Context, to string, message []byte) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	dial := s.dial
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	conn, err := dial(deadlineCtx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("connect SMTP: %w", err)
	}
	defer conn.Close()
	stopClose := context.AfterFunc(deadlineCtx, func() { _ = conn.Close() })
	defer stopClose()
	if deadline, ok := deadlineCtx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, "localhost")
	if err != nil {
		return fmt.Errorf("open SMTP session: %w", err)
	}
	defer client.Close()
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("SMTP sender: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP data: %w", err)
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("SMTP write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("SMTP deliver: %w", err)
	}
	return client.Quit()
}

var _ goauth.NotificationSender = (*smtpSender)(nil)
