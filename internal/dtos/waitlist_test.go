package dtos_test

import (
	"testing"

	"code-base-golang/internal/dtos"
)

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
