package dtos_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
)

func TestWaitlistRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.WaitlistRequest
		wantErr bool
	}{
		{
			name: "valid request complete",
			req: dtos.WaitlistRequest{
				Email:       "user@example.com",
				Platform:    "google_meet",
				CompanySize: "11-50",
			},
			wantErr: false,
		},
		{
			name: "valid request minimal optional fields",
			req: dtos.WaitlistRequest{
				Email: "founder@startup.io",
			},
			wantErr: false,
		},
		{
			name: "empty email",
			req: dtos.WaitlistRequest{
				Email: "",
			},
			wantErr: true,
		},
		{
			name: "whitespace only email",
			req: dtos.WaitlistRequest{
				Email: "   \t  ",
			},
			wantErr: true,
		},
		{
			name: "invalid email format",
			req: dtos.WaitlistRequest{
				Email: "invalid-email-without-at",
			},
			wantErr: true,
		},
		{
			name: "email exceeding max length",
			req: dtos.WaitlistRequest{
				Email: strings.Repeat("a", 250) + "@domain.com",
			},
			wantErr: true,
		},
		{
			name: "platform exceeding max length",
			req: dtos.WaitlistRequest{
				Email:    "user@example.com",
				Platform: strings.Repeat("p", 51),
			},
			wantErr: true,
		},
		{
			name: "company size exceeding max length",
			req: dtos.WaitlistRequest{
				Email:       "user@example.com",
				CompanySize: strings.Repeat("c", 51),
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

func TestWaitlistResponse_Structure(t *testing.T) {
	resp := dtos.WaitlistResponse{
		Email:       "user@example.com",
		Platform:    "zoom",
		CompanySize: "51-200",
		Status:      "PENDING",
		Message:     "successfully registered",
	}

	if resp.Email != "user@example.com" {
		t.Errorf("unexpected email: %s", resp.Email)
	}
	if resp.Platform != "zoom" {
		t.Errorf("unexpected platform: %s", resp.Platform)
	}
	if resp.CompanySize != "51-200" {
		t.Errorf("unexpected company size: %s", resp.CompanySize)
	}
	if resp.Status != "PENDING" {
		t.Errorf("unexpected status: %s", resp.Status)
	}
	if resp.Message != "successfully registered" {
		t.Errorf("unexpected message: %s", resp.Message)
	}
}
