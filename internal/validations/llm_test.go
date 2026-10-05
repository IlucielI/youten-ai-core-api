package validations_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

func TestRecordingChatRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.RecordingChatRequest
		wantErr bool
	}{
		{
			name: "valid request with minimal message",
			req: dtos.RecordingChatRequest{
				Message: "What were the key takeaways?",
			},
			wantErr: false,
		},
		{
			name: "valid request with conversation history and token",
			req: dtos.RecordingChatRequest{
				Message: "Tell me more about the roadmap.",
				ConversationHistory: []dtos.ChatMessageInput{
					{Role: "user", Content: "Hello"},
					{Role: "assistant", Content: "Hi, how can I help?"},
				},
				OwnershipToken: "token-abc-123",
			},
			wantErr: false,
		},
		{
			name: "empty message",
			req: dtos.RecordingChatRequest{
				Message: "",
			},
			wantErr: true,
		},
		{
			name: "whitespace-only message",
			req: dtos.RecordingChatRequest{
				Message: "    \t\n  ",
			},
			wantErr: true,
		},
		{
			name: "message exceeding 4000 characters",
			req: dtos.RecordingChatRequest{
				Message: strings.Repeat("a", 4001),
			},
			wantErr: true,
		},
		{
			name: "boundary message exactly 4000 characters",
			req: dtos.RecordingChatRequest{
				Message: strings.Repeat("b", 4000),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validations.ValidateRecordingChatRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
