package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

func TestControllers_InviteWorkspaceMember_InvalidUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	ctx.Params = gin.Params{gin.Param{Key: "id", Value: "invalid-uuid"}}
	body, _ := json.Marshal(dtos.InviteMemberRequest{
		Email: "colleague@example.com",
		Role:  "member",
	})
	ctx.Request, _ = http.NewRequest(http.MethodPost, "/v1/workspaces/invalid-uuid/invites", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrl := &Controllers{svc: &services.Service{}}
	ctrl.InviteWorkspaceMember(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid UUID, got %d", w.Code)
	}
}

func TestControllers_GetInvitationDetail_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	ctx.Params = gin.Params{gin.Param{Key: "token", Value: ""}}
	ctx.Request, _ = http.NewRequest(http.MethodGet, "/v1/invites/", nil)

	ctrl := &Controllers{svc: &services.Service{}}
	ctrl.GetInvitationDetail(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty token, got %d", w.Code)
	}
}

func TestControllers_RevokeWorkspaceInvitation_InvalidInviteID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	wsID := uuid.New().String()
	ctx.Params = gin.Params{
		gin.Param{Key: "id", Value: wsID},
		gin.Param{Key: "inviteId", Value: "invalid-uuid"},
	}
	ctx.Request, _ = http.NewRequest(http.MethodDelete, "/v1/workspaces/"+wsID+"/invites/invalid-uuid", nil)

	ctrl := &Controllers{svc: &services.Service{}}
	ctrl.RevokeWorkspaceInvitation(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid inviteId, got %d", w.Code)
	}
}
