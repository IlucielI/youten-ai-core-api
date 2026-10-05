package dtos_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
)

func TestSemanticSearchQuery_Validate(t *testing.T) {
	tests := []struct {
		name    string
		query   dtos.SemanticSearchQuery
		wantErr bool
	}{
		{
			name: "valid query minimal",
			query: dtos.SemanticSearchQuery{
				Q: "quarterly revenue",
			},
			wantErr: false,
		},
		{
			name: "valid query with limit and threshold",
			query: dtos.SemanticSearchQuery{
				Q:         "roadmap discussion",
				Limit:     20,
				Threshold: 0.75,
			},
			wantErr: false,
		},
		{
			name: "empty query",
			query: dtos.SemanticSearchQuery{
				Q: "",
			},
			wantErr: true,
		},
		{
			name: "whitespace only query",
			query: dtos.SemanticSearchQuery{
				Q: "    ",
			},
			wantErr: true,
		},
		{
			name: "negative limit",
			query: dtos.SemanticSearchQuery{
				Q:     "valid query",
				Limit: -1,
			},
			wantErr: true,
		},
		{
			name: "limit exceeding max 50",
			query: dtos.SemanticSearchQuery{
				Q:     "valid query",
				Limit: 51,
			},
			wantErr: true,
		},
		{
			name: "negative threshold",
			query: dtos.SemanticSearchQuery{
				Q:         "valid query",
				Threshold: -0.1,
			},
			wantErr: true,
		},
		{
			name: "threshold exceeding 1.0",
			query: dtos.SemanticSearchQuery{
				Q:         "valid query",
				Threshold: 1.1,
			},
			wantErr: true,
		},
		{
			name: "query exceeding max length 1000",
			query: dtos.SemanticSearchQuery{
				Q: strings.Repeat("a", 1001),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWorkspaceAskRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.WorkspaceAskRequest
		wantErr bool
	}{
		{
			name: "valid question minimal",
			req: dtos.WorkspaceAskRequest{
				Question: "What were the key decisions in last week's meetings?",
			},
			wantErr: false,
		},
		{
			name: "valid question with history",
			req: dtos.WorkspaceAskRequest{
				Question: "Who owns the marketing deliverable?",
				History: []dtos.ChatMessageInput{
					{Role: "user", Content: "Hello"},
					{Role: "assistant", Content: "Hi, how can I help?"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty question",
			req: dtos.WorkspaceAskRequest{
				Question: "",
			},
			wantErr: true,
		},
		{
			name: "whitespace only question",
			req: dtos.WorkspaceAskRequest{
				Question: "    \t\n  ",
			},
			wantErr: true,
		},
		{
			name: "question exceeding max length 4000",
			req: dtos.WorkspaceAskRequest{
				Question: strings.Repeat("q", 4001),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

