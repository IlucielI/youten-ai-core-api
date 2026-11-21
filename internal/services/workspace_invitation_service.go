package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

// InviteMember invites a colleague to a workspace via email.
func (s *Service) InviteMember(ctx context.Context, workspaceID uuid.UUID, req dtos.InviteMemberRequest) (*dtos.WorkspaceInvitationResponse, error) {
	callerID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	caller, err := s.repo.FindWorkspaceMember(ctx, workspaceID, callerID)
	if err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}
	if !caller.Role.CanManageMembers() {
		return nil, constants.ErrForbidden
	}

	ws, err := s.repo.FindWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrWorkspaceNotFound
		}
		return nil, err
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	role := constants.WorkspaceRole(strings.ToLower(strings.TrimSpace(req.Role)))
	if role == "" {
		role = constants.WorkspaceRoleMember
	}
	if !role.IsValid() || role == constants.WorkspaceRoleOwner {
		return nil, constants.ErrInvalidWorkspaceRole
	}

	// Check if already an existing member
	if existingUser, err := s.repo.FindUserByEmail(ctx, email); err == nil && existingUser != nil {
		if _, err := s.repo.FindWorkspaceMember(ctx, workspaceID, existingUser.ID); err == nil {
			return nil, constants.ErrAlreadyWorkspaceMember
		}
	}

	// Invalidate any active previous pending invitation for this email in this workspace
	if prevInv, err := s.repo.FindPendingInvitationByWorkspaceAndEmail(ctx, workspaceID, email); err == nil && prevInv != nil {
		_ = s.repo.DeleteInvitation(ctx, prevInv.ID)
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, constants.ErrInternalServerError
	}
	token := hex.EncodeToString(tokenBytes)

	inv := &models.WorkspaceInvitation{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		WorkspaceID:     workspaceID,
		Email:           email,
		InviteToken:     token,
		Role:            role,
		InvitedByUserID: callerID,
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	if err := s.repo.CreateInvitation(ctx, inv); err != nil {
		return nil, err
	}

	s.dispatchInvitationEmail(ctx, ws.Name, email, string(role), token)

	return &dtos.WorkspaceInvitationResponse{
		ID:              inv.ID,
		WorkspaceID:     inv.WorkspaceID,
		WorkspaceName:   ws.Name,
		Email:           inv.Email,
		Role:            string(inv.Role),
		InvitedByUserID: inv.InvitedByUserID,
		ExpiresAt:       inv.ExpiresAt,
		AcceptedAt:      inv.AcceptedAt,
		CreatedAt:       inv.CreatedAt,
	}, nil
}

func (s *Service) dispatchInvitationEmail(ctx context.Context, workspaceName, email, role, token string) {
	if s.mailer == nil {
		return
	}

	inviteURL := fmt.Sprintf("https://youten.bayuanugerah.my.id/invitations/%s", token)
	msg := dtos.EmailMessage{
		To:       []string{email},
		Subject:  fmt.Sprintf("You've been invited to join %s on Youten AI", workspaceName),
		TextBody: fmt.Sprintf("You have been invited to join %s as a %s.\nAccept invitation: %s\nThis link expires in 7 days.", workspaceName, role, inviteURL),
		HTMLBody: fmt.Sprintf("<p>You have been invited to join <strong>%s</strong> as a <strong>%s</strong>.</p><p><a href=\"%s\">Accept Invitation</a></p><p>This invitation expires in 7 days.</p>", workspaceName, role, inviteURL),
	}

	// Dispatch in background so API response is immediate
	go func() {
		_ = s.mailer.Send(context.Background(), msg)
	}()
}

