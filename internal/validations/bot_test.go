package validations

import (
	"testing"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

func TestValidateDispatchBotRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.DispatchBotRequest
		wantErr bool
	}{
		{
			name: "valid discord request",
			req: dtos.DispatchBotRequest{
				Provider:  constants.BotProviderDiscord,
				ChannelID: "1234567890",
				GuildID:   "9876543210",
				Title:     "Team Standup",
			},
			wantErr: false,
		},
		{
			name: "invalid discord request without channel_id",
			req: dtos.DispatchBotRequest{
				Provider: constants.BotProviderDiscord,
				GuildID:  "9876543210",
			},
			wantErr: true,
		},
		{
			name: "valid google meet request",
			req: dtos.DispatchBotRequest{
				Provider:   constants.BotProviderGoogleMeet,
				MeetingURL: "https://meet.google.com/abc-defg-hij",
			},
			wantErr: false,
		},
		{
			name: "invalid google meet request without url",
			req: dtos.DispatchBotRequest{
				Provider: constants.BotProviderGoogleMeet,
			},
			wantErr: true,
		},
		{
			name: "invalid google meet request with non-url",
			req: dtos.DispatchBotRequest{
				Provider:   constants.BotProviderGoogleMeet,
				MeetingURL: "not-a-valid-url",
			},
			wantErr: true,
		},
		{
			name: "unsupported provider",
			req: dtos.DispatchBotRequest{
				Provider: "unknown_provider",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDispatchBotRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDispatchBotRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Run("nil request pointer", func(t *testing.T) {
		err := ValidateDispatchBotRequest(nil)
		if err == nil {
			t.Errorf("expected error when request pointer is nil")
		}
	})
}

func TestValidateGoogleMeetWebhookRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *dtos.GoogleMeetWebhookRequest
		wantErr bool
	}{
		{
			name: "valid waiting_admit event",
			req: &dtos.GoogleMeetWebhookRequest{
				ExternalSessionID: "meet-12345",
				Event:             "waiting_admit",
			},
			wantErr: false,
		},
		{
			name: "valid completed event",
			req: &dtos.GoogleMeetWebhookRequest{
				ExternalSessionID: "meet-12345",
				Event:             "completed",
			},
			wantErr: false,
		},
		{
			name: "missing external session id",
			req: &dtos.GoogleMeetWebhookRequest{
				Event: "joined",
			},
			wantErr: true,
		},
		{
			name: "invalid event",
			req: &dtos.GoogleMeetWebhookRequest{
				ExternalSessionID: "meet-12345",
				Event:             "unknown_event",
			},
			wantErr: true,
		},
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGoogleMeetWebhookRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGoogleMeetWebhookRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMSTeamsWebhookRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *dtos.MSTeamsWebhookRequest
		wantErr bool
	}{
		{
			name: "valid call_connected event",
			req: &dtos.MSTeamsWebhookRequest{
				ExternalSessionID: "teams-12345",
				Event:             "call_connected",
			},
			wantErr: false,
		},
		{
			name: "valid recording_started event",
			req: &dtos.MSTeamsWebhookRequest{
				ExternalSessionID: "teams-12345",
				Event:             "recording_started",
			},
			wantErr: false,
		},
		{
			name: "valid completed event",
			req: &dtos.MSTeamsWebhookRequest{
				ExternalSessionID: "teams-12345",
				Event:             "completed",
			},
			wantErr: false,
		},
		{
			name: "missing external session id",
			req: &dtos.MSTeamsWebhookRequest{
				Event: "call_connected",
			},
			wantErr: true,
		},
		{
			name: "invalid event",
			req: &dtos.MSTeamsWebhookRequest{
				ExternalSessionID: "teams-12345",
				Event:             "unknown_event",
			},
			wantErr: true,
		},
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMSTeamsWebhookRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMSTeamsWebhookRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

