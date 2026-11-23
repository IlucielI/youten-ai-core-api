package validations_test

import (
	"testing"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

func TestValidateGoogleCalendarCallbackRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.GoogleCalendarCallbackRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: dtos.GoogleCalendarCallbackRequest{
				Code:  "4/0AY0e-g7test-code",
				State: "csrf-state-uuid",
			},
			wantErr: false,
		},
		{
			name: "missing code",
			req: dtos.GoogleCalendarCallbackRequest{
				Code:  "",
				State: "csrf-state-uuid",
			},
			wantErr: true,
		},
		{
			name: "missing state",
			req: dtos.GoogleCalendarCallbackRequest{
				Code:  "valid-code",
				State: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validations.ValidateGoogleCalendarCallbackRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGoogleCalendarCallbackRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUpdateCalendarSettingsRequest(t *testing.T) {
	validLead := 5
	invalidNegative := -1
	invalidLarge := 30

	tests := []struct {
		name    string
		req     dtos.UpdateCalendarSettingsRequest
		wantErr bool
	}{
		{
			name: "valid lead time",
			req: dtos.UpdateCalendarSettingsRequest{
				LeadTimeMinutes: &validLead,
			},
			wantErr: false,
		},
		{
			name: "negative lead time",
			req: dtos.UpdateCalendarSettingsRequest{
				LeadTimeMinutes: &invalidNegative,
			},
			wantErr: true,
		},
		{
			name: "too large lead time",
			req: dtos.UpdateCalendarSettingsRequest{
				LeadTimeMinutes: &invalidLarge,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validations.ValidateUpdateCalendarSettingsRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdateCalendarSettingsRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateListCalendarEventsQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   dtos.ListCalendarEventsQuery
		wantErr bool
	}{
		{
			name: "valid defaults",
			query: dtos.ListCalendarEventsQuery{
				Page:  1,
				Limit: 20,
			},
			wantErr: false,
		},
		{
			name: "valid platform and status filter",
			query: dtos.ListCalendarEventsQuery{
				Platform:       constants.MeetingPlatformGoogleMeet,
				DispatchStatus: constants.CalendarDispatchStatusPending,
			},
			wantErr: false,
		},
		{
			name: "invalid platform",
			query: dtos.ListCalendarEventsQuery{
				Platform: "skype",
			},
			wantErr: true,
		},
		{
			name: "invalid dispatch status",
			query: dtos.ListCalendarEventsQuery{
				DispatchStatus: "UNKNOWN_STATUS",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validations.ValidateListCalendarEventsQuery(&tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateListCalendarEventsQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

