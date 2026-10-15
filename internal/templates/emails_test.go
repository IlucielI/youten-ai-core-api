package templates_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/templates"
)

func TestTemplates_RenderVerifyEmail(t *testing.T) {
	data := templates.VerifyEmailData{
		FullName:  "Bayu Anugerah",
		Code:      "849201",
		VerifyURL: "https://youten.ai/verify-email?token=test-token-123",
	}

	htmlBody, textBody, err := templates.RenderVerifyEmail(data)
	if err != nil {
		t.Fatalf("unexpected error rendering verify email: %v", err)
	}

	// HTML assertions
	if !strings.Contains(htmlBody, "Bayu Anugerah") {
		t.Errorf("expected FullName in HTML body, got: %s", htmlBody)
	}
	if !strings.Contains(htmlBody, "849201") {
		t.Errorf("expected OTP code in HTML body, got: %s", htmlBody)
	}
	if !strings.Contains(htmlBody, "https://youten.ai/verify-email?token=test-token-123") {
		t.Errorf("expected VerifyURL in HTML body, got: %s", htmlBody)
	}
	if !strings.Contains(htmlBody, "<!DOCTYPE html>") {
		t.Errorf("expected DOCTYPE in HTML body, got: %s", htmlBody)
	}

	// Text assertions
	if !strings.Contains(textBody, "Hello Bayu Anugerah") {
		t.Errorf("expected greeting in text body, got: %s", textBody)
	}
	if !strings.Contains(textBody, "849201") {
		t.Errorf("expected OTP code in text body, got: %s", textBody)
	}
}

func TestTemplates_RenderResetPasswordEmail(t *testing.T) {
	data := templates.ResetPasswordEmailData{
		FullName: "Alice Johnson",
		Token:    "reset-tok-999",
		ResetURL: "https://youten.ai/reset-password?token=reset-tok-999",
	}

	htmlBody, textBody, err := templates.RenderResetPasswordEmail(data)
	if err != nil {
		t.Fatalf("unexpected error rendering reset password email: %v", err)
	}

	if !strings.Contains(htmlBody, "Alice Johnson") {
		t.Errorf("expected FullName in HTML body, got: %s", htmlBody)
	}
	if !strings.Contains(htmlBody, "reset-tok-999") {
		t.Errorf("expected Token in HTML body, got: %s", htmlBody)
	}
	if !strings.Contains(htmlBody, "https://youten.ai/reset-password?token=reset-tok-999") {
		t.Errorf("expected ResetURL in HTML body, got: %s", htmlBody)
	}
	if !strings.Contains(textBody, "Hello Alice Johnson") {
		t.Errorf("expected greeting in text body, got: %s", textBody)
	}
	if !strings.Contains(textBody, "reset-tok-999") {
		t.Errorf("expected Token in text body, got: %s", textBody)
	}
	if !strings.Contains(textBody, "https://youten.ai/reset-password?token=reset-tok-999") {
		t.Errorf("expected ResetURL in text body, got: %s", textBody)
	}
}

func TestTemplates_RenderWaitlistEmail(t *testing.T) {
	data := templates.WaitlistEmailData{
		Platform:    "Google Meet",
		CompanySize: "10-50",
		Status:      "pending",
	}

	htmlBody, textBody, err := templates.RenderWaitlistEmail(data)
	if err != nil {
		t.Fatalf("unexpected error rendering waitlist email: %v", err)
	}

	if !strings.Contains(htmlBody, "Google Meet") {
		t.Errorf("expected Platform in HTML body, got: %s", htmlBody)
	}
	if !strings.Contains(textBody, "Google Meet") {
		t.Errorf("expected Platform in text body, got: %s", textBody)
	}
}
