package dtos_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
)

func TestRegisterRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.RegisterRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: dtos.RegisterRequest{
				Email:    "test@example.com",
				Password: "password123",
				FullName: "John Doe",
			},
			wantErr: false,
		},
		{
			name: "missing email",
			req: dtos.RegisterRequest{
				Email:    "",
				Password: "password123",
				FullName: "John Doe",
			},
			wantErr: true,
		},
		{
			name: "invalid email format",
			req: dtos.RegisterRequest{
				Email:    "invalid-email",
				Password: "password123",
				FullName: "John Doe",
			},
			wantErr: true,
		},
		{
			name: "missing full name",
			req: dtos.RegisterRequest{
				Email:    "test@example.com",
				Password: "password123",
				FullName: "",
			},
			wantErr: true,
		},
		{
			name: "full name too short",
			req: dtos.RegisterRequest{
				Email:    "test@example.com",
				Password: "password123",
				FullName: "J",
			},
			wantErr: true,
		},
		{
			name: "password too short (< 8 chars)",
			req: dtos.RegisterRequest{
				Email:    "test@example.com",
				Password: "pass1",
				FullName: "John Doe",
			},
			wantErr: true,
		},
		{
			name: "password missing digit",
			req: dtos.RegisterRequest{
				Email:    "test@example.com",
				Password: "passwordonly",
				FullName: "John Doe",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoginRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.LoginRequest
		wantErr bool
	}{
		{
			name: "valid login request",
			req: dtos.LoginRequest{
				Email:    "user@example.com",
				Password: "anyPassword123",
			},
			wantErr: false,
		},
		{
			name: "missing email",
			req: dtos.LoginRequest{
				Email:    "",
				Password: "anyPassword123",
			},
			wantErr: true,
		},
		{
			name: "invalid email format",
			req: dtos.LoginRequest{
				Email:    "invalid-email",
				Password: "anyPassword123",
			},
			wantErr: true,
		},
		{
			name: "missing password",
			req: dtos.LoginRequest{
				Email:    "user@example.com",
				Password: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("LoginRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRefreshTokenRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.RefreshTokenRequest
		wantErr bool
	}{
		{
			name: "valid refresh token request",
			req: dtos.RefreshTokenRequest{
				RefreshToken: "valid-jwt-token-string",
			},
			wantErr: false,
		},
		{
			name: "missing refresh token",
			req: dtos.RefreshTokenRequest{
				RefreshToken: "",
			},
			wantErr: true,
		},
		{
			name: "whitespace only refresh token",
			req: dtos.RefreshTokenRequest{
				RefreshToken: "   ",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("RefreshTokenRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLogoutRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.LogoutRequest
		wantErr bool
	}{
		{
			name: "valid logout request",
			req: dtos.LogoutRequest{
				RefreshToken: "valid-jwt-token-string",
			},
			wantErr: false,
		},
		{
			name: "missing refresh token",
			req: dtos.LogoutRequest{
				RefreshToken: "",
			},
			wantErr: true,
		},
		{
			name: "whitespace only refresh token",
			req: dtos.LogoutRequest{
				RefreshToken: "   ",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("LogoutRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestForgotPasswordRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.ForgotPasswordRequest
		wantErr bool
	}{
		{
			name: "valid email",
			req: dtos.ForgotPasswordRequest{
				Email: "user@example.com",
			},
			wantErr: false,
		},
		{
			name: "valid email with uppercase and whitespace",
			req: dtos.ForgotPasswordRequest{
				Email: "  USER@EXAMPLE.COM  ",
			},
			wantErr: false,
		},
		{
			name: "missing email",
			req: dtos.ForgotPasswordRequest{
				Email: "",
			},
			wantErr: true,
		},
		{
			name: "invalid email format",
			req: dtos.ForgotPasswordRequest{
				Email: "not-an-email",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ForgotPasswordRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResetPasswordRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.ResetPasswordRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: dtos.ResetPasswordRequest{
				Token:       "secure-32-byte-hex-token",
				NewPassword: "newPassword123",
			},
			wantErr: false,
		},
		{
			name: "missing token",
			req: dtos.ResetPasswordRequest{
				Token:       "",
				NewPassword: "newPassword123",
			},
			wantErr: true,
		},
		{
			name: "missing password",
			req: dtos.ResetPasswordRequest{
				Token:       "secure-32-byte-hex-token",
				NewPassword: "",
			},
			wantErr: true,
		},
		{
			name: "password too short",
			req: dtos.ResetPasswordRequest{
				Token:       "secure-32-byte-hex-token",
				NewPassword: "short1",
			},
			wantErr: true,
		},
		{
			name: "password missing digit",
			req: dtos.ResetPasswordRequest{
				Token:       "secure-32-byte-hex-token",
				NewPassword: "noDigitsHerePassword",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ResetPasswordRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpdateProfileRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.UpdateProfileRequest
		wantErr bool
	}{
		{
			name: "valid full name",
			req: dtos.UpdateProfileRequest{
				FullName: "Jane Doe",
			},
			wantErr: false,
		},
		{
			name: "empty full name",
			req: dtos.UpdateProfileRequest{
				FullName: "",
			},
			wantErr: true,
		},
		{
			name: "whitespace only full name",
			req: dtos.UpdateProfileRequest{
				FullName: "   ",
			},
			wantErr: true,
		},
		{
			name: "single char full name",
			req: dtos.UpdateProfileRequest{
				FullName: "J",
			},
			wantErr: true,
		},
		{
			name: "too long full name",
			req: dtos.UpdateProfileRequest{
				FullName: strings.Repeat("a", 101),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateProfileRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestChangePasswordRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.ChangePasswordRequest
		wantErr bool
	}{
		{
			name: "valid change password payload",
			req: dtos.ChangePasswordRequest{
				OldPassword: "oldSecurePassword1",
				NewPassword: "newSecurePassword2",
			},
			wantErr: false,
		},
		{
			name: "empty old password",
			req: dtos.ChangePasswordRequest{
				OldPassword: "",
				NewPassword: "newSecurePassword2",
			},
			wantErr: true,
		},
		{
			name: "empty new password",
			req: dtos.ChangePasswordRequest{
				OldPassword: "oldSecurePassword1",
				NewPassword: "",
			},
			wantErr: true,
		},
		{
			name: "new password too short",
			req: dtos.ChangePasswordRequest{
				OldPassword: "oldSecurePassword1",
				NewPassword: "short1",
			},
			wantErr: true,
		},
		{
			name: "new password missing digit",
			req: dtos.ChangePasswordRequest{
				OldPassword: "oldSecurePassword1",
				NewPassword: "noDigitsInPassword",
			},
			wantErr: true,
		},
		{
			name: "new password same as old password",
			req: dtos.ChangePasswordRequest{
				OldPassword: "samePassword123",
				NewPassword: "samePassword123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ChangePasswordRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
