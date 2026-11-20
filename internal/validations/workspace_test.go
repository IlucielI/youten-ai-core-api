package validations_test

import (
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
	"strings"
	"testing"
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
			err := validations.ValidateSemanticSearchQuery(&tt.query)
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
			err := validations.ValidateWorkspaceAskRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWorkspaceManagement_Validations(t *testing.T) {
	t.Run("CreateWorkspace", func(t *testing.T) {
		valid := dtos.CreateWorkspaceRequest{Name: "Acme Corp"}
		if err := validations.ValidateCreateWorkspaceRequest(&valid); err != nil {
			t.Errorf("expected valid, got: %v", err)
		}

		short := dtos.CreateWorkspaceRequest{Name: "A"}
		if err := validations.ValidateCreateWorkspaceRequest(&short); err == nil {
			t.Error("expected error for 1-char name")
		}

		empty := dtos.CreateWorkspaceRequest{Name: "   "}
		if err := validations.ValidateCreateWorkspaceRequest(&empty); err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("UpdateWorkspace", func(t *testing.T) {
		name := "New Name"
		valid := dtos.UpdateWorkspaceRequest{Name: &name}
		if err := validations.ValidateUpdateWorkspaceRequest(&valid); err != nil {
			t.Errorf("expected valid, got: %v", err)
		}

		logo := "https://example.com/logo.png"
		logoOnly := dtos.UpdateWorkspaceRequest{LogoURL: &logo}
		if err := validations.ValidateUpdateWorkspaceRequest(&logoOnly); err != nil {
			t.Errorf("expected logo only to be valid, got: %v", err)
		}

		emptyStr := ""
		empty := dtos.UpdateWorkspaceRequest{Name: &emptyStr}
		if err := validations.ValidateUpdateWorkspaceRequest(&empty); err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("UpdateMemberRole", func(t *testing.T) {
		validRoles := []string{"owner", "admin", "member", "viewer", " ADMIN "}
		for _, r := range validRoles {
			req := dtos.UpdateWorkspaceMemberRoleRequest{Role: r}
			if err := validations.ValidateUpdateWorkspaceMemberRoleRequest(&req); err != nil {
				t.Errorf("expected valid role %q, got: %v", r, err)
			}
		}

		invalid := dtos.UpdateWorkspaceMemberRoleRequest{Role: "superuser"}
		if err := validations.ValidateUpdateWorkspaceMemberRoleRequest(&invalid); err == nil {
			t.Error("expected error for invalid role")
		}
	})
}

