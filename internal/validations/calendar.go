package validations

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// ValidateGoogleCalendarCallbackRequest validates OAuth code exchange payload.
func ValidateGoogleCalendarCallbackRequest(r *dtos.GoogleCalendarCallbackRequest) error {
	r.Code = strings.TrimSpace(r.Code)
	r.State = strings.TrimSpace(r.State)

	return validation.ValidateStruct(r,
		validation.Field(&r.Code, validation.Required, validation.Length(1, 1024)),
		validation.Field(&r.State, validation.Required, validation.Length(1, 256)),
	)
}

// ValidateUpdateCalendarSettingsRequest validates user settings updates.
func ValidateUpdateCalendarSettingsRequest(r *dtos.UpdateCalendarSettingsRequest) error {
	if r.LeadTimeMinutes != nil {
		if *r.LeadTimeMinutes < 0 || *r.LeadTimeMinutes > 15 {
			return validation.Errors{
				"lead_time_minutes": validation.NewError("validation_range", "lead_time_minutes must be between 0 and 15 minutes"),
			}
		}
	}
	return nil
}

// ValidateListCalendarEventsQuery validates query parameters for listing events.
func ValidateListCalendarEventsQuery(q *dtos.ListCalendarEventsQuery) error {
	q.SetDefaults()

	if q.Platform != "" {
		p := strings.ToLower(strings.TrimSpace(q.Platform))
		if p != constants.MeetingPlatformGoogleMeet && p != constants.MeetingPlatformZoom && p != constants.MeetingPlatformMSTeams && p != constants.MeetingPlatformNone {
			return validation.Errors{
				"platform": validation.NewError("validation_invalid", "unsupported meeting platform filter"),
			}
		}
	}

	if q.DispatchStatus != "" {
		s := strings.ToUpper(strings.TrimSpace(q.DispatchStatus))
		validStatuses := map[string]bool{
			constants.CalendarDispatchStatusPending:       true,
			constants.CalendarDispatchStatusDispatching:   true,
			constants.CalendarDispatchStatusDispatched:    true,
			constants.CalendarDispatchStatusSkippedManual: true,
			constants.CalendarDispatchStatusSkippedQuota:  true,
			constants.CalendarDispatchStatusFailed:        true,
		}
		if !validStatuses[s] {
			return validation.Errors{
				"dispatch_status": validation.NewError("validation_invalid", "unsupported dispatch_status filter"),
			}
		}
	}

	return nil
}

