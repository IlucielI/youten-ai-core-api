package validations_test

import (
	"testing"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

func TestValidateCreateFolderRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.CreateFolderRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: dtos.CreateFolderRequest{
				Name:  "Sprint Planning",
				Color: "#4F46E5",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			req: dtos.CreateFolderRequest{
				Name:  "",
				Color: "#4F46E5",
			},
			wantErr: true,
		},
		{
			name: "default color populated",
			req: dtos.CreateFolderRequest{
				Name:  "General",
				Color: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validations.ValidateCreateFolderRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateFolderRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUpdateFolderRequest(t *testing.T) {
	validName := "Updated Folder"
	emptyName := ""
	validColor := "#10B981"

	tests := []struct {
		name    string
		req     dtos.UpdateFolderRequest
		wantErr bool
	}{
		{
			name: "valid name update",
			req: dtos.UpdateFolderRequest{
				Name: &validName,
			},
			wantErr: false,
		},
		{
			name: "empty name update fails",
			req: dtos.UpdateFolderRequest{
				Name: &emptyName,
			},
			wantErr: true,
		},
		{
			name: "valid color update",
			req: dtos.UpdateFolderRequest{
				Color: &validColor,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validations.ValidateUpdateFolderRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdateFolderRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

