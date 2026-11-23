package constants

import "time"

const (
	// Google Calendar Connection Statuses
	CalendarConnectionStatusActive  = "active"
	CalendarConnectionStatusRevoked = "revoked"
	CalendarConnectionStatusExpired = "expired"

	// Scheduled Calendar Event Dispatch Statuses
	CalendarDispatchStatusPending       = "PENDING"
	CalendarDispatchStatusDispatching   = "DISPATCHING"
	CalendarDispatchStatusDispatched    = "DISPATCHED"
	CalendarDispatchStatusSkippedManual = "SKIPPED_MANUAL"
	CalendarDispatchStatusSkippedQuota  = "SKIPPED_QUOTA"
	CalendarDispatchStatusFailed        = "FAILED"

	// User Calendar Event Response Statuses
	CalendarResponseStatusAccepted  = "accepted"
	CalendarResponseStatusTentative = "tentative"
	CalendarResponseStatusDeclined  = "declined"

	// Defaults
	DefaultCalendarLeadTimeMinutes = 2
	DefaultCalendarSyncInterval    = 15 * time.Minute
	DefaultCalendarPollWindowDays  = 7
)

