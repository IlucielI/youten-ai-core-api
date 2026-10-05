package services

import (
	"context"
	"fmt"
	"html"
	"strings"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
)

// JoinBotWaitlist registers or updates an applicant in the meeting voice bot beta waitlist.
// Duplicate submissions with the same email update platform and company size idempotently.
// If a transactional mailer is configured, a confirmation email (Penpot template 12-D) is dispatched.
func (s *Service) JoinBotWaitlist(ctx context.Context, req dtos.WaitlistRequest) (*dtos.WaitlistResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "google_meet"
	}
	companySize := strings.TrimSpace(req.CompanySize)
	if companySize == "" {
		companySize = "1-10"
	}

	entry := &models.BotWaitlist{
		Email:       email,
		Platform:    platform,
		CompanySize: companySize,
		Status:      "PENDING",
	}

	saved, err := s.repo.UpsertBotWaitlist(ctx, entry)
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	// Dispatch transactional confirmation email if mailer is configured (Penpot template 12-D)
	if s.mailer != nil {
		msg := EmailMessage{
			To:      []string{saved.Email},
			Subject: "Welcome to Youten Voice Bot Beta Waitlist!",
			TextBody: fmt.Sprintf(
				"Hello,\n\nThank you for signing up for the Youten Meeting Voice Bot Beta waitlist!\n\nDetails:\n- Preferred Platform: %s\n- Company Size: %s\n- Status: %s\n\nWe will notify you as soon as your access is ready.\n\nBest regards,\nThe Youten Team\n",
				saved.Platform, saved.CompanySize, saved.Status,
			),
			HTMLBody: fmt.Sprintf(
				"<div style=\"font-family: Arial, sans-serif; color: #1e293b; max-width: 600px; margin: 0 auto; padding: 24px; border: 1px solid #e2e8f0; border-radius: 8px;\">"+
					"<h2 style=\"color: #0f172a; margin-top: 0;\">Welcome to Youten Voice Bot Beta!</h2>"+
					"<p>Thank you for your interest in joining the <strong>Meeting Voice Bot Beta</strong> program.</p>"+
					"<div style=\"background-color: #f8fafc; padding: 16px; border-radius: 6px; margin: 20px 0;\">"+
					"<p style=\"margin: 4px 0;\"><strong>Preferred Platform:</strong> %s</p>"+
					"<p style=\"margin: 4px 0;\"><strong>Company Size:</strong> %s</p>"+
					"<p style=\"margin: 4px 0;\"><strong>Status:</strong> %s</p>"+
					"</div>"+
					"<p>We are rolling out beta invites gradually. You will receive an invitation email as soon as your workspace is provisioned.</p>"+
					"<p style=\"margin-bottom: 0;\">Best regards,<br><strong>The Youten Team</strong></p>"+
					"</div>",
				html.EscapeString(saved.Platform), html.EscapeString(saved.CompanySize), html.EscapeString(saved.Status),
			),
		}
		if err := s.mailer.Send(ctx, msg); err != nil {
			return nil, s.wrapError(ctx, err)
		}
	}

	return &dtos.WaitlistResponse{
		Email:       saved.Email,
		Platform:    saved.Platform,
		CompanySize: saved.CompanySize,
		Status:      saved.Status,
		Message:     "successfully registered for voice bot beta waitlist",
	}, nil
}
