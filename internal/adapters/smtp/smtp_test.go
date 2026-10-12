package smtp_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"code-base-golang/internal/adapters/smtp"
	"code-base-golang/internal/config"
	"code-base-golang/internal/dtos"
)

func TestSMTP_Address(t *testing.T) {
	cfg := config.Config{
		SMTPHost: "smtp.example.com",
		SMTPPort: "587",
	}
	adapter := smtp.New(cfg)
	if addr := adapter.Address(); addr != "smtp.example.com:587" {
		t.Errorf("expected smtp.example.com:587, got %s", addr)
	}
}

func TestSMTP_ValidationErrors(t *testing.T) {
	var nilAdapter *smtp.SMTP
	ctx := context.Background()

	if err := nilAdapter.Ping(ctx); err == nil {
		t.Error("expected error pinging nil adapter")
	}

	if err := nilAdapter.Send(ctx, dtos.EmailMessage{To: []string{"a@b.com"}}); err == nil {
		t.Error("expected error sending via nil adapter")
	}

	adapter := smtp.New(config.Config{SMTPHost: "localhost", SMTPPort: "1025"})
	if err := adapter.Send(ctx, dtos.EmailMessage{To: nil}); err == nil {
		t.Error("expected error sending with empty To")
	}
}

func TestSMTP_Ping_MockServer(t *testing.T) {
	// Start dummy TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	adapter := smtp.New(config.Config{
		SMTPHost: "127.0.0.1",
		SMTPPort: port,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := adapter.Ping(ctx); err != nil {
		t.Errorf("expected successful ping, got %v", err)
	}
}

func TestSMTP_Send_MockServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	// Simple dummy SMTP conversation mock
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		// 1. Initial greeting
		conn.Write([]byte("220 mock-smtp ESMTP ready\r\n"))

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			lineUpper := strings.ToUpper(strings.TrimSpace(line))
			if strings.HasPrefix(lineUpper, "EHLO") || strings.HasPrefix(lineUpper, "HELO") {
				conn.Write([]byte("250-mock-smtp\r\n250 OK\r\n"))
			} else if strings.HasPrefix(lineUpper, "MAIL FROM") {
				conn.Write([]byte("250 2.1.0 Ok\r\n"))
			} else if strings.HasPrefix(lineUpper, "RCPT TO") {
				conn.Write([]byte("250 2.1.5 Ok\r\n"))
			} else if strings.HasPrefix(lineUpper, "DATA") {
				conn.Write([]byte("354 End data with <CR><LF>.<CR><LF>\r\n"))
			} else if strings.HasPrefix(lineUpper, "QUIT") {
				conn.Write([]byte("221 2.0.0 Bye\r\n"))
				return
			} else if line == ".\r\n" {
				conn.Write([]byte("250 2.0.0 Ok: queued\r\n"))
			}
		}
	}()

	adapter := smtp.New(config.Config{
		SMTPHost:      "127.0.0.1",
		SMTPPort:      port,
		SMTPFromEmail: "system@example.com",
		SMTPFromName:  "Test System",
	})

	err = adapter.Send(context.Background(), dtos.EmailMessage{
		To:       []string{"user@example.com"},
		Subject:  "Welcome!",
		TextBody: "Hello world!",
		HTMLBody: "<h1>Hello world!</h1>",
	})
	if err != nil {
		t.Errorf("expected send to succeed with mock server, got %v", err)
	}
}
