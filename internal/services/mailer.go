package services

import (
	"context"

	"code-base-golang/internal/dtos"
)

// EmailSender defines the abstract contract for transactional email dispatchers.
// Concrete implementations (SMTP, AWS SES, SendGrid, Resend) can satisfy this interface.
type EmailSender interface {
	// Ping checks if the mail delivery service / SMTP host is reachable.
	Ping(ctx context.Context) error

	// Send dispatches an email message to the specified recipients.
	Send(ctx context.Context, msg dtos.EmailMessage) error
}
