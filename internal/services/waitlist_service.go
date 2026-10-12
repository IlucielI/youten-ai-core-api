package services

import (
	"context"
	"fmt"
	"strings"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/templates"
)

// JoinBotWaitlist registers or updates an applicant in the meeting voice bot beta waitlist.
// Duplicate submissions with the same email update platform and company size idempotently.
// If a transactional mailer is configured, a confirmation email (Penpot template 12-D) is dispatched.
func (s *Service) JoinBotWaitlist(ctx context.Context, req dtos.WaitlistRequest) (*dtos.WaitlistResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = constants.DefaultBotPlatform
	}
	companySize := strings.TrimSpace(req.CompanySize)
	if companySize == "" {
		companySize = constants.DefaultBotCompanySize
	}

	entry := &models.BotWaitlist{
		Email:       email,
		Platform:    platform,
		CompanySize: companySize,
		Status:      constants.WaitlistStatusPending,
	}

	saved, err := s.repo.UpsertBotWaitlist(ctx, entry)
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	// Dispatch transactional confirmation email if mailer is configured (Penpot template 12-D)
	if s.mailer != nil {
		htmlBody, textBody, err := templates.RenderWaitlistEmail(templates.WaitlistEmailData{
			Platform:    saved.Platform,
			CompanySize: saved.CompanySize,
			Status:      saved.Status,
		})
		if err != nil {
			return nil, s.wrapError(ctx, fmt.Errorf("render waitlist email template: %w", err))
		}

		msg := dtos.EmailMessage{
			To:       []string{saved.Email},
			Subject:  "Welcome to Youten Voice Bot Beta Waitlist!",
			TextBody: textBody,
			HTMLBody: htmlBody,
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
