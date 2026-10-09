package validations

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// ValidateDispatchBotRequest validates parameters for initiating a bot session.
func ValidateDispatchBotRequest(r *dtos.DispatchBotRequest) error {
	if r == nil {
		return validation.NewError("validation_required", "request is required")
	}

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
	case constants.BotProviderGoogleMeet, constants.BotProviderMSTeams, constants.BotProviderZoom:
		if r.MeetingURL == "" {
			return validation.NewError("validation_required", "meeting_url is required for "+r.Provider)
		}
		if urlErr := validation.Validate(r.MeetingURL, is.URL); urlErr != nil {
			return validation.NewError("validation_invalid", "invalid meeting_url format")
		}
	}

	return nil
}
