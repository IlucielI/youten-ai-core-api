package controllers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/sse"
)

// ListWorkspaceChatSessions handles listing conversation sessions for the caller.
func (c *Controllers) ListWorkspaceChatSessions(ctx *gin.Context) {
	workspaceID, err := c.resolveWorkspaceID(ctx)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	var folderID *uuid.UUID
	if fStr := strings.TrimSpace(ctx.Query("folder_id")); fStr != "" {
		if parsed, err := uuid.Parse(fStr); err == nil && parsed != uuid.Nil {
			folderID = &parsed
		}
	}

	res, err := c.svc.ListWorkspaceChatSessions(ctx.Request.Context(), workspaceID, folderID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "workspace chat sessions retrieved successfully", res)
}

// CreateWorkspaceChatSession initializes a new multi-meeting conversation session.
func (c *Controllers) CreateWorkspaceChatSession(ctx *gin.Context) {
	workspaceID, err := c.resolveWorkspaceID(ctx)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	var req dtos.CreateWorkspaceChatSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := req.Validate(); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	res, err := c.svc.CreateWorkspaceChatSession(ctx.Request.Context(), workspaceID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondCreated(ctx, "workspace chat session created successfully", res)
}

// GetWorkspaceChatSessionHistory retrieves past conversation turns for a session.
func (c *Controllers) GetWorkspaceChatSessionHistory(ctx *gin.Context) {
	workspaceID, err := c.resolveWorkspaceID(ctx)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	sessionID, err := uuid.Parse(ctx.Param("sessionId"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid session id"))
		return
	}

	res, err := c.svc.GetWorkspaceChatSessionHistory(ctx.Request.Context(), workspaceID, sessionID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "chat session history retrieved successfully", res)
}

// DeleteWorkspaceChatSession removes a past chat session and its dialogue history.
func (c *Controllers) DeleteWorkspaceChatSession(ctx *gin.Context) {
	workspaceID, err := c.resolveWorkspaceID(ctx)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	sessionID, err := uuid.Parse(ctx.Param("sessionId"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid session id"))
		return
	}

	if err := c.svc.DeleteWorkspaceChatSession(ctx.Request.Context(), workspaceID, sessionID); err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "chat session deleted successfully", nil)
}

// StreamWorkspaceChat streams multi-meeting RAG answer via Server-Sent Events (SSE).
func (c *Controllers) StreamWorkspaceChat(ctx *gin.Context) {
	workspaceID, err := c.resolveWorkspaceID(ctx)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	var req dtos.StreamWorkspaceChatRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := req.Validate(); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	result, err := c.svc.InitiateWorkspaceChatStream(ctx.Request.Context(), workspaceID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	// Setup SSE headers
	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Header().Set("Transfer-Encoding", "chunked")
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Flush()

	// Initial session handshake event
	_ = sse.WriteChatSession(ctx.Writer, result.SessionID.String())

	// Citations metadata event
	if len(result.Sources) > 0 {
		_ = sse.WriteChatCitations(ctx.Writer, result.Sources)
	}

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	var fullContent strings.Builder
	streaming := true

	for streaming {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-pingTicker.C:
			if err := sse.WritePing(ctx.Writer); err != nil {
				return
			}
		case chunk, ok := <-result.StreamChannel:
			if !ok {
				streaming = false
				break
			}
			if chunk.Err != nil {
				_ = sse.WriteChatError(ctx.Writer, chunk.Err.Error())
				return
			}
			if chunk.Content != "" {
				fullContent.WriteString(chunk.Content)
				if err := sse.WriteChatToken(ctx.Writer, chunk.Content); err != nil {
					return
				}
			}
		}
	}

	if fullContent.Len() == 0 {
		_ = sse.WriteChatError(ctx.Writer, "AI model failed to generate a response. Please try again.")
		return
	}

	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	asstMsg, err := c.svc.SaveAssistantWorkspaceChatMessage(
		saveCtx,
		workspaceID,
		result.SessionID,
		fullContent.String(),
		result.Sources,
		result.RetrievedChunkIDs,
	)
	if err != nil {
		_ = sse.WriteChatError(ctx.Writer, "failed to persist assistant message")
		return
	}

	_ = sse.WriteChatDone(ctx.Writer, sse.ChatDoneEvent{
		SessionID: result.SessionID.String(),
		MessageID: asstMsg.ID.String(),
	})
}
