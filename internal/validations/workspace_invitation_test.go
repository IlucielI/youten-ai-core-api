package validations

import (
	"testing"

	"code-base-golang/internal/dtos"
)

func TestValidateInviteMemberRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.InviteMemberRequest
		wantErr bool
	}{
		{
			name: "valid invitation with explicit role",
			req: dtos.InviteMemberRequest{
				Email: "colleague@example.com",
				Role:  "admin",
			},
			wantErr: false,
		},
		{
			name: "valid invitation with default member role fallback",
			req: dtos.InviteMemberRequest{
				Email: "viewer@example.com",
				Role:  "",
			},
			wantErr: false,
		},
		{
			name: "invalid email format",
			req: dtos.InviteMemberRequest{
				Email: "not-an-email",
				Role:  "member",
			},
			wantErr: true,
		},
		{
			name: "empty email",
			req: dtos.InviteMemberRequest{
				Email: "",
				Role:  "member",
			},
			wantErr: true,
		},
		{
			name: "invalid role owner (cannot invite as owner)",
			req: dtos.InviteMemberRequest{
				Email: "user@example.com",
				Role:  "owner",
			},
			wantErr: true,
		},
		{
			name: "invalid role unknown",
			req: dtos.InviteMemberRequest{
				Email: "user@example.com",
				Role:  "superuser",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInviteMemberRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInviteMemberRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
