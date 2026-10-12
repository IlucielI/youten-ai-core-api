package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"code-base-golang/internal/config"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

// SMTP implements services.EmailSender using standard SMTP protocol.
type SMTP struct {
	host      string
	port      string
	username  string
	password  string
	fromEmail string
	fromName  string
	useTLS    bool
	useSSL    bool
}

// Ensure SMTP satisfies services.EmailSender at compile time.
var _ services.EmailSender = (*SMTP)(nil)

// New initializes an SMTP adapter from application configuration.
func New(cfg config.Config) *SMTP {
	return &SMTP{
		host:      strings.TrimSpace(cfg.SMTPHost),
		port:      strings.TrimSpace(cfg.SMTPPort),
		username:  strings.TrimSpace(cfg.SMTPUsername),
		password:  strings.TrimSpace(cfg.SMTPPassword),
		fromEmail: strings.TrimSpace(cfg.SMTPFromEmail),
		fromName:  strings.TrimSpace(cfg.SMTPFromName),
		useTLS:    cfg.SMTPUseTLS,
		useSSL:    cfg.SMTPUseSSL,
	}
}

// Address returns the combined host:port string.
func (s *SMTP) Address() string {
	if s.port == "" {
		return net.JoinHostPort(s.host, "25")
	}
	return net.JoinHostPort(s.host, s.port)
}

// Ping checks if the SMTP server is reachable via TCP dial.
func (s *SMTP) Ping(ctx context.Context) error {
	if s == nil || s.host == "" {
		return errors.New("smtp host is not configured")
	}

	addr := s.Address()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to reach smtp server at %s: %w", addr, err)
	}
	defer conn.Close()
	return nil
}

// Send formats and transmits an email message via SMTP.
func (s *SMTP) Send(ctx context.Context, msg dtos.EmailMessage) error {
	if s == nil || s.host == "" {
		return errors.New("smtp client is nil or unconfigured")
	}
	if len(msg.To) == 0 {
		return errors.New("at least one recipient (To) is required")
	}

	fromHeader := s.fromEmail
	if s.fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", s.fromName, s.fromEmail)
	}

	// Build raw MIME headers and body
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(msg.To, ", ")))
	if len(msg.Cc) > 0 {
		sb.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(msg.Cc, ", ")))
	}
	if msg.ReplyTo != "" {
		sb.WriteString(fmt.Sprintf("Reply-To: %s\r\n", msg.ReplyTo))
	}
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))
	sb.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	sb.WriteString("MIME-Version: 1.0\r\n")

	// Determine content type (HTML preferred if available, multipart alternative if both)
	if msg.HTMLBody != "" && msg.TextBody != "" {
		boundary := fmt.Sprintf("boundary-%d", time.Now().UnixNano())
		sb.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))

		sb.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		sb.WriteString(msg.TextBody + "\r\n")

		sb.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		sb.WriteString(msg.HTMLBody + "\r\n")

		sb.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else if msg.HTMLBody != "" {
		sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		sb.WriteString(msg.HTMLBody + "\r\n")
	} else {
		sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		sb.WriteString(msg.TextBody + "\r\n")
	}

	// Aggregate all recipients (To + Cc + Bcc) for envelope delivery
	allRecipients := make([]string, 0, len(msg.To)+len(msg.Cc)+len(msg.Bcc))
	allRecipients = append(allRecipients, msg.To...)
	allRecipients = append(allRecipients, msg.Cc...)
	allRecipients = append(allRecipients, msg.Bcc...)

	// Setup authentication if credentials provided
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	addr := s.Address()
	bodyBytes := []byte(sb.String())

	// Handle direct SSL (e.g. port 465)
	if s.useSSL {
		tlsConfig := &tls.Config{
			ServerName: s.host,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("smtp tls dial failed: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return fmt.Errorf("smtp new client failed: %w", err)
		}
		defer client.Close()

		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth failed: %w", err)
			}
		}

		if err := client.Mail(s.fromEmail); err != nil {
			return fmt.Errorf("smtp mail from failed: %w", err)
		}
		for _, rcpt := range allRecipients {
			if err := client.Rcpt(rcpt); err != nil {
				return fmt.Errorf("smtp rcpt to failed for %s: %w", rcpt, err)
			}
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("smtp data writer failed: %w", err)
		}
		if _, err := w.Write(bodyBytes); err != nil {
			return fmt.Errorf("smtp write body failed: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("smtp close writer failed: %w", err)
		}
		return client.Quit()
	}

	// Standard transmission (port 25, 587, 1025 with optional STARTTLS)
	return smtp.SendMail(addr, auth, s.fromEmail, allRecipients, bodyBytes)
}