// ListWorkspaceInvitations returns all active pending invitations for a workspace.
func (s *Service) ListWorkspaceInvitations(ctx context.Context, workspaceID uuid.UUID) ([]dtos.WorkspaceInvitationResponse, error) {
	callerID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	if _, err := s.repo.FindWorkspaceMember(ctx, workspaceID, callerID); err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}

	invites, err := s.repo.FindPendingInvitationsByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	res := make([]dtos.WorkspaceInvitationResponse, 0, len(invites))
	for _, inv := range invites {
		var inviterName string
		if inv.InvitedBy != nil {
			inviterName = inv.InvitedBy.FullName
		}

		res = append(res, dtos.WorkspaceInvitationResponse{
			ID:              inv.ID,
			WorkspaceID:     inv.WorkspaceID,
			Email:           inv.Email,
			Role:            string(inv.Role),
			InvitedByUserID: inv.InvitedByUserID,
			InvitedByName:   inviterName,
			ExpiresAt:       inv.ExpiresAt,
			AcceptedAt:      inv.AcceptedAt,
			CreatedAt:       inv.CreatedAt,
		})
	}

	return res, nil
}

// RevokeInvitation cancels a pending workspace invitation.
func (s *Service) RevokeInvitation(ctx context.Context, workspaceID, inviteID uuid.UUID) error {
	callerID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return constants.ErrUnauthorized
	}

	caller, err := s.repo.FindWorkspaceMember(ctx, workspaceID, callerID)
	if err != nil {
		return constants.ErrWorkspaceAccessDenied
	}
	if !caller.Role.CanManageMembers() {
		return constants.ErrForbidden
	}

	inv, err := s.repo.FindInvitationByID(ctx, inviteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrInvitationNotFound
		}
		return err
	}
	if inv.WorkspaceID != workspaceID {
		return constants.ErrInvitationNotFound
	}

	return s.repo.DeleteInvitation(ctx, inviteID)
}

// GetInvitationDetail retrieves public invitation details for verification.
func (s *Service) GetInvitationDetail(ctx context.Context, token string) (*dtos.InvitationDetailResponse, error) {
	if token == "" {
		return nil, constants.ErrBadRequest
	}

	inv, err := s.repo.FindInvitationByToken(ctx, token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvitationNotFound
		}
		return nil, err
	}

	var wsName string
	if inv.Workspace != nil {
		wsName = inv.Workspace.Name
	}
	var inviterName string
	if inv.InvitedBy != nil {
		inviterName = inv.InvitedBy.FullName
	}

	now := time.Now()
	isExpired := now.After(inv.ExpiresAt)
	isAccepted := inv.AcceptedAt != nil

	return &dtos.InvitationDetailResponse{
		Token:         inv.InviteToken,
		WorkspaceID:   inv.WorkspaceID,
		WorkspaceName: wsName,
		Email:         inv.Email,
		Role:          string(inv.Role),
		InvitedByName: inviterName,
		ExpiresAt:     inv.ExpiresAt,
		IsExpired:     isExpired,
		IsAccepted:    isAccepted,
	}, nil
}

// AcceptInvitation processes acceptance of an invitation by the authenticated user.
func (s *Service) AcceptInvitation(ctx context.Context, token string) (*dtos.AcceptInvitationResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, constants.ErrUnauthorized
	}

	inv, err := s.repo.FindInvitationByToken(ctx, token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvitationNotFound
		}
		return nil, err
	}

	if inv.AcceptedAt != nil {
		return nil, constants.ErrInvitationAlreadyAccepted
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, constants.ErrInvitationExpired
	}

	if !strings.EqualFold(user.Email, inv.Email) {
		return nil, constants.ErrInvitationEmailMismatch
	}

	// Check if already a member
	if _, err := s.repo.FindWorkspaceMember(ctx, inv.WorkspaceID, userID); err == nil {
		return &dtos.AcceptInvitationResponse{
			WorkspaceID: inv.WorkspaceID,
			Role:        string(inv.Role),
			Message:     "Already a member of this workspace",
		}, nil
	}

	member := &models.WorkspaceMember{
		BaseModel:   models.BaseModel{ID: uuid.New()},
		WorkspaceID: inv.WorkspaceID,
		UserID:      userID,
		Role:        inv.Role,
	}

	if err := s.repo.AcceptInvitationInTransaction(ctx, inv.ID, member); err != nil {
		return nil, err
	}

	var wsName string
	if inv.Workspace != nil {
		wsName = inv.Workspace.Name
	}

	return &dtos.AcceptInvitationResponse{
		WorkspaceID:   inv.WorkspaceID,
		WorkspaceName: wsName,
		Role:          string(inv.Role),
		Message:       "Invitation accepted successfully",
	}, nil
}
