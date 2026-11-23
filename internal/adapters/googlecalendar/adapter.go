package googlecalendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/services"
)

var (
	meetRegex  = regexp.MustCompile(`https://meet\.google\.com/[a-z]{3}-[a-z]{4}-[a-z]{3}`)
	zoomRegex  = regexp.MustCompile(`https://[a-zA-Z0-9\-\.]*zoom\.us/[a-zA-Z0-9_\-\?=/]+`)
	teamsRegex = regexp.MustCompile(`https://teams\.microsoft\.com/l/meetup-join/[^\s<>\x60]+`)
)

// Config holds runtime configuration for Google Calendar OAuth and API client.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	HTTPClient   *http.Client
}

// Adapter implements services.GoogleCalendarClient.
type Adapter struct {
	cfg        Config
	httpClient *http.Client
}

// NewAdapter initializes a new Google Calendar adapter.
func NewAdapter(cfg Config) *Adapter {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Adapter{
		cfg:        cfg,
		httpClient: client,
	}
}

// GetAuthURL generates the Google OAuth 2.0 authorization URL with requested scopes.
func (a *Adapter) GetAuthURL(state string) string {
	baseURL := "https://accounts.google.com/o/oauth2/v2/auth"
	params := url.Values{}
	params.Set("client_id", a.cfg.ClientID)
	params.Set("redirect_uri", a.cfg.RedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "https://www.googleapis.com/auth/calendar.events.readonly https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile")
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	params.Set("state", state)

	return fmt.Sprintf("%s?%s", baseURL, params.Encode())
}

// ExchangeCode exchanges an authorization code for Google access and refresh tokens, and fetches user info.
func (a *Adapter) ExchangeCode(ctx context.Context, code string) (*services.CalendarOAuthTokenResult, *services.CalendarUserInfo, error) {
	if strings.TrimSpace(code) == "" {
		return nil, nil, errors.New("authorization code cannot be empty")
	}

	tokenURL := "https://oauth2.googleapis.com/token"
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", a.cfg.ClientID)
	data.Set("client_secret", a.cfg.ClientSecret)
	data.Set("redirect_uri", a.cfg.RedirectURI)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("token exchange http call failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read token exchange response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("google token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse google token response: %w", err)
	}

	tokenResult := &services.CalendarOAuthTokenResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    time.Duration(tokenResp.ExpiresIn) * time.Second,
		TokenType:    tokenResp.TokenType,
	}

	userInfo, err := a.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch user info with access token: %w", err)
	}

	return tokenResult, userInfo, nil
}

// RefreshToken obtains a fresh access token from Google using a refresh token.
func (a *Adapter) RefreshToken(ctx context.Context, refreshToken string) (*services.CalendarOAuthTokenResult, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("refresh token cannot be empty")
	}

	tokenURL := "https://oauth2.googleapis.com/token"
	data := url.Values{}
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", a.cfg.ClientID)
	data.Set("client_secret", a.cfg.ClientSecret)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google refresh token failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh token response: %w", err)
	}

	return &services.CalendarOAuthTokenResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    time.Duration(tokenResp.ExpiresIn) * time.Second,
		TokenType:    tokenResp.TokenType,
	}, nil
}

func (a *Adapter) fetchUserInfo(ctx context.Context, accessToken string) (*services.CalendarUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request returned status %d", resp.StatusCode)
	}

	var userResp struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		return nil, err
	}

	return &services.CalendarUserInfo{
		ID:    userResp.ID,
		Email: userResp.Email,
		Name:  userResp.Name,
	}, nil
}

type googleEventResponse struct {
	Items         []googleCalendarItem `json:"items"`
	NextSyncToken string               `json:"nextSyncToken"`
}

type googleCalendarItem struct {
	ID          string `json:"id"`
	Status      string `json:"status"` // 'confirmed', 'tentative', 'cancelled'
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Start       struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
	} `json:"start"`
	End struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
	} `json:"end"`
	ConferenceData struct {
		EntryPoints []struct {
			EntryPointType string `json:"entryPointType"`
			URI            string `json:"uri"`
		} `json:"entryPoints"`
	} `json:"conferenceData"`
	Organizer struct {
		Email string `json:"email"`
	} `json:"organizer"`
	Attendees []struct {
		Email          string `json:"email"`
		Self           bool   `json:"self"`
		ResponseStatus string `json:"responseStatus"`
	} `json:"attendees"`
}

