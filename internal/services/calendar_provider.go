package services

import (
	"context"
	"time"
)

// CalendarOAuthTokenResult contains exchanged access and refresh tokens from Google OAuth.
type CalendarOAuthTokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
	TokenType    string
}

// CalendarUserInfo contains authenticated Google account profile metadata.
type CalendarUserInfo struct {
	ID    string
	Email string
	Name  string
}

// CalendarEventItem represents a parsed Google Calendar event with extracted meeting platform info.
type CalendarEventItem struct {
	ID                 string
	Title              string
	Description        string
	StartTime          time.Time
	EndTime            time.Time
	MeetingURL         string
	MeetingPlatform    string
	OrganizerEmail     string
	UserResponseStatus string
	Status             string // 'confirmed', 'tentative', 'cancelled'
}

// CalendarSyncResult contains the list of events and next sync token from Google Calendar API.
type CalendarSyncResult struct {
	Events        []CalendarEventItem
	NextSyncToken string
}

// GoogleCalendarClient defines the consumer port interface for interacting with Google OAuth and Calendar API.
type GoogleCalendarClient interface {
	GetAuthURL(state string) string
	ExchangeCode(ctx context.Context, code string) (*CalendarOAuthTokenResult, *CalendarUserInfo, error)
	RefreshToken(ctx context.Context, refreshToken string) (*CalendarOAuthTokenResult, error)
	FetchEvents(ctx context.Context, accessToken string, syncToken string, timeMin time.Time) (*CalendarSyncResult, error)
}

