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
}