// FetchEvents calls Google Calendar API to fetch events either incrementally via syncToken or within time window.
func (a *Adapter) FetchEvents(ctx context.Context, accessToken string, syncToken string, timeMin time.Time) (*services.CalendarSyncResult, error) {
	baseURL := "https://www.googleapis.com/calendar/v3/calendars/primary/events"
	params := url.Values{}

	if strings.TrimSpace(syncToken) != "" {
		params.Set("syncToken", syncToken)
	} else {
		params.Set("singleEvents", "true")
		params.Set("orderBy", "startTime")
		params.Set("timeMin", timeMin.UTC().Format(time.RFC3339))
		params.Set("maxResults", "100")
	}

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar events request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calendar events call failed: %w", err)
	}
	defer resp.Body.Close()

	// If syncToken expired (410 Gone), retry full sync from timeMin without syncToken
	if resp.StatusCode == http.StatusGone && syncToken != "" {
		return a.FetchEvents(ctx, accessToken, "", timeMin)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google calendar api returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsedResp googleEventResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsedResp); err != nil {
		return nil, fmt.Errorf("failed to decode google events json: %w", err)
	}

	items := make([]services.CalendarEventItem, 0, len(parsedResp.Items))
	for _, item := range parsedResp.Items {
		confURI := ""
		for _, ep := range item.ConferenceData.EntryPoints {
			if ep.EntryPointType == "video" || strings.Contains(ep.URI, "meet.google.com") {
				confURI = ep.URI
				break
			}
		}

		meetingURL, platform := ExtractMeetingLink(confURI, item.Location, item.Description)

		startTime := parseGoogleEventTime(item.Start.DateTime, item.Start.Date)
		endTime := parseGoogleEventTime(item.End.DateTime, item.End.Date)

		responseStatus := constants.CalendarResponseStatusAccepted
		for _, att := range item.Attendees {
			if att.Self {
				responseStatus = att.ResponseStatus
				break
			}
		}

		items = append(items, services.CalendarEventItem{
			ID:                 item.ID,
			Title:              item.Summary,
			Description:        item.Description,
			StartTime:          startTime,
			EndTime:            endTime,
			MeetingURL:         meetingURL,
			MeetingPlatform:    platform,
			OrganizerEmail:     item.Organizer.Email,
			UserResponseStatus: responseStatus,
			Status:             item.Status,
		})
	}

	return &services.CalendarSyncResult{
		Events:        items,
		NextSyncToken: parsedResp.NextSyncToken,
	}, nil
}

// ExtractMeetingLink extracts and categorizes meeting platform links from conference data, location, or description.
func ExtractMeetingLink(confURI, location, description string) (meetingURL string, platform string) {
	// 1. Direct conferenceData
	if strings.Contains(confURI, "meet.google.com") {
		match := meetRegex.FindString(confURI)
		if match != "" {
			return match, constants.MeetingPlatformGoogleMeet
		}
	}

	candidates := []string{confURI, location, description}
	for _, text := range candidates {
		if text == "" {
			continue
		}

		// Google Meet
		if match := meetRegex.FindString(text); match != "" {
			return match, constants.MeetingPlatformGoogleMeet
		}

		// Zoom
		if match := zoomRegex.FindString(text); match != "" {
			return match, constants.MeetingPlatformZoom
		}

		// MS Teams
		if match := teamsRegex.FindString(text); match != "" {
			return match, constants.MeetingPlatformMSTeams
		}
	}

	return "", constants.MeetingPlatformNone
}

func parseGoogleEventTime(dateTimeStr, dateStr string) time.Time {
	if dateTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, dateTimeStr); err == nil {
			return t
		}
	}
	if dateStr != "" {
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			return t
		}
	}
	return time.Time{}
}
