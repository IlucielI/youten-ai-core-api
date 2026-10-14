package validations

import (
	"net/url"
	"regexp"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

var (
	// Google Meet code regex: 3 letters - 4 letters - 3 letters (e.g. abc-defg-hij)
	googleMeetCodeRegex = regexp.MustCompile(`^[a-z]{3}-[a-z]{4}-[a-z]{3}$`)
)

func validateGoogleMeetURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return validation.NewError("validation_required", "meeting_url is required for google_meet")
	}

	parsedURL := trimmed
	lowerURL := strings.ToLower(parsedURL)
	if !strings.HasPrefix(lowerURL, "http://") && !strings.HasPrefix(lowerURL, "https://") {
		parsedURL = "https://" + parsedURL
	}

	u, err := url.Parse(parsedURL)
	if err != nil {
		return validation.NewError("validation_invalid", "invalid meeting_url format")
	}

	host := strings.ToLower(u.Host)
	if host != "meet.google.com" {
		return validation.NewError("validation_invalid", "host must be meet.google.com")
	}

	path := strings.ToLower(strings.Trim(u.Path, "/"))
	if !googleMeetCodeRegex.MatchString(path) {
		return validation.NewError("validation_invalid", "invalid Google Meet meeting code format, expected abc-defg-hij")
	}

	return nil
}

// ValidateDispatchBotRequest validates parameters for initiating a bot session.
func ValidateDispatchBotRequest(r *dtos.DispatchBotRequest) error {
	err := validation.ValidateStruct(r,
		validation.Field(&r.Provider, validation.Required, validation.In(
			constants.BotProviderDiscord,
			constants.BotProviderGoogleMeet,
			constants.BotProviderMSTeams,
			constants.BotProviderZoom,
		)),
		validation.Field(&r.Title, validation.Length(0, 255)),
		validation.Field(&r.Template, validation.Length(0, 100)),
		validation.Field(&r.Language, validation.Length(0, 50)),
	)
	if err != nil {
		return err
	}

	switch r.Provider {
	case constants.BotProviderDiscord:
		if r.ChannelID == "" {
			return validation.NewError("validation_required", "channel_id is required for Discord bot dispatch")
		}
	case constants.BotProviderGoogleMeet:
		if err := validateGoogleMeetURL(r.MeetingURL); err != nil {
			return err
		}
	case constants.BotProviderMSTeams, constants.BotProviderZoom:
		if r.MeetingURL == "" {
			return validation.NewError("validation_required", "meeting_url is required for "+r.Provider)
		}
		if urlErr := validation.Validate(r.MeetingURL, is.URL); urlErr != nil {
			return validation.NewError("validation_invalid", "invalid meeting_url format")
		}
	}

	return nil
}

// ValidateGoogleMeetWebhookRequest validates Google Meet bot worker callback payloads.
func ValidateGoogleMeetWebhookRequest(r *dtos.GoogleMeetWebhookRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.ExternalSessionID, validation.Required),
		validation.Field(&r.Event, validation.Required, validation.In(
			constants.BotWebhookEventWaitingAdmit,
			constants.BotWebhookEventJoined,
			constants.BotWebhookEventRecording,
			constants.BotWebhookEventCompleted,
			constants.BotWebhookEventFailed,
		)),
	)
}

// ValidateMSTeamsWebhookRequest validates Microsoft Teams bot worker callback payloads.
func ValidateMSTeamsWebhookRequest(r *dtos.MSTeamsWebhookRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.ExternalSessionID, validation.Required),
		validation.Field(&r.Event, validation.Required, validation.In(
			constants.BotWebhookEventWaitingAdmit,
			constants.BotWebhookEventJoined,
			constants.BotWebhookEventCallConnected,
			constants.BotWebhookEventRecording,
			constants.BotWebhookEventRecordingStarted,
			constants.BotWebhookEventCompleted,
			constants.BotWebhookEventFailed,
		)),
	)
}

// ValidateZoomWebhookRequest validates Zoom bot worker callback payloads.
func ValidateZoomWebhookRequest(r *dtos.ZoomWebhookRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.ExternalSessionID, validation.Required),
		validation.Field(&r.Event, validation.Required, validation.In(
			constants.BotWebhookEventWaitingAdmit,
			constants.BotWebhookEventJoined,
			constants.BotWebhookEventRecording,
			constants.BotWebhookEventRecordingStarted,
			constants.BotWebhookEventCompleted,
			constants.BotWebhookEventFailed,
		)),
	)
}
