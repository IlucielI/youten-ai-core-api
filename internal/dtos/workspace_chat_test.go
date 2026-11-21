package dtos

import (
	"strings"
	"testing"
)

func TestStreamWorkspaceChatRequest_SetDefaults_And_Validate(t *testing.T) {
	t.Run("question populated directly", func(t *testing.T) {
		req := StreamWorkspaceChatRequest{
			Question: "What was discussed?",
		}
		req.SetDefaults()
		if req.Question != "What was discussed?" {
			t.Fatalf("expected question to remain unchanged, got %s", req.Question)
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected validate to succeed, got %v", err)
		}
	})

	t.Run("query alias falls back to question", func(t *testing.T) {
		req := StreamWorkspaceChatRequest{
			Query: "What were the key decisions made recently?",
		}
		req.SetDefaults()
		if req.Question != "What were the key decisions made recently?" {
			t.Fatalf("expected question to be populated from query, got %s", req.Question)
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected validate to succeed, got %v", err)
		}
	})

	t.Run("both empty fails validation", func(t *testing.T) {
		req := StreamWorkspaceChatRequest{}
		req.SetDefaults()
		if err := req.Validate(); err == nil {
			t.Fatal("expected validation error for empty question/query, got nil")
		}
	})

	t.Run("question exceeds limit", func(t *testing.T) {
		req := StreamWorkspaceChatRequest{
			Question: strings.Repeat("a", 2001),
		}
		req.SetDefaults()
		if err := req.Validate(); err == nil {
			t.Fatal("expected validation error for length > 2000, got nil")
		}
	})
}
